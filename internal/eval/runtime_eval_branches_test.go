package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func recursiveLimitOperand(t *testing.T) (ast.Expr, map[string]Value, ExprOptions, *evalCtx) {
	t.Helper()
	frame := NewRootFrame(nil)
	fn := fnExpr(nil, exprStmt(callExpr(ident("loop"))))
	defineFunctionInFrame(t, frame, "loop", fn)
	opts := ExprOptions{Frame: frame, MaxFunctionCallDepth: 1}
	return callExpr(ident("loop")), nil, opts, newEvalCtx(frame)
}

func requireRecursionAbort(t *testing.T, got Value, diags *diag.Diagnostics, ctx *evalCtx) {
	t.Helper()
	if got.Kind != KindNull {
		t.Fatalf("expected null after recursion abort, got %#v", got)
	}
	if ctx == nil || !ctx.recursionLimitHit() {
		t.Fatalf("expected recursion limit flag to be set")
	}
	if count := diagCount(diags, "E106"); count != 1 {
		t.Fatalf("expected one recursion-depth diagnostic, got %d: %s", count, diags.String())
	}
	if !strings.Contains(diags.String(), "maximum function recursion depth of 1 reached") {
		t.Fatalf("expected recursion-depth message, got: %s", diags.String())
	}
}

func TestEvalBoolConditionForDirectHelper(t *testing.T) {
	t.Run("nil frame uses env root", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got, ok := EvalBoolConditionFor("while", ident("x"), map[string]Value{"x": Int(2)}, diags, ExprOptions{})
		if !ok || !got {
			t.Fatalf("expected truthy condition, got value=%v ok=%v", got, ok)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("provided frame is used", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{"x": String("")})
		diags := &diag.Diagnostics{}
		got, ok := EvalBoolConditionFor("if", ident("x"), map[string]Value{"x": Bool(true)}, diags, ExprOptions{Frame: frame})
		if !ok || got {
			t.Fatalf("expected falsey frame value, got value=%v ok=%v", got, ok)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("expression error returns not ok", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got, ok := EvalBoolConditionFor("if", ident("missing"), nil, diags, ExprOptions{})
		if ok || got {
			t.Fatalf("expected failed condition, got value=%v ok=%v", got, ok)
		}
		if diagCount(diags, "E100") != 1 {
			t.Fatalf("expected unknown-variable diagnostic, got: %s", diags.String())
		}
	})

	t.Run("recursion limit returns not ok", func(t *testing.T) {
		expr, env, opts, _ := recursiveLimitOperand(t)
		diags := &diag.Diagnostics{}
		got, ok := EvalBoolConditionFor("if", expr, env, diags, opts)
		if ok || got {
			t.Fatalf("expected recursion-limited condition to fail, got value=%v ok=%v", got, ok)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected recursion-depth diagnostic, got: %s", diags.String())
		}
	})
}

func TestEvalContextLimitAndFrameHelpers(t *testing.T) {
	var nilCtx *evalCtx
	nilCtx.markRecursionLimitHit()

	ctx := &evalCtx{}
	ctx.markRecursionLimitHit()
	ctx.markRecursionLimitHit()
	if !ctx.recursionLimitHit() {
		t.Fatalf("expected recursion limit flag")
	}
	if ctx.abort == nil {
		t.Fatalf("expected abort state to be initialized")
	}

	frame := NewRootFrame(map[string]Value{"x": Int(1)})
	next := nilCtx.withFrame(frame)
	if next == nil || next.frame != frame {
		t.Fatalf("nil withFrame did not create a context with requested frame")
	}
	if next.overflowWarned == nil || next.abort == nil {
		t.Fatalf("new context is missing helper state")
	}

	current := &evalCtx{}
	child := current.withFrame(frame)
	if child == current {
		t.Fatalf("withFrame should return a distinct context")
	}
	if child.frame != frame || child.overflowWarned == nil || child.abort == nil {
		t.Fatalf("withFrame did not populate helper fields: %#v", child)
	}

	parentAbort := &evalAbortState{}
	parentOverflow := map[string]struct{}{"seen": {}}
	current = &evalCtx{frame: NewRootFrame(nil), overflowWarned: parentOverflow, abort: parentAbort, callDepth: 3}
	child = current.withFrame(frame)
	if _, ok := child.overflowWarned["seen"]; child.frame != frame || child.abort != parentAbort || !ok || child.callDepth != 3 {
		t.Fatalf("withFrame did not preserve existing context state")
	}
	if current.frame == frame {
		t.Fatalf("withFrame mutated the original context")
	}
}

func TestEvalExprWithCtxUnassignedQualifiedLocals(t *testing.T) {
	tests := []struct {
		name  string
		local string
		expr  ast.Expr
	}{
		{
			name:  "unassigned namespace",
			local: "lib",
			expr:  ast.QualifiedIdentExpr{Namespace: "lib", Name: "x", Span: spanAt(1801, 1)},
		},
		{
			name:  "unassigned dotted fallback",
			local: "lib.x",
			expr:  ast.QualifiedIdentExpr{Namespace: "lib", Name: "x", Span: spanAt(1802, 1)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame := NewRootFrame(nil)
			frame.DeclareLocal(tc.local)
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{Frame: frame})
			if got.Kind != KindNull {
				t.Fatalf("expected null, got %#v", got)
			}
			if diagCount(diags, "E100") != 1 || !strings.Contains(diags.String(), "used before assignment") {
				t.Fatalf("expected unassigned-local diagnostic, got: %s", diags.String())
			}
		})
	}
}

func TestEvalExprWithCtxMemberUnknownColumn(t *testing.T) {
	diags := &diag.Diagnostics{}
	comb := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	got := EvalExprWithOptions(ast.MemberExpr{
		Base: ident("tbl"),
		Name: "missing",
		Span: spanAt(1803, 1),
	}, map[string]Value{"tbl": comb}, diags, ExprOptions{})
	if got.Kind != KindNull {
		t.Fatalf("expected null, got %#v", got)
	}
	if diagCount(diags, "E100") != 1 {
		t.Fatalf("expected unknown-column diagnostic, got: %s", diags.String())
	}
}

func TestEvalExprWithCtxRecursionLimitAbortWrappers(t *testing.T) {
	tests := []struct {
		name    string
		context EvalContext
		build   func(ast.Expr) ast.Expr
	}{
		{
			name:  "member base",
			build: func(child ast.Expr) ast.Expr { return ast.MemberExpr{Base: child, Name: "x", Span: spanAt(1810, 1)} },
		},
		{
			name:  "list item",
			build: func(child ast.Expr) ast.Expr { return ast.ListExpr{Items: []ast.Expr{child}, Span: spanAt(1811, 1)} },
		},
		{
			name:  "tuple item",
			build: func(child ast.Expr) ast.Expr { return ast.TupleExpr{Items: []ast.Expr{child}, Span: spanAt(1812, 1)} },
		},
		{
			name: "range start",
			build: func(child ast.Expr) ast.Expr {
				return ast.RangeExpr{Start: child, Stop: intExpr(3), Span: spanAt(1813, 1)}
			},
		},
		{
			name: "range stop",
			build: func(child ast.Expr) ast.Expr {
				return ast.RangeExpr{Start: intExpr(0), Stop: child, Span: spanAt(1814, 1)}
			},
		},
		{
			name: "range step",
			build: func(child ast.Expr) ast.Expr {
				return ast.RangeExpr{Start: intExpr(0), Stop: intExpr(3), Step: child, Span: spanAt(1815, 1)}
			},
		},
		{
			name: "function default",
			build: func(child ast.Expr) ast.Expr {
				return ast.FunctionExpr{
					Params: []ast.FuncParam{{Name: "x", Default: child, Span: spanAt(1816, 1)}},
					Body:   []ast.FuncBodyStmt{exprStmt(ident("x"))},
					Span:   spanAt(1816, 1),
				}
			},
		},
		{
			name: "call argument",
			build: func(child ast.Expr) ast.Expr {
				return callExpr(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x"))), posArg(child))
			},
		},
		{
			name: "index base",
			build: func(child ast.Expr) ast.Expr {
				return ast.IndexExpr{Base: child, Items: []ast.Expr{intExpr(0)}, Span: spanAt(1818, 1)}
			},
		},
		{
			name:  "unary operand",
			build: func(child ast.Expr) ast.Expr { return ast.UnaryExpr{Op: "-", Expr: child, Span: spanAt(1819, 1)} },
		},
		{
			name:    "relaxed table binary",
			context: EvalCtxBindingAssign,
			build: func(child ast.Expr) ast.Expr {
				return ast.BinaryExpr{
					Left:  ast.AliasExpr{Expr: child, Alias: "x", Span: spanAt(1820, 1)},
					Op:    "+",
					Right: ast.AliasExpr{Expr: intExpr(1), Alias: "y", Span: spanAt(1820, 8)},
					Span:  spanAt(1820, 4),
				}
			},
		},
		{
			name: "binary left",
			build: func(child ast.Expr) ast.Expr {
				return ast.BinaryExpr{Left: child, Op: "+", Right: intExpr(1), Span: spanAt(1821, 1)}
			},
		},
		{
			name: "binary right",
			build: func(child ast.Expr) ast.Expr {
				return ast.BinaryExpr{Left: intExpr(1), Op: "+", Right: child, Span: spanAt(1822, 1)}
			},
		},
		{
			name: "compare left",
			build: func(child ast.Expr) ast.Expr {
				return ast.CompareExpr{Left: child, Op: "==", Right: intExpr(1), Span: spanAt(1823, 1)}
			},
		},
		{
			name: "compare right",
			build: func(child ast.Expr) ast.Expr {
				return ast.CompareExpr{Left: intExpr(1), Op: "==", Right: child, Span: spanAt(1824, 1)}
			},
		},
		{
			name: "conditional condition",
			build: func(child ast.Expr) ast.Expr {
				return ast.ConditionalExpr{Then: intExpr(1), Cond: child, Else: intExpr(2), Span: spanAt(1825, 1)}
			},
		},
		{
			name: "conditional then",
			build: func(child ast.Expr) ast.Expr {
				return ast.ConditionalExpr{Then: child, Cond: boolExpr(true), Else: intExpr(2), Span: spanAt(1826, 1)}
			},
		},
		{
			name: "conditional else",
			build: func(child ast.Expr) ast.Expr {
				return ast.ConditionalExpr{Then: intExpr(1), Cond: boolExpr(false), Else: child, Span: spanAt(1827, 1)}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			child, env, opts, ctx := recursiveLimitOperand(t)
			opts.Context = tc.context
			diags := &diag.Diagnostics{}
			got := evalExprWithCtx(tc.build(child), env, diags, opts, ctx)
			requireRecursionAbort(t, got, diags, ctx)
		})
	}
}

func TestExecuteFunctionBodyRecursionLimitExits(t *testing.T) {
	tests := []struct {
		name  string
		build func(ast.Expr) []ast.FuncBodyStmt
	}{
		{
			name: "local assignment expression",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.LocalAssignStmt{Name: "x", Op: ast.AssignEq, Expr: child}}
			},
		},
		{
			name:  "return expression",
			build: func(child ast.Expr) []ast.FuncBodyStmt { return []ast.FuncBodyStmt{ast.ReturnStmt{Expr: child}} },
		},
		{
			name: "if condition",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncIfStmt{Cond: child, Then: []ast.FuncBodyStmt{exprStmt(intExpr(1))}}}
			},
		},
		{
			name: "if then body",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncIfStmt{Cond: boolExpr(true), Then: []ast.FuncBodyStmt{exprStmt(child)}}}
			},
		},
		{
			name: "elif condition",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncIfStmt{
					Cond:  boolExpr(false),
					Elifs: []ast.FuncElifBranch{{Cond: child, Body: []ast.FuncBodyStmt{exprStmt(intExpr(2))}}},
				}}
			},
		},
		{
			name: "elif body",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncIfStmt{
					Cond:  boolExpr(false),
					Elifs: []ast.FuncElifBranch{{Cond: boolExpr(true), Body: []ast.FuncBodyStmt{exprStmt(child)}}},
				}}
			},
		},
		{
			name: "else body",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncIfStmt{Cond: boolExpr(false), Else: []ast.FuncBodyStmt{exprStmt(child)}}}
			},
		},
		{
			name: "for body",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncForStmt{
					Target:   "x",
					Iterable: listExpr(intExpr(1)),
					Body:     []ast.FuncBodyStmt{exprStmt(child)},
				}}
			},
		},
		{
			name: "while body",
			build: func(child ast.Expr) []ast.FuncBodyStmt {
				return []ast.FuncBodyStmt{ast.FuncWhileStmt{
					Cond: boolExpr(true),
					Body: []ast.FuncBodyStmt{exprStmt(child)},
				}}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			child, env, opts, ctx := recursiveLimitOperand(t)
			diags := &diag.Diagnostics{}
			result := executeFunctionBody(tc.build(child), env, diags, opts, ctx)
			if result.Value.Kind != KindNull {
				t.Fatalf("expected null result after recursion abort, got %#v", result)
			}
			if !ctx.recursionLimitHit() {
				t.Fatalf("expected recursion limit flag")
			}
			if diagCount(diags, "E106") != 1 {
				t.Fatalf("expected one recursion-depth diagnostic, got: %s", diags.String())
			}
		})
	}

	t.Run("pre-marked context exits before first statement", func(t *testing.T) {
		ctx := newEvalCtx(NewRootFrame(nil))
		ctx.markRecursionLimitHit()
		diags := &diag.Diagnostics{}
		result := executeFunctionBody([]ast.FuncBodyStmt{exprStmt(intExpr(1))}, nil, diags, ExprOptions{}, ctx)
		if result.Value.Kind != KindNull {
			t.Fatalf("expected null value from early recursion abort, got %#v", result)
		}
		if len(diags.Items) != 0 {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestAssignBinaryOpAndCompoundAssignmentBranches(t *testing.T) {
	opTests := []struct {
		op   ast.AssignOp
		want string
	}{
		{ast.AssignPlusEq, "+"},
		{ast.AssignMinusEq, "-"},
		{ast.AssignStarEq, "*"},
		{ast.AssignSlashEq, "/"},
		{ast.AssignPctEq, "%"},
		{ast.AssignEq, ""},
		{ast.AssignOp("@="), ""},
	}
	for _, tc := range opTests {
		if got := assignBinaryOp(tc.op); got != tc.want {
			t.Fatalf("assignBinaryOp(%q) = %q, want %q", tc.op, got, tc.want)
		}
	}

	tests := []struct {
		name     string
		stmt     ast.LocalAssignStmt
		initial  Value
		want     Value
		wantCode string
	}{
		{name: "minus", stmt: ast.LocalAssignStmt{Name: "x", Op: ast.AssignMinusEq, Expr: intExpr(3)}, initial: Int(10), want: Int(7)},
		{name: "multiply", stmt: ast.LocalAssignStmt{Name: "x", Op: ast.AssignStarEq, Expr: intExpr(3)}, initial: Int(4), want: Int(12)},
		{name: "divide", stmt: ast.LocalAssignStmt{Name: "x", Op: ast.AssignSlashEq, Expr: intExpr(4)}, initial: Int(10), want: Float(2.5)},
		{name: "modulo", stmt: ast.LocalAssignStmt{Name: "x", Op: ast.AssignPctEq, Expr: intExpr(4)}, initial: Int(10), want: Int(2)},
		{name: "unsupported op assigns null", stmt: ast.LocalAssignStmt{Name: "x", Op: ast.AssignOp("@="), Expr: intExpr(1)}, initial: Int(10), want: Null(), wantCode: "E109"},
		{name: "type error assigns null", stmt: ast.LocalAssignStmt{Name: "x", Op: ast.AssignMinusEq, Expr: intExpr(1)}, initial: String("a"), want: Null(), wantCode: "E105"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame := NewRootFrame(map[string]Value{"x": tc.initial})
			ctx := newEvalCtx(frame)
			diags := &diag.Diagnostics{}
			executeLocalAssign(tc.stmt, nil, diags, ExprOptions{}, ctx)
			got, ok := frame.Read("x", diag.Span{}, &diag.Diagnostics{})
			if !ok {
				t.Fatalf("expected x to remain readable")
			}
			if !Equal(got, tc.want) {
				t.Fatalf("compound assignment result got=%#v want=%#v diagnostics=%s", got, tc.want, diags.String())
			}
			if tc.wantCode == "" {
				if diags.HasErrors() {
					t.Fatalf("unexpected diagnostics: %s", diags.String())
				}
				return
			}
			if diagCount(diags, tc.wantCode) != 1 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
		})
	}
}

func TestEvalNamesValueCallVariants(t *testing.T) {
	span := spanAt(1830, 1)
	opts := ExprOptions{Names: NewNameCatalog([]string{"z", "a"}, nil)}
	table := CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: Int(2)}}}},
	})
	dict := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "left"}, Value: Int(1)},
		{Key: DictKey{Kind: DictKeyInt, I: 2}, Value: String("two")},
	})
	fn := Function(&FunctionValue{BuiltinName: "sum"})

	tests := []struct {
		name     string
		args     []CallValueArg
		opts     ExprOptions
		want     Value
		wantCode string
	}{
		{name: "missing metadata", opts: ExprOptions{}, wantCode: "E106"},
		{name: "zero args", opts: opts, want: List([]Value{String("a"), String("z")})},
		{name: "table", opts: opts, args: []CallValueArg{{Value: table, Span: span}}, want: List([]Value{String("x"), String("y")})},
		{name: "dict", opts: opts, args: []CallValueArg{{Value: dict, Span: span}}, want: List([]Value{String("left"), Int(2)})},
		{name: "named values", opts: opts, args: []CallValueArg{{Name: "values", Value: List([]Value{table}), Span: span}}, want: List([]Value{String("x"), String("y")})},
		{name: "too many args", opts: opts, args: []CallValueArg{{Value: table, Span: span}, {Value: dict, Span: span}}, wantCode: "E106"},
		{name: "list rejected", opts: opts, args: []CallValueArg{{Value: List([]Value{Int(1)}), Span: span}}, wantCode: "E106"},
		{name: "function rejected", opts: opts, args: []CallValueArg{{Value: fn, Span: span}}, wantCode: "E106"},
		{name: "null rejected", opts: opts, args: []CallValueArg{{Value: Null(), Span: span}}, wantCode: "E106"},
		{name: "bad named values expansion", opts: opts, args: []CallValueArg{{Name: "values", Value: Int(1), Span: span}}, wantCode: "E106"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalNamesValueCall(tc.args, span, diags, tc.opts)
			if tc.wantCode != "" {
				if got.Kind != KindNull {
					t.Fatalf("expected null, got %#v", got)
				}
				if diagCount(diags, tc.wantCode) == 0 {
					t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("names value-call got=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestEvalDeleteValueCallVariants(t *testing.T) {
	span := spanAt(1840, 1)

	t.Run("positional strings delete locals", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{"x": Int(1), "y": Int(2)})
		diags := &diag.Diagnostics{}
		got := evalDeleteValueCall([]CallValueArg{
			{Value: String("x"), Span: span},
			{Value: String("y"), Span: span},
		}, span, diags, ExprOptions{Frame: frame}, newEvalCtx(frame))
		if got.Kind != KindNull {
			t.Fatalf("delete should return null, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if frame.HasLocal("x") || frame.HasLocal("y") {
			t.Fatalf("delete value-call did not remove locals")
		}
	})

	t.Run("named values expansion deletes locals", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{"x": Int(1), "y": Int(2)})
		diags := &diag.Diagnostics{}
		_ = evalDeleteValueCall([]CallValueArg{
			{Name: "names", Value: Tuple([]Value{String("x"), String("y")}), Span: span},
		}, span, diags, ExprOptions{Frame: frame}, newEvalCtx(frame))
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if frame.HasLocal("x") || frame.HasLocal("y") {
			t.Fatalf("delete(names=...) did not remove locals")
		}
	})

	tests := []struct {
		name     string
		args     []CallValueArg
		wantCode string
		wantText string
	}{
		{
			name:     "bad named values expansion",
			args:     []CallValueArg{{Name: "names", Value: Int(1), Span: span}},
			wantCode: "E106",
			wantText: "call expansion expects a list or tuple",
		},
		{
			name:     "no names",
			args:     []CallValueArg{{Name: "names", Value: List(nil), Span: span}},
			wantCode: "E106",
			wantText: "expects at least one variable",
		},
		{
			name:     "non-string and empty targets",
			args:     []CallValueArg{{Value: Int(1), Span: span}, {Value: String(""), Span: span}},
			wantCode: "E106",
			wantText: "targets must be strings",
		},
		{
			name:     "duplicate target",
			args:     []CallValueArg{{Value: String("x"), Span: span}, {Value: String("x"), Span: span}},
			wantCode: "E106",
			wantText: "listed more than once",
		},
		{
			name:     "unknown local",
			args:     []CallValueArg{{Value: String("missing"), Span: span}},
			wantCode: "E100",
			wantText: "unknown local variable",
		},
		{
			name:     "builtin constant",
			args:     []CallValueArg{{Value: String("None"), Span: span}},
			wantCode: "E106",
			wantText: "cannot delete built-in value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame := NewRootFrame(nil)
			diags := &diag.Diagnostics{}
			got := evalDeleteValueCall(tc.args, span, diags, ExprOptions{Frame: frame}, newEvalCtx(frame))
			if got.Kind != KindNull {
				t.Fatalf("delete should return null, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
			if tc.wantText != "" && !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected diagnostic containing %q, got: %s", tc.wantText, diags.String())
			}
		})
	}
}
