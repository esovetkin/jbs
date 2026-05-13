package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func fnExpr(params []ast.FuncParam, body ...ast.FuncBodyStmt) ast.FunctionExpr {
	return ast.FunctionExpr{
		Params: params,
		Body:   body,
	}
}

func exprStmt(expr ast.Expr) ast.ExprStmt {
	return ast.ExprStmt{Expr: expr}
}

func localAssign(name string, expr ast.Expr) ast.LocalAssignStmt {
	return ast.LocalAssignStmt{Name: name, Op: ast.AssignEq, Expr: expr}
}

func posArg(expr ast.Expr) ast.CallArg {
	return ast.PosCallArg(expr)
}

func namedArg(name string, expr ast.Expr) ast.CallArg {
	return ast.CallArg{Kind: ast.CallArgNamed, Name: name, Expr: expr, Span: expr.GetSpan()}
}

func posSpreadArg(expr ast.Expr) ast.CallArg {
	return ast.CallArg{Kind: ast.CallArgPositionalSpread, Expr: expr, Span: expr.GetSpan()}
}

func kwSpreadArg(expr ast.Expr) ast.CallArg {
	return ast.CallArg{Kind: ast.CallArgKeywordSpread, Expr: expr, Span: expr.GetSpan()}
}

func callExpr(callee ast.Expr, args ...ast.CallArg) ast.CallExpr {
	return ast.CallExpr{Callee: callee, Args: args}
}

func intExpr(v int64) ast.NumberExpr {
	return ast.NumberExpr{Int: true, IntValue: v}
}

func ident(name string) ast.IdentExpr {
	return ast.IdentExpr{Name: name}
}

func defineFunctionInFrame(t *testing.T, frame *Frame, name string, fn ast.FunctionExpr) Value {
	t.Helper()
	diags := &diag.Diagnostics{}
	value := EvalExprWithOptions(fn, nil, diags, ExprOptions{Frame: frame})
	if diags.HasErrors() {
		t.Fatalf("unexpected function definition diagnostics: %s", diags.String())
	}
	frame.AssignLocal(name, value, diag.Span{})
	return value
}

func recursiveFunctionFrame(t *testing.T, name string, fn ast.FunctionExpr) (*Frame, Value) {
	t.Helper()
	frame := NewRootFrame(nil)
	value := defineFunctionInFrame(t, frame, name, fn)
	return frame, value
}

func TestFunctionLiteralAndDirectCall(t *testing.T) {
	diags := &diag.Diagnostics{}
	fn := fnExpr(
		[]ast.FuncParam{{Name: "x"}},
		exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "+",
			Right: intExpr(1),
		}),
	)
	value := EvalExprWithOptions(fn, nil, diags, ExprOptions{})
	if value.Kind != KindFunction || value.Fn == nil {
		t.Fatalf("expected function value, got %#v", value)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for literal: %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(fn, posArg(intExpr(1))), nil, diags, ExprOptions{})
	if !Equal(got, Int(2)) {
		t.Fatalf("expected function call to return 2, got %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for call: %s", diags.String())
	}
}

func TestFunctionIfStatements(t *testing.T) {
	tests := []struct {
		name string
		fn   ast.FunctionExpr
		want Value
	}{
		{
			name: "true branch return",
			fn: fnExpr(nil, ast.FuncIfStmt{
				Cond: ast.BoolExpr{Value: true},
				Then: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(1)}},
				Else: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(2)}},
			}),
			want: Int(1),
		},
		{
			name: "false branch return",
			fn: fnExpr(nil, ast.FuncIfStmt{
				Cond: ast.BoolExpr{Value: false},
				Then: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(1)}},
				Else: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(2)}},
			}),
			want: Int(2),
		},
		{
			name: "branch local assignment",
			fn: fnExpr(nil,
				localAssign("x", intExpr(1)),
				ast.FuncIfStmt{
					Cond: ast.BoolExpr{Value: true},
					Then: []ast.FuncBodyStmt{localAssign("x", intExpr(3))},
				},
				exprStmt(ident("x")),
			),
			want: Int(3),
		},
		{
			name: "nested if",
			fn: fnExpr(nil, ast.FuncIfStmt{
				Cond: ast.BoolExpr{Value: true},
				Then: []ast.FuncBodyStmt{ast.FuncIfStmt{
					Cond: ast.BoolExpr{Value: false},
					Then: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(1)}},
					Else: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(4)}},
				}},
			}),
			want: Int(4),
		},
		{
			name: "elif branch return",
			fn: fnExpr(nil, ast.FuncIfStmt{
				Cond: ast.BoolExpr{Value: false},
				Then: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(1)}},
				Elifs: []ast.FuncElifBranch{{
					Cond: ast.BoolExpr{Value: true},
					Body: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(2)}},
				}},
				Else: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(3)}},
			}),
			want: Int(2),
		},
		{
			name: "elif final else",
			fn: fnExpr(nil, ast.FuncIfStmt{
				Cond: ast.BoolExpr{Value: false},
				Then: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(1)}},
				Elifs: []ast.FuncElifBranch{{
					Cond: ast.BoolExpr{Value: false},
					Body: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(2)}},
				}},
				Else: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(3)}},
			}),
			want: Int(3),
		},
		{
			name: "elif local assignment",
			fn: fnExpr(nil,
				ast.FuncIfStmt{
					Cond: ast.BoolExpr{Value: false},
					Then: []ast.FuncBodyStmt{localAssign("y", intExpr(1))},
					Elifs: []ast.FuncElifBranch{{
						Cond: ast.BoolExpr{Value: true},
						Body: []ast.FuncBodyStmt{localAssign("y", intExpr(2))},
					}},
				},
				exprStmt(ident("y")),
			),
			want: Int(2),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(tc.fn), nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestFunctionIfRejectsNonBoolCondition(t *testing.T) {
	diags := &diag.Diagnostics{}
	fn := fnExpr(nil,
		localAssign("x", intExpr(1)),
		ast.FuncIfStmt{
			Cond: intExpr(1),
			Then: []ast.FuncBodyStmt{localAssign("x", intExpr(2))},
		},
		exprStmt(ident("x")),
	)
	got := EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
	if !Equal(got, Int(1)) {
		t.Fatalf("expected invalid condition to skip branch, got %#v", got)
	}
	if diagCount(diags, "E102") != 1 {
		t.Fatalf("expected one E102, got: %s", diags.String())
	}
}

func TestFunctionElifRejectsNonBoolCondition(t *testing.T) {
	diags := &diag.Diagnostics{}
	fn := fnExpr(nil,
		localAssign("x", intExpr(1)),
		ast.FuncIfStmt{
			Cond: ast.BoolExpr{Value: false},
			Then: []ast.FuncBodyStmt{localAssign("x", intExpr(2))},
			Elifs: []ast.FuncElifBranch{{
				Cond: intExpr(1),
				Body: []ast.FuncBodyStmt{localAssign("x", intExpr(3))},
			}},
			Else: []ast.FuncBodyStmt{localAssign("x", intExpr(4))},
		},
		exprStmt(ident("x")),
	)
	got := EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
	if !Equal(got, Int(1)) {
		t.Fatalf("expected invalid elif condition to skip branch chain, got %#v", got)
	}
	if diagCount(diags, "E102") != 1 || !strings.Contains(diags.String(), "elif condition requires boolean value") {
		t.Fatalf("expected one elif E102, got: %s", diags.String())
	}
}

func TestFunctionForLoopStatements(t *testing.T) {
	tests := []struct {
		name string
		fn   ast.FunctionExpr
		want Value
	}{
		{
			name: "sums list values",
			fn: fnExpr([]ast.FuncParam{{Name: "values"}},
				localAssign("total", intExpr(0)),
				ast.FuncForStmt{
					Target:   "x",
					Iterable: ident("values"),
					Body: []ast.FuncBodyStmt{
						ast.LocalAssignStmt{Name: "total", Op: ast.AssignPlusEq, Expr: ident("x")},
					},
				},
				exprStmt(ident("total")),
			),
			want: Int(6),
		},
		{
			name: "continue skips body remainder",
			fn: fnExpr([]ast.FuncParam{{Name: "values"}},
				localAssign("total", intExpr(0)),
				ast.FuncForStmt{
					Target:   "x",
					Iterable: ident("values"),
					Body: []ast.FuncBodyStmt{
						ast.FuncIfStmt{
							Cond: ast.CompareExpr{Left: ident("x"), Op: "==", Right: intExpr(2)},
							Then: []ast.FuncBodyStmt{ast.ContinueStmt{}},
						},
						ast.LocalAssignStmt{Name: "total", Op: ast.AssignPlusEq, Expr: ident("x")},
					},
				},
				exprStmt(ident("total")),
			),
			want: Int(4),
		},
		{
			name: "break exits nearest loop",
			fn: fnExpr([]ast.FuncParam{{Name: "values"}},
				localAssign("total", intExpr(0)),
				ast.FuncForStmt{
					Target:   "x",
					Iterable: ident("values"),
					Body: []ast.FuncBodyStmt{
						ast.FuncIfStmt{
							Cond: ast.CompareExpr{Left: ident("x"), Op: "==", Right: intExpr(3)},
							Then: []ast.FuncBodyStmt{ast.BreakStmt{}},
						},
						ast.LocalAssignStmt{Name: "total", Op: ast.AssignPlusEq, Expr: ident("x")},
					},
				},
				exprStmt(ident("total")),
			),
			want: Int(3),
		},
		{
			name: "return exits function",
			fn: fnExpr([]ast.FuncParam{{Name: "values"}},
				ast.FuncForStmt{
					Target:   "x",
					Iterable: ident("values"),
					Body:     []ast.FuncBodyStmt{ast.ReturnStmt{Expr: ident("x")}},
				},
				exprStmt(intExpr(0)),
			),
			want: Int(1),
		},
		{
			name: "loop target remains visible",
			fn: fnExpr([]ast.FuncParam{{Name: "values"}},
				ast.FuncForStmt{
					Target:   "x",
					Iterable: ident("values"),
					Body:     []ast.FuncBodyStmt{},
				},
				exprStmt(ident("x")),
			),
			want: Int(3),
		},
	}
	args := []ast.CallArg{posArg(ast.ListExpr{Items: []ast.Expr{intExpr(1), intExpr(2), intExpr(3)}})}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(tc.fn, args...), nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestFunctionForLoopErrors(t *testing.T) {
	t.Run("scalar iterable", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(nil, ast.FuncForStmt{Target: "x", Iterable: intExpr(1)})
		_ = EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
	t.Run("empty loop target unassigned", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(nil,
			ast.FuncForStmt{Target: "x", Iterable: ast.ListExpr{}},
			exprStmt(ident("x")),
		)
		_ = EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
		if diagCount(diags, "E100") == 0 || !strings.Contains(diags.String(), "before assignment") {
			t.Fatalf("expected unassigned local diagnostic, got: %s", diags.String())
		}
	})
}

func TestFunctionWhileLoopStatements(t *testing.T) {
	diags := &diag.Diagnostics{}
	fn := fnExpr(nil,
		localAssign("x", intExpr(0)),
		ast.FuncWhileStmt{
			Cond: ast.CompareExpr{Left: ident("x"), Op: "<", Right: intExpr(5)},
			Body: []ast.FuncBodyStmt{
				ast.LocalAssignStmt{Name: "x", Op: ast.AssignPlusEq, Expr: intExpr(1)},
				ast.FuncIfStmt{
					Cond: ast.CompareExpr{Left: ident("x"), Op: "==", Right: intExpr(3)},
					Then: []ast.FuncBodyStmt{ast.BreakStmt{}},
				},
			},
		},
		exprStmt(ident("x")),
	)
	got := EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
	if !Equal(got, Int(3)) {
		t.Fatalf("got %#v want 3", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestFunctionWhileLoopErrors(t *testing.T) {
	t.Run("non bool condition", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(nil, ast.FuncWhileStmt{Cond: intExpr(1)})
		_ = EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
		if diagCount(diags, "E102") == 0 {
			t.Fatalf("expected E102, got: %s", diags.String())
		}
	})
	t.Run("iteration limit", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(nil, ast.FuncWhileStmt{Cond: ast.BoolExpr{Value: true}})
		_ = EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func TestFunctionNamedArgsDefaultsAndBindingErrors(t *testing.T) {
	t.Run("named args", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{{Name: "a"}, {Name: "b"}},
			exprStmt(ast.BinaryExpr{Left: ident("a"), Op: "+", Right: ident("b")}),
		)
		got := EvalExprWithOptions(callExpr(fn, posArg(intExpr(1)), namedArg("b", intExpr(2))), nil, diags, ExprOptions{})
		if !Equal(got, Int(3)) {
			t.Fatalf("expected named-arg call to return 3, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("default uses earlier param", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{
				{Name: "a"},
				{Name: "b", Default: ast.BinaryExpr{Left: ident("a"), Op: "+", Right: intExpr(1)}},
			},
			exprStmt(ident("b")),
		)
		got := EvalExprWithOptions(callExpr(fn, posArg(intExpr(2))), nil, diags, ExprOptions{})
		if !Equal(got, Int(3)) {
			t.Fatalf("expected defaulted call to return 3, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("default uses outer lexical value", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{{Name: "a", Default: ident("y")}},
			exprStmt(ident("a")),
		)
		got := EvalExprWithOptions(callExpr(fn), map[string]Value{"y": Int(7)}, diags, ExprOptions{})
		if !Equal(got, Int(7)) {
			t.Fatalf("expected outer lexical default to return 7, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("unknown named argument rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr([]ast.FuncParam{{Name: "a"}}, exprStmt(ident("a")))
		got := EvalExprWithOptions(callExpr(fn, namedArg("b", intExpr(1))), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null on bad named argument, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("duplicate parameter value rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr([]ast.FuncParam{{Name: "a"}}, exprStmt(ident("a")))
		got := EvalExprWithOptions(callExpr(fn, posArg(intExpr(1)), namedArg("a", intExpr(2))), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null on duplicate parameter binding, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("missing required argument rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr([]ast.FuncParam{{Name: "a"}}, exprStmt(ident("a")))
		got := EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null on missing required arg, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("positional after named rejected at runtime too", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr([]ast.FuncParam{{Name: "a"}, {Name: "b"}}, exprStmt(ident("a")))
		got := EvalExprWithOptions(ast.CallExpr{
			Callee: fn,
			Args: []ast.CallArg{
				namedArg("a", intExpr(1)),
				posArg(intExpr(2)),
			},
		}, nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null on positional-after-named call, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func TestFunctionRestArgumentsAndCallExpansion(t *testing.T) {
	t.Run("args rest captures extra positionals", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{{Kind: ast.FuncParamArgs, Name: "args"}},
			exprStmt(ident("args")),
		)
		got := EvalExprWithOptions(callExpr(fn, posArg(intExpr(1)), posArg(intExpr(2))), nil, diags, ExprOptions{})
		if !Equal(got, List([]Value{Int(1), Int(2)})) {
			t.Fatalf("unexpected args rest value: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("kwargs rest captures unknown named", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{{Kind: ast.FuncParamKwargs, Name: "kwargs"}},
			exprStmt(ident("kwargs")),
		)
		got := EvalExprWithOptions(callExpr(fn, namedArg("x", intExpr(1)), namedArg("y", intExpr(2))), nil, diags, ExprOptions{})
		want := DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)},
			{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: Int(2)},
		})
		if !Equal(got, want) {
			t.Fatalf("unexpected kwargs rest value: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("fixed args and kwargs combine", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{
				{Kind: ast.FuncParamValue, Name: "a"},
				{Kind: ast.FuncParamArgs, Name: "args"},
				{Kind: ast.FuncParamKwargs, Name: "kwargs"},
			},
			exprStmt(listExpr(ident("a"), ident("args"), ident("kwargs"))),
		)
		got := EvalExprWithOptions(callExpr(fn, posArg(intExpr(1)), posArg(intExpr(2)), namedArg("x", intExpr(3))), nil, diags, ExprOptions{})
		want := List([]Value{
			Int(1),
			List([]Value{Int(2)}),
			DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(3)}}),
		})
		if !Equal(got, want) {
			t.Fatalf("unexpected mixed rest value: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("call site expansion succeeds", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		fn := fnExpr(
			[]ast.FuncParam{
				{Kind: ast.FuncParamValue, Name: "a"},
				{Kind: ast.FuncParamValue, Name: "b"},
				{Kind: ast.FuncParamKwargs, Name: "kwargs"},
			},
			exprStmt(listExpr(ident("a"), ident("b"), ident("kwargs"))),
		)
		kwargs := DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(3)}})
		got := EvalExprWithOptions(callExpr(fn,
			posSpreadArg(listExpr(intExpr(1), intExpr(2))),
			kwSpreadArg(ident("kwargs")),
		), map[string]Value{"kwargs": kwargs}, diags, ExprOptions{})
		want := List([]Value{
			Int(1),
			Int(2),
			DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(3)}}),
		})
		if !Equal(got, want) {
			t.Fatalf("unexpected spread call value: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestFunctionCallExpansionDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		args []ast.CallArg
		env  map[string]Value
	}{
		{
			name: "star requires sequence",
			args: []ast.CallArg{posSpreadArg(intExpr(1))},
		},
		{
			name: "starstar requires dict",
			args: []ast.CallArg{kwSpreadArg(intExpr(1))},
		},
		{
			name: "starstar requires string keys",
			args: []ast.CallArg{kwSpreadArg(ident("kwargs"))},
			env:  map[string]Value{"kwargs": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyInt, I: 1}, Value: Int(1)}})},
		},
		{
			name: "duplicate named after starstar",
			args: []ast.CallArg{namedArg("x", intExpr(1)), kwSpreadArg(ident("kwargs"))},
			env:  map[string]Value{"kwargs": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(2)}})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			fn := fnExpr(
				[]ast.FuncParam{{Kind: ast.FuncParamValue, Name: "x"}, {Kind: ast.FuncParamKwargs, Name: "kwargs"}},
				exprStmt(ident("x")),
			)
			got := EvalExprWithOptions(callExpr(fn, tc.args...), tc.env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null on bad expansion, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 {
				t.Fatalf("expected E106, got: %s", diags.String())
			}
		})
	}
}

func TestFunctionDefaultIndexItemReferenceUsesCallArgument(t *testing.T) {
	diags := &diag.Diagnostics{}
	fn := fnExpr(
		[]ast.FuncParam{
			{Name: "key"},
			{
				Name: "value",
				Default: ast.IndexExpr{
					Base:  ident("cfg"),
					Items: []ast.Expr{ident("key")},
				},
			},
		},
		exprStmt(ident("value")),
	)
	env := map[string]Value{
		"cfg": DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "a"}, Value: Int(1)},
			{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: Int(2)},
		}),
	}

	got := EvalExprWithOptions(callExpr(fn, posArg(stringExpr("b"))), env, diags, ExprOptions{})
	if !Equal(got, Int(2)) {
		t.Fatalf("got %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestFunctionClosuresHigherOrderAndLocals(t *testing.T) {
	t.Run("closure captures outer variable by reference", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		closureFactory := fnExpr(nil,
			localAssign("x", intExpr(1)),
			localAssign("g", fnExpr(nil, exprStmt(ident("x")))),
			localAssign("x", intExpr(2)),
			exprStmt(ident("g")),
		)
		got := EvalExprWithOptions(callExpr(callExpr(closureFactory)), nil, diags, ExprOptions{})
		if !Equal(got, Int(2)) {
			t.Fatalf("expected closure to observe updated x=2, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("returned closure works", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		makeAdder := fnExpr(
			[]ast.FuncParam{{Name: "a"}},
			exprStmt(fnExpr(
				[]ast.FuncParam{{Name: "b"}},
				exprStmt(ast.BinaryExpr{Left: ident("a"), Op: "+", Right: ident("b")}),
			)),
		)
		got := EvalExprWithOptions(callExpr(callExpr(makeAdder, posArg(intExpr(1))), posArg(intExpr(2))), nil, diags, ExprOptions{})
		if !Equal(got, Int(3)) {
			t.Fatalf("expected make_adder(1)(2) == 3, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("higher-order function works", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		applyFn := fnExpr(
			[]ast.FuncParam{{Name: "f"}, {Name: "x"}},
			exprStmt(callExpr(ident("f"), posArg(ident("x")))),
		)
		increment := fnExpr(
			[]ast.FuncParam{{Name: "a"}},
			exprStmt(ast.BinaryExpr{Left: ident("a"), Op: "+", Right: intExpr(1)}),
		)
		got := EvalExprWithOptions(callExpr(applyFn, posArg(increment), posArg(intExpr(3))), nil, diags, ExprOptions{})
		if !Equal(got, Int(4)) {
			t.Fatalf("expected higher-order function result 4, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("local assignment shadows outer name", func(t *testing.T) {
		env := map[string]Value{"x": Int(10)}
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(fnExpr(nil,
			localAssign("x", intExpr(1)),
			exprStmt(ident("x")),
		)), env, diags, ExprOptions{})
		if !Equal(got, Int(1)) {
			t.Fatalf("expected local x=1, got %#v", got)
		}
		if env["x"].I != 10 {
			t.Fatalf("expected outer env x to remain unchanged, got %#v", env["x"])
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("uninitialized local read reports error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(fnExpr(nil,
			exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: intExpr(1)}),
			localAssign("x", intExpr(2)),
		)), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null after uninitialized local read, got %#v", got)
		}
		if diagCount(diags, "E100") == 0 || !strings.Contains(diags.String(), "before assignment") {
			t.Fatalf("expected uninitialized-local diagnostic, got: %s", diags.String())
		}
	})

	t.Run("builtin name shadowing works", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		shadow := fnExpr(
			[]ast.FuncParam{{Name: "len"}},
			exprStmt(callExpr(ident("len"), posArg(intExpr(1)))),
		)
		got := EvalExprWithOptions(callExpr(shadow, posArg(fnExpr(
			[]ast.FuncParam{{Name: "x"}},
			exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: intExpr(1)}),
		))), nil, diags, ExprOptions{})
		if !Equal(got, Int(2)) {
			t.Fatalf("expected shadowed builtin call to use local function, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestFunctionRecursionDepthLimit(t *testing.T) {
	fn := fnExpr(
		[]ast.FuncParam{{Name: "n"}},
		exprStmt(callExpr(
			ident("loop"),
			posArg(ast.BinaryExpr{Left: ident("n"), Op: "+", Right: intExpr(1)}),
		)),
	)
	frame, _ := recursiveFunctionFrame(t, "loop", fn)

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		callExpr(ident("loop"), posArg(intExpr(0))),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 4},
	)
	if got.Kind != KindNull {
		t.Fatalf("expected null after recursion-depth failure, got %#v", got)
	}
	if count := diagCount(diags, "E106"); count != 1 {
		t.Fatalf("expected one E106, got %d: %s", count, diags.String())
	}
	if !strings.Contains(diags.String(), "maximum function recursion depth of 4 reached") {
		t.Fatalf("expected recursion-depth diagnostic, got: %s", diags.String())
	}
}

func TestFunctionRecursionDepthLimitStopsSiblingEvaluation(t *testing.T) {
	fibBody := ast.ConditionalExpr{
		Then: intExpr(1),
		Cond: ast.BinaryExpr{
			Left:  ast.CompareExpr{Left: intExpr(1), Op: "==", Right: ident("n")},
			Op:    "|",
			Right: ast.CompareExpr{Left: intExpr(2), Op: "==", Right: ident("n")},
		},
		Else: ast.BinaryExpr{
			Left: callExpr(ident("fib"), posArg(ast.BinaryExpr{
				Left: ident("n"), Op: "-", Right: intExpr(1),
			})),
			Op: "+",
			Right: callExpr(ident("fib"), posArg(ast.BinaryExpr{
				Left: ident("n"), Op: "-", Right: intExpr(2),
			})),
		},
	}
	frame, _ := recursiveFunctionFrame(t, "fib", fnExpr(
		[]ast.FuncParam{{Name: "n"}},
		exprStmt(fibBody),
	))

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		callExpr(ident("fib"), posArg(intExpr(0))),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 6},
	)
	if got.Kind != KindNull {
		t.Fatalf("expected null after recursion-depth failure, got %#v", got)
	}
	if count := diagCount(diags, "E106"); count != 1 {
		t.Fatalf("expected one recursion-depth error, got %d: %s", count, diags.String())
	}
}

func TestFunctionRecursionDepthBoundaryAndIndependentEvaluations(t *testing.T) {
	frame := NewRootFrame(nil)
	defineFunctionInFrame(t, frame, "inner", fnExpr(nil, exprStmt(intExpr(2))))
	defineFunctionInFrame(t, frame, "outer", fnExpr(nil, exprStmt(callExpr(ident("inner")))))

	diags := &diag.Diagnostics{}
	blocked := EvalExprWithOptions(
		callExpr(ident("outer")),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 1},
	)
	if blocked.Kind != KindNull {
		t.Fatalf("expected null when nested call exceeds depth 1, got %#v", blocked)
	}
	if count := diagCount(diags, "E106"); count != 1 {
		t.Fatalf("expected one E106, got %d: %s", count, diags.String())
	}

	diags = &diag.Diagnostics{}
	allowed := EvalExprWithOptions(
		callExpr(ident("outer")),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 2},
	)
	if !Equal(allowed, Int(2)) {
		t.Fatalf("expected nested call to succeed at depth 2, got %#v", allowed)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics after independent evaluation: %s", diags.String())
	}
}

func TestFunctionValidRecursionUnderLimit(t *testing.T) {
	body := ast.ConditionalExpr{
		Then: intExpr(0),
		Cond: ast.CompareExpr{Left: ident("n"), Op: "==", Right: intExpr(0)},
		Else: callExpr(ident("countdown"), posArg(ast.BinaryExpr{
			Left: ident("n"), Op: "-", Right: intExpr(1),
		})),
	}
	frame, _ := recursiveFunctionFrame(t, "countdown", fnExpr(
		[]ast.FuncParam{{Name: "n"}},
		exprStmt(body),
	))

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		callExpr(ident("countdown"), posArg(intExpr(5))),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 16},
	)
	if !Equal(got, Int(0)) {
		t.Fatalf("expected countdown to return 0, got %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestFunctionMutualRecursionDepthLimit(t *testing.T) {
	frame := NewRootFrame(nil)
	defineFunctionInFrame(t, frame, "even", fnExpr(
		[]ast.FuncParam{{Name: "n"}},
		exprStmt(callExpr(ident("odd"), posArg(ast.BinaryExpr{
			Left: ident("n"), Op: "+", Right: intExpr(1),
		}))),
	))
	defineFunctionInFrame(t, frame, "odd", fnExpr(
		[]ast.FuncParam{{Name: "n"}},
		exprStmt(callExpr(ident("even"), posArg(ast.BinaryExpr{
			Left: ident("n"), Op: "+", Right: intExpr(1),
		}))),
	))

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		callExpr(ident("even"), posArg(intExpr(0))),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 5},
	)
	if got.Kind != KindNull {
		t.Fatalf("expected null after mutual recursion-depth failure, got %#v", got)
	}
	if count := diagCount(diags, "E106"); count != 1 {
		t.Fatalf("expected one E106, got %d: %s", count, diags.String())
	}
}

func TestFunctionRecursiveDefaultDepthLimit(t *testing.T) {
	frame, _ := recursiveFunctionFrame(t, "f", fnExpr(
		[]ast.FuncParam{
			{Name: "n"},
			{Name: "x", Default: callExpr(ident("f"), posArg(ident("n")))},
		},
		exprStmt(ident("x")),
	))

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		callExpr(ident("f"), posArg(intExpr(0))),
		nil,
		diags,
		ExprOptions{Frame: frame, MaxFunctionCallDepth: 4},
	)
	if got.Kind != KindNull {
		t.Fatalf("expected null after recursive default-depth failure, got %#v", got)
	}
	if count := diagCount(diags, "E106"); count != 1 {
		t.Fatalf("expected one E106, got %d: %s", count, diags.String())
	}
}

func TestFunctionHigherOrderDepthLimit(t *testing.T) {
	t.Run("sequential callbacks do not accumulate depth", func(t *testing.T) {
		frame, _ := recursiveFunctionFrame(t, "inc", fnExpr(
			[]ast.FuncParam{{Name: "x"}},
			exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: intExpr(1)}),
		))

		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			callExpr(ident("map"),
				posArg(ident("inc")),
				posArg(ast.ListExpr{Items: []ast.Expr{intExpr(1), intExpr(2), intExpr(3)}}),
			),
			nil,
			diags,
			ExprOptions{Frame: frame, MaxFunctionCallDepth: 1},
		)
		want := List([]Value{Int(2), Int(3), Int(4)})
		if !Equal(got, want) {
			t.Fatalf("expected map callback result %#v, got %#v", want, got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("recursive callback is limited", func(t *testing.T) {
		frame, _ := recursiveFunctionFrame(t, "loop", fnExpr(
			[]ast.FuncParam{{Name: "x"}},
			exprStmt(callExpr(ident("loop"), posArg(ident("x")))),
		))

		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			callExpr(ident("map"),
				posArg(ident("loop")),
				posArg(ast.ListExpr{Items: []ast.Expr{intExpr(1)}}),
			),
			nil,
			diags,
			ExprOptions{Frame: frame, MaxFunctionCallDepth: 3},
		)
		if got.Kind != KindNull {
			t.Fatalf("expected null after recursive callback-depth failure, got %#v", got)
		}
		if count := diagCount(diags, "E106"); count != 1 {
			t.Fatalf("expected one E106, got %d: %s", count, diags.String())
		}
	})
}

func TestFunctionValueBehaviorOutsideCalls(t *testing.T) {
	t.Run("list and tuple preserve function values", func(t *testing.T) {
		fn := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x")))
		diags := &diag.Diagnostics{}
		listValue := EvalExprWithOptions(callExpr(ident("list"), posArg(fn)), nil, diags, ExprOptions{})
		if listValue.Kind != KindList || len(listValue.L) != 1 || listValue.L[0].Kind != KindFunction {
			t.Fatalf("expected list(function) to preserve function value, got %#v", listValue)
		}
		tupleValue := EvalExprWithOptions(callExpr(ident("tuple"), posArg(fn)), nil, diags, ExprOptions{})
		if tupleValue.Kind != KindTuple || len(tupleValue.L) != 1 || tupleValue.L[0].Kind != KindFunction {
			t.Fatalf("expected tuple(function) to preserve function value, got %#v", tupleValue)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("stringification returns placeholder", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("str"), posArg(fnExpr(nil, exprStmt(intExpr(1))))), nil, diags, ExprOptions{})
		if got.Kind != KindString || got.S != "<function>" {
			t.Fatalf("expected str(function) == <function>, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("arithmetic on function values is rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(ast.BinaryExpr{
			Left:  fnExpr(nil, exprStmt(intExpr(1))),
			Op:    "+",
			Right: intExpr(1),
		}, nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null from invalid function arithmetic, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("comparisons on function values are rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		eq := EvalExprWithOptions(ast.CompareExpr{
			Left:  fnExpr(nil, exprStmt(intExpr(1))),
			Op:    "==",
			Right: fnExpr(nil, exprStmt(intExpr(1))),
		}, nil, diags, ExprOptions{})
		if eq.Kind != KindBool || eq.B {
			t.Fatalf("expected false placeholder result for rejected equality, got %#v", eq)
		}
		order := EvalExprWithOptions(ast.CompareExpr{
			Left:  fnExpr(nil, exprStmt(intExpr(1))),
			Op:    "<",
			Right: fnExpr(nil, exprStmt(intExpr(1))),
		}, nil, diags, ExprOptions{})
		if order.Kind != KindBool || order.B {
			t.Fatalf("expected false placeholder result for rejected ordering compare, got %#v", order)
		}
		if diagCount(diags, "E110") < 2 {
			t.Fatalf("expected function comparison diagnostics, got: %s", diags.String())
		}
	})
}

func TestFunctionReadCSVUsesDefinitionBaseDir(t *testing.T) {
	makeCasesFile := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cases.csv"), []byte("x\n1\n2\n"), 0o644); err != nil {
			t.Fatalf("write cases.csv: %v", err)
		}
	}

	t.Run("direct function call uses definition dir", func(t *testing.T) {
		defDir := filepath.Join(t.TempDir(), "def")
		makeCasesFile(t, defDir)
		otherDir := t.TempDir()

		diags := &diag.Diagnostics{}
		fn := EvalExprWithOptions(fnExpr(nil,
			exprStmt(callExpr(ident("read_csv"), posArg(ast.StringExpr{Value: "./cases.csv"}))),
		), nil, diags, ExprOptions{
			Context: EvalCtxBindingAssign,
			Files:   &FileAccess{BaseDir: defDir},
		})
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics while defining read_csv function: %s", diags.String())
		}

		diags = &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("f")), map[string]Value{"f": fn}, diags, ExprOptions{
			Context: EvalCtxBindingAssign,
			Files:   &FileAccess{BaseDir: otherDir},
		})
		if !IsComb(got) || CombRowCount(got) != 2 {
			t.Fatalf("expected closure read_csv to load 2 rows, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics while calling read_csv function: %s", diags.String())
		}
	})

	t.Run("returned closure keeps definition dir", func(t *testing.T) {
		defDir := filepath.Join(t.TempDir(), "def")
		makeCasesFile(t, defDir)
		otherDir := t.TempDir()

		diags := &diag.Diagnostics{}
		factory := EvalExprWithOptions(fnExpr(nil,
			exprStmt(fnExpr(nil,
				exprStmt(callExpr(ident("read_csv"), posArg(ast.StringExpr{Value: "./cases.csv"}))),
			)),
		), nil, diags, ExprOptions{
			Context: EvalCtxBindingAssign,
			Files:   &FileAccess{BaseDir: defDir},
		})
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics while defining closure factory: %s", diags.String())
		}

		diags = &diag.Diagnostics{}
		closure := EvalExprWithOptions(callExpr(ident("factory")), map[string]Value{"factory": factory}, diags, ExprOptions{
			Context: EvalCtxBindingAssign,
			Files:   &FileAccess{BaseDir: otherDir},
		})
		if closure.Kind != KindFunction {
			t.Fatalf("expected factory to return function, got %#v", closure)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics while creating closure: %s", diags.String())
		}

		diags = &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("closure")), map[string]Value{"closure": closure}, diags, ExprOptions{
			Context: EvalCtxBindingAssign,
			Files:   &FileAccess{BaseDir: otherDir},
		})
		if !IsComb(got) || CombRowCount(got) != 2 {
			t.Fatalf("expected returned closure read_csv to load 2 rows, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics while calling returned closure: %s", diags.String())
		}
	})
}
