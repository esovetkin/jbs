package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestResolveTopLevelGlobalsMixedValidationAndState(t *testing.T) {
	span := func(off int) diag.Span {
		start := diag.NewPos(off, 1, off+1)
		end := diag.NewPos(off+1, 1, off+2)
		return diag.NewSpan("in.jbs", start, end)
	}
	defaults := map[string]eval.Value{
		"jbs_name":    eval.String("default_name"),
		"jbs_outpath": eval.String("default_out"),
		"jbs_comment": eval.String(""),
	}
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "unknown_global",
				Expr: ast.StringExpr{Value: "x", Span: span(1)},
				Span: span(1),
			},
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.ModeExpr{
					Mode: "python",
					Expr: ast.StringExpr{Value: "ignored", Span: span(2)},
					Span: span(2),
				},
				Span: span(2),
			},
			ast.GlobalAssign{
				Name: "jbs_name",
				Expr: ast.StringExpr{Value: "bench", Span: span(3)},
				Span: span(3),
			},
			ast.GlobalAssign{
				Name: "jbs_outpath",
				Expr: ast.NumberExpr{Int: true, IntValue: 42, Raw: "42", Span: span(4)},
				Span: span(4),
			},
			ast.GlobalAssign{
				Name: "jbs_outpath",
				Expr: ast.StringExpr{Value: "runs", Span: span(5)},
				Span: span(5),
			},
			ast.GlobalAssign{
				Name: "jbs_comment",
				Expr: ast.TupleExpr{
					Items: []ast.Expr{
						ast.StringExpr{Value: "bad", Span: span(6)},
					},
					Span: span(6),
				},
				Span: span(6),
			},
			ast.GlobalAssign{
				Name: "jbs_comment",
				Expr: ast.ModeExpr{
					Mode: "shell",
					Expr: ast.NumberExpr{Int: true, IntValue: 7, Raw: "7", Span: span(7)},
					Span: span(7),
				},
				Span: span(7),
			},
			ast.GlobalAssign{
				Name: "jbs_comment",
				Op:   ast.AssignPlusEq,
				Expr: ast.StringExpr{Value: "_tail", Span: span(8)},
				Span: span(8),
			},
		},
	}

	diags := &diag.Diagnostics{}
	got := resolveTopLevelGlobals(prog, defaults, diags)

	if countDiagCode(diags, "E303") != 1 {
		t.Fatalf("expected 1 E303, got %d: %s", countDiagCode(diags, "E303"), diags.String())
	}
	if countDiagCode(diags, "E302") != 1 {
		t.Fatalf("expected 1 E302, got %d: %s", countDiagCode(diags, "E302"), diags.String())
	}
	if countDiagCode(diags, "E304") != 1 {
		t.Fatalf("expected 1 E304, got %d: %s", countDiagCode(diags, "E304"), diags.String())
	}
	if countDiagCode(diags, "E215") != 1 {
		t.Fatalf("expected 1 E215 from shell(number), got %d: %s", countDiagCode(diags, "E215"), diags.String())
	}
	if countDiagCode(diags, "W300") != 0 {
		t.Fatalf("did not expect W300 for reassignment, got %d: %s", countDiagCode(diags, "W300"), diags.String())
	}

	if got.Values["jbs_name"].Kind != eval.KindString || got.Values["jbs_name"].S != "bench" {
		t.Fatalf("unexpected jbs_name value: %#v", got.Values["jbs_name"])
	}
	if got.Values["jbs_outpath"].Kind != eval.KindString || got.Values["jbs_outpath"].S != "runs" {
		t.Fatalf("unexpected jbs_outpath value: %#v", got.Values["jbs_outpath"])
	}
	if got.Values["jbs_comment"].Kind != eval.KindString || got.Values["jbs_comment"].S != "7_tail" {
		t.Fatalf("unexpected jbs_comment value after += rewrite: %#v", got.Values["jbs_comment"])
	}
	if _, exists := got.Modes["jbs_comment"]; exists {
		t.Fatalf("expected mode to be cleared after non-mode reassignment")
	}
	if got.Spans["jbs_comment"] != span(8) {
		t.Fatalf("expected jbs_comment span to point to last assignment, got=%+v want=%+v", got.Spans["jbs_comment"], span(8))
	}
}

func TestResolveTopLevelGlobalsJbsNameLiteralRule(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	defaults := map[string]eval.Value{
		"jbs_name":    eval.String("default_name"),
		"jbs_outpath": eval.String("default_out"),
		"jbs_comment": eval.String(""),
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
		"jbs_name":    eval.String("default_name"),
		"jbs_outpath": eval.String("default_out"),
		"jbs_comment": eval.String(""),
	}
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "jbs_comment",
				Expr: ast.IdentExpr{Name: "jbs_name", Span: span},
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
	if got.Values["jbs_comment"].S != "default_name" {
		t.Fatalf("expected jbs_comment to read the seed value for jbs_name, got %#v", got.Values["jbs_comment"])
	}
	if got.Values["jbs_name"].S != "override" {
		t.Fatalf("expected later jbs_name assignment to apply to jbs_name itself, got %#v", got.Values["jbs_name"])
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

func TestUniqueForwardSimpleWriteIgnoresProjectedImports(t *testing.T) {
	plan := &globalPlan{
		Steps: []globalInputStep{
			{ID: 0, Kind: globalInputExpr, EffectiveExpr: ast.IdentExpr{Name: "x"}},
			{ID: 1, Kind: globalInputProjectedImport, Name: "x", IsSimple: true},
			{ID: 2, Kind: globalInputAssign, Name: "y", IsSimple: true},
			{ID: 3, Kind: globalInputAssign, Name: "z", IsSimple: true},
		},
		SimpleWritesByName: map[string][]int{
			"x": {1},
			"y": {2},
			"z": {3},
		},
	}
	activeSet := map[int]struct{}{0: {}, 1: {}, 2: {}, 3: {}}

	if depID, ok := uniqueForwardSimpleWrite("x", 0, plan, activeSet); ok {
		t.Fatalf("did not expect projected import to qualify for forward binding, got depID=%d", depID)
	}
	if depID, ok := uniqueForwardSimpleWrite("y", 0, plan, activeSet); !ok || depID != 2 {
		t.Fatalf("expected local simple assignment to qualify for forward binding, got depID=%d ok=%v", depID, ok)
	}
}
