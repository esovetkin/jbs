package sema

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

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
		"jbs_name": eval.String("bench"),
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
		"jbs_name": eval.String("bench"),
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

func TestAnalyzeDictionaryIndexAndIteration(t *testing.T) {
	src := `
d = dict(a = 1, b = 2)
keys = ()
for key in d {
        keys += (key,)
}
d["a"]
keys
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 2 {
		t.Fatalf("expected two top-level expression results, got %#v", res.TopLevelExprs)
	}
	if !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(1)) {
		t.Fatalf("unexpected dictionary index result: %#v", res.TopLevelExprs[0])
	}
	wantKeys := eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")})
	if !eval.Equal(res.TopLevelExprs[1].Value, wantKeys) {
		t.Fatalf("unexpected dictionary iteration result: got=%#v want=%#v", res.TopLevelExprs[1].Value, wantKeys)
	}
}

func TestAnalyzeSequenceIndexing(t *testing.T) {
	src := `
xs = [10, 20, 30]
last = xs[-1]
selected = xs[[0, -1]]
masked = xs[[true, false]]
last
selected
masked
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 3 {
		t.Fatalf("expected three top-level expression results, got %#v", res.TopLevelExprs)
	}
	want := []eval.Value{
		eval.Int(30),
		eval.List([]eval.Value{eval.Int(10), eval.Int(30)}),
		eval.List([]eval.Value{eval.Int(10), eval.Int(30)}),
	}
	for i, wantValue := range want {
		if !eval.Equal(res.TopLevelExprs[i].Value, wantValue) {
			t.Fatalf("unexpected result %d: got=%#v want=%#v", i, res.TopLevelExprs[i].Value, wantValue)
		}
	}
}

func TestAnalyzePrintEventsRequireOption(t *testing.T) {
	src := "print(\"quiet\", nrow = 3)\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.PrintEvents) != 0 {
		t.Fatalf("expected default analysis to collect no print events, got %#v", res.PrintEvents)
	}

	diags = &diag.Diagnostics{}
	res = AnalyzeWithOptions(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, AnalyzeOptions{CollectPrints: true}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.PrintEvents) != 1 || len(res.PrintEvents[0].Values) != 1 || res.PrintEvents[0].Values[0].S != "quiet" || res.PrintEvents[0].Options.NRow != 3 {
		t.Fatalf("unexpected collected print events: %#v", res.PrintEvents)
	}
	if len(res.TopLevelExprs) != 1 || res.TopLevelExprs[0].Echo {
		t.Fatalf("expected top-level print expression echo to be suppressed, got %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeCollectsPrintEventsInOrder(t *testing.T) {
	src := `
print("start")
1
f = function(x) {
        print(x)
        x + 1
}
f(2)
for x in (3, 4) {
        print(x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := AnalyzeWithOptions(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, AnalyzeOptions{CollectPrints: true}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.PrintEvents) != 4 {
		t.Fatalf("expected four print events, got %#v", res.PrintEvents)
	}
	want := []eval.Value{eval.String("start"), eval.Int(2), eval.Int(3), eval.Int(4)}
	wantSeq := []int{1, 4, 6, 8}
	for i, wantValue := range want {
		event := res.PrintEvents[i]
		if len(event.Values) != 1 || !eval.Equal(event.Values[0], wantValue) {
			t.Fatalf("event %d: got %#v want %#v", i, event.Values, wantValue)
		}
		if event.Seq != wantSeq[i] {
			t.Fatalf("event %d: got sequence %d want %d", i, event.Seq, wantSeq[i])
		}
	}
	if len(res.TopLevelExprs) != 5 {
		t.Fatalf("expected five expression results, got %#v", res.TopLevelExprs)
	}
	if res.TopLevelExprs[0].Echo || !res.TopLevelExprs[1].Echo || !res.TopLevelExprs[2].Echo || res.TopLevelExprs[3].Echo || res.TopLevelExprs[4].Echo {
		t.Fatalf("unexpected expression echo flags: %#v", res.TopLevelExprs)
	}
	if !eval.Equal(res.TopLevelExprs[1].Value, eval.Int(1)) || !eval.Equal(res.TopLevelExprs[2].Value, eval.Int(3)) {
		t.Fatalf("unexpected echoed expression values: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeWithImportsCollectsEntryPrintsOnly(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	libSrc := "print(\"imported\")\nf = function() { print(\"called\", nrow = 4); 1 }\n"
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", "use \"./lib.jbs\" as lib\nlib.f()\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImportsOptions(loadRes, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, AnalyzeOptions{CollectPrints: true}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.PrintEvents) != 1 || len(res.PrintEvents[0].Values) != 1 || res.PrintEvents[0].Values[0].S != "called" || res.PrintEvents[0].Options.NRow != 4 {
		t.Fatalf("expected only called imported function print, got %#v", res.PrintEvents)
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(1)) {
		t.Fatalf("unexpected expression result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeReturnsNamesResults(t *testing.T) {
	src := "x = 1\nadd = function(a, b) { a + b }\nnames()\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 {
		t.Fatalf("expected one top-level expr result, got %#v", res.TopLevelExprs)
	}
	want := eval.List([]eval.Value{
		eval.String("add"),
		eval.String("jbs_name"),
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
		"jbs_name": eval.String("bench"),
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

func TestAnalyzeWithImportsPreservesDictionaryValues(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte(`config = dict(name = "case", threads = 4)`+"\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", `use "./lib.jbs" as lib`+"\n"+`lib.config["threads"]`+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(4)) {
		t.Fatalf("unexpected imported dictionary result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeWithImportsMaterializesDictionaryFunctions(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	libSrc := "mk = function(x) { x + 1 }\nbundle = dict(fn = mk)\n"
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", `use "./lib.jbs" as lib`+"\n"+`lib.bundle["fn"](4)`+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(5)) {
		t.Fatalf("unexpected imported dictionary function result: %#v", res.TopLevelExprs)
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
		"jbs_name": eval.String("bench"),
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
		"jbs_name": eval.String("bench"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := eval.List([]eval.Value{
		eval.String("add"),
		eval.String("jbs_name"),
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
		"jbs_name": eval.String("bench"),
	}, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected visibility-order error")
	}
	if !hasDiagCode(diags, "E100") {
		t.Fatalf("expected E100 for namespace before use, got %s", diags.String())
	}
}

func TestAnalyzeReturnsTableNamesResults(t *testing.T) {
	src := "x = range(2)\ny = range(3)\nparams = table(x = x) * table(y = y)\nnames(params[\"x\"])\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
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
	src := "x = range(2)\ny = range(3)\nparams = t(x = x) * t(y = y)\nnames(params[\"x\"])\n"
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
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
		"jbs_name": eval.String("bench"),
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
		"jbs_name": eval.String("bench"),
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
	firstBinding := res.BindingsByKey[first.SourceKey]
	secondBinding := res.BindingsByKey[second.SourceKey]
	if firstBinding == nil || secondBinding == nil {
		t.Fatalf("expected snapshot bindings, first=%#v second=%#v", firstBinding, secondBinding)
	}
	if first.SourceKey == second.SourceKey {
		t.Fatalf("expected rebound imports to have distinct source keys, first=%#v second=%#v", first.SourceKey, second.SourceKey)
	}
	if firstBinding.Name != "cases" || secondBinding.Name != "cases" || firstBinding.PublicName != "cases" || secondBinding.PublicName != "cases" {
		t.Fatalf("expected public snapshot binding names for cases, first=%#v second=%#v", firstBinding, secondBinding)
	}
	if firstBinding.VersionID == "" || secondBinding.VersionID == "" || firstBinding.VersionID == secondBinding.VersionID {
		t.Fatalf("expected rebound snapshot bindings to have different versions, first=%#v second=%#v", firstBinding, secondBinding)
	}
	if !eval.Equal(firstBinding.Vars["x"][0], eval.Int(1)) || !eval.Equal(secondBinding.Vars["x"][0], eval.Int(2)) {
		t.Fatalf("unexpected snapshot values: first=%#v second=%#v", firstBinding.Vars["x"], secondBinding.Vars["x"])
	}
}

func TestAnalyzeDoBlocksPreserveBindingVersionAcrossSnapshots(t *testing.T) {
	src := `
cases = table(x = (1))
do first
        with cases
{
        echo ${x}
}
do second
        with cases
{
        echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("same_snapshot.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("parse failed: %s", diags.String())
	}
	res := Analyze(prog, nil, diags)
	if diags.HasErrors() {
		t.Fatalf("analysis failed: %s", diags.String())
	}

	first := res.StepScopeByName["first"].Effective["x"]
	second := res.StepScopeByName["second"].Effective["x"]
	firstBinding := res.BindingsByKey[first.SourceKey]
	secondBinding := res.BindingsByKey[second.SourceKey]
	if firstBinding == nil || secondBinding == nil {
		t.Fatalf("expected snapshot bindings, first=%#v second=%#v", firstBinding, secondBinding)
	}
	if first.SourceKey != second.SourceKey {
		t.Fatalf("expected unchanged snapshots to share a source key, got %#v and %#v", first.SourceKey, second.SourceKey)
	}
	if firstBinding.Name != "cases" || secondBinding.Name != "cases" {
		t.Fatalf("expected public snapshot parameter-set names, got %#v and %#v", firstBinding, secondBinding)
	}
	if firstBinding.VersionID == "" || firstBinding.VersionID != secondBinding.VersionID {
		t.Fatalf("expected unchanged binding snapshots to share a version, first=%#v second=%#v", firstBinding, secondBinding)
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
		"jbs_name": eval.String("bench"),
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
