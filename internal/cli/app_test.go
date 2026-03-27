package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCompileToStdout(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a * b
}

do prep with p {
  echo prep
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "in.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{in}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "parameterset:") {
		t.Fatalf("expected yaml output, got: %s", out.String())
	}
}

func TestRunNoArgsListsGlobals(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run(nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "jbs_queue") {
		t.Fatalf("expected globals listing, got: %s", out.String())
	}
}

func TestRunShowsSourceExcerptOnError(t *testing.T) {
	src := `
param p {
  a = @
  a
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.jbs")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{in}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	errText := errBuf.String()
	if !strings.Contains(errText, "a = @") {
		t.Fatalf("expected failing source line in diagnostics, got: %s", errText)
	}
	if !strings.Contains(errText, "^") {
		t.Fatalf("expected caret marker in diagnostics, got: %s", errText)
	}
}
