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
	"syscall"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

type ArchiveResult struct {
	BenchmarkName string
	ArchivePath   string
	Prefix        string
	RunCount      int
	RemovedRoot   bool
}

type archivedPathKind int

const (
	archivedPathFile archivedPathKind = iota
	archivedPathDir
)

type archivedPath struct {
	FSPath string
	Kind   archivedPathKind
}

type archiveSnapshot struct {
	Paths []archivedPath
}

type archiveCleanupResult struct {
	RemovedRoot bool
	Removed     int
}

var archiveRemove = os.Remove

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
		writeArchiveSummary(opts.Stdout, result)
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
		writeArchiveSummary(opts.Stdout, result)
	}
	return nil
}

func writeArchiveSummary(out io.Writer, result ArchiveResult) {
	if result.RemovedRoot {
		fmt.Fprintf(out, "archived %s to %s as %s and removed %s\n", result.BenchmarkName, result.ArchivePath, result.Prefix, result.BenchmarkName)
		return
	}
	fmt.Fprintf(out, "archived %s to %s as %s and removed archived entries from %s\n", result.BenchmarkName, result.ArchivePath, result.Prefix, result.BenchmarkName)
}

func ArchiveRoot(root, archivePath string, now time.Time) (ArchiveResult, error) {
	root = filepath.Clean(root)
	archivePath = filepath.Clean(archivePath)
	locks := &heldRootLocks{}
	released := false
	defer func() {
		if !released {
			locks.release()
		}
	}()
	unlock, err := acquireExistingRootLock(root)
	if err != nil {
		return ArchiveResult{}, err
	}
	locks.unlocks = append(locks.unlocks, unlock)

	inventory, err := discoverArchiveRuns(root)
	if err != nil {
		return ArchiveResult{}, err
	}
	if len(inventory.Runs) == 0 {
		return ArchiveResult{}, fmt.Errorf("no run directories found in %s", root)
	}
	for _, component := range inventory.ComponentRoots {
		unlock, err := acquireExistingRootLock(filepath.Join(root, component))
		if err != nil {
			return ArchiveResult{}, err
		}
		locks.unlocks = append(locks.unlocks, unlock)
	}
	if err := validateArchiveRuns(root, inventory.Runs); err != nil {
		return ArchiveResult{}, err
	}

	prefix := archiveTimestamp(now)
	prefix, err = uniqueArchivePrefix(archivePath, prefix)
	if err != nil {
		return ArchiveResult{}, err
	}
	snapshot, err := rewriteArchiveWithSnapshot(root, archivePath, prefix, inventory.Runs)
	if err != nil {
		return ArchiveResult{}, err
	}
	cleanup, err := removeArchivedSnapshot(root, snapshot)
	if err != nil {
		return ArchiveResult{}, err
	}
	locks.release()
	released = true
	cleanup, err = pruneArchivedSnapshotDirs(root, snapshot, cleanup)
	if err != nil {
		return ArchiveResult{}, err
	}

	benchmark := filepath.Base(root)
	return ArchiveResult{
		BenchmarkName: benchmark,
		ArchivePath:   archivePath,
		Prefix:        path.Join(prefix, filepath.ToSlash(benchmark)),
		RunCount:      len(inventory.Runs),
		RemovedRoot:   cleanup.RemovedRoot,
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

type archiveRunInventory struct {
	Runs           []string
	ComponentRoots []string
}

func discoverArchiveRuns(root string) (archiveRunInventory, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return archiveRunInventory{}, err
	}
	runs := make([]string, 0)
	componentRoots := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if numericRunDir.MatchString(name) {
			ok, err := looksLikeArchiveRun(filepath.Join(root, name))
			if err != nil {
				return archiveRunInventory{}, err
			}
			if ok {
				runs = append(runs, name)
			}
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		componentDir := filepath.Join(root, name)
		componentEntries, err := os.ReadDir(componentDir)
		if err != nil {
			return archiveRunInventory{}, err
		}
		hadRuns := false
		for _, run := range componentEntries {
			if !run.IsDir() || !numericRunDir.MatchString(run.Name()) {
				continue
			}
			ok, err := looksLikeArchiveRun(filepath.Join(componentDir, run.Name()))
			if err != nil {
				return archiveRunInventory{}, err
			}
			if !ok {
				continue
			}
			rel := filepath.Join(name, run.Name())
			runs = append(runs, rel)
			hadRuns = true
		}
		if hadRuns {
			componentRoots = append(componentRoots, name)
		}
	}
	slices.Sort(runs)
	slices.Sort(componentRoots)
	return archiveRunInventory{Runs: runs, ComponentRoots: componentRoots}, nil
}

func looksLikeArchiveRun(dir string) (bool, error) {
	for _, name := range []string{"status", "manifest.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

func archiveableRuns(root string) ([]string, error) {
	inventory, err := discoverArchiveRuns(root)
	if err != nil {
		return nil, err
	}
	if err := validateArchiveRuns(root, inventory.Runs); err != nil {
		return nil, err
	}
	return inventory.Runs, nil
}

func validateArchiveRuns(root string, runs []string) error {
	for _, run := range runs {
		if err := validateArchiveRun(root, run); err != nil {
			return err
		}
	}
	return nil
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

func rewriteArchiveWithSnapshot(root, archivePath, timestamp string, runs []string) (archiveSnapshot, error) {
	dir := archiveParentDir(archivePath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(archivePath)+".tmp-*")
	if err != nil {
		return archiveSnapshot{}, err
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
		return archiveSnapshot{}, err
	}

	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	if err := copyExistingArchive(archivePath, tw); err != nil {
		tw.Close()
		gz.Close()
		tmp.Close()
		return archiveSnapshot{}, err
	}
	snapshot, err := appendBenchmarkSnapshot(tw, root, timestamp, runs, time.Now().UTC())
	if err != nil {
		tw.Close()
		gz.Close()
		tmp.Close()
		return archiveSnapshot{}, err
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		tmp.Close()
		return archiveSnapshot{}, err
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return archiveSnapshot{}, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return archiveSnapshot{}, err
	}
	if err := tmp.Close(); err != nil {
		return archiveSnapshot{}, err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return archiveSnapshot{}, err
	}
	cleanup = false
	if err := fsutil.SyncDir(dir); err != nil {
		return archiveSnapshot{}, err
	}
	return snapshot, nil
}

func archiveParentDir(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "" {
		return "."
	}
	return dir
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

func appendBenchmarkSnapshot(tw *tar.Writer, root, timestamp string, runs []string, now time.Time) (archiveSnapshot, error) {
	snapshot := archiveSnapshot{}
	rootBase := filepath.Base(filepath.Clean(root))
	archiveRoot := path.Join(timestamp, rootBase)
	if err := appendSnapshotDir(tw, filepath.Clean(root), archiveRoot, 0o755, now, &snapshot); err != nil {
		return archiveSnapshot{}, err
	}
	seenDirs := map[string]struct{}{archiveRoot: {}}
	for _, run := range runs {
		if err := appendRunParentDirs(tw, root, archiveRoot, run, now, &snapshot, seenDirs); err != nil {
			return archiveSnapshot{}, err
		}
		runPath := filepath.Join(root, run)
		prefix := path.Join(archiveRoot, filepath.ToSlash(run))
		paths, err := appendTree(tw, runPath, prefix)
		if err != nil {
			return archiveSnapshot{}, err
		}
		snapshot.Paths = append(snapshot.Paths, paths...)
	}
	paths, err := appendManifestDatabases(tw, root, archiveRoot, runs)
	if err != nil {
		return archiveSnapshot{}, err
	}
	snapshot.Paths = append(snapshot.Paths, paths...)
	return snapshot, nil
}

func appendSnapshotDir(tw *tar.Writer, fsPath, archiveName string, mode int64, modTime time.Time, snapshot *archiveSnapshot) error {
	if err := writeDirHeader(tw, archiveName, mode, modTime); err != nil {
		return err
	}
	snapshot.Paths = append(snapshot.Paths, archivedPath{FSPath: filepath.Clean(fsPath), Kind: archivedPathDir})
	return nil
}

func appendRunParentDirs(tw *tar.Writer, root, archiveRoot, run string, now time.Time, snapshot *archiveSnapshot, seen map[string]struct{}) error {
	rel := filepath.ToSlash(run)
	parts := strings.Split(rel, "/")
	currentFS := filepath.Clean(root)
	currentArchive := archiveRoot
	for _, part := range parts[:len(parts)-1] {
		currentFS = filepath.Join(currentFS, part)
		currentArchive = path.Join(currentArchive, part)
		if _, ok := seen[currentArchive]; ok {
			continue
		}
		if err := appendSnapshotDir(tw, currentFS, currentArchive, 0o755, now, snapshot); err != nil {
			return err
		}
		seen[currentArchive] = struct{}{}
	}
	return nil
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

func appendTree(tw *tar.Writer, fsRoot, archiveRoot string) ([]archivedPath, error) {
	paths := make([]archivedPath, 0)
	err := filepath.WalkDir(fsRoot, func(filePath string, _ fs.DirEntry, walkErr error) error {
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
		paths = append(paths, archivedPath{FSPath: filepath.Clean(filePath), Kind: archivedKindFor(info)})
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
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func archivedKindFor(info os.FileInfo) archivedPathKind {
	if info.IsDir() {
		return archivedPathDir
	}
	return archivedPathFile
}

func appendManifestDatabases(tw *tar.Writer, root, archiveRoot string, runs []string) ([]archivedPath, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rootAbs = filepath.Clean(rootAbs)
	seen := make(map[string]struct{})
	paths := make([]archivedPath, 0)
	for _, run := range runs {
		manifest, err := LoadManifest(filepath.Join(root, run, "manifest.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect archive database for run %s: %w", run, err)
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
			return nil, err
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
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("analyse database path %s is a directory", dbAbs)
		}
		written, err := appendTree(tw, dbAbs, path.Join(archiveRoot, relSlash))
		if err != nil {
			return nil, err
		}
		paths = append(paths, written...)
		seen[relSlash] = struct{}{}
	}
	return paths, nil
}

func removeArchivedSnapshot(root string, snapshot archiveSnapshot) (archiveCleanupResult, error) {
	paths, rootAbs, err := archivedCleanupPaths(root, snapshot.Paths)
	if err != nil {
		return archiveCleanupResult{}, err
	}
	files, dirs := splitArchivedPaths(paths)
	result := archiveCleanupResult{}
	for _, item := range files {
		removed, err := removeArchivedFile(item.FSPath)
		if err != nil {
			return result, err
		}
		if removed {
			result.Removed++
		}
	}
	result, err = pruneArchivedDirs(dirs, rootAbs, result)
	if err != nil {
		return result, err
	}
	if err := syncArchiveCleanupDirs(paths); err != nil {
		return result, err
	}
	return result, nil
}

func pruneArchivedSnapshotDirs(root string, snapshot archiveSnapshot, result archiveCleanupResult) (archiveCleanupResult, error) {
	paths, rootAbs, err := archivedCleanupPaths(root, snapshot.Paths)
	if err != nil {
		return result, err
	}
	_, dirs := splitArchivedPaths(paths)
	result, err = pruneArchivedDirs(dirs, rootAbs, result)
	if err != nil {
		return result, err
	}
	if err := syncArchiveCleanupDirs(paths); err != nil {
		return result, err
	}
	return result, nil
}

func archivedCleanupPaths(root string, paths []archivedPath) ([]archivedPath, string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, "", fmt.Errorf("archive written, but failed to resolve benchmark root %s for cleanup: %w", root, err)
	}
	rootAbs = filepath.Clean(rootAbs)
	seen := make(map[string]archivedPathKind)
	for _, item := range paths {
		pathAbs, err := filepath.Abs(item.FSPath)
		if err != nil {
			return nil, "", fmt.Errorf("archive written, but failed to resolve archived path %s for cleanup: %w", item.FSPath, err)
		}
		pathAbs = filepath.Clean(pathAbs)
		rel, err := filepath.Rel(rootAbs, pathAbs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, "", fmt.Errorf("archive written, but refusing to remove archived path outside benchmark root: %s", pathAbs)
		}
		if existing, ok := seen[pathAbs]; !ok || existing != archivedPathDir {
			seen[pathAbs] = item.Kind
		}
	}
	deduped := make([]archivedPath, 0, len(seen))
	for pathAbs, kind := range seen {
		deduped = append(deduped, archivedPath{FSPath: pathAbs, Kind: kind})
	}
	slices.SortFunc(deduped, func(a, b archivedPath) int {
		return strings.Compare(a.FSPath, b.FSPath)
	})
	return deduped, rootAbs, nil
}

func splitArchivedPaths(paths []archivedPath) ([]archivedPath, []archivedPath) {
	files := make([]archivedPath, 0, len(paths))
	dirs := make([]archivedPath, 0, len(paths))
	for _, item := range paths {
		if item.Kind == archivedPathDir {
			dirs = append(dirs, item)
			continue
		}
		files = append(files, item)
	}
	slices.SortFunc(files, compareArchivedPathDepthDesc)
	slices.SortFunc(dirs, compareArchivedPathDepthDesc)
	return files, dirs
}

func compareArchivedPathDepthDesc(a, b archivedPath) int {
	aDepth := pathDepth(a.FSPath)
	bDepth := pathDepth(b.FSPath)
	if aDepth > bDepth {
		return -1
	}
	if aDepth < bDepth {
		return 1
	}
	return strings.Compare(a.FSPath, b.FSPath)
}

func pathDepth(filePath string) int {
	clean := filepath.Clean(filePath)
	if clean == string(filepath.Separator) {
		return 0
	}
	return strings.Count(clean, string(filepath.Separator))
}

func pruneArchivedDirs(dirs []archivedPath, rootAbs string, result archiveCleanupResult) (archiveCleanupResult, error) {
	for _, item := range dirs {
		removed, err := removeArchivedDirIfEmpty(item.FSPath)
		if err != nil {
			return result, err
		}
		if removed {
			result.Removed++
			if filepath.Clean(item.FSPath) == rootAbs {
				result.RemovedRoot = true
			}
		}
	}
	return result, nil
}

func removeArchivedFile(filePath string) (bool, error) {
	info, err := os.Lstat(filePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("archive written, but failed to inspect archived file %s for removal: %w", filePath, err)
	}
	if info.IsDir() {
		return false, nil
	}
	if err := archiveRemove(filePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("archive written, but failed to remove archived file %s: %w", filePath, err)
	}
	return true, nil
}

func removeArchivedDirIfEmpty(dir string) (bool, error) {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("archive written, but failed to inspect archived directory %s for removal: %w", dir, err)
	}
	if !info.IsDir() {
		return false, nil
	}
	if err := archiveRemove(dir); err != nil {
		switch {
		case os.IsNotExist(err):
			return false, nil
		case isDirectoryNotEmpty(err):
			return false, nil
		default:
			return false, fmt.Errorf("archive written, but failed to remove archived directory %s: %w", dir, err)
		}
	}
	return true, nil
}

func isDirectoryNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
}

func syncArchiveCleanupDirs(paths []archivedPath) error {
	dirs := make(map[string]struct{}, len(paths))
	for _, item := range paths {
		dirs[filepath.Dir(item.FSPath)] = struct{}{}
	}
	ordered := make([]string, 0, len(dirs))
	for dir := range dirs {
		ordered = append(ordered, dir)
	}
	slices.Sort(ordered)
	for _, dir := range ordered {
		if err := fsutil.SyncDir(dir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("archive written, but failed to sync cleanup directory %s: %w", dir, err)
		}
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
