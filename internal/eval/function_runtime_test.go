package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestFunctionRuntimeTopLevelControlAndBranchBehavior(t *testing.T) {
	tests := []struct {
		name     string
		fn       ast.FunctionExpr
		want     Value
		wantCode string
	}{
		{
			name: "return without expression",
			fn: fnExpr(nil,
				exprStmt(intExpr(1)),
				ast.ReturnStmt{},
			),
			want: Null(),
		},
		{
			name: "top level break rejected",
			fn: fnExpr(nil,
				exprStmt(intExpr(1)),
				ast.BreakStmt{Span: spanAt(1701, 1)},
			),
			want:     Null(),
			wantCode: "E080",
		},
		{
			name: "top level continue rejected",
			fn: fnExpr(nil,
				exprStmt(intExpr(1)),
				ast.ContinueStmt{Span: spanAt(1701, 2)},
			),
			want:     Null(),
			wantCode: "E080",
		},
		{
			name: "elif condition error skips else",
			fn: fnExpr(nil,
				localAssign("x", intExpr(1)),
				ast.FuncIfStmt{
					Cond: ast.BoolExpr{Value: false},
					Then: []ast.FuncBodyStmt{localAssign("x", intExpr(2))},
					Elifs: []ast.FuncElifBranch{{
						Cond: ast.IdentExpr{Name: "missing", Span: spanAt(1701, 3)},
						Body: []ast.FuncBodyStmt{localAssign("x", intExpr(3))},
					}},
					Else: []ast.FuncBodyStmt{localAssign("x", intExpr(4))},
				},
				exprStmt(ident("x")),
			),
			want:     Int(1),
			wantCode: "E100",
		},
		{
			name: "else body updates last value",
			fn: fnExpr(nil,
				ast.FuncIfStmt{
					Cond: ast.BoolExpr{Value: false},
					Then: []ast.FuncBodyStmt{exprStmt(intExpr(1))},
					Else: []ast.FuncBodyStmt{exprStmt(intExpr(5))},
				},
			),
			want: Int(5),
		},
		{
			name: "while body returns",
			fn: fnExpr(nil,
				ast.FuncWhileStmt{
					Cond: ast.BoolExpr{Value: true},
					Body: []ast.FuncBodyStmt{ast.ReturnStmt{Expr: intExpr(6)}},
				},
			),
			want: Int(6),
		},
		{
			name: "while continue",
			fn: fnExpr(nil,
				localAssign("x", intExpr(0)),
				ast.FuncWhileStmt{
					Cond: ast.CompareExpr{Left: ident("x"), Op: "<", Right: intExpr(3)},
					Body: []ast.FuncBodyStmt{
						ast.LocalAssignStmt{Name: "x", Op: ast.AssignPlusEq, Expr: intExpr(1)},
						ast.FuncIfStmt{
							Cond: ast.CompareExpr{Left: ident("x"), Op: "<", Right: intExpr(3)},
							Then: []ast.FuncBodyStmt{ast.ContinueStmt{}},
						},
						exprStmt(ident("x")),
					},
				},
			),
			want: Int(3),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(tc.fn), nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("got %#v want %#v diagnostics %s", got, tc.want, diags.String())
			}
			if tc.wantCode == "" {
				if diags.HasErrors() {
					t.Fatalf("unexpected diagnostics: %s", diags.String())
				}
				return
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
		})
	}
}

func TestFunctionRuntimeLoopAndAssignmentDiagnostics(t *testing.T) {
	items := make([]Value, MaxLoopIterations+1)
	for i := range items {
		items[i] = Int(int64(i))
	}

	diags := &diag.Diagnostics{}
	ctx := newEvalCtx(NewRootFrame(map[string]Value{"values": List(items)}))
	result := executeFuncForStmt(ast.FuncForStmt{
		Target:   "x",
		Iterable: ident("values"),
		Span:     spanAt(1702, 1),
	}, nil, diags, ExprOptions{}, ctx)
	if result.Value.Kind != KindNull || diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), LoopLimitExceededMessage()) {
		t.Fatalf("expected max-loop diagnostic, got result %#v diagnostics %s", result, diags.String())
	}

	diags = &diag.Diagnostics{}
	executeLocalAssign(ast.LocalAssignStmt{Name: "missing", Op: ast.AssignMinusEq, Expr: intExpr(1), Span: spanAt(1702, 2)}, nil, diags, ExprOptions{}, newEvalCtx(NewRootFrame(nil)))
	if diagCount(diags, "E100") == 0 {
		t.Fatalf("compound assignment to missing local should report E100, got: %s", diags.String())
	}
}

func TestLocalCompoundAssignmentReadsTargetBeforeRHS(t *testing.T) {
	shellCalls := 0
	frame := NewRootFrame(nil)
	frame.DeclareLocal("x")
	diags := &diag.Diagnostics{}

	executeLocalAssign(ast.LocalAssignStmt{
		Name: "x",
		Op:   ast.AssignPlusEq,
		Expr: callExpr(
			ast.IdentExpr{Name: "shell", Span: spanAt(1703, 5)},
			posArg(ast.StringExpr{Value: "printf 1", Span: spanAt(1703, 11)}),
		),
		Span: spanAt(1703, 1),
	}, nil, diags, ExprOptions{
		ShellRunner: func(ShellCommand) ([]byte, error) {
			shellCalls++
			return []byte("1"), nil
		},
	}, newEvalCtx(frame))

	if shellCalls != 0 {
		t.Fatalf("RHS shell() ran before invalid compound target was rejected")
	}
	if diagCount(diags, "E100") != 1 {
		t.Fatalf("expected one E100 diagnostic, got: %s", diags.String())
	}
}

func TestLocalCompoundAssignmentEvaluatesRHSWhenTargetAssigned(t *testing.T) {
	shellCalls := 0
	frame := NewRootFrame(map[string]Value{"x": Int(1)})
	diags := &diag.Diagnostics{}

	executeLocalAssign(ast.LocalAssignStmt{
		Name: "x",
		Op:   ast.AssignPlusEq,
		Expr: callExpr(
			ast.IdentExpr{Name: "shell", Span: spanAt(1704, 5)},
			posArg(ast.StringExpr{Value: "printf 2", Span: spanAt(1704, 11)}),
		),
		Span: spanAt(1704, 1),
	}, nil, diags, ExprOptions{
		ShellRunner: func(ShellCommand) ([]byte, error) {
			shellCalls++
			return []byte("2"), nil
		},
	}, newEvalCtx(frame))

	got, ok := frame.Read("x", diag.Span{}, &diag.Diagnostics{})
	if !ok || !Equal(got, String("12")) {
		t.Fatalf("unexpected x after compound assignment: %#v", got)
	}
	if shellCalls != 1 {
		t.Fatalf("expected RHS shell() to run once, got %d", shellCalls)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestFunctionCompoundAssignmentSkipsRHSForUnassignedLocal(t *testing.T) {
	shellCalls := 0
	diags := &diag.Diagnostics{}
	fn := fnExpr(nil,
		ast.LocalAssignStmt{
			Name: "x",
			Op:   ast.AssignPlusEq,
			Expr: callExpr(
				ast.IdentExpr{Name: "shell", Span: spanAt(1705, 5)},
				posArg(ast.StringExpr{Value: "printf 1", Span: spanAt(1705, 11)}),
			),
			Span: spanAt(1705, 1),
		},
	)

	got := EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{
		ShellRunner: func(ShellCommand) ([]byte, error) {
			shellCalls++
			return []byte("1"), nil
		},
	})

	if got.Kind != KindNull {
		t.Fatalf("expected null result after invalid assignment, got %#v", got)
	}
	if shellCalls != 0 {
		t.Fatalf("RHS shell() ran before invalid compound target was rejected")
	}
	if diagCount(diags, "E100") != 1 {
		t.Fatalf("expected one E100 diagnostic, got: %s", diags.String())
	}
}
