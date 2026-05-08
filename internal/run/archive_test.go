package run

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type archiveTestEntry struct {
	Header tar.Header
	Body   string
}

func TestArchiveRootCreatesTarGz(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	now := time.Date(2026, 5, 8, 14, 30, 12, 123456789, time.UTC)
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{
		"manifest.json":         `{"schema":1}`,
		"run/000000/run.sh":     "#!/bin/sh\n",
		"run/000000/stdout":     "out\n",
		"run/000000/stderr":     "",
		"run/000000/exitcode":   "0\n",
		"run/000000/status":     `{"schema":1,"status":"FINISHED","step":"run","row":0}`,
		"run/analyse.csv":       "run_id,value\n000000,1\n",
		"run/000000/output.txt": "payload\n",
	})

	result, err := ArchiveRoot(root, archive, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.BenchmarkName != "bench" || result.ArchivePath != archive || result.Prefix != "20260508T143012.123456789Z/bench" || result.RunCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("expected original root to be removed, stat error: %v", err)
	}

	entries := readTarGzEntries(t, archive)
	for _, name := range []string{
		"20260508T143012.123456789Z/bench/",
		"20260508T143012.123456789Z/bench/000000/",
		"20260508T143012.123456789Z/bench/000000/status",
		"20260508T143012.123456789Z/bench/000000/manifest.json",
		"20260508T143012.123456789Z/bench/000000/run/000000/stdout",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("archive missing %q; entries=%v", name, archiveEntryNames(entries))
		}
	}
	if got := entries["20260508T143012.123456789Z/bench/000000/run/000000/stdout"].Body; got != "out\n" {
		t.Fatalf("stdout body = %q", got)
	}
}

func TestArchiveRootPreservesExistingSnapshots(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	timeA := time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)
	timeB := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)

	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "first\n"})
	if _, err := ArchiveRoot(root, archive, timeA); err != nil {
		t.Fatal(err)
	}
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "second\n"})
	if _, err := ArchiveRoot(root, archive, timeB); err != nil {
		t.Fatal(err)
	}

	entries := readTarGzEntries(t, archive)
	if got := entries["20260508T140000.000000000Z/bench/000000/run/000000/stdout"].Body; got != "first\n" {
		t.Fatalf("first snapshot body = %q", got)
	}
	if got := entries["20260508T150000.000000000Z/bench/000000/run/000000/stdout"].Body; got != "second\n" {
		t.Fatalf("second snapshot body = %q", got)
	}
}

func TestArchiveRootPreservesSymlinks(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{
		"run/000000/stdout": "out\n",
	})
	linkPath := filepath.Join(root, "000000", "run", "000000", "prepare")
	if err := os.Symlink("../000001", linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if _, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	entries := readTarGzEntries(t, archive)
	entry, ok := entries["20260508T140000.000000000Z/bench/000000/run/000000/prepare"]
	if !ok {
		t.Fatalf("missing symlink entry; entries=%v", archiveEntryNames(entries))
	}
	if entry.Header.Typeflag != tar.TypeSymlink || entry.Header.Linkname != "../000001" {
		t.Fatalf("unexpected symlink header: type=%v link=%q", entry.Header.Typeflag, entry.Header.Linkname)
	}
}

func TestArchiveRootRejectsRunningRun(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	createArchiveRun(t, root, "000000", StatusRunning, map[string]string{"run/000000/stdout": ""})

	_, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "status is RUNNING") {
		t.Fatalf("expected RUNNING rejection, got %v", err)
	}
	if _, statErr := os.Stat(archive); !os.IsNotExist(statErr) {
		t.Fatalf("expected no archive, stat error: %v", statErr)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("expected root to remain: %v", statErr)
	}
}

func TestArchiveRootIgnoresTransientRootEntries(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "out\n"})
	if err := os.WriteFile(filepath.Join(root, rootLockReclaimName), []byte("reclaim"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".creating-000001-123"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	for name := range readTarGzEntries(t, archive) {
		if strings.Contains(name, ".jbs.lock") || strings.Contains(name, ".creating-") || strings.Contains(name, "notes.txt") {
			t.Fatalf("transient root entry archived: %q", name)
		}
	}
}

func TestArchiveRootIncludesManifestDatabaseInsideRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	dbPath := filepath.Join(root, "results.sqlite")
	manifest := Manifest{
		Schema:              1,
		RunID:               "000000",
		BenchmarkName:       "bench",
		AnalyseDatabasePath: dbPath,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{
		"manifest.json": string(manifestData),
	})
	if err := os.WriteFile(dbPath, []byte("sqlite-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	entries := readTarGzEntries(t, archive)
	entry, ok := entries["20260508T140000.000000000Z/bench/results.sqlite"]
	if !ok {
		t.Fatalf("missing database entry; entries=%v", archiveEntryNames(entries))
	}
	if entry.Body != "sqlite-data" {
		t.Fatalf("database body = %q", entry.Body)
	}
}

func TestArchiveRootReplacesAtomicallyOnCorruptExistingArchive(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	original := []byte("not gzip")
	if err := os.WriteFile(archive, original, 0o644); err != nil {
		t.Fatal(err)
	}
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "out\n"})

	_, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "read existing archive") {
		t.Fatalf("expected corrupt archive error, got %v", err)
	}
	got, readErr := os.ReadFile(archive)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("archive bytes changed: %q", got)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("expected root to remain: %v", statErr)
	}
}

func TestArchiveRootUsesUniqueTimestampSuffix(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	now := time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)

	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "first\n"})
	if _, err := ArchiveRoot(root, archive, now); err != nil {
		t.Fatal(err)
	}
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "second\n"})
	result, err := ArchiveRoot(root, archive, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Prefix != "20260508T140000.000000000Z-001/bench" {
		t.Fatalf("prefix = %q", result.Prefix)
	}
	entries := readTarGzEntries(t, archive)
	if _, ok := entries["20260508T140000.000000000Z/bench/000000/status"]; !ok {
		t.Fatalf("missing original timestamp; entries=%v", archiveEntryNames(entries))
	}
	if _, ok := entries["20260508T140000.000000000Z-001/bench/000000/status"]; !ok {
		t.Fatalf("missing suffixed timestamp; entries=%v", archiveEntryNames(entries))
	}
}

func TestArchivePathForInput(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "benchmark.jbs", want: "benchmark.tar.gz"},
		{input: "path/to/bench.jbs", want: "bench.tar.gz"},
		{input: "bench", want: "bench.tar.gz"},
		{input: "bench.test.jbs", want: "bench.test.tar.gz"},
	}
	for _, tc := range cases {
		got, err := archivePathForInput(tc.input)
		if err != nil {
			t.Fatalf("archivePathForInput(%q): %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("archivePathForInput(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestArchivePathForInputRejectsEmptyStem(t *testing.T) {
	for _, input := range []string{"", ".jbs"} {
		if got, err := archivePathForInput(input); err == nil {
			t.Fatalf("archivePathForInput(%q) = %q, want error", input, got)
		}
	}
}

func TestArchiveRootReportsCleanupFailure(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "out\n"})

	oldRemoveAll := archiveRemoveAll
	archiveRemoveAll = func(string) error { return errors.New("blocked") }
	defer func() { archiveRemoveAll = oldRemoveAll }()

	_, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "failed to remove archived benchmark directory") || !strings.Contains(err.Error(), ".archived-bench-") {
		t.Fatalf("expected cleanup error with leftover path, got %v", err)
	}
	if _, statErr := os.Stat(archive); statErr != nil {
		t.Fatalf("expected archive to exist: %v", statErr)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("expected original root to be moved, stat error: %v", statErr)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".archived-bench-") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected leftover archived directory in %s", dir)
	}
}

func createArchiveRun(t *testing.T, root, runID string, status Status, files map[string]string) {
	t.Helper()
	runDir := filepath.Join(root, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeArchiveStatus(t, filepath.Join(runDir, "status"), RootStatus{
		Schema:     1,
		Status:     status,
		SourceHash: "sha256:test",
		CreatedAt:  time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC),
	})
	for name, content := range files {
		path := filepath.Join(runDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeArchiveStatus(t *testing.T, path string, status RootStatus) {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTarGzEntries(t *testing.T, archivePath string) map[string]archiveTestEntry {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := make(map[string]archiveTestEntry)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entry := archiveTestEntry{Header: *hdr}
		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			body, readErr := readTarBody(tr)
			if readErr != nil {
				t.Fatal(readErr)
			}
			entry.Body = body
		}
		entries[hdr.Name] = entry
	}
	return entries
}

func readTarBody(r *tar.Reader) (string, error) {
	data, err := io.ReadAll(r)
	return string(data), err
}

func archiveEntryNames(entries map[string]archiveTestEntry) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	return names
}
