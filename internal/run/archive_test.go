package run

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

type archiveTestEntry struct {
	Header tar.Header
	Body   string
}

func TestArchiveReportsMissingResult(t *testing.T) {
	err := Archive(context.Background(), Options{})
	if err == nil || !strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("expected missing result error, got %v", err)
	}
}

func TestArchiveUsesBenchmarkNameAndWritesSummary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	createArchiveRun(t, filepath.Join(dir, "bench"), "000000", StatusFinished, map[string]string{
		"run/000000/stdout": "out\n",
	})
	var stdout bytes.Buffer
	err := Archive(context.Background(), Options{
		Input: "bench.jbs",
		Result: &sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name": eval.String("bench"),
		}}},
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bench.tar.gz")); err != nil {
		t.Fatalf("expected archive to be written: %v", err)
	}
	text := stdout.String()
	if !strings.Contains(text, "archived bench to bench.tar.gz as ") || !strings.Contains(text, " and removed bench") {
		t.Fatalf("summary = %q", text)
	}
}

func TestArchivePropagatesBenchmarkNameError(t *testing.T) {
	err := Archive(context.Background(), Options{
		Input: "bench.jbs",
		Result: &sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name": eval.Int(1),
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "jbs_name must be a string") {
		t.Fatalf("expected benchmark name error, got %v", err)
	}
}

func TestArchivePropagatesArchivePathError(t *testing.T) {
	err := Archive(context.Background(), Options{
		Result: &sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name": eval.String("bench"),
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty input path") {
		t.Fatalf("expected archive path error, got %v", err)
	}
}

func TestArchivePropagatesArchiveRootError(t *testing.T) {
	err := Archive(context.Background(), Options{
		Input: "bench.jbs",
		Result: &sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name": eval.String("missing"),
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot lock benchmark root") {
		t.Fatalf("expected archive root error, got %v", err)
	}
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

func TestArchiveRootRejectsLockedRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": ""})
	unlock, err := acquireExistingRootLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	_, err = ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected lock error, got %v", err)
	}
}

func TestArchiveRootRejectsEmptyRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ArchiveRoot(root, filepath.Join(dir, "bench.tar.gz"), time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "no run directories") {
		t.Fatalf("expected no-runs error, got %v", err)
	}
}

func TestArchiveableRunsRejectsMissingStatus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	if err := os.MkdirAll(filepath.Join(root, "000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := archiveableRuns(root)
	if err == nil || !strings.Contains(err.Error(), "cannot archive run 000000") {
		t.Fatalf("expected missing status error, got %v", err)
	}
}

func TestArchiveRootPropagatesRewriteError(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": ""})
	_, err := ArchiveRoot(root, filepath.Join(dir, "missing", "bench.tar.gz"), time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected rewrite error")
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("expected root to remain after rewrite error: %v", statErr)
	}
}

func TestArchiveableRunsReportsReadDirError(t *testing.T) {
	_, err := archiveableRuns(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected readdir error")
	}
}

func TestArchiveRootIncludesComponentRuns(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	createArchiveRun(t, filepath.Join(root, "small"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "small\n"})
	createArchiveRun(t, filepath.Join(root, "large"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "large\n"})
	archive := filepath.Join(dir, "bench.tar.gz")
	if _, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	entries := readTarGzEntries(t, archive)
	if _, ok := entries["20260508T140000.000000000Z/bench/small/000000/status"]; !ok {
		t.Fatalf("archive missing small component status; names=%v", sortedArchiveNames(entries))
	}
	if _, ok := entries["20260508T140000.000000000Z/bench/large/000000/status"]; !ok {
		t.Fatalf("archive missing large component status; names=%v", sortedArchiveNames(entries))
	}
}

func TestArchiveRootIncludesMixedDirectAndComponentRuns(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	archive := filepath.Join(dir, "bench.tar.gz")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{"run/000000/stdout": "direct\n"})
	createArchiveRun(t, filepath.Join(root, "small"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "small\n"})
	createArchiveRun(t, filepath.Join(root, "large"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "large\n"})

	result, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.RunCount != 3 {
		t.Fatalf("RunCount = %d, want 3", result.RunCount)
	}

	entries := readTarGzEntries(t, archive)
	for _, name := range []string{
		"20260508T140000.000000000Z/bench/000000/status",
		"20260508T140000.000000000Z/bench/small/000000/status",
		"20260508T140000.000000000Z/bench/large/000000/status",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("archive missing %q; entries=%v", name, sortedArchiveNames(entries))
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("expected original root to be removed, stat error: %v", err)
	}
}

func TestArchiveRootRejectsLockedComponentRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	createArchiveRun(t, filepath.Join(root, "small"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "small\n"})
	unlock, err := acquireExistingRootLock(filepath.Join(root, "small"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	_, err = ArchiveRoot(root, filepath.Join(dir, "bench.tar.gz"), time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected component lock error, got %v", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("expected root to remain after component lock failure: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "bench.tar.gz")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no archive after component lock failure, stat error: %v", statErr)
	}
}

func TestArchiveRootDoesNotArchiveComponentLockFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	createArchiveRun(t, filepath.Join(root, "small"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "small\n"})
	createArchiveRun(t, filepath.Join(root, "large"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "large\n"})
	archive := filepath.Join(dir, "bench.tar.gz")
	if _, err := ArchiveRoot(root, archive, time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	for name := range readTarGzEntries(t, archive) {
		if strings.Contains(name, rootLockName) || strings.Contains(name, rootLockReclaimName) {
			t.Fatalf("component lock entry archived: %q", name)
		}
	}
}

func TestArchiveRootRejectsRunningComponentRun(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	createArchiveRun(t, filepath.Join(root, "small"), "000000", StatusFinished, map[string]string{"run/000000/stdout": "small\n"})
	createArchiveRun(t, filepath.Join(root, "large"), "000000", StatusRunning, map[string]string{"run/000000/stdout": "large\n"})
	_, err := ArchiveRoot(root, filepath.Join(dir, "bench.tar.gz"), time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "large") || !strings.Contains(err.Error(), "RUNNING") {
		t.Fatalf("expected running component rejection, got %v", err)
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

func TestRewriteArchiveWithSnapshotReportsExistingArchiveReadError(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "bench.tar.gz")
	if err := os.WriteFile(archive, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := rewriteArchiveWithSnapshot(root, archive, "stamp", nil)
	if err == nil || !strings.Contains(err.Error(), "read existing archive") {
		t.Fatalf("expected existing archive read error, got %v", err)
	}
}

func TestRewriteArchiveWithSnapshotReportsCreateTempError(t *testing.T) {
	err := rewriteArchiveWithSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "missing", "bench.tar.gz"), "stamp", nil)
	if err == nil {
		t.Fatal("expected create-temp error")
	}
}

func TestRewriteArchiveWithSnapshotReportsAppendError(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "bench.tar.gz")
	err := rewriteArchiveWithSnapshot(root, archive, "stamp", []string{"000000"})
	if err == nil {
		t.Fatal("expected append error for missing run")
	}
	if _, statErr := os.Stat(archive); !os.IsNotExist(statErr) {
		t.Fatalf("expected archive not to be renamed into place, stat error: %v", statErr)
	}
}

func TestRemoveArchivedRootReportsRenameError(t *testing.T) {
	err := removeArchivedRoot(filepath.Join(t.TempDir(), "missing"), "stamp")
	if err == nil || !strings.Contains(err.Error(), "failed to move benchmark directory") {
		t.Fatalf("expected rename error, got %v", err)
	}
}

func TestUniqueRemovalPathUsesFallbackStampAndAvoidsCollision(t *testing.T) {
	parent := t.TempDir()
	got, err := uniqueRemovalPath(parent, "bench", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(got), "archive-") {
		t.Fatalf("fallback path = %q", got)
	}

	first := filepath.Join(parent, ".archived-bench-stamp-"+fmtInt(os.Getpid()))
	if err := os.Mkdir(first, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = uniqueRemovalPath(parent, "bench", "stamp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "-001") {
		t.Fatalf("collision path = %q", got)
	}
}

func TestUniqueRemovalPathReportsLstatErrorAndExhaustion(t *testing.T) {
	fileParent := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(fileParent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := uniqueRemovalPath(fileParent, "bench", "stamp"); err == nil {
		t.Fatal("expected lstat error below file parent")
	}

	parent := t.TempDir()
	stamp := "full"
	for i := 0; i < 1000; i++ {
		suffix := stamp + "-" + fmtInt(os.Getpid())
		if i > 0 {
			suffix = stamp + "-" + fmtInt(os.Getpid()) + "-" + fmt.Sprintf("%03d", i)
		}
		if err := os.Mkdir(filepath.Join(parent, ".archived-bench-"+suffix), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := uniqueRemovalPath(parent, "bench", stamp); err == nil || !strings.Contains(err.Error(), "could not choose removal path") {
		t.Fatalf("expected exhaustion error, got %v", err)
	}
}

func TestCopyExistingArchiveReportsWriteError(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "existing.tar.gz")
	writeTarGzEntries(t, archive, []archiveTestEntry{{
		Header: tar.Header{Name: "old/file.txt", Mode: 0o644, Size: int64(len("old\n")), Typeflag: tar.TypeReg},
		Body:   "old\n",
	}})
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := copyExistingArchive(archive, tw); err == nil {
		t.Fatal("expected write-after-close error")
	}
}

func TestWalkExistingArchiveReportsUnsafeEntryAndVisitError(t *testing.T) {
	unsafeArchive := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	writeTarGzEntries(t, unsafeArchive, []archiveTestEntry{{
		Header: tar.Header{Name: "../evil", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg},
		Body:   "x",
	}})
	err := walkExistingArchive(unsafeArchive, func(*tar.Header, *tar.Reader) error {
		t.Fatal("visit should not be called for unsafe entries")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe archive entry") {
		t.Fatalf("expected unsafe entry error, got %v", err)
	}

	validArchive := filepath.Join(t.TempDir(), "valid.tar.gz")
	writeTarGzEntries(t, validArchive, []archiveTestEntry{{
		Header: tar.Header{Name: "old/file.txt", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg},
		Body:   "x",
	}})
	err = walkExistingArchive(validArchive, func(*tar.Header, *tar.Reader) error {
		return errors.New("visit failed")
	})
	if err == nil || !strings.Contains(err.Error(), "visit failed") {
		t.Fatalf("expected visit error, got %v", err)
	}
}

func TestWalkExistingArchiveReportsTarReadError(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad-tar.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte("not a tar stream")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	err = walkExistingArchive(archive, func(*tar.Header, *tar.Reader) error {
		t.Fatal("visit should not be called for malformed tar")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "read existing archive") {
		t.Fatalf("expected tar read error, got %v", err)
	}
}

func TestValidateArchiveNameRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "/abs", "../x", "a/../x", "."} {
		if err := validateArchiveName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
	if err := validateArchiveName("safe/path"); err != nil {
		t.Fatal(err)
	}
}

func TestAppendBenchmarkSnapshotReportsClosedWriter(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	err := appendBenchmarkSnapshot(tw, t.TempDir(), "stamp", nil, time.Now().UTC())
	if err == nil {
		t.Fatal("expected closed writer error")
	}
}

func TestAppendTreeReportsMissingRootAndClosedWriter(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := appendTree(tw, filepath.Join(t.TempDir(), "missing"), "root"); err == nil {
		t.Fatal("expected missing root error")
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendTree(tw, root, "root"); err == nil {
		t.Fatal("expected closed writer error")
	}
}

func TestAppendManifestDatabasesHandlesRelativeDuplicateMissingExternalAndRunLocalDatabases(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeArchiveManifest(t, filepath.Join(root, "000000", "manifest.json"), Manifest{AnalyseDatabasePath: "results.sqlite"})
	writeArchiveManifest(t, filepath.Join(root, "000001", "manifest.json"), Manifest{AnalyseDatabasePath: "results.sqlite"})
	writeArchiveManifest(t, filepath.Join(root, "000002", "manifest.json"), Manifest{AnalyseDatabasePath: filepath.Join(root, "000002", "inside.sqlite")})
	writeArchiveManifest(t, filepath.Join(root, "000003", "manifest.json"), Manifest{AnalyseDatabasePath: filepath.Join(t.TempDir(), "external.sqlite")})
	writeArchiveManifest(t, filepath.Join(root, "000004", "manifest.json"), Manifest{})
	writeArchiveManifest(t, filepath.Join(root, "000006", "manifest.json"), Manifest{AnalyseDatabasePath: filepath.Join(root, "missing.sqlite")})
	if err := os.WriteFile(filepath.Join(root, "results.sqlite"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := appendManifestDatabases(tw, root, "stamp/bench", []string{"000000", "000001", "000002", "000003", "000004", "000005", "000006"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readTarEntriesFromBytes(t, buf.Bytes())
	if got := entries["stamp/bench/results.sqlite"].Body; got != "db" {
		t.Fatalf("database body = %q entries=%v", got, archiveEntryNames(entries))
	}
	for name := range entries {
		if strings.Contains(name, "inside.sqlite") || strings.Contains(name, "external.sqlite") || strings.Contains(name, "missing.sqlite") {
			t.Fatalf("unexpected database entry %q", name)
		}
	}
}

func TestAppendManifestDatabasesReportsManifestAndDirectoryErrors(t *testing.T) {
	t.Run("manifest read", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "000000"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "000000", "manifest.json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		err := appendManifestDatabases(tar.NewWriter(&buf), root, "stamp/bench", []string{"000000"})
		if err == nil || !strings.Contains(err.Error(), "inspect archive database") {
			t.Fatalf("expected manifest read error, got %v", err)
		}
	})

	t.Run("directory database", func(t *testing.T) {
		root := t.TempDir()
		dbDir := filepath.Join(root, "results.sqlite")
		if err := os.Mkdir(dbDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeArchiveManifest(t, filepath.Join(root, "000000", "manifest.json"), Manifest{AnalyseDatabasePath: dbDir})
		var buf bytes.Buffer
		err := appendManifestDatabases(tar.NewWriter(&buf), root, "stamp/bench", []string{"000000"})
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("expected directory database error, got %v", err)
		}
	})
}

func TestAppendManifestDatabasesPropagatesAppendTreeError(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "results.sqlite")
	if err := os.WriteFile(dbPath, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArchiveManifest(t, filepath.Join(root, "000000", "manifest.json"), Manifest{AnalyseDatabasePath: dbPath})
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	err := appendManifestDatabases(tw, root, "stamp/bench", []string{"000000"})
	if err == nil {
		t.Fatal("expected append tree error")
	}
}

func TestIsInsideArchivedRun(t *testing.T) {
	if !isInsideArchivedRun("000000/results.sqlite", []string{"000000"}) {
		t.Fatal("expected run-local database to be detected")
	}
	if isInsideArchivedRun("results.sqlite", []string{"000000"}) {
		t.Fatal("root database should not be run-local")
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

func writeArchiveManifest(t *testing.T, filePath string, manifest Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sortedArchiveNames(entries map[string]archiveTestEntry) []string {
	return slices.Sorted(maps.Keys(entries))
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
		if hdr.Typeflag == tar.TypeReg {
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

func writeTarGzEntries(t *testing.T, archivePath string, entries []archiveTestEntry) {
	t.Helper()
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		header := entry.Header
		if header.Typeflag == 0 {
			header.Typeflag = tar.TypeReg
		}
		if header.Typeflag == tar.TypeReg && header.Size == 0 {
			header.Size = int64(len(entry.Body))
		}
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func readTarEntriesFromBytes(t *testing.T, data []byte) map[string]archiveTestEntry {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
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
		if hdr.Typeflag == tar.TypeReg {
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

func archiveEntryNames(entries map[string]archiveTestEntry) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	return names
}

func fmtInt(v int) string {
	return strconv.FormatInt(int64(v), 10)
}
