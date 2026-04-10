package eval

import (
	"math"
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

func TestEvalIntOverflowAddWarns(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
		Op:    "+",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(1, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != math.MinInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntOverflowSubWarns(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MinInt64},
		Op:    "-",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(2, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != math.MaxInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntOverflowMulWarns(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
		Op:    "*",
		Right: ast.NumberExpr{Int: true, IntValue: 2},
		Span:  spanAt(3, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != -2 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntOverflowUnaryWarns(t *testing.T) {
	expr := ast.UnaryExpr{
		Op:   "-",
		Expr: ast.NumberExpr{Int: true, IntValue: math.MinInt64},
		Span: spanAt(4, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != math.MinInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntNoOverflowBoundariesNoWarning(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want int64
	}{
		{
			name: "max-plus-zero",
			expr: ast.BinaryExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 0},
				Span:  spanAt(5, 1),
			},
			want: math.MaxInt64,
		},
		{
			name: "min-plus-one",
			expr: ast.BinaryExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: math.MinInt64},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 1},
				Span:  spanAt(6, 1),
			},
			want: math.MinInt64 + 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExpr(tc.expr, map[string]Value{}, diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got.Kind != KindInt || got.I != tc.want {
				t.Fatalf("unexpected evaluated value: %#v", got)
			}
			if got := diagCount(diags, "W102"); got != 0 {
				t.Fatalf("did not expect W102 warning, got %d: %s", got, diags.String())
			}
		})
	}
}

func TestEvalIntOverflowVectorWarnDedup(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.ListExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
				ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
			},
		},
		Op:    "+",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(7, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 2 || got.L[0].I != math.MinInt64 || got.L[1].I != math.MinInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one deduplicated W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalCompareSupportedOps(t *testing.T) {
	tests := []struct {
		name string
		op   string
		l    Value
		r    Value
		want bool
	}{
		{name: "eq-int-float", op: "==", l: Int(2), r: Float(2.0), want: true},
		{name: "ne-string", op: "!=", l: String("a"), r: String("b"), want: true},
		{name: "lt-string", op: "<", l: String("alpha"), r: String("beta"), want: true},
		{name: "le-string", op: "<=", l: String("beta"), r: String("beta"), want: true},
		{name: "gt-float-int", op: ">", l: Float(2.5), r: Int(2), want: true},
		{name: "ge-int-float", op: ">=", l: Int(3), r: Float(3.0), want: true},
		{name: "lt-int-int-false", op: "<", l: Int(7), r: Int(5), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalCompare(tc.op, tc.l, tc.r, spanAt(20, 1), diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got.Kind != KindBool || got.B != tc.want {
				t.Fatalf("unexpected compare result: %#v (want bool=%v)", got, tc.want)
			}
		})
	}
}

func TestEvalCompareUnsupportedReportsE110(t *testing.T) {
	tests := []struct {
		name string
		op   string
		l    Value
		r    Value
	}{
		{name: "type-mismatch-relational", op: "<", l: String("x"), r: Int(1)},
		{name: "unknown-op-on-numeric", op: "===", l: Int(1), r: Int(1)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalCompare(tc.op, tc.l, tc.r, spanAt(21, 1), diags)
			if got.Kind != KindBool || got.B {
				t.Fatalf("unexpected compare fallback result: %#v", got)
			}
			if !diags.HasErrors() {
				t.Fatalf("expected error diagnostics, got none")
			}
			if count := diagCount(diags, "E110"); count != 1 {
				t.Fatalf("expected one E110, got %d: %s", count, diags.String())
			}
		})
	}
}

func diagCount(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, d := range diags.Items {
		if d.Code == code {
			count++
		}
	}
	return count
}

func spanAt(line, col int) diag.Span {
	pos := diag.NewPos(0, line, col)
	return diag.NewSpan("eval_test.jbs", pos, pos)
}
