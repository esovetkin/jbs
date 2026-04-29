package sema

import (
	"os"
	"path/filepath"
	"reflect"
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
	src := "x = 1\nadd = function(a, b) { a + b }\nnames()\n"
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
		eval.String("add"),
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
	if err := os.WriteFile(libPath, []byte("value = 41\nother = 7\nadd = function(a, b) { a + b }\n"), 0o644); err != nil {
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
	want := eval.List([]eval.Value{eval.String("add"), eval.String("other"), eval.String("value")})
	if !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected namespace names result: got=%#v want=%#v", res.TopLevelExprs[0].Value, want)
	}
}

func TestAnalyzeWithImportsReturnsNamesSelectiveFunctionResults(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("value = 41\nadd = function(a, b) { a + b }\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", "use add from \"./lib.jbs\"\nnames()\n", cwd, cwd, diags)
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
	want := eval.List([]eval.Value{
		eval.String("add"),
		eval.String("jbs_comment"),
		eval.String("jbs_name"),
		eval.String("jbs_outpath"),
	})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected selective names() result: %#v", res.TopLevelExprs)
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

func TestAnalyzeReturnsTableNamesResults(t *testing.T) {
	src := "x = range(2)\ny = range(3)\nparams = product(table(x = x), table(y = y))\nnames(select(params, x))\n"
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
		t.Fatalf("expected one table names result, got %#v", res.TopLevelExprs)
	}
	want := eval.List([]eval.Value{eval.String("x")})
	if !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected table names result: got=%#v want=%#v", res.TopLevelExprs[0].Value, want)
	}
}

func TestAnalyzeSupportsTableShortcut(t *testing.T) {
	src := "x = range(2)\ny = range(3)\nparams = product(t(x = x), t(y = y))\nnames(select(params, x))\n"
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
	want := eval.List([]eval.Value{eval.String("x")})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected table shortcut result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeKeepsFunctionGlobalsVisibleWithoutBindings(t *testing.T) {
	src := `
base = 40
mk = function(delta) {
	function(x) {
		x + delta + base
	}
}
inc = mk(1)
value = inc(1)
value
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("functions.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if res.Globals.Values["inc"].Kind != eval.KindFunction {
		t.Fatalf("expected function global inc in Globals.Values, got %#v", res.Globals.Values["inc"])
	}
	if _, ok := res.BindingsByName["inc"]; ok {
		t.Fatalf("did not expect function global inc to become a binding")
	}
	if res.GlobalVarByName["value"] == nil || !eval.Equal(res.GlobalVarByName["value"].Value, eval.Int(42)) {
		t.Fatalf("expected analyzed value=42, got %#v", res.GlobalVarByName["value"])
	}
	if !reflect.DeepEqual(res.GlobalVarByName["value"].DependsOn, []string{"base", "inc", "mk"}) {
		t.Fatalf("unexpected runtime dependency set for value: %#v", res.GlobalVarByName["value"])
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(42)) {
		t.Fatalf("unexpected top-level expr results: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeSupportsMapReduceWithClosureGlobals(t *testing.T) {
	src := `
base = 10
inc = function(x) {
	x + base
}
mapped = map(inc, [1,2,3])
total = reduce(function(acc, x) {
	acc + x
}, mapped)
mapped
total
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("higher_order.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	wantMapped := eval.List([]eval.Value{eval.Int(11), eval.Int(12), eval.Int(13)})
	if gv := res.GlobalVarByName["mapped"]; gv == nil || !eval.Equal(gv.Value, wantMapped) {
		t.Fatalf("expected mapped global %#v, got %#v", wantMapped, gv)
	}
	if gv := res.GlobalVarByName["total"]; gv == nil || !eval.Equal(gv.Value, eval.Int(36)) {
		t.Fatalf("expected total=36, got %#v", gv)
	}
	if len(res.TopLevelExprs) != 2 || !eval.Equal(res.TopLevelExprs[0].Value, wantMapped) || !eval.Equal(res.TopLevelExprs[1].Value, eval.Int(36)) {
		t.Fatalf("unexpected top-level expr results: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeDoBlocksUseSourceOrderSnapshots(t *testing.T) {
	src := `
cases = table(x = (1))
do first
        with cases
{
        echo ${x}
}
cases = table(x = (2))
do second
        with cases
{
        echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("snapshots.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("parse failed: %s", diags.String())
	}
	res := Analyze(prog, nil, diags)
	if diags.HasErrors() {
		t.Fatalf("analysis failed: %s", diags.String())
	}

	first := res.StepScopeByName["first"].Effective["x"]
	second := res.StepScopeByName["second"].Effective["x"]
	firstBinding := res.BindingsByName[first.Source]
	secondBinding := res.BindingsByName[second.Source]
	if firstBinding == nil || secondBinding == nil {
		t.Fatalf("expected snapshot bindings, first=%#v second=%#v", firstBinding, secondBinding)
	}
	if firstBinding.Name == secondBinding.Name || firstBinding.PublicName != "cases" || secondBinding.PublicName != "cases" {
		t.Fatalf("expected distinct snapshot bindings for public cases, first=%#v second=%#v", firstBinding, secondBinding)
	}
	if !eval.Equal(firstBinding.Vars["x"][0], eval.Int(1)) || !eval.Equal(secondBinding.Vars["x"][0], eval.Int(2)) {
		t.Fatalf("unexpected snapshot values: first=%#v second=%#v", firstBinding.Vars["x"], secondBinding.Vars["x"])
	}
}

func TestAnalyzeWithImportsSupportsMapReduceCallbacks(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("inc = function(x) {\n  x + 1\n}\nsum2 = function(acc, x) {\n  acc + x\n}\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", "use \"./lib.jbs\" as lib\nuse sum2 from \"./lib.jbs\"\nmap(lib.inc, [1,2,3])\nreduce(sum2, [1,2,3])\n", cwd, cwd, diags)
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
	wantMapped := eval.List([]eval.Value{eval.Int(2), eval.Int(3), eval.Int(4)})
	if len(res.TopLevelExprs) != 2 || !eval.Equal(res.TopLevelExprs[0].Value, wantMapped) || !eval.Equal(res.TopLevelExprs[1].Value, eval.Int(6)) {
		t.Fatalf("unexpected top-level expr results: %#v", res.TopLevelExprs)
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
