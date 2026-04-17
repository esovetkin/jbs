package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/diag"
)

func writeCLIFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestAnalyzeInputWithImports(t *testing.T) {
	cwd := t.TempDir()
	libPath := writeCLIFile(t, cwd, "lib.jbs", "value = 3\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use value from \"./lib.jbs\"\na = value + 1\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if bundle == nil {
		t.Fatalf("expected analysis bundle")
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if _, ok := bundle.Sources[mainPath]; !ok {
		t.Fatalf("expected main source in bundle")
	}
	if _, ok := bundle.Sources[libPath]; !ok {
		t.Fatalf("expected imported lib source in bundle")
	}
	if gv := bundle.Result.GlobalVarByName["a"]; gv == nil || gv.Value.I != 4 {
		t.Fatalf("expected imported value to resolve in semantic analysis, got %#v", gv)
	}
}

func TestAnalyzeSourceWithNamespaceAwareImportPlan(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "x = (1, 2)\njobs = comb(x)\n")
	src := "use \"./lib.jbs\" as lib\n" +
		"do s\n" +
		"        with x from lib.jobs\n" +
		"{\n" +
		"        echo ${x}\n" +
		"}\n"

	diags := &diag.Diagnostics{}
	bundle, err := analyzeSource("<input>", src, cwd, diags)
	if err != nil {
		t.Fatalf("analyzeSource failed: %v", err)
	}
	if bundle == nil {
		t.Fatalf("expected analysis bundle")
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	plan := bundle.Result.StepScopeByName["s"]
	if plan == nil {
		t.Fatalf("expected step scope plan for imported namespace step")
	}
	origin, ok := plan.Effective["x"]
	if !ok || origin.Source != "lib.jobs" || origin.SourceVar != "x" {
		t.Fatalf("expected namespace-qualified effective import, got %#v", plan.Effective)
	}
}

func TestRunPrintParamWithImportedModule(t *testing.T) {
	cwd := t.TempDir()
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use \"./lib.jbs\" as lib\n"+
		"do s\n"+
		"        with lib.jobs\n"+
		"{\n"+
		"        echo ${x} ${y}\n"+
		"}\n")
	writeCLIFile(t, cwd, "lib.jbs", "x = (1, 2)\ny = (\"a\", \"b\")\njobs = comb(x + y)\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"printparam", "-t", "pretty", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful printparam run, code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lib.jobs.x") || !strings.Contains(out, "lib.jobs.y") || !strings.Contains(out, "do: s") {
		t.Fatalf("expected imported module columns in printparam output, got:\n%s", out)
	}
	errText := stderr.String()
	if !strings.Contains(errText, "WARNING W310") || !strings.Contains(errText, "lib.jbs") {
		t.Fatalf("expected warning diagnostics from imported globals, got %q", errText)
	}
}

func TestRunYAMLWithImportedModule(t *testing.T) {
	cwd := t.TempDir()
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use \"./lib.jbs\" as lib\n"+
		"do s\n"+
		"        with lib.jobs\n"+
		"{\n"+
		"        echo ${x}\n"+
		"}\n")
	writeCLIFile(t, cwd, "lib.jbs", "x = (1, 2)\njobs = comb(x)\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--output", "-", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful YAML run, code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "name:") || !strings.Contains(out, "step:") || !strings.Contains(out, "- name: s") {
		t.Fatalf("expected benchmark YAML output for imported module input, got:\n%s", out)
	}
}

func TestRunCheckFormatsDiagnosticsAcrossImportedSources(t *testing.T) {
	cwd := t.TempDir()
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use \"./lib.jbs\" as lib\n")
	libPath := writeCLIFile(t, cwd, "lib.jbs", "value = (\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", mainPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failing check for imported-source syntax error, code=%d stderr=%s", code, stderr.String())
	}
	text := stderr.String()
	if !strings.Contains(text, libPath) || !strings.Contains(text, "value = (") {
		t.Fatalf("expected diagnostics to include imported file path and excerpt, got:\n%s", text)
	}
}
