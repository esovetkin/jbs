package eval

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func listExpr(items ...ast.Expr) ast.ListExpr {
	return ast.ListExpr{Items: items}
}

func tupleExpr(items ...ast.Expr) ast.TupleExpr {
	return ast.TupleExpr{Items: items}
}

func TestMapCallSupportsListsTuplesDefaultsClosuresAndComposition(t *testing.T) {
	t.Run("list result", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		inc := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "+",
			Right: intExpr(1),
		}))
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(inc),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(2), Int(3), Int(4)})
		if !Equal(got, want) {
			t.Fatalf("unexpected map list result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("tuple result preserves tuple kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		double := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "*",
			Right: intExpr(2),
		}))
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(double),
			posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		want := Tuple([]Value{Int(2), Int(4), Int(6)})
		if !Equal(got, want) || got.Kind != KindTuple {
			t.Fatalf("unexpected map tuple result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("callback defaults work", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		addDefault := fnExpr(
			[]ast.FuncParam{{Name: "x"}, {Name: "delta", Default: intExpr(1)}},
			exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: ident("delta")}),
		)
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(addDefault),
			posArg(listExpr(intExpr(1), intExpr(2))),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(2), Int(3)})
		if !Equal(got, want) {
			t.Fatalf("unexpected defaulted map result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("closure callback reads captured value", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		makeAdder := fnExpr(
			[]ast.FuncParam{{Name: "delta"}},
			exprStmt(fnExpr(
				[]ast.FuncParam{{Name: "x"}},
				exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: ident("delta")}),
			)),
		)
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(callExpr(makeAdder, posArg(intExpr(10)))),
			posArg(listExpr(intExpr(1), intExpr(2))),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(11), Int(12)})
		if !Equal(got, want) {
			t.Fatalf("unexpected closure map result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("nested map and reduce compose", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		inc := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "+",
			Right: intExpr(1),
		}))
		sum2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(sum2),
			posArg(callExpr(ident("map"),
				posArg(inc),
				posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
			)),
		), nil, diags, ExprOptions{})
		if !Equal(got, Int(9)) {
			t.Fatalf("unexpected composed reduce(map(...)) result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestReduceCallSupportsListsTuplesAndSingletons(t *testing.T) {
	t.Run("list reduction", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		sum2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(sum2),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4))),
		), nil, diags, ExprOptions{})
		if !Equal(got, Int(10)) {
			t.Fatalf("unexpected reduce list result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("tuple reduction", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		cat2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(cat2),
			posArg(tupleExpr(ast.StringExpr{Value: "a"}, ast.StringExpr{Value: "b"}, ast.StringExpr{Value: "c"})),
		), nil, diags, ExprOptions{})
		if !Equal(got, String("abc")) {
			t.Fatalf("unexpected reduce tuple result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("singleton sequence returns item unchanged", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		sum2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(sum2),
			posArg(listExpr(intExpr(7))),
		), nil, diags, ExprOptions{})
		if !Equal(got, Int(7)) {
			t.Fatalf("unexpected singleton reduce result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestHigherOrderBuiltinsReportErrors(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		wantCode string
	}{
		{
			name:     "map wrong arity",
			expr:     callExpr(ident("map"), posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x"))))),
			wantCode: "E106",
		},
		{
			name:     "reduce wrong arity",
			expr:     callExpr(ident("reduce"), posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x"))))),
			wantCode: "E106",
		},
		{
			name: "map rejects named builtin args",
			expr: ast.CallExpr{
				Callee: ident("map"),
				Args: []ast.CallArg{
					namedArg("fn", fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x")))),
					posArg(listExpr(intExpr(1))),
				},
			},
			wantCode: "E106",
		},
		{
			name: "reduce rejects named builtin args",
			expr: ast.CallExpr{
				Callee: ident("reduce"),
				Args: []ast.CallArg{
					posArg(fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("acc")))),
					namedArg("values", listExpr(intExpr(1))),
				},
			},
			wantCode: "E106",
		},
		{
			name:     "map first arg must be function",
			expr:     callExpr(ident("map"), posArg(intExpr(1)), posArg(listExpr(intExpr(1)))),
			wantCode: "E106",
		},
		{
			name:     "reduce first arg must be function",
			expr:     callExpr(ident("reduce"), posArg(intExpr(1)), posArg(listExpr(intExpr(1)))),
			wantCode: "E106",
		},
		{
			name: "map second arg must be list or tuple",
			expr: callExpr(ident("map"),
				posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x")))),
				posArg(intExpr(1)),
			),
			wantCode: "E106",
		},
		{
			name: "reduce second arg must be list or tuple",
			expr: callExpr(ident("reduce"),
				posArg(fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("acc")))),
				posArg(intExpr(1)),
			),
			wantCode: "E106",
		},
		{
			name: "reduce empty input rejected",
			expr: callExpr(ident("reduce"),
				posArg(fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("acc")))),
				posArg(listExpr()),
			),
			wantCode: "E106",
		},
		{
			name: "builtins are not first class callback values",
			expr: callExpr(ident("map"),
				posArg(ident("int")),
				posArg(listExpr(ast.StringExpr{Value: "1"})),
			),
			wantCode: "E100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
		})
	}
}

func TestHigherOrderBuiltinsFailFastOnCallbackErrors(t *testing.T) {
	t.Run("map aborts on first callback error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		bad := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("missing")))
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(bad),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null result, got %#v", got)
		}
		if count := diagCount(diags, "E100"); count != 1 {
			t.Fatalf("expected exactly one E100, got %d: %s", count, diags.String())
		}
	})

	t.Run("reduce aborts on first callback error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		bad := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("missing")))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(bad),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null result, got %#v", got)
		}
		if count := diagCount(diags, "E100"); count != 1 {
			t.Fatalf("expected exactly one E100, got %d: %s", count, diags.String())
		}
	})
}
