package repl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHistoryPathUsesCwd(t *testing.T) {
	dir := t.TempDir()
	path, err := ResolveHistoryPath(dir)
	if err != nil {
		t.Fatalf("ResolveHistoryPath returned error: %v", err)
	}
	want := filepath.Join(dir, ".jbs_history")
	if path != want {
		t.Fatalf("history path mismatch: got=%q want=%q", path, want)
	}
}

func TestResolveHistoryPathFromEmptyCwd(t *testing.T) {
	path, err := ResolveHistoryPath("")
	if err != nil {
		t.Fatalf("ResolveHistoryPath returned error: %v", err)
	}
	if filepath.Base(path) != ".jbs_history" {
		t.Fatalf("unexpected basename: %q", path)
	}
}

func TestEnsureHistoryDirCreatesParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", ".jbs_history")
	if err := EnsureHistoryDir(path); err != nil {
		t.Fatalf("EnsureHistoryDir returned error: %v", err)
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("expected parent dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("parent is not a directory: %q", parent)
	}
}
