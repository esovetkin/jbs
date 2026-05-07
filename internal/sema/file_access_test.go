package sema

import (
	"path/filepath"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestBaseDirForProgramFile(t *testing.T) {
	if got := baseDirForProgramFile(""); got != "" {
		t.Fatalf("expected empty base dir for empty file, got %q", got)
	}
	if got := baseDirForProgramFile("<repl>"); got != "" {
		t.Fatalf("expected empty base dir for pseudo file, got %q", got)
	}
	if got := baseDirForProgramFile("dir/main.jbs"); got != "dir" {
		t.Fatalf("expected relative base dir 'dir', got %q", got)
	}
	abs := filepath.Join("/tmp", "jbs", "main.jbs")
	if got := baseDirForProgramFile(abs); got != filepath.Dir(abs) {
		t.Fatalf("expected absolute base dir %q, got %q", filepath.Dir(abs), got)
	}
}

func TestFileAccessForSpan(t *testing.T) {
	span := diag.NewSpan("/tmp/jbs/main.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	access := fileAccessForSpan(map[string]string{"/tmp/jbs/main.jbs": "/tmp/jbs"}, span)
	if access == nil || access.BaseDir != "/tmp/jbs" {
		t.Fatalf("expected exact-match file access, got %#v", access)
	}

	fallback := fileAccessForSpan(nil, span)
	if fallback != nil {
		t.Fatalf("expected nil fallback without base-dir map, got %#v", fallback)
	}

	auto := fileAccessForSpan(map[string]string{"other": "/tmp/other"}, span)
	if auto == nil || auto.BaseDir != "/tmp/jbs" {
		t.Fatalf("expected program-file fallback, got %#v", auto)
	}

	single := fileAccessForSpan(map[string]string{"<repl>": "/tmp/repl"}, diag.Span{})
	if single == nil || single.BaseDir != "/tmp/repl" {
		t.Fatalf("expected single-entry fallback, got %#v", single)
	}
}
