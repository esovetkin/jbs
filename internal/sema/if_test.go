package sema

import (
	"reflect"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func analyzeIfSource(t *testing.T, src string) (*Result, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("if.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("parse failed: %s", diags.String())
	}
	res := Analyze(prog, nil, diags)
	return res, diags
}

func TestAnalyzeTopLevelIfBranches(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want eval.Value
	}{
		{
			name: "true branch",
			src: `
flag = true
if flag { x = 1 } else { x = 2 }
`,
			want: eval.Int(1),
		},
		{
			name: "false branch",
			src: `
flag = false
if flag { x = 1 } else { x = 2 }
`,
			want: eval.Int(2),
		},
		{
			name: "nested branch",
			src: `
a = true
b = false
if a { if b { x = 1 } else { x = 3 } } else { x = 2 }
`,
			want: eval.Int(3),
		},
		{
			name: "reassignment and compound assignment",
			src: `
flag = true
x = 1
if flag {
	x += 4
} else {
	x = 2
}
`,
			want: eval.Int(5),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, diags := analyzeIfSource(t, tc.src)
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !eval.Equal(res.Globals.Values["x"], tc.want) {
				t.Fatalf("x=%#v want %#v", res.Globals.Values["x"], tc.want)
			}
		})
	}
}

func TestAnalyzeTopLevelElifBranches(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want eval.Value
	}{
		{
			name: "elif branch",
			src: `
mode = "b"
if mode == "a" {
	x = 1
} elif mode == "b" {
	x = 2
} else {
	x = 3
}
`,
			want: eval.Int(2),
		},
		{
			name: "final else",
			src: `
mode = "z"
if mode == "a" {
	x = 1
} elif mode == "b" {
	x = 2
} else {
	x = 3
}
`,
			want: eval.Int(3),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, diags := analyzeIfSource(t, tc.src)
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !eval.Equal(res.Globals.Values["x"], tc.want) {
				t.Fatalf("x=%#v want %#v", res.Globals.Values["x"], tc.want)
			}
		})
	}
}

func TestAnalyzeTopLevelIfExpressionResults(t *testing.T) {
	res, diags := analyzeIfSource(t, `
if true { 1 } else { 2 }
if false { 3 } else { 4 }
if false { 5 }
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 2 {
		t.Fatalf("expected two selected expression results, got %#v", res.TopLevelExprs)
	}
	if !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(1)) || !eval.Equal(res.TopLevelExprs[1].Value, eval.Int(4)) {
		t.Fatalf("unexpected expression results: %#v", res.TopLevelExprs)
	}
}

func TestAnalyzeTopLevelElifRejectsNonBoolCondition(t *testing.T) {
	_, diags := analyzeIfSource(t, `
if false {
	x = 1
} elif 1 {
	x = 2
} else {
	x = 3
}
`)
	if !hasDiagCode(diags, "E102") {
		t.Fatalf("expected E102, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "elif condition requires boolean value") {
		t.Fatalf("expected elif condition diagnostic, got: %s", diags.String())
	}
}

func TestAnalyzeTopLevelIfRejectsNonBoolCondition(t *testing.T) {
	res, diags := analyzeIfSource(t, `
x = 1
if 1 { x = 2 }
`)
	if !hasDiagCode(diags, "E102") {
		t.Fatalf("expected E102, got: %s", diags.String())
	}
	if !eval.Equal(res.Globals.Values["x"], eval.Int(1)) {
		t.Fatalf("invalid condition should not execute branch, got x=%#v", res.Globals.Values["x"])
	}
}

func TestAnalyzeTopLevelIfGuardDependencies(t *testing.T) {
	res, diags := analyzeIfSource(t, `
flag = true
a = 1
b = 2
if flag { x = a } else { x = b }
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	got := res.GlobalVarByName["x"].DependsOn
	want := []string{"a", "flag"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dependencies: got=%#v want=%#v", got, want)
	}
}

func TestAnalyzeTopLevelElifGuardDependencies(t *testing.T) {
	res, diags := analyzeIfSource(t, `
a = false
b = true
v1 = 1
v2 = 2
if a {
	x = v1
} elif b {
	x = v2
} else {
	x = 3
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	got := res.GlobalVarByName["x"].DependsOn
	want := []string{"a", "b", "v2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dependencies: got=%#v want=%#v", got, want)
	}
}

func TestAnalyzeTopLevelIfUnselectedBranchNotExported(t *testing.T) {
	res, diags := analyzeIfSource(t, `
if false { x = 1 } else { y = 2 }
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if _, ok := res.Globals.Values["x"]; ok {
		t.Fatalf("did not expect unselected branch global x to be exported")
	}
	if !eval.Equal(res.Globals.Values["y"], eval.Int(2)) {
		t.Fatalf("expected selected branch global y=2, got %#v", res.Globals.Values["y"])
	}
}
