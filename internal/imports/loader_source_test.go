package imports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestLoadAndExpandSourceSupportsSelectiveLocalImports(t *testing.T) {
	cwd := t.TempDir()
	localPath := writeTestFile(t, cwd, "mylib.jbs", "local_value = 9\n")

	src := strings.Join([]string{
		"use local_value from \"./mylib.jbs\"",
		"result = local_value",
	}, "\n")
	diags := &diag.Diagnostics{}
	res, err := LoadAndExpandSource("<repl>", src, cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
	if info := res.Modules[res.Entry.ID]; info == nil || info.Program.File != "<repl>" {
		t.Fatalf("unexpected entry module info: %#v", info)
	}
	if _, ok := res.Sources["<repl>"]; !ok {
		t.Fatalf("expected entry source in map")
	}
	if _, ok := res.Sources[localPath]; !ok {
		t.Fatalf("expected local module source in map")
	}
	if hasErrorDiagnostics(diags) {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestLoadAndExpandSourceResolvesNestedQuotedImportsRelativeToImporter(t *testing.T) {
	projectDir := t.TempDir()
	cwd := t.TempDir()
	subDir := filepath.Join(projectDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	writeTestFile(t, subDir, "b.jbs", "base = 41\n")
	aPath := writeTestFile(t, subDir, "a.jbs", strings.Join([]string{
		"use base from \"./b.jbs\"",
		"value = base + 1",
	}, "\n"))

	src := strings.Join([]string{
		"use value from \"./sub/a.jbs\"",
		"result = value",
	}, "\n")
	diags := &diag.Diagnostics{}
	res, err := LoadAndExpandSource("<repl>", src, projectDir, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
	if _, ok := res.Sources["<repl>"]; !ok {
		t.Fatalf("expected entry source in map")
	}
	if _, ok := res.Sources[aPath]; !ok {
		t.Fatalf("expected sub/a.jbs source in map")
	}
	bPath := filepath.Join(subDir, "b.jbs")
	if _, ok := res.Sources[bPath]; !ok {
		t.Fatalf("expected sub/b.jbs source in map")
	}
	if hasErrorDiagnostics(diags) {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestLoadAndExpandSourceRejectsBareLocalImports(t *testing.T) {
	cwd := t.TempDir()
	writeTestFile(t, cwd, "mylib.jbs", "local_value = 9\n")

	diags := &diag.Diagnostics{}
	res, err := LoadAndExpandSource("<repl>", "use local_value from mylib\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("unexpected loader error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result even with diagnostics")
	}
	if !hasDiagCode(diags, "E537") {
		t.Fatalf("expected E537 diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "rewrite it as `use local_value from \"./mylib.jbs\"` for the local file") {
		t.Fatalf("expected exact migration hint, got: %s", diags.String())
	}
}

func TestLoadAndExpandSourceReportsQuotedPathErrors(t *testing.T) {
	cwd := t.TempDir()
	src := "use \"./x.txt\" as x\n"
	diags := &diag.Diagnostics{}
	res, err := LoadAndExpandSource("<repl>", src, cwd, cwd, diags)
	if err != nil {
		t.Fatalf("unexpected loader error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result even with diagnostics")
	}
	if !hasDiagCode(diags, "E535") {
		t.Fatalf("expected E535 diagnostic, got: %s", diags.String())
	}
	if !hasDiagCode(diags, "E531") {
		t.Fatalf("expected E531 diagnostic, got: %s", diags.String())
	}
}

func TestLoadAndExpandFileModeControlWithUse(t *testing.T) {
	cwd := t.TempDir()
	writeTestFile(t, cwd, "lib.jbs", "v = 5\n")
	entry := writeTestFile(t, cwd, "entry.jbs", "use v from \"./lib.jbs\"\nresult = v\n")
	res, err := LoadAndExpand(entry, cwd, &diag.Diagnostics{})
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if res == nil || res.Modules[res.Entry.ID] == nil {
		t.Fatalf("expected non-nil result with entry module")
	}
}

func TestLoadAndExpandSourceDiagnosticsLabelEntryAsRepl(t *testing.T) {
	cwd := t.TempDir()
	diags := &diag.Diagnostics{}
	_, err := LoadAndExpandSource("<repl>", "use v from \"./missing.jbs\"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("unexpected loader error: %v", err)
	}
	found := false
	for _, item := range diags.Items {
		if item.Span.File == "<repl>" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one diagnostic anchored to <repl>, got: %s", diags.String())
	}
}

func hasErrorDiagnostics(diags *diag.Diagnostics) bool {
	for _, item := range diags.Items {
		if item.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
