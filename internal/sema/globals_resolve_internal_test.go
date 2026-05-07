package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestResolveTopLevelGlobalsJbsNameLiteralRule(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	defaults := map[string]eval.Value{
		"jbs_name":  eval.String("default_name"),
		"jbs_nproc": eval.Int(0),
	}
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
				Span: span,
			},
		},
	}

	diags := &diag.Diagnostics{}
	got := resolveTopLevelGlobals(prog, defaults, diags)
	if countDiagCode(diags, "E301") != 1 {
		t.Fatalf("expected 1 E301 for non-string jbs_name, got %d: %s", countDiagCode(diags, "E301"), diags.String())
	}
	if got.Values["jbs_name"].S != "default_name" {
		t.Fatalf("expected default jbs_name to remain unchanged, got %#v", got.Values["jbs_name"])
	}
}

func TestResolveTopLevelGlobalsKeepsSeedPriorityOverForwardOverride(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	defaults := map[string]eval.Value{
		"jbs_name":  eval.String("default_name"),
		"jbs_nproc": eval.Int(0),
	}
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "jbs_nproc",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args:   ast.PosCallArgs(ast.IdentExpr{Name: "jbs_name", Span: span}),
					Span:   span,
				},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.StringExpr{Value: "override", Span: span},
				Span: span,
			},
		},
	}
	diags := &diag.Diagnostics{}
	got := resolveTopLevelGlobals(prog, defaults, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Values["jbs_nproc"].I != int64(len("default_name")) {
		t.Fatalf("expected jbs_nproc to read the seed value for jbs_name, got %#v", got.Values["jbs_nproc"])
	}
	if got.Values["jbs_name"].S != "override" {
		t.Fatalf("expected later jbs_name assignment to apply to jbs_name itself, got %#v", got.Values["jbs_name"])
	}
}

func TestResolveTopLevelGlobalsAllowsDuplicateBuiltinDefinitionsAndCompoundAssign(t *testing.T) {
	span := func(off int) diag.Span {
		start := diag.NewPos(off, 1, off+1)
		end := diag.NewPos(off+1, 1, off+2)
		return diag.NewSpan("in.jbs", start, end)
	}
	defaults := map[string]eval.Value{
		"jbs_name":  eval.String("default_name"),
		"jbs_nproc": eval.Int(0),
	}
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.StringExpr{Value: "bench", Span: span(1)},
				Span: span(1),
			},
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.StringExpr{Value: "other", Span: span(2)},
				Span: span(2),
			},
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.StringExpr{Value: "head", Span: span(3)},
				Span: span(3),
			},
			ast.GlobalAssign{
				Name: "jbs_name",
				Op:   ast.AssignPlusEq,
				Expr: ast.StringExpr{Value: "_tail", Span: span(4)},
				Span: span(4),
			},
		},
	}

	diags := &diag.Diagnostics{}
	got := resolveTopLevelGlobals(prog, defaults, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Values["jbs_name"].S != "head_tail" {
		t.Fatalf("expected jbs_name += to append, got %#v", got.Values["jbs_name"])
	}
	if got.Spans["jbs_name"] != span(4) {
		t.Fatalf("expected jbs_name span to point to the compound assignment, got=%+v want=%+v", got.Spans["jbs_name"], span(4))
	}
}

func TestResolveTopLevelGlobalsRejectsTupleBuiltinValue(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	defaults := map[string]eval.Value{
		"jbs_name":  eval.String("default_name"),
		"jbs_nproc": eval.Int(0),
	}
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "jbs_nproc",
				Expr: ast.TupleExpr{
					Items: []ast.Expr{ast.StringExpr{Value: "bad", Span: span}},
					Span:  span,
				},
				Span: span,
			},
		},
	}

	diags := &diag.Diagnostics{}
	got := resolveTopLevelGlobals(prog, defaults, diags)
	if countDiagCode(diags, "E304") != 1 {
		t.Fatalf("expected one tuple/list scalar-global diagnostic, got %d: %s", countDiagCode(diags, "E304"), diags.String())
	}
	if got.Values["jbs_nproc"].I != 0 {
		t.Fatalf("expected default jbs_nproc to remain unchanged, got %#v", got.Values["jbs_nproc"])
	}
}

func TestIsScalarGlobalValue(t *testing.T) {
	tests := []struct {
		name string
		v    eval.Value
		want bool
	}{
		{name: "null", v: eval.Null(), want: true},
		{name: "string", v: eval.String("x"), want: true},
		{name: "int", v: eval.Int(1), want: true},
		{name: "float", v: eval.Float(1.5), want: true},
		{name: "bool", v: eval.Bool(true), want: true},
		{name: "list", v: eval.List([]eval.Value{eval.Int(1)}), want: false},
		{name: "tuple", v: eval.Tuple([]eval.Value{eval.Int(1)}), want: false},
	}
	for _, tt := range tests {
		if got := isScalarGlobalValue(tt.v); got != tt.want {
			t.Fatalf("isScalarGlobalValue(%s)=%v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHasNestedList(t *testing.T) {
	tests := []struct {
		name string
		v    eval.Value
		want bool
	}{
		{name: "scalar", v: eval.String("x"), want: false},
		{name: "flat list", v: eval.List([]eval.Value{eval.Int(1), eval.String("a")}), want: false},
		{name: "flat tuple", v: eval.Tuple([]eval.Value{eval.Int(1), eval.Bool(true)}), want: false},
		{
			name: "direct nested list",
			v: eval.List([]eval.Value{
				eval.Int(1),
				eval.List([]eval.Value{eval.Int(2)}),
			}),
			want: true,
		},
		{
			name: "direct nested tuple",
			v: eval.Tuple([]eval.Value{
				eval.String("x"),
				eval.Tuple([]eval.Value{eval.String("y")}),
			}),
			want: true,
		},
	}
	for _, tt := range tests {
		if got := hasNestedList(tt.v); got != tt.want {
			t.Fatalf("hasNestedList(%s)=%v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveTopLevelGlobalsRejectsForwardReference(t *testing.T) {
	span := diag.NewSpan("forward.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	prog := ast.Program{
		File: "forward.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "x",
				Expr: ast.IdentExpr{Name: "y", Span: span},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "y",
				Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
				Span: span,
			},
		},
	}

	diags := &diag.Diagnostics{}
	out, order := compileUserGlobals(prog, nil, diags)
	if countDiagCode(diags, "E100") == 0 {
		t.Fatalf("expected unknown-variable diagnostic for forward reference, got: %s", diags.String())
	}
	if _, ok := out["x"]; ok {
		t.Fatalf("did not expect invalid forward assignment to publish x, got %#v", out["x"])
	}
	if !reflect.DeepEqual(order, []string{"y"}) {
		t.Fatalf("expected only y to publish, got %#v", order)
	}
}
