package run

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

type ArchiveResult struct {
	BenchmarkName string
	ArchivePath   string
	Prefix        string
	RunCount      int
}

var archiveRemoveAll = os.RemoveAll

func Archive(ctx context.Context, opts Options) error {
	_ = ctx
	if opts.Result == nil {
		return fmt.Errorf("missing analysis result")
	}
	root, err := benchmarkName(opts.Result)
	if err != nil {
		return err
	}
	archivePath, err := archivePathForInput(opts.Input)
	if err != nil {
		return err
	}
	result, err := ArchiveRoot(root, archivePath, time.Now().UTC())
	if err != nil {
		return err
	}
	if opts.Stdout != nil {
		fmt.Fprintf(opts.Stdout, "archived %s to %s as %s and removed %s\n", result.BenchmarkName, result.ArchivePath, result.Prefix, result.BenchmarkName)
	}
	return nil
}

func ArchiveBenchmarkDir(ctx context.Context, opts BenchmarkDirOptions) error {
	_ = ctx
	root := filepath.Clean(opts.Root)
	archivePath, err := archivePathForInput(root)
	if err != nil {
		return err
	}
	result, err := ArchiveRoot(root, archivePath, time.Now().UTC())
	if err != nil {
		return err
	}
	if opts.Stdout != nil {
		fmt.Fprintf(opts.Stdout, "archived %s to %s as %s and removed %s\n", result.BenchmarkName, result.ArchivePath, result.Prefix, result.BenchmarkName)
	}
	return nil
}

func ArchiveRoot(root, archivePath string, now time.Time) (ArchiveResult, error) {
	root = filepath.Clean(root)
	archivePath = filepath.Clean(archivePath)
	unlock, err := acquireExistingRootLock(root)
	if err != nil {
		return ArchiveResult{}, err
	}
	defer unlock()

	runs, err := archiveableRuns(root)
	if err != nil {
		return ArchiveResult{}, err
	}
	if len(runs) == 0 {
		return ArchiveResult{}, fmt.Errorf("no run directories found in %s", root)
	}

	prefix := archiveTimestamp(now)
	prefix, err = uniqueArchivePrefix(archivePath, prefix)
	if err != nil {
		return ArchiveResult{}, err
	}
	if err := rewriteArchiveWithSnapshot(root, archivePath, prefix, runs); err != nil {
		return ArchiveResult{}, err
	}
	if err := removeArchivedRoot(root, prefix); err != nil {
		return ArchiveResult{}, err
	}

	benchmark := filepath.Base(root)
	return ArchiveResult{
		BenchmarkName: benchmark,
		ArchivePath:   archivePath,
		Prefix:        path.Join(prefix, filepath.ToSlash(benchmark)),
		RunCount:      len(runs),
	}, nil
}

func archivePathForInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("cannot derive archive name from empty input path")
	}
	base := filepath.Base(filepath.Clean(input))
	if filepath.Ext(base) == ".jbs" {
		base = strings.TrimSuffix(base, ".jbs")
	}
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "", fmt.Errorf("cannot derive archive name from %q", input)
	}
	return base + ".tar.gz", nil
}

func archiveTimestamp(now time.Time) string {
	return now.UTC().Format("20060102T150405.000000000Z")
}

func archiveableRuns(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	runs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !numericRunDir.MatchString(entry.Name()) {
			continue
		}
		if err := validateArchiveRun(root, entry.Name()); err != nil {
			return nil, err
		}
		runs = append(runs, entry.Name())
	}
	if len(runs) > 0 {
		slices.Sort(runs)
		return runs, nil
	}
	for _, component := range entries {
		if !component.IsDir() || strings.HasPrefix(component.Name(), ".") || numericRunDir.MatchString(component.Name()) {
			continue
		}
		componentDir := filepath.Join(root, component.Name())
		componentEntries, err := os.ReadDir(componentDir)
		if err != nil {
			return nil, err
		}
		for _, run := range componentEntries {
			if !run.IsDir() || !numericRunDir.MatchString(run.Name()) {
				continue
			}
			rel := filepath.Join(component.Name(), run.Name())
			if err := validateArchiveRun(root, rel); err != nil {
				return nil, err
			}
			runs = append(runs, rel)
		}
	}
	slices.Sort(runs)
	return runs, nil
}

func validateArchiveRun(root, run string) error {
	status, err := LoadRootStatus(filepath.Join(root, run, "status"))
	if err != nil {
		return fmt.Errorf("cannot archive run %s: %w", run, err)
	}
	if status.Status == StatusRunning {
		return fmt.Errorf("cannot archive %s: run %s status is RUNNING", root, run)
	}
	return nil
}

func rewriteArchiveWithSnapshot(root, archivePath, timestamp string, runs []string) error {
	dir := archiveParentDir(archivePath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(archivePath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}

	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	if err := copyExistingArchive(archivePath, tw); err != nil {
		tw.Close()
		gz.Close()
		tmp.Close()
		return err
	}
	if err := appendBenchmarkSnapshot(tw, root, timestamp, runs, time.Now().UTC()); err != nil {
		tw.Close()
		gz.Close()
		tmp.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		tmp.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return err
	}
	cleanup = false
	return fsutil.SyncDir(dir)
}

func archiveParentDir(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "" {
		return "."
	}
	return dir
}

func removeArchivedRoot(root, prefix string) error {
	cleanRoot := filepath.Clean(root)
	parent := filepath.Dir(cleanRoot)
	base := filepath.Base(cleanRoot)
	trash, err := uniqueRemovalPath(parent, base, prefix)
	if err != nil {
		return err
	}
	if err := os.Rename(cleanRoot, trash); err != nil {
		return fmt.Errorf("archive written, but failed to move benchmark directory %s for removal: %w", cleanRoot, err)
	}
	if err := fsutil.SyncDir(parent); err != nil {
		return fmt.Errorf("archive written, but failed to sync benchmark parent after moving %s to %s: %w", cleanRoot, trash, err)
	}
	if err := archiveRemoveAll(trash); err != nil {
		return fmt.Errorf("archive written, but failed to remove archived benchmark directory %s: %w", trash, err)
	}
	if err := fsutil.SyncDir(parent); err != nil {
		return fmt.Errorf("archive written, but failed to sync benchmark parent after removing %s: %w", trash, err)
	}
	return nil
}

func uniqueRemovalPath(parent, base, prefix string) (string, error) {
	stamp := safePathComponent(prefix)
	if stamp == "" {
		stamp = fmt.Sprintf("archive-%d", os.Getpid())
	}
	for i := 0; i < 1000; i++ {
		suffix := fmt.Sprintf("%s-%d", stamp, os.Getpid())
		if i > 0 {
			suffix = fmt.Sprintf("%s-%d-%03d", stamp, os.Getpid(), i)
		}
		candidate := filepath.Join(parent, ".archived-"+base+"-"+suffix)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not choose removal path for %s", filepath.Join(parent, base))
}

func copyExistingArchive(archivePath string, out *tar.Writer) error {
	return walkExistingArchive(archivePath, func(hdr *tar.Header, tr *tar.Reader) error {
		copied := *hdr
		if err := out.WriteHeader(&copied); err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg {
			_, err := io.Copy(out, tr)
			return err
		}
		return nil
	})
}

func existingTopLevelNames(archivePath string) (map[string]struct{}, error) {
	used := make(map[string]struct{})
	err := walkExistingArchive(archivePath, func(hdr *tar.Header, _ *tar.Reader) error {
		clean := path.Clean(hdr.Name)
		top, _, _ := strings.Cut(clean, "/")
		if top != "" && top != "." {
			used[top] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return used, nil
}

func uniqueArchivePrefix(archivePath, base string) (string, error) {
	used, err := existingTopLevelNames(archivePath)
	if err != nil {
		return "", err
	}
	if _, ok := used[base]; !ok {
		return base, nil
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%03d", base, i)
		if _, ok := used[candidate]; !ok {
			return candidate, nil
		}
	}
}

func walkExistingArchive(archivePath string, visit func(*tar.Header, *tar.Reader) error) error {
	f, err := os.Open(archivePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read existing archive %s: %w", archivePath, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read existing archive %s: %w", archivePath, err)
		}
		if err := validateArchiveName(hdr.Name); err != nil {
			return err
		}
		if err := visit(hdr, tr); err != nil {
			return err
		}
	}
}

func validateArchiveName(name string) error {
	if name == "" || strings.HasPrefix(name, "/") {
		return fmt.Errorf("unsafe archive entry %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("unsafe archive entry %q", name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("unsafe archive entry %q", name)
	}
	return nil
}

func appendBenchmarkSnapshot(tw *tar.Writer, root, timestamp string, runs []string, now time.Time) error {
	rootBase := filepath.Base(filepath.Clean(root))
	archiveRoot := path.Join(timestamp, rootBase)
	if err := writeDirHeader(tw, archiveRoot, 0o755, now); err != nil {
		return err
	}
	for _, run := range runs {
		runPath := filepath.Join(root, run)
		prefix := path.Join(archiveRoot, filepath.ToSlash(run))
		if err := appendTree(tw, runPath, prefix); err != nil {
			return err
		}
	}
	return appendManifestDatabases(tw, root, archiveRoot, runs)
}

func writeDirHeader(tw *tar.Writer, name string, mode int64, modTime time.Time) error {
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		ModTime:  modTime,
		Typeflag: tar.TypeDir,
	})
}

func appendTree(tw *tar.Writer, fsRoot, archiveRoot string) error {
	return filepath.WalkDir(fsRoot, func(filePath string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fsRoot, filePath)
		if err != nil {
			return err
		}
		name := archiveRoot
		if rel != "." {
			name = path.Join(archiveRoot, filepath.ToSlash(rel))
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(filePath)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = name
		if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func appendManifestDatabases(tw *tar.Writer, root, archiveRoot string, runs []string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rootAbs = filepath.Clean(rootAbs)
	seen := make(map[string]struct{})
	for _, run := range runs {
		manifest, err := LoadManifest(filepath.Join(root, run, "manifest.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect archive database for run %s: %w", run, err)
		}
		if manifest.AnalyseDatabasePath == "" {
			continue
		}
		dbPath := manifest.AnalyseDatabasePath
		if !filepath.IsAbs(dbPath) {
			dbPath = filepath.Join(".", dbPath)
		}
		dbAbs, err := filepath.Abs(dbPath)
		if err != nil {
			return err
		}
		dbAbs = filepath.Clean(dbAbs)
		rel, err := filepath.Rel(rootAbs, dbAbs)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		relSlash := filepath.ToSlash(rel)
		if isInsideArchivedRun(relSlash, runs) {
			continue
		}
		if _, ok := seen[relSlash]; ok {
			continue
		}
		info, err := os.Lstat(dbAbs)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("analyse database path %s is a directory", dbAbs)
		}
		if err := appendTree(tw, dbAbs, path.Join(archiveRoot, relSlash)); err != nil {
			return err
		}
		seen[relSlash] = struct{}{}
	}
	return nil
}

func isInsideArchivedRun(relSlash string, runs []string) bool {
	for _, run := range runs {
		runSlash := filepath.ToSlash(run)
		if relSlash == runSlash || strings.HasPrefix(relSlash, runSlash+"/") {
			return true
		}
	}
	return false
}
