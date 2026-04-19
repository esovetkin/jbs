package sema

import (
	"os"
	"path/filepath"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/imports"
	"jbs/internal/parser"
)

func TestAnalyzeCollectsSubmitAndCompilesSubmitSpec(t *testing.T) {
	src := `
x = 1

do prep {
  echo prep
}

submit run
  after prep
{
  account = "a"
  queue = "q"
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)

	if res == nil {
		t.Fatalf("Analyze returned nil result")
	}
	if len(res.DoBlocks) != 1 || res.DoBlocks[0].Name != "prep" {
		t.Fatalf("unexpected do blocks in analysis result: %#v", res.DoBlocks)
	}
	if len(res.Submits) != 1 || res.Submits[0].Name != "run" {
		t.Fatalf("unexpected submit blocks in analysis result: %#v", res.Submits)
	}
	if _, ok := res.SubmitByName["run"]; !ok {
		t.Fatalf("expected compiled submit spec for run, got %#v", res.SubmitByName)
	}
	if _, ok := res.StepScopeByName["run"]; !ok {
		t.Fatalf("expected step scope plan for run submit step, got %#v", res.StepScopeByName)
	}
	if _, ok := res.GlobalVarByName["x"]; !ok {
		t.Fatalf("expected global variable x to be compiled, got %#v", res.GlobalVarByName)
	}
}

func TestAnalyzeCollectsAnalyseBlocks(t *testing.T) {
	src := `
do run {
  echo "N: 1" > out.log
}

analyse run {
  n = "N: %d" in "out.log"
  (n)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)

	if res == nil {
		t.Fatalf("Analyze returned nil result")
	}
	if len(res.Analyse) != 1 || res.Analyse[0] == nil {
		t.Fatalf("expected one compiled analyse spec, got %#v", res.Analyse)
	}
	if res.Analyse[0].Block.StepName != "run" {
		t.Fatalf("unexpected analyse target step: %#v", res.Analyse[0])
	}
}

func TestAnalyzeReturnsTopLevelExprResults(t *testing.T) {
	src := "x = 1\nx\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 {
		t.Fatalf("expected one top-level expr result, got %#v", res.TopLevelExprs)
	}
	if res.TopLevelExprs[0].Index != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(1)) {
		t.Fatalf("unexpected top-level expr result: %#v", res.TopLevelExprs[0])
	}
}

func TestAnalyzeReturnsNamesResults(t *testing.T) {
	src := "x = 1\nnames()\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 {
		t.Fatalf("expected one top-level expr result, got %#v", res.TopLevelExprs)
	}
	want := eval.List([]eval.Value{
		eval.String("jbs_comment"),
		eval.String("jbs_name"),
		eval.String("jbs_outpath"),
		eval.String("x"),
	})
	if !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected names() result: got=%#v want=%#v", res.TopLevelExprs[0].Value, want)
	}
}

func TestAnalyzeWithImportsReturnsTopLevelExprResults(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("value = 41\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", "use \"./lib.jbs\" as lib\nlib.value\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 {
		t.Fatalf("expected one imported top-level expr result, got %#v", res.TopLevelExprs)
	}
	if !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(41)) {
		t.Fatalf("unexpected imported top-level expr value: %#v", res.TopLevelExprs[0])
	}
}

func TestAnalyzeWithImportsReturnsNamesNamespaceResults(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("value = 41\nother = 7\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", "use \"./lib.jbs\" as lib\nnames(lib)\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 {
		t.Fatalf("expected one imported top-level expr result, got %#v", res.TopLevelExprs)
	}
	want := eval.List([]eval.Value{eval.String("other"), eval.String("value")})
	if !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected namespace names result: got=%#v want=%#v", res.TopLevelExprs[0].Value, want)
	}
}

func TestAnalyzeWithImportsNamesNamespaceRespectsVisibilityOrder(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("value = 41\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", "names(lib)\nuse \"./lib.jbs\" as lib\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	_ = AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected visibility-order error")
	}
	if !hasDiagCode(diags, "E100") {
		t.Fatalf("expected E100 for namespace before use, got %s", diags.String())
	}
}

func TestAnalyzeReturnsCombNamesResults(t *testing.T) {
	src := "x = range(2)\ny = range(3)\nparams = comb(x * y)\nnames(params[x])\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 {
		t.Fatalf("expected one comb names result, got %#v", res.TopLevelExprs)
	}
	want := eval.List([]eval.Value{eval.String("x")})
	if !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected comb names result: got=%#v want=%#v", res.TopLevelExprs[0].Value, want)
	}
}

func hasDiagCode(diags *diag.Diagnostics, code string) bool {
	for _, item := range diags.Items {
		if string(item.Code) == code {
			return true
		}
	}
	return false
}
