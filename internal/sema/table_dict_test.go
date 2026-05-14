package sema

import (
	"reflect"
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestAnalyzeSupportsTableFromDictConversion(t *testing.T) {
	src := `
x = range(5)
y = range(10)
cases = table(dict(x = x, y = y))
names(cases)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	binding := res.BindingsByName["cases"]
	if binding == nil || binding.Shape != BindingTable {
		t.Fatalf("expected table binding, got %#v", binding)
	}
	if len(binding.Rows) != 10 {
		t.Fatalf("expected 10 rows, got %#v", binding.Rows)
	}
	if !reflect.DeepEqual(binding.Order, []string{"x", "y"}) {
		t.Fatalf("unexpected column order: %#v", binding.Order)
	}
	wantNames := eval.List([]eval.Value{eval.String("x"), eval.String("y")})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantNames) {
		t.Fatalf("unexpected names(cases) result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeSupportsDictFromTableConversion(t *testing.T) {
	src := `
cases = table(x = [1, 2], y = ["a", "b"])
columns = dict(cases)
columns["x"]
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	wantDict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "x"}, Value: eval.List([]eval.Value{eval.Int(1), eval.Int(2)})},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "y"}, Value: eval.List([]eval.Value{eval.String("a"), eval.String("b")})},
	})
	if gv := res.GlobalVarByName["columns"]; gv == nil || !eval.Equal(gv.Value, wantDict) {
		t.Fatalf("unexpected columns dictionary: %#v", gv)
	}
	wantX := eval.List([]eval.Value{eval.Int(1), eval.Int(2)})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantX) {
		t.Fatalf("unexpected columns[\"x\"] result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeNamesSupportsDictionaryKeys(t *testing.T) {
	src := `
settings = {"x": 1, 2: "two", true: "enabled"}
names(settings)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := eval.List([]eval.Value{eval.String("x"), eval.Int(2), eval.Bool(true)})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, want) {
		t.Fatalf("unexpected names(settings) result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeDoWithDictMarksSourceUsed(t *testing.T) {
	src := `
settings = dict(host = ("h0", "h1"), rank = (0, 1))

do run with settings {
        echo "${host} ${rank}"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got := countWarningsWithParts(diags, diag.CodeW310, "global 'settings'"); got != 0 {
		t.Fatalf("did not expect W310 for dict source used through columns, got %d: %s", got, diags.String())
	}
}

func TestAnalyzeSupportsRowsFromTableConversion(t *testing.T) {
	src := `
cases = table(x = [1, 2], y = ["a", "b"])
r = rows(cases)
r
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	wantRows := eval.List([]eval.Value{
		eval.DictValue([]eval.DictEntry{
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "x"}, Value: eval.Int(1)},
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "y"}, Value: eval.String("a")},
		}),
		eval.DictValue([]eval.DictEntry{
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "x"}, Value: eval.Int(2)},
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "y"}, Value: eval.String("b")},
		}),
	})
	if gv := res.GlobalVarByName["r"]; gv == nil || !eval.Equal(gv.Value, wantRows) {
		t.Fatalf("unexpected rows list: %#v", gv)
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantRows) {
		t.Fatalf("unexpected top-level rows result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeSupportsTableFromRowDictListConversion(t *testing.T) {
	src := `
cases = table([dict(x = 1, y = "a"), dict(y = "b", x = 2)])
names(cases)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	binding := res.BindingsByName["cases"]
	if binding == nil || binding.Shape != BindingTable {
		t.Fatalf("expected table binding, got %#v", binding)
	}
	if len(binding.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %#v", binding.Rows)
	}
	if !reflect.DeepEqual(binding.Order, []string{"x", "y"}) {
		t.Fatalf("unexpected column order: %#v", binding.Order)
	}
	wantNames := eval.List([]eval.Value{eval.String("x"), eval.String("y")})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantNames) {
		t.Fatalf("unexpected names(cases) result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeSupportsTableRowsRoundTrip(t *testing.T) {
	src := `
cases = table(x = [1, 2], y = ["a", "b"])
roundtrip = table(rows(cases))
names(roundtrip)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	binding := res.BindingsByName["roundtrip"]
	if binding == nil || binding.Shape != BindingTable {
		t.Fatalf("expected table binding, got %#v", binding)
	}
	if len(binding.Rows) != 2 || !reflect.DeepEqual(binding.Order, []string{"x", "y"}) {
		t.Fatalf("unexpected roundtrip binding: %#v", binding)
	}
	if !eval.Equal(binding.Rows[0].Values["x"].Value, eval.Int(1)) || !eval.Equal(binding.Rows[1].Values["y"].Value, eval.String("b")) {
		t.Fatalf("unexpected roundtrip rows: %#v", binding.Rows)
	}
}

func TestAnalyzeSupportsZeroRowTableRowsRoundTrip(t *testing.T) {
	src := `
cases = table(x = [], y = [])
roundtrip = table(rows(cases))
names(roundtrip)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	binding := res.BindingsByName["roundtrip"]
	if binding == nil || binding.Shape != BindingTable {
		t.Fatalf("expected table binding, got %#v", binding)
	}
	if len(binding.Rows) != 0 || !reflect.DeepEqual(binding.Order, []string{"x", "y"}) {
		t.Fatalf("unexpected zero-row roundtrip binding: %#v", binding)
	}
	wantNames := eval.List([]eval.Value{eval.String("x"), eval.String("y")})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantNames) {
		t.Fatalf("unexpected names(roundtrip) result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeRenameBuiltin(t *testing.T) {
	src := `
cases = rename(table(x = [1, 2], y = ["a", "b"]), x = "id")
names(cases)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	binding := res.BindingsByName["cases"]
	if binding == nil || binding.Shape != BindingTable {
		t.Fatalf("expected table binding, got %#v", binding)
	}
	if len(binding.Rows) != 2 || !reflect.DeepEqual(binding.Order, []string{"id", "y"}) {
		t.Fatalf("unexpected renamed binding: %#v", binding)
	}
	if !eval.Equal(binding.Rows[0].Values["id"].Value, eval.Int(1)) || !eval.Equal(binding.Rows[1].Values["y"].Value, eval.String("b")) {
		t.Fatalf("unexpected renamed rows: %#v", binding.Rows)
	}
	wantNames := eval.List([]eval.Value{eval.String("id"), eval.String("y")})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantNames) {
		t.Fatalf("unexpected names(cases) result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeRenameMissingColumn(t *testing.T) {
	src := `cases = rename(table(x = [1]), missing = "id")`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if countDiagCode(diags, "E106") == 0 {
		t.Fatalf("expected missing-column diagnostic, got: %s", diags.String())
	}
}

func TestAnalyzeRenameBuiltinDependency(t *testing.T) {
	src := `
cases = rename(table(x = [1]), x = "id")
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	cases := res.GlobalVarByName["cases"]
	if cases == nil {
		t.Fatalf("missing cases global")
	}
	if slices.Contains(cases.DependsOn, "rename") {
		t.Fatalf("unshadowed rename should not be recorded as dependency: %#v", cases.DependsOn)
	}
	for _, key := range cases.DependsOnKeys {
		if key.Public == "rename" {
			t.Fatalf("unshadowed rename dependency key recorded: %#v", cases.DependsOnKeys)
		}
	}
}

func TestAnalyzeShadowedRenameDependency(t *testing.T) {
	src := `
rename = function(value, **mapping) { value }
cases = rename(table(x = [1]), x = "id")
names(cases)
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	cases := res.GlobalVarByName["cases"]
	if cases == nil || !slices.Contains(cases.DependsOn, "rename") {
		t.Fatalf("expected dependency on shadowed rename, got %#v", cases)
	}
	binding := res.BindingsByName["cases"]
	if binding == nil || !reflect.DeepEqual(binding.Order, []string{"x"}) {
		t.Fatalf("expected shadowed function to return original table, got %#v", binding)
	}
	wantNames := eval.List([]eval.Value{eval.String("x")})
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, wantNames) {
		t.Fatalf("unexpected names(cases) result: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeRejectsInvalidTableFromRowDictList(t *testing.T) {
	src := `cases = table([dict(x = [1])])`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if countDiagCode(diags, "E106") == 0 {
		t.Fatalf("expected invalid row-list diagnostic, got: %s", diags.String())
	}
}

func TestAnalyzeDoWithListOfDictsKeepsListImportSemantics(t *testing.T) {
	src := `
rows = [dict(x = 1), dict(x = 2)]

do run with rows {
        echo "${rows}"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := countDiagCode(diags, "W314"); got == 0 {
		t.Fatalf("expected W314 list-import warning, got %d: %s", got, diags.String())
	}
}

func TestAnalyzeTableBroadcastWarning(t *testing.T) {
	src := `cases = table(x = range(3), y = range(10))`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := countDiagCode(diags, "W101"); got != 1 {
		t.Fatalf("expected one W101, got %d: %s", got, diags.String())
	}
	binding := res.BindingsByName["cases"]
	if binding == nil || len(binding.Rows) != 10 {
		t.Fatalf("expected 10 broadcast rows, got %#v", binding)
	}
}
