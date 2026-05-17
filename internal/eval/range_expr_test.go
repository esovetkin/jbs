package eval

import (
	"math"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func floatExprForRange(v float64) ast.NumberExpr {
	return ast.NumberExpr{Int: false, FloatValue: v}
}

func TestEvalRangeDescendingAndShortcut(t *testing.T) {
	opts := ExprOptions{Context: EvalCtxBindingAssign}
	span := spanAt(4000, 1)
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "range descending default step",
			expr: callExpr(ident("range"), posArg(intExpr(10)), posArg(intExpr(1))),
			want: List([]Value{Int(10), Int(9), Int(8), Int(7), Int(6), Int(5), Int(4), Int(3), Int(2)}),
		},
		{
			name: "range descending explicit step",
			expr: callExpr(ident("range"), posArg(intExpr(10)), posArg(intExpr(1)), posArg(intExpr(-2))),
			want: List([]Value{Int(10), Int(8), Int(6), Int(4), Int(2)}),
		},
		{
			name: "shortcut ascending",
			expr: ast.RangeExpr{Start: intExpr(1), Stop: intExpr(5), Span: span},
			want: List([]Value{Int(1), Int(2), Int(3), Int(4)}),
		},
		{
			name: "shortcut descending",
			expr: ast.RangeExpr{Start: intExpr(5), Stop: intExpr(1), Span: span},
			want: List([]Value{Int(5), Int(4), Int(3), Int(2)}),
		},
		{
			name: "shortcut descending explicit step",
			expr: ast.RangeExpr{Start: intExpr(10), Stop: intExpr(-2), Step: intExpr(-2), Span: span},
			want: List([]Value{Int(10), Int(8), Int(6), Int(4), Int(2), Int(0)}),
		},
		{
			name: "explicit step away from stop is empty",
			expr: callExpr(ident("range"), posArg(intExpr(1)), posArg(intExpr(10)), posArg(intExpr(-1))),
			want: List(nil),
		},
		{
			name: "positive explicit step away from descending stop is empty",
			expr: callExpr(ident("range"), posArg(intExpr(10)), posArg(intExpr(1)), posArg(intExpr(1))),
			want: List(nil),
		},
		{
			name: "named descending default step",
			expr: callExpr(ident("range"),
				namedArg("start", intExpr(10)),
				namedArg("stop", intExpr(1)),
			),
			want: List([]Value{Int(10), Int(9), Int(8), Int(7), Int(6), Int(5), Int(4), Int(3), Int(2)}),
		},
		{
			name: "named descending explicit step",
			expr: callExpr(ident("range"),
				namedArg("start", intExpr(10)),
				namedArg("stop", intExpr(1)),
				namedArg("step", intExpr(-2)),
			),
			want: List([]Value{Int(10), Int(8), Int(6), Int(4), Int(2)}),
		},
		{
			name: "named stop only remains one argument",
			expr: callExpr(ident("range"), namedArg("stop", intExpr(3))),
			want: List([]Value{Int(0), Int(1), Int(2)}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, opts)
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestEvalRangeShortcutFloatValues(t *testing.T) {
	opts := ExprOptions{Context: EvalCtxBindingAssign}
	tests := []struct {
		name string
		expr ast.Expr
		want []float64
	}{
		{
			name: "ascending",
			expr: ast.RangeExpr{
				Start: floatExprForRange(0.1),
				Stop:  intExpr(1),
				Step:  floatExprForRange(0.2),
				Span:  spanAt(4010, 1),
			},
			want: []float64{0.1, 0.3, 0.5, 0.7, 0.9},
		},
		{
			name: "descending",
			expr: ast.RangeExpr{
				Start: intExpr(1),
				Stop:  floatExprForRange(0.1),
				Step:  floatExprForRange(-0.2),
				Span:  spanAt(4011, 1),
			},
			want: []float64{1.0, 0.8, 0.6, 0.4, 0.2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, opts)
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if got.Kind != KindList || len(got.L) != len(tc.want) {
				t.Fatalf("unexpected result: %#v", got)
			}
			for i, want := range tc.want {
				if got.L[i].Kind != KindFloat || math.Abs(got.L[i].F-want) > 1e-12 {
					t.Fatalf("value %d got %#v want %g", i, got.L[i], want)
				}
			}
		})
	}
}

func TestEvalRangeErrors(t *testing.T) {
	tests := []struct {
		name string
		args []Value
	}{
		{name: "zero int step", args: []Value{Int(1), Int(10), Int(0)}},
		{name: "zero float step", args: []Value{Float(0), Float(1), Float(0)}},
		{name: "non numeric explicit arg", args: []Value{Int(0), Int(1), String("x")}},
		{name: "non progress positive float", args: []Value{Float(10000000000000000), Float(10000000000000010), Float(1)}},
		{name: "non progress negative float", args: []Value{Float(-10000000000000000), Float(-10000000000000010), Float(-1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalRangeCall(tc.args, spanAt(4020, 1), diags)
			if got.Kind != KindNull {
				t.Fatalf("expected null, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 {
				t.Fatalf("expected E106 diagnostic, got: %s", diags.String())
			}
		})
	}
}
