package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestSortOrderDefaultComparator(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "sort int list",
			expr: callExpr(ident("sort"), posArg(listExpr(intExpr(3), intExpr(1), intExpr(2)))),
			want: List([]Value{Int(1), Int(2), Int(3)}),
		},
		{
			name: "sort string list",
			expr: callExpr(ident("sort"), posArg(listExpr(stringExpr("b"), stringExpr("a"), stringExpr("c")))),
			want: List([]Value{String("a"), String("b"), String("c")}),
		},
		{
			name: "sort tuple preserves tuple",
			expr: callExpr(ident("sort"), posArg(tupleExpr(intExpr(3), intExpr(1), intExpr(2)))),
			want: Tuple([]Value{Int(1), Int(2), Int(3)}),
		},
		{
			name: "order string list",
			expr: callExpr(ident("order"), posArg(listExpr(stringExpr("b"), stringExpr("a"), stringExpr("c")))),
			want: List([]Value{Int(1), Int(0), Int(2)}),
		},
		{
			name: "sort empty list",
			expr: callExpr(ident("sort"), posArg(listExpr())),
			want: List(nil),
		},
		{
			name: "order singleton tuple",
			expr: callExpr(ident("order"), posArg(tupleExpr(stringExpr("x")))),
			want: List([]Value{Int(0)}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestSortOrderStableDuplicates(t *testing.T) {
	diags := &diag.Diagnostics{}
	orderGot := EvalExprWithOptions(callExpr(ident("order"),
		posArg(listExpr(intExpr(2), intExpr(1), intExpr(2), intExpr(1))),
	), nil, diags, ExprOptions{})
	if !Equal(orderGot, List([]Value{Int(1), Int(3), Int(0), Int(2)})) {
		t.Fatalf("unexpected stable order result: %#v", orderGot)
	}
	sortGot := EvalExprWithOptions(callExpr(ident("sort"),
		posArg(listExpr(intExpr(2), intExpr(1), intExpr(2), intExpr(1))),
	), nil, diags, ExprOptions{})
	if !Equal(sortGot, List([]Value{Int(1), Int(1), Int(2), Int(2)})) {
		t.Fatalf("unexpected stable sort result: %#v", sortGot)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestSortOrderCustomComparator(t *testing.T) {
	t.Run("descending numbers", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		desc := fnExpr([]ast.FuncParam{{Name: "a"}, {Name: "b"}}, exprStmt(ast.CompareExpr{
			Left:  ident("b"),
			Op:    "<",
			Right: ident("a"),
		}))
		got := EvalExprWithOptions(callExpr(ident("sort"),
			posArg(listExpr(intExpr(1), intExpr(3), intExpr(2))),
			posArg(desc),
		), nil, diags, ExprOptions{})
		if !Equal(got, List([]Value{Int(3), Int(2), Int(1)})) {
			t.Fatalf("unexpected descending sort result: %#v", got)
		}
		orderGot := EvalExprWithOptions(callExpr(ident("order"),
			posArg(listExpr(intExpr(1), intExpr(3), intExpr(2))),
			namedArg("by", desc),
		), nil, diags, ExprOptions{})
		if !Equal(orderGot, List([]Value{Int(1), Int(2), Int(0)})) {
			t.Fatalf("unexpected descending order result: %#v", orderGot)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("string length", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		byLen := fnExpr([]ast.FuncParam{{Name: "a"}, {Name: "b"}}, exprStmt(ast.CompareExpr{
			Left:  callExpr(ident("len"), posArg(ident("a"))),
			Op:    "<",
			Right: callExpr(ident("len"), posArg(ident("b"))),
		}))
		got := EvalExprWithOptions(callExpr(ident("sort"),
			posArg(listExpr(stringExpr("bbbb"), stringExpr("a"), stringExpr("cc"))),
			posArg(byLen),
		), nil, diags, ExprOptions{})
		if !Equal(got, List([]Value{String("a"), String("cc"), String("bbbb")})) {
			t.Fatalf("unexpected length sort result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestSortOrderNamedArgumentsAndNoneComparator(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "order named values",
			expr: callExpr(ident("order"), namedArg("values", listExpr(intExpr(3), intExpr(1), intExpr(2)))),
			want: List([]Value{Int(1), Int(2), Int(0)}),
		},
		{
			name: "sort named inplace false",
			expr: callExpr(ident("sort"),
				namedArg("values", listExpr(intExpr(3), intExpr(1), intExpr(2))),
				namedArg("inplace", boolExpr(false)),
			),
			want: List([]Value{Int(1), Int(2), Int(3)}),
		},
		{
			name: "sort none comparator",
			expr: callExpr(ident("sort"),
				posArg(listExpr(intExpr(2), intExpr(1))),
				namedArg("by", ast.IdentExpr{Name: "None"}),
			),
			want: List([]Value{Int(1), Int(2)}),
		},
		{
			name: "order none comparator",
			expr: callExpr(ident("order"),
				posArg(listExpr(intExpr(2), intExpr(1))),
				posArg(ast.IdentExpr{Name: "None"}),
			),
			want: List([]Value{Int(1), Int(0)}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestSortInPlace(t *testing.T) {
	t.Run("mutates list and returns null", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{
			"x": List([]Value{Int(3), Int(1), Int(2)}),
		})
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("sort"),
			posArg(ident("x")),
			namedArg("inplace", boolExpr(true)),
		), nil, diags, ExprOptions{Frame: frame})
		if got.Kind != KindNull {
			t.Fatalf("expected null result from in-place sort, got %#v", got)
		}
		value, ok := frame.Read("x", diag.Span{}, diags)
		if !ok || !Equal(value, List([]Value{Int(1), Int(2), Int(3)})) {
			t.Fatalf("unexpected in-place list value: %#v", value)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("failed in-place leaves list unchanged", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{
			"x": List([]Value{Int(1), String("a")}),
		})
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("sort"),
			posArg(ident("x")),
			namedArg("inplace", boolExpr(true)),
		), nil, diags, ExprOptions{Frame: frame})
		if got.Kind != KindNull {
			t.Fatalf("expected null result from failed in-place sort, got %#v", got)
		}
		value, ok := frame.Read("x", diag.Span{}, &diag.Diagnostics{})
		if !ok || !Equal(value, List([]Value{Int(1), String("a")})) {
			t.Fatalf("failed in-place sort mutated input: %#v", value)
		}
		if diagCount(diags, "E110") == 0 {
			t.Fatalf("expected comparison error, got: %s", diags.String())
		}
	})
}

func TestSortOrderBuiltinFunctionValuesAndHigherOrderUse(t *testing.T) {
	for _, name := range []string{"sort", "order"} {
		value, ok := BuiltinFunctionValue(name)
		if !ok || value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != name {
			t.Fatalf("expected builtin function value for %q, got ok=%v value=%#v", name, ok, value)
		}
	}

	frame := NewRootFrame(nil)
	assignBuiltinFunction(t, frame, "s", "sort")
	assignBuiltinFunction(t, frame, "o", "order")
	diags := &diag.Diagnostics{}
	sorted := EvalExprWithOptions(callExpr(ident("s"), posArg(listExpr(intExpr(2), intExpr(1)))), nil, diags, ExprOptions{Frame: frame})
	ordered := EvalExprWithOptions(callExpr(ident("o"), posArg(listExpr(intExpr(2), intExpr(1)))), nil, diags, ExprOptions{Frame: frame})
	if !Equal(sorted, List([]Value{Int(1), Int(2)})) || !Equal(ordered, List([]Value{Int(1), Int(0)})) {
		t.Fatalf("unexpected first-class sort/order results: sorted=%#v ordered=%#v", sorted, ordered)
	}

	mappedSort := EvalExprWithOptions(callExpr(ident("map"),
		posArg(ident("sort")),
		posArg(listExpr(
			listExpr(intExpr(2), intExpr(1)),
			listExpr(intExpr(4), intExpr(3)),
		)),
	), nil, diags, ExprOptions{})
	mappedOrder := EvalExprWithOptions(callExpr(ident("map"),
		posArg(ident("order")),
		posArg(listExpr(
			listExpr(intExpr(2), intExpr(1)),
			listExpr(intExpr(4), intExpr(3)),
		)),
	), nil, diags, ExprOptions{})
	if !Equal(mappedSort, List([]Value{
		List([]Value{Int(1), Int(2)}),
		List([]Value{Int(3), Int(4)}),
	})) {
		t.Fatalf("unexpected map(sort, ...) result: %#v", mappedSort)
	}
	if !Equal(mappedOrder, List([]Value{
		List([]Value{Int(1), Int(0)}),
		List([]Value{Int(1), Int(0)}),
	})) {
		t.Fatalf("unexpected map(order, ...) result: %#v", mappedOrder)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestSortOrderShadowing(t *testing.T) {
	t.Run("user function shadows sort and order", func(t *testing.T) {
		frame := NewRootFrame(nil)
		defineFunctionInFrame(t, frame, "sort", fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(intExpr(42))))
		defineFunctionInFrame(t, frame, "order", fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(intExpr(99))))
		diags := &diag.Diagnostics{}
		sorted := EvalExprWithOptions(callExpr(ident("sort"), posArg(listExpr(intExpr(2), intExpr(1)))), nil, diags, ExprOptions{Frame: frame})
		ordered := EvalExprWithOptions(callExpr(ident("order"), posArg(listExpr(intExpr(2), intExpr(1)))), nil, diags, ExprOptions{Frame: frame})
		if !Equal(sorted, Int(42)) || !Equal(ordered, Int(99)) {
			t.Fatalf("expected shadowing functions to win, got sort=%#v order=%#v", sorted, ordered)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("data global shadows builtin identifier", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(ident("sort"), map[string]Value{"sort": Int(12)}, diags, ExprOptions{})
		if !Equal(got, Int(12)) {
			t.Fatalf("expected data global named sort to shadow builtin, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestSortOrderErrors(t *testing.T) {
	badComparator := fnExpr([]ast.FuncParam{{Name: "a"}, {Name: "b"}}, exprStmt(intExpr(1)))
	oneArgComparator := fnExpr([]ast.FuncParam{{Name: "a"}}, exprStmt(boolExpr(true)))
	tableExpr := callExpr(ident("table"), namedArg("x", listExpr(intExpr(1))))
	tests := []struct {
		name     string
		expr     ast.Expr
		wantCode string
		wantText string
	}{
		{
			name:     "sort missing values",
			expr:     callExpr(ident("sort")),
			wantCode: "E106",
			wantText: "sort() expects arguments",
		},
		{
			name:     "order too many positional",
			expr:     callExpr(ident("order"), posArg(listExpr(intExpr(1))), posArg(ast.IdentExpr{Name: "None"}), posArg(intExpr(1))),
			wantCode: "E106",
			wantText: "order() received too many positional arguments",
		},
		{
			name:     "sort unknown named",
			expr:     callExpr(ident("sort"), posArg(listExpr(intExpr(1))), namedArg("reverse", boolExpr(true))),
			wantCode: "E106",
			wantText: "unknown named argument 'reverse' for sort()",
		},
		{
			name:     "sort scalar input",
			expr:     callExpr(ident("sort"), posArg(intExpr(1))),
			wantCode: "E106",
			wantText: "sort() expects list or tuple as first argument",
		},
		{
			name:     "order dict input",
			expr:     callExpr(ident("order"), posArg(ast.DictExpr{})),
			wantCode: "E106",
			wantText: "order() expects list or tuple as first argument",
		},
		{
			name:     "sort table input",
			expr:     callExpr(ident("sort"), posArg(tableExpr)),
			wantCode: "E106",
			wantText: "sort() expects list or tuple as first argument",
		},
		{
			name:     "sort invalid by",
			expr:     callExpr(ident("sort"), posArg(listExpr(intExpr(1), intExpr(2))), namedArg("by", intExpr(1))),
			wantCode: "E106",
			wantText: "sort() by argument must be a function",
		},
		{
			name:     "sort invalid inplace",
			expr:     callExpr(ident("sort"), posArg(listExpr(intExpr(1), intExpr(2))), namedArg("inplace", stringExpr("yes"))),
			wantCode: "E106",
			wantText: "sort() inplace argument must be a boolean",
		},
		{
			name:     "sort tuple inplace",
			expr:     callExpr(ident("sort"), posArg(tupleExpr(intExpr(2), intExpr(1))), namedArg("inplace", boolExpr(true))),
			wantCode: "E106",
			wantText: "sort() inplace argument requires a list input",
		},
		{
			name:     "sort comparator returns non bool",
			expr:     callExpr(ident("sort"), posArg(listExpr(intExpr(2), intExpr(1))), posArg(badComparator)),
			wantCode: "E106",
			wantText: "sort() comparator must return a boolean value",
		},
		{
			name:     "sort default mixed incomparable",
			expr:     callExpr(ident("sort"), posArg(listExpr(intExpr(1), stringExpr("x")))),
			wantCode: "E110",
			wantText: "unsupported comparison '<' for operand types",
		},
		{
			name:     "sort nested list default non scalar compare",
			expr:     callExpr(ident("sort"), posArg(listExpr(listExpr(intExpr(2)), listExpr(intExpr(1))))),
			wantCode: "E106",
			wantText: "sort() default comparison did not produce a boolean",
		},
		{
			name:     "sort comparator arity error",
			expr:     callExpr(ident("sort"), posArg(listExpr(intExpr(2), intExpr(1))), posArg(oneArgComparator)),
			wantCode: "E106",
			wantText: "too many positional arguments",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 || !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected %s containing %q, got: %s", tc.wantCode, tc.wantText, diags.String())
			}
		})
	}
}
