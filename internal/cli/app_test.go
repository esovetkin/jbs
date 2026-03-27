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

func TestRunNoArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run(nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage text, got: %s", out.String())
	}
}

func TestRunHelpGlobals(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	code := Run([]string{"help", "globals"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	text := out.String()
	if !strings.Contains(text, `# Benchmark name (root name field). maps_to: root:name. mode: -`) {
		t.Fatalf("expected jbs_name comment line, got: %s", text)
	}
	if !strings.Contains(text, `jbs_name = "jbs_benchmark"`) {
		t.Fatalf("expected jbs_name assignment, got: %s", text)
	}
	if !strings.Contains(text, `jbs_queue = python(`) {
		t.Fatalf("expected globals help output, got: %s", out.String())
	}
	if strings.Contains(text, "Globals:") {
		t.Fatalf("expected script-style help output without legacy Globals section, got: %s", text)
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

func TestRunNoDuplicateUnknownParamsetDiagnostic(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do setup with x from missing_set {
  echo setup
}
`
	dir := t.TempDir()
	in := filepath.Join(dir, "bad_unknown.jbs")
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
	if got := strings.Count(errText, "unknown parameterset 'missing_set'"); got != 1 {
		t.Fatalf("expected exactly one unknown-paramset diagnostic, got %d\n%s", got, errText)
	}
}
