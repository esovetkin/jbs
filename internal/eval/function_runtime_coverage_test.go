package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestFunctionRuntimeDefaultReferenceScannerCoverage(t *testing.T) {
	names := map[string]struct{}{"x": {}, "ns": {}}

	tests := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{
			name: "qualified namespace reference",
			expr: ast.QualifiedIdentExpr{Namespace: "ns", Name: "value"},
			want: true,
		},
		{
			name: "nested function parameter shadows body reference",
			expr: fnExpr(
				[]ast.FuncParam{{Name: "x"}},
				exprStmt(ident("x")),
			),
			want: false,
		},
		{
			name: "nested function default sees outer reference",
			expr: fnExpr(
				[]ast.FuncParam{{Name: "arg", Default: ident("x")}},
				exprStmt(ident("arg")),
			),
			want: true,
		},
		{
			name: "nested local declaration shadows body reference",
			expr: fnExpr(
				nil,
				localAssign("x", intExpr(1)),
				exprStmt(ident("x")),
			),
			want: false,
		},
		{
			name: "for iterable sees outer reference",
			expr: fnExpr(nil, ast.FuncForStmt{
				Target:   "i",
				Iterable: ident("x"),
				Body:     []ast.FuncBodyStmt{exprStmt(ident("i"))},
			}),
			want: true,
		},
		{
			name: "for target shadows body reference",
			expr: fnExpr(nil, ast.FuncForStmt{
				Target:   "x",
				Iterable: ast.ListExpr{Items: []ast.Expr{intExpr(1)}},
				Body:     []ast.FuncBodyStmt{exprStmt(ident("x"))},
			}),
			want: false,
		},
		{
			name: "for body sees unshadowed reference",
			expr: fnExpr(nil, ast.FuncForStmt{
				Target:   "i",
				Iterable: ast.ListExpr{Items: []ast.Expr{intExpr(1)}},
				Body:     []ast.FuncBodyStmt{exprStmt(ident("x"))},
			}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := exprReferencesAnyName(tc.expr, names); got != tc.want {
				t.Fatalf("exprReferencesAnyName() = %v, want %v", got, tc.want)
			}
		})
	}

	if exprReferencesAnyName(nil, names) {
		t.Fatal("nil expression should not reference any name")
	}
	if exprReferencesAnyName(ident("x"), nil) {
		t.Fatal("empty name set should not match")
	}
}

func TestFunctionRuntimeCatalogAndContextHelpersCoverage(t *testing.T) {
	catalog := NewNameCatalog([]string{"global"}, map[string][]string{"mod": {"a", "b"}})
	clone := cloneNameCatalog(catalog)
	if clone == catalog {
		t.Fatal("cloneNameCatalog returned original catalog")
	}
	if clone.Namespaces["mod"].Members[0] != "a" {
		t.Fatalf("unexpected cloned namespace: %#v", clone.Namespaces["mod"])
	}
	catalog.Namespaces["mod"].Members[0] = "changed"
	if clone.Namespaces["mod"].Members[0] != "a" {
		t.Fatalf("namespace members were not cloned: %#v", clone.Namespaces["mod"])
	}

	if got := callNameCatalog(nil, nil); got != nil {
		t.Fatalf("callNameCatalog(nil, nil) = %#v, want nil", got)
	}

	frame := NewRootFrame(map[string]Value{"local": Int(1)})
	callCatalog := callNameCatalog(catalog, frame)
	if callCatalog == nil || !stringSliceContains(callCatalog.Visible, "local") || !stringSliceContains(callCatalog.Visible, "global") {
		t.Fatalf("call catalog missing visible names: %#v", callCatalog)
	}
	catalog.Namespaces["mod"].Members[0] = "mutated"
	if stringSliceContains(callCatalog.Namespaces["mod"].Members, "mutated") || !stringSliceContains(callCatalog.Namespaces["mod"].Members, "changed") {
		t.Fatalf("call catalog namespace was not cloned: %#v", callCatalog.Namespaces["mod"])
	}

	ctx := (*evalCtx)(nil).withFrame(frame)
	if ctx == nil || ctx.frame != frame || ctx.overflowWarned == nil || ctx.abort == nil {
		t.Fatalf("nil withFrame did not create a complete context: %#v", ctx)
	}
	base := &evalCtx{}
	next := base.withFrame(frame)
	if next == base || next.frame != frame || next.overflowWarned == nil || next.abort == nil {
		t.Fatalf("withFrame did not clone and initialize context: %#v", next)
	}
}

func TestFunctionRuntimeDirectHelperErrorCoverage(t *testing.T) {
	diags := &diag.Diagnostics{}
	defaultCtx := newEvalCtx(nil)
	defaultCtx.callDepth = 1
	defaults := preEvaluateFunctionDefaults(fnExpr(
		[]ast.FuncParam{{Name: "x", Default: callExpr(ident("f"))}},
		exprStmt(ident("x")),
	), map[string]Value{
		"f": Function(&FunctionValue{Body: []ast.FuncBodyStmt{exprStmt(intExpr(1))}}),
	}, diags, ExprOptions{MaxFunctionCallDepth: 1}, defaultCtx)
	if !defaultCtx.recursionLimitHit() || len(defaults) != 1 || defaults[0].Value.Kind != KindNull {
		t.Fatalf("default pre-evaluation should stop on recursion abort, got %#v diagnostics %s", defaults, diags.String())
	}

	diags = &diag.Diagnostics{}
	gotDirect := executeFunctionCallValues(&FunctionValue{
		Body: []ast.FuncBodyStmt{exprStmt(intExpr(9))},
	}, nil, nil, spanAt(1700, 1), diags, ExprOptions{}, nil)
	if !Equal(gotDirect, Int(9)) {
		t.Fatalf("direct call with nil context got %#v diagnostics %s", gotDirect, diags.String())
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	ctx := newEvalCtx(nil)
	ctx.markRecursionLimitHit()
	if checkFunctionCallDepth(ctx, ExprOptions{}, spanAt(1700, 1), diags) {
		t.Fatal("depth check should fail after abort state is marked")
	}

	diags = &diag.Diagnostics{}
	if !checkFunctionCallDepth(nil, ExprOptions{MaxFunctionCallDepth: 1}, spanAt(1700, 2), diags) {
		t.Fatalf("nil context should be accepted before the depth limit: %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	got := executeFunctionCallValues(nil, nil, nil, spanAt(1700, 3), diags, ExprOptions{}, nil)
	if got.Kind != KindNull || diagCount(diags, "E199") != 1 {
		t.Fatalf("nil callable should report E199 and return null, got %#v diagnostics %s", got, diags.String())
	}

	diags = &diag.Diagnostics{}
	ctx = newEvalCtx(nil)
	ctx.markRecursionLimitHit()
	args, ok := evalCallValueArgs([]ast.CallArg{posArg(intExpr(1))}, nil, diags, ExprOptions{}, ctx)
	if ok || args != nil {
		t.Fatalf("argument evaluation should stop after recursion abort, got %#v ok=%v", args, ok)
	}

	diags = &diag.Diagnostics{}
	missingKey := DictKey{Kind: DictKeyString, S: "missing"}
	entries, ok := callKeywordEntries(Value{Kind: KindDict, D: &Dict{
		Order:   []DictKey{missingKey, {Kind: DictKeyString, S: "present"}},
		Entries: map[DictKey]Value{{Kind: DictKeyString, S: "present"}: Int(2)},
	}}, spanAt(1700, 4), diags)
	if !ok || len(entries) != 1 || entries[0].Name != "present" {
		t.Fatalf("keyword entries should skip missing ordered keys, got %#v ok=%v", entries, ok)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}

	if astExprSpan(nil) != (diag.Span{}) {
		t.Fatalf("nil expression span should be zero")
	}
}

func TestFunctionRuntimeTopLevelControlAndBranchCoverage(t *testing.T) {
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

func TestFunctionRuntimeLoopAndAssignmentHelperCoverage(t *testing.T) {
	items := make([]Value, MaxLoopIterations+1)
	for i := range items {
		items[i] = Int(int64(i))
	}

	diags := &diag.Diagnostics{}
	aborted := newEvalCtx(NewRootFrame(nil))
	aborted.markRecursionLimitHit()
	if result := executeFunctionBody([]ast.FuncBodyStmt{exprStmt(intExpr(1))}, nil, diags, ExprOptions{}, aborted); result.Value.Kind != KindNull {
		t.Fatalf("aborted function body should return null, got %#v", result)
	}
	if result := executeFuncForStmt(ast.FuncForStmt{Iterable: ast.ListExpr{}}, nil, diags, ExprOptions{}, aborted); result.Value.Kind != KindNull {
		t.Fatalf("aborted for loop should return null, got %#v", result)
	}
	if result := executeFuncWhileStmt(ast.FuncWhileStmt{Cond: ast.BoolExpr{Value: true}}, nil, diags, ExprOptions{}, aborted); result.Value.Kind != KindNull {
		t.Fatalf("aborted while loop should return null, got %#v", result)
	}

	diags = &diag.Diagnostics{}
	ctx := newEvalCtx(NewRootFrame(map[string]Value{"values": List(items)}))
	result := executeFuncForStmt(ast.FuncForStmt{
		Target:   "x",
		Iterable: ident("values"),
		Span:     spanAt(1702, 1),
	}, nil, diags, ExprOptions{}, ctx)
	if result.Value.Kind != KindNull || diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), "loop exceeded") {
		t.Fatalf("expected max-loop diagnostic, got result %#v diagnostics %s", result, diags.String())
	}

	diags = &diag.Diagnostics{}
	assignCtx := newEvalCtx(NewRootFrame(nil))
	assignCtx.markRecursionLimitHit()
	executeLocalAssign(ast.LocalAssignStmt{Name: "x", Expr: intExpr(1)}, nil, diags, ExprOptions{}, assignCtx)
	if _, ok := assignCtx.frame.LookupCell("x"); ok {
		t.Fatal("aborted local assignment should not assign")
	}

	diags = &diag.Diagnostics{}
	executeLocalAssign(ast.LocalAssignStmt{Name: "missing", Op: ast.AssignMinusEq, Expr: intExpr(1), Span: spanAt(1702, 2)}, nil, diags, ExprOptions{}, newEvalCtx(NewRootFrame(nil)))
	if diagCount(diags, "E100") == 0 {
		t.Fatalf("compound assignment to missing local should report E100, got: %s", diags.String())
	}

	executeLocalAssign(ast.LocalAssignStmt{Name: "x", Expr: intExpr(1)}, nil, &diag.Diagnostics{}, ExprOptions{}, nil)
	executeLocalAssign(ast.LocalAssignStmt{Expr: intExpr(1)}, nil, &diag.Diagnostics{}, ExprOptions{}, newEvalCtx(NewRootFrame(nil)))

	assignCases := map[ast.AssignOp]string{
		ast.AssignMinusEq: "-",
		ast.AssignStarEq:  "*",
		ast.AssignSlashEq: "/",
		ast.AssignPctEq:   "%",
		ast.AssignOp("?"): "",
	}
	for op, want := range assignCases {
		if got := assignBinaryOp(op); got != want {
			t.Fatalf("assignBinaryOp(%v) = %q, want %q", op, got, want)
		}
	}
}

func TestFunctionRuntimeBindingCoverage(t *testing.T) {
	diags := &diag.Diagnostics{}
	fn := &FunctionValue{Params: []ast.FuncParam{{Name: ""}, {Name: "x"}}}
	binding, ok := bindFunctionArguments(fn, []CallValueArg{{Value: Int(1), Span: spanAt(1703, 1)}}, spanAt(1703, 1), diags)
	if !ok || !Equal(binding.Fixed[1], Int(1)) {
		t.Fatalf("binding with empty parameter name failed: %#v ok=%v diagnostics %s", binding, ok, diags.String())
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
