package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func writeCLIFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func cliHasDiagCode(diags *diag.Diagnostics, code string) bool {
	if diags == nil {
		return false
	}
	for _, item := range diags.Items {
		if item.Code == code {
			return true
		}
	}
	return false
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

func TestAnalyzeInputSelectiveImportRequiresANewLocalName(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "b.jbs", "x = 1\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use x from \"./b.jbs\"\nx1 = x + 1\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if gv := bundle.Result.GlobalVarByName["x"]; gv == nil || gv.Value.I != 1 {
		t.Fatalf("expected imported value x=1 to remain available, got %#v", gv)
	}
	if gv := bundle.Result.GlobalVarByName["x1"]; gv == nil || gv.Value.I != 2 {
		t.Fatalf("expected explicit successor binding x1=2, got %#v", gv)
	}
}

func TestAnalyzeInputRejectsBareLocalImportWithMigrationHint(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "value = 3\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use value from lib\na = value + 1\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if bundle == nil {
		t.Fatalf("expected analysis bundle even with diagnostics")
	}
	if !cliHasDiagCode(diags, "E537") {
		t.Fatalf("expected E537 diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "rewrite it as `use value from \"./lib.jbs\"` for the local file") {
		t.Fatalf("expected bare-local migration hint, got: %s", diags.String())
	}
}

func TestAnalyzeInputSelectiveImportUsesSourceModuleScope(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib_dep.jbs", "y = 1\nx = y + 1\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use x from \"./lib_dep.jbs\"\nz = x + 10\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if gv := bundle.Result.GlobalVarByName["x"]; gv == nil || gv.Value.I != 2 {
		t.Fatalf("expected projected import x=2 from source module scope, got %#v", gv)
	}
	if gv := bundle.Result.GlobalVarByName["z"]; gv == nil || gv.Value.I != 12 {
		t.Fatalf("expected dependent local z=12, got %#v", gv)
	}
}

func TestAnalyzeInputSelectiveImportOrderIsStableForDependentGlobals(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib_dep.jbs", "y = 1\nx = y + 1\n")
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "x_then_y",
			source: "use x, y from \"./lib_dep.jbs\"\n" +
				"do run with x {\n" +
				"  echo ${x}\n" +
				"}\n",
		},
		{
			name: "y_then_x",
			source: "use y, x from \"./lib_dep.jbs\"\n" +
				"do run with x {\n" +
				"  echo ${x}\n" +
				"}\n",
		},
	}
	for _, tc := range cases {
		mainPath := writeCLIFile(t, cwd, tc.name+".jbs", tc.source)
		diags := &diag.Diagnostics{}
		bundle, err := analyzeInput(mainPath, diags)
		if err != nil {
			t.Fatalf("%s: analyzeInput failed: %v", tc.name, err)
		}
		if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
			t.Fatalf("%s: expected no error diagnostics: %s", tc.name, diags.String())
		}
		x := bundle.Result.GlobalVarByName["x"]
		y := bundle.Result.GlobalVarByName["y"]
		if x == nil || x.Value.I != 2 || y == nil || y.Value.I != 1 {
			t.Fatalf("%s: unexpected compiled globals: x=%#v y=%#v", tc.name, x, y)
		}
	}
}

func TestAnalyzeInputReadCSVBuildsComb(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "cases.csv", "x,y\n1,2\n3,4\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "params = read_csv(\"./cases.csv\")\nnames(params)\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	params := bundle.Result.GlobalVarByName["params"]
	if params == nil || !eval.IsComb(params.Value) {
		t.Fatalf("expected params comb global, got %#v", params)
	}
	if got := params.Value.C.Order; len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Fatalf("unexpected comb order: %#v", got)
	}
	want := eval.List([]eval.Value{eval.String("x"), eval.String("y")})
	if len(bundle.Result.TopLevelExprs) != 1 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected top-level expr results: %#v", bundle.Result.TopLevelExprs)
	}
}

func TestAnalyzeInputImportedModuleReadCSVUsesModuleBaseDir(t *testing.T) {
	cwd := t.TempDir()
	libDir := filepath.Join(cwd, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	writeCLIFile(t, libDir, "cases.csv", "x,y\n1,2\n3,4\n")
	writeCLIFile(t, libDir, "module.jbs", "params = read_csv(\"./cases.csv\")\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use params from \"./lib/module.jbs\"\nnames(params)\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	params := bundle.Result.GlobalVarByName["params"]
	if params == nil || !eval.IsComb(params.Value) {
		t.Fatalf("expected imported params comb, got %#v", params)
	}
	if first := params.Value.C.Rows[0].Values["x"].Value; first.Kind != eval.KindInt || first.I != 1 {
		t.Fatalf("unexpected imported first x cell: %#v", first)
	}
	want := eval.List([]eval.Value{eval.String("x"), eval.String("y")})
	if len(bundle.Result.TopLevelExprs) != 1 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected imported read_csv expr results: %#v", bundle.Result.TopLevelExprs)
	}
}

func TestAnalyzeInputExprStmtRespectsSelectiveImportVisibility(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "x = 41\n")

	t.Run("before_use_fails", func(t *testing.T) {
		mainPath := writeCLIFile(t, cwd, "before.jbs", "x\nuse x from \"./lib.jbs\"\n")
		diags := &diag.Diagnostics{}
		_, err := analyzeInput(mainPath, diags)
		if err != nil {
			t.Fatalf("analyzeInput failed: %v", err)
		}
		if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) == 0 {
			t.Fatalf("expected error diagnostics when expr line appears before selective import")
		}
	})

	t.Run("after_use_succeeds", func(t *testing.T) {
		mainPath := writeCLIFile(t, cwd, "after.jbs", "use x from \"./lib.jbs\"\nx\n")
		diags := &diag.Diagnostics{}
		bundle, err := analyzeInput(mainPath, diags)
		if err != nil {
			t.Fatalf("analyzeInput failed: %v", err)
		}
		if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
			t.Fatalf("expected no error diagnostics: %s", diags.String())
		}
		if len(bundle.Result.TopLevelExprs) != 1 || bundle.Result.TopLevelExprs[0].Value.I != 41 {
			t.Fatalf("unexpected top-level expr results: %#v", bundle.Result.TopLevelExprs)
		}
	})
}

func TestAnalyzeInputSelectiveImportedFunctionIsCallable(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "add = function(a, b) {\n  a + b\n}\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use add from \"./lib.jbs\"\nadd(1, 2)\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if gv := bundle.Result.GlobalVarByName["add"]; gv == nil || gv.Value.Kind != eval.KindFunction {
		t.Fatalf("expected imported function global add, got %#v", gv)
	}
	if len(bundle.Result.TopLevelExprs) != 1 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, eval.Int(3)) {
		t.Fatalf("unexpected top-level expr results: %#v", bundle.Result.TopLevelExprs)
	}
}

func TestAnalyzeInputNamespaceImportedFunctionIsCallable(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "base = 40\nadd = function(a, b) {\n  a + b + base\n}\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use \"./lib.jbs\" as lib\nlib.add(1, 2)\n")

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if value, ok := bundle.Result.Globals.Values["lib.add"]; !ok || value.Kind != eval.KindFunction {
		t.Fatalf("expected namespaced function global lib.add, got %#v", bundle.Result.Globals.Values["lib.add"])
	}
	if len(bundle.Result.TopLevelExprs) != 1 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, eval.Int(43)) {
		t.Fatalf("unexpected top-level expr results: %#v", bundle.Result.TopLevelExprs)
	}
}

func TestAnalyzeInputImportedFunctionsCoexistWithDataGlobals(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", strings.Join([]string{
		"value = 40",
		"mk = function(delta) {",
		"  function(x) {",
		"    x + delta + value",
		"  }",
		"}",
	}, "\n"))
	mainPath := writeCLIFile(t, cwd, "main.jbs", strings.Join([]string{
		"use value, mk from \"./lib.jbs\"",
		"add = mk(1)",
		"value + add(1)",
		"names()",
	}, "\n"))

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if bundle.Result.GlobalVarByName["mk"] == nil || bundle.Result.GlobalVarByName["mk"].Value.Kind != eval.KindFunction {
		t.Fatalf("expected imported function global mk, got %#v", bundle.Result.GlobalVarByName["mk"])
	}
	if bundle.Result.GlobalVarByName["value"] == nil || bundle.Result.GlobalVarByName["value"].Value.I != 40 {
		t.Fatalf("expected imported data global value, got %#v", bundle.Result.GlobalVarByName["value"])
	}
	if len(bundle.Result.TopLevelExprs) != 2 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, eval.Int(82)) {
		t.Fatalf("unexpected top-level expr results: %#v", bundle.Result.TopLevelExprs)
	}
	wantNames := eval.List([]eval.Value{
		eval.String("add"),
		eval.String("jbs_benchmarks"),
		eval.String("jbs_database"),
		eval.String("jbs_name"),
		eval.String("jbs_nproc"),
		eval.String("mk"),
		eval.String("value"),
	})
	if !eval.Equal(bundle.Result.TopLevelExprs[1].Value, wantNames) {
		t.Fatalf("unexpected names() result: %#v", bundle.Result.TopLevelExprs[1].Value)
	}
}

func TestAnalyzeInputMapReduceWithImportedFunctions(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", strings.Join([]string{
		"inc = function(x) {",
		"  x + 1",
		"}",
		"sum2 = function(acc, x) {",
		"  acc + x",
		"}",
	}, "\n"))
	mainPath := writeCLIFile(t, cwd, "main.jbs", strings.Join([]string{
		"use \"./lib.jbs\" as lib",
		"use sum2 from \"./lib.jbs\"",
		"mapped = map(lib.inc, [1,2,3])",
		"total = reduce(sum2, mapped)",
		"mapped",
		"total",
	}, "\n"))

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	wantMapped := eval.List([]eval.Value{eval.Int(2), eval.Int(3), eval.Int(4)})
	if gv := bundle.Result.GlobalVarByName["mapped"]; gv == nil || !eval.Equal(gv.Value, wantMapped) {
		t.Fatalf("expected mapped global %#v, got %#v", wantMapped, gv)
	}
	if gv := bundle.Result.GlobalVarByName["total"]; gv == nil || !eval.Equal(gv.Value, eval.Int(9)) {
		t.Fatalf("expected total=9, got %#v", gv)
	}
	if len(bundle.Result.TopLevelExprs) != 2 || !eval.Equal(bundle.Result.TopLevelExprs[0].Value, wantMapped) || !eval.Equal(bundle.Result.TopLevelExprs[1].Value, eval.Int(9)) {
		t.Fatalf("unexpected top-level expr results: %#v", bundle.Result.TopLevelExprs)
	}
}

func TestAnalyzeInputCombProjectionAndMemberAccessCompose(t *testing.T) {
	cwd := t.TempDir()
	mainPath := writeCLIFile(t, cwd, "main.jbs", strings.Join([]string{
		"p0 = product(table(x = range(10)), table(y = rev(range(5))))",
		"p1 = product(table(x = range(5)), table(y = rev(range(10))))",
		"",
		"p0[x] + p1[y]",
		"p0[x].x",
		"p0[x].x as y + p1[x]",
		"",
	}, "\n"))

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(mainPath, diags)
	if err != nil {
		t.Fatalf("analyzeInput failed: %v", err)
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	if len(bundle.Result.TopLevelExprs) != 3 {
		t.Fatalf("expected 3 top-level expr results, got %#v", bundle.Result.TopLevelExprs)
	}
	if !eval.IsComb(bundle.Result.TopLevelExprs[0].Value) {
		t.Fatalf("expected first expr result to be comb, got %#v", bundle.Result.TopLevelExprs[0].Value)
	}
	if bundle.Result.TopLevelExprs[1].Value.Kind != eval.KindList || len(bundle.Result.TopLevelExprs[1].Value.L) != 10 {
		t.Fatalf("expected second expr result to be a 10-item list, got %#v", bundle.Result.TopLevelExprs[1].Value)
	}
	if !eval.IsComb(bundle.Result.TopLevelExprs[2].Value) {
		t.Fatalf("expected third expr result to be comb, got %#v", bundle.Result.TopLevelExprs[2].Value)
	}
}

func TestAnalyzeSourceWithNamespaceAwareImportPlan(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "x = (1, 2)\njobs = table(x = x)\n")
	src := "use \"./lib.jbs\" as lib\n" +
		"do s\n" +
		"        with lib.jobs[x]\n" +
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
	if !ok || origin.SourceVar != "x" {
		t.Fatalf("expected namespace-qualified effective import, got %#v", plan.Effective)
	}
	binding := bundle.Result.BindingsByName[origin.Source]
	if binding == nil || binding.PublicName != "lib.jobs" {
		t.Fatalf("expected effective import to resolve through lib.jobs snapshot binding, source=%q binding=%#v", origin.Source, binding)
	}
}

func TestRunParamWithImportedModule(t *testing.T) {
	cwd := t.TempDir()
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use \"./lib.jbs\" as lib\n"+
		"do s\n"+
		"        with lib.jobs\n"+
		"{\n"+
		"        echo ${x} ${y}\n"+
		"}\n")
	writeCLIFile(t, cwd, "lib.jbs", "x = (1, 2)\ny = (\"a\", \"b\")\njobs = table(x = x, y = y)\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"param", "-t", "pretty", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful param run, code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "lib.jobs.x") || !strings.Contains(out, "lib.jobs.y") || !strings.Contains(out, "do: s") {
		t.Fatalf("expected imported module columns in param output, got:\n%s", out)
	}
	errText := stderr.String()
	if strings.Contains(errText, "ERROR") {
		t.Fatalf("did not expect errors from imported globals, got %q", errText)
	}
}

func TestRunParamDoesNotDuplicateHiddenDimensions(t *testing.T) {
	cwd := t.TempDir()
	mainPath := writeCLIFile(t, cwd, "repro.jbs", ""+
		"a = table(a=range(6))\n"+
		"b = table(b = (\"a\", \"b\", \"c\"))\n"+
		"c = table(c = (\"x\",\"z\"))\n"+
		"d = table(d = (true, false))\n"+
		"p0 = c*(a+b)*d\n\n"+
		"do step0\n"+
		"        with p0[a]\n"+
		"{\n"+
		"        echo \"a=${a}\" > step0.out\n"+
		"}\n\n"+
		"do step1\n"+
		"        after step0\n"+
		"        with p0[b,c]\n"+
		"{\n"+
		"        echo \"a=${a}\" > step1.out\n"+
		"        echo \"b=${b}\" >> step1.out\n"+
		"        echo \"c=${c}\" >> step1.out\n"+
		"}\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"param", "-t", "csv", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful param run, code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if got := strings.Count(out, ",do: step0"); got != 6 {
		t.Fatalf("expected 6 step0 rows, got %d\n%s", got, out)
	}
	if got := strings.Count(out, ",do: step1"); got != 12 {
		t.Fatalf("expected 12 step1 rows, got %d\n%s", got, out)
	}
	if got := strings.Count(out, "0,a,x,do: step1"); got != 1 {
		t.Fatalf("expected one visible tuple for step1 0,a,x, got %d\n%s", got, out)
	}
	if !strings.Contains(stderr.String(), "WARNING W310") {
		t.Fatalf("expected warning diagnostics for unused d, got %q", stderr.String())
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

func TestRunCheckWithSelectiveImportedFunction(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "add = function(a, b) {\n  a + b\n}\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use add from \"./lib.jbs\"\nadd(1, 2)\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunCheckWithNamespacedImportedFunction(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", "base = 40\nadd = function(a, b) {\n  a + b + base\n}\n")
	mainPath := writeCLIFile(t, cwd, "main.jbs", "use \"./lib.jbs\" as lib\nlib.add(1, 2)\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful check, code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunCheckWithImportedHigherOrderInitializer(t *testing.T) {
	cwd := t.TempDir()
	writeCLIFile(t, cwd, "lib.jbs", strings.Join([]string{
		"base = 40",
		"mk = function(delta) {",
		"  function(x) {",
		"    x + delta + base",
		"  }",
		"}",
		"",
	}, "\n"))
	mainPath := writeCLIFile(t, cwd, "main.jbs", strings.Join([]string{
		"use mk from \"./lib.jbs\"",
		"inc = mk(1)",
		"x = inc(1)",
		"do run with x {",
		"  echo ${x}",
		"}",
		"",
	}, "\n"))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--check", mainPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected successful check for imported higher-order initializer, code=%d stderr=%s", code, stderr.String())
	}
}
