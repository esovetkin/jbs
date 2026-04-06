package eval

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestEvalVectorArithmetic(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.IdentExpr{Name: "x"},
		Op:   "+",
		Right: ast.NumberExpr{
			Int:      true,
			IntValue: 10,
		},
	}
	env := map[string]Value{
		"x": List([]Value{Int(1), Int(2), Int(3)}),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, env, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 3 || got.L[0].I != 11 || got.L[2].I != 13 {
		t.Fatalf("unexpected vector eval result: %#v", got)
	}
}

func TestEvalConditionalRequiresBool(t *testing.T) {
	expr := ast.ConditionalExpr{
		Then: ast.NumberExpr{Int: true, IntValue: 1},
		Cond: ast.NumberExpr{Int: true, IntValue: 2},
		Else: ast.NumberExpr{Int: true, IntValue: 0},
	}
	diags := &diag.Diagnostics{}
	_ = EvalExpr(expr, map[string]Value{}, diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E102" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E102, got: %s", diags.String())
	}
}

func TestEvalLargeIntegerLiteralExact(t *testing.T) {
	expr := ast.NumberExpr{Int: true, IntValue: 9007199254740993}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != 9007199254740993 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
}
