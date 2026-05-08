package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicReplacesContentAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("new\n"), 0o640, AtomicWriteOptions{}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("expected permissions 0640, got %o", got)
	}
}

func TestWriteJSONAtomicWritesIndentedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status")
	value := map[string]any{
		"status": "FINISHED",
		"schema": float64(1),
	}

	if err := WriteJSONAtomic(path, value, 0o644, AtomicWriteOptions{}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "\n  \"schema\": 1") || !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected indented JSON with trailing newline, got %q", got)
	}
	var decoded map[string]any
	if err := ReadJSON(path, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["status"] != "FINISHED" || decoded["schema"] != float64(1) {
		t.Fatalf("unexpected decoded JSON: %#v", decoded)
	}
}

func TestWriteCSVAtomicReplacesAndEscapes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analyse.csv")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rows := [][]string{{"run_id", "value"}, {"000000", "a,b \"c\"\nnext"}}
	if err := WriteCSVAtomic(path, rows, 0o644, AtomicWriteOptions{}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "old") {
		t.Fatalf("old content was not replaced: %q", got)
	}
	if !strings.Contains(got, "\"a,b \"\"c\"\"\nnext\"") {
		t.Fatalf("csv escaping missing from %q", got)
	}
}

func TestWriteFileAtomicCleansTempOnFillError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	errBoom := errors.New("boom")

	err := writeAtomic(path, 0o644, AtomicWriteOptions{TempSuffix: "test"}, func(*os.File) error {
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected boom, got %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".out.txt.test-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files remain: %v", matches)
	}
}

func TestWriteFileAtomicSyncDirOptionSucceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := WriteFileAtomic(path, []byte("ok\n"), 0o644, AtomicWriteOptions{SyncDir: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}
