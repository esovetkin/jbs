package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestRunCheckAcceptsLoopComputedGlobals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	src := `
jbs_name = "loop_demo"
values = ()
for x in range(3) {
	values += (x,)
}
do run with values {
	echo $values
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no check output, got %q", stdout.String())
	}
}

func TestAnalyzeInputImportsLoopProducedGlobal(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", `
sum = 0
for x in range(3) {
	sum += x
}
`)
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use sum from \"./lib.jbs\"\nsum\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if len(bundle.Result.TopLevelExprs) != 1 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, eval.Int(3)) {
		t.Fatalf("unexpected imported loop value: %#v", bundle.Result.TopLevelExprs)
	}
}

func TestRunRejectsDeclarationInsideLoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.jbs")
	if err := os.WriteFile(path, []byte("for x in range(1) { do run { echo bad } }\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{path}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run failure, stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout on parser error, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "ERROR E080") {
		t.Fatalf("expected E080, got %q", stderr.String())
	}
}
