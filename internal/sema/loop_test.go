package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func analyzeLoopSource(t *testing.T, src string) (*Result, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("loop.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("parse failed: %s", diags.String())
	}
	res := Analyze(prog, nil, diags)
	return res, diags
}

func TestAnalyzeTopLevelForLoop(t *testing.T) {
	res, diags := analyzeLoopSource(t, `
sum = 0
for x in range(5) {
	sum += x
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !eval.Equal(res.Globals.Values["sum"], eval.Int(10)) {
		t.Fatalf("sum=%#v", res.Globals.Values["sum"])
	}
	if !eval.Equal(res.Globals.Values["x"], eval.Int(4)) {
		t.Fatalf("x=%#v", res.Globals.Values["x"])
	}
}

func TestAnalyzeTopLevelWhileLoop(t *testing.T) {
	res, diags := analyzeLoopSource(t, `
x = 0
while x < 3 {
	x += 1
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !eval.Equal(res.Globals.Values["x"], eval.Int(3)) {
		t.Fatalf("x=%#v", res.Globals.Values["x"])
	}
}

func TestAnalyzeTopLevelLoopBreakContinue(t *testing.T) {
	res, diags := analyzeLoopSource(t, `
sum = 0
for x in range(10) {
	if x == 2 {
		continue
	}
	if x == 5 {
		break
	}
	sum += x
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !eval.Equal(res.Globals.Values["sum"], eval.Int(8)) {
		t.Fatalf("sum=%#v", res.Globals.Values["sum"])
	}
}

func TestAnalyzeLoopSnapshot(t *testing.T) {
	res, diags := analyzeLoopSource(t, `
sum = 0
for x in range(3) {
	sum += x
}
do run with sum {
	echo $sum
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	snap := res.ScopeSnapshotsByBlock[doBlockSnapshotKey(res.DoBlocks[0])]
	if snap == nil {
		t.Fatalf("expected snapshot for run")
	}
	if !eval.Equal(snap.Globals.Values["sum"], eval.Int(3)) {
		t.Fatalf("snapshot sum=%#v", snap.Globals.Values["sum"])
	}
}

func TestAnalyzeLoopErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "for scalar", src: "for x in 1 { y = x }\n", code: "E106"},
		{name: "while non bool", src: "while 1 { x = 1 }\n", code: "E102"},
		{name: "while infinite", src: "while true {}\n", code: "E106"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := analyzeLoopSource(t, tc.src)
			if !hasDiagCode(diags, tc.code) {
				t.Fatalf("expected %s, got: %s", tc.code, diags.String())
			}
		})
	}
}

func TestAnalyzeLoopDependencies(t *testing.T) {
	res, diags := analyzeLoopSource(t, `
items = (0, 1, 2)
sum = 0
for x in items {
	sum += x
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	got := res.GlobalVarByName["sum"].DependsOn
	want := []string{"items", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dependencies: got=%#v want=%#v", got, want)
	}
}
