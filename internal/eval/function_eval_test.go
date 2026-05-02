package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
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
	return ast.CallArg{Name: name, Expr: expr, Span: expr.GetSpan()}
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
