package sema

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestAnalyzeRangeShortcutSource(t *testing.T) {
	src := `
xs = 1:5
ys = 5:1
zs = 10:-2:-2

do s with xs, ys, zs {
	echo $xs $ys $zs
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("range_shortcut.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.DoBlocks) != 1 || res.DoBlocks[0].Name != "s" {
		t.Fatalf("expected do block s, got %#v", res.DoBlocks)
	}
	want := map[string]eval.Value{
		"xs": eval.List([]eval.Value{eval.Int(1), eval.Int(2), eval.Int(3), eval.Int(4)}),
		"ys": eval.List([]eval.Value{eval.Int(5), eval.Int(4), eval.Int(3), eval.Int(2)}),
		"zs": eval.List([]eval.Value{eval.Int(10), eval.Int(8), eval.Int(6), eval.Int(4), eval.Int(2), eval.Int(0)}),
	}
	for name, wantValue := range want {
		if !eval.Equal(res.Globals.Values[name], wantValue) {
			t.Fatalf("%s got %#v want %#v", name, res.Globals.Values[name], wantValue)
		}
	}
}

func TestAnalyzeRangeShortcutInFunctionLoop(t *testing.T) {
	src := `
f = function() {
	total = 0
	for x in 1:4 {
		total += x
	}
	return total
}
sum = f()
sum
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("range_function.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !eval.Equal(res.Globals.Values["sum"], eval.Int(6)) {
		t.Fatalf("sum got %#v want 6", res.Globals.Values["sum"])
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(6)) {
		t.Fatalf("unexpected top-level expression results: %#v", res.TopLevelExprs)
	}
}
