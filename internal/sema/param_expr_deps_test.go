package sema

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestCollectExprLocalIdentDeps_AllCases(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))

	deps := map[string]struct{}{}
	collectExprLocalIdentDeps(nil, deps)
	if len(deps) != 0 {
		t.Fatalf("expected no deps for nil expr, got %#v", deps)
	}

	expr := ast.ConditionalExpr{
		Then: ast.BinaryExpr{
			Left: ast.UnaryExpr{
				Op: "-",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "list", Span: sp},
					Args: []ast.Expr{
						ast.ModeExpr{
							Mode: "python",
							Expr: ast.ListExpr{
								Items: []ast.Expr{
									ast.IdentExpr{Name: "a", Span: sp},
									ast.QualifiedIdentExpr{Namespace: "ns", Name: "skip", Span: sp},
									ast.TupleExpr{
										Items: []ast.Expr{
											ast.IdentExpr{Name: "b", Span: sp},
											ast.CompareExpr{
												Left:  ast.IdentExpr{Name: "c", Span: sp},
												Op:    "==",
												Right: ast.IdentExpr{Name: "d", Span: sp},
												Span:  sp,
											},
										},
										Span: sp,
									},
								},
								Span: sp,
							},
							Span: sp,
						},
					},
					Span: sp,
				},
				Span: sp,
			},
			Op:    "+",
			Right: ast.IdentExpr{Name: "a", Span: sp},
			Span:  sp,
		},
		Cond: ast.CompareExpr{
			Left:  ast.IdentExpr{Name: "e", Span: sp},
			Op:    "!=",
			Right: ast.NumberExpr{Int: true, IntValue: 0, Raw: "0", Span: sp},
			Span:  sp,
		},
		Else: ast.IdentExpr{Name: "f", Span: sp},
		Span: sp,
	}

	collectExprLocalIdentDeps(expr, deps)

	want := []string{"a", "b", "c", "d", "e", "f"}
	if len(deps) != len(want) {
		t.Fatalf("unexpected dep count: got=%d want=%d deps=%#v", len(deps), len(want), deps)
	}
	for _, name := range want {
		if _, ok := deps[name]; !ok {
			t.Fatalf("missing dependency %q in %#v", name, deps)
		}
	}
	if _, ok := deps["ns.skip"]; ok {
		t.Fatalf("qualified identifier should not be collected: %#v", deps)
	}
}

func TestWarnUnusedParamLocals_EarlyReturnAndReachability(t *testing.T) {
	sp := func(off int) diag.Span {
		return diag.NewSpan("in.jbs", diag.NewPos(off, 1, off+1), diag.NewPos(off+1, 1, off+2))
	}
	assigns := map[string]localAssignMeta{
		"a": {Expr: ast.IdentExpr{Name: "b", Span: sp(1)}, Span: sp(1)},
		"b": {Expr: ast.IdentExpr{Name: "c", Span: sp(2)}, Span: sp(2)},
		"c": {Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: sp(3)}, Span: sp(3)},
		"x": {Expr: ast.StringExpr{Value: "x", Span: sp(4)}, Span: sp(4)},
		"y": {Expr: ast.IdentExpr{Name: "x", Span: sp(5)}, Span: sp(5)},
	}
	order := []string{"ghost", "a", "b", "c", "x", "y"}

	diags := &diag.Diagnostics{}
	warnUnusedParamLocals(assigns, order, nil, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("expected no diagnostics when seed is empty, got: %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	warnUnusedParamLocals(nil, order, []string{"a"}, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("expected no diagnostics when assigns is empty, got: %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	warnUnusedParamLocals(assigns, order, []string{"missing", "a"}, diags)
	if countDiagCode(diags, "W312") != 2 {
		t.Fatalf("expected 2 W312 warnings for unreachable x/y, got %d: %s", countDiagCode(diags, "W312"), diags.String())
	}
	if !containsWarningForVar(diags, "x") || !containsWarningForVar(diags, "y") {
		t.Fatalf("expected W312 for x and y, got: %s", diags.String())
	}
	if containsWarningForVar(diags, "a") || containsWarningForVar(diags, "b") || containsWarningForVar(diags, "c") {
		t.Fatalf("did not expect W312 for reachable a/b/c, got: %s", diags.String())
	}
}

func containsWarningForVar(diags *diag.Diagnostics, name string) bool {
	target := "param variable '" + name + "'"
	for _, d := range diags.Items {
		if d.Code != "W312" {
			continue
		}
		if d.Severity != diag.SeverityWarning {
			continue
		}
		if strings.Contains(d.Message, target) {
			return true
		}
	}
	return false
}

func TestWarnUnusedParamContributors_ShadowedImportedStillWarns(t *testing.T) {
	sp := func(off int) diag.Span {
		return diag.NewSpan("in.jbs", diag.NewPos(off, 1, off+1), diag.NewPos(off+1, 1, off+2))
	}
	assigns := map[string]localAssignMeta{
		"x": {
			Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: sp(1)},
			Span: sp(1),
		},
	}
	imported := map[string]importedContribution{
		"x": {Source: "base", SourceVar: "x", Span: sp(2)},
	}
	diags := &diag.Diagnostics{}
	warnUnusedParamContributors(assigns, []string{"x"}, imported, []string{"x"}, []string{"x"}, diags)
	if countDiagCode(diags, "W312") != 1 {
		t.Fatalf("expected one W312, got %d: %s", countDiagCode(diags, "W312"), diags.String())
	}
	if !strings.Contains(diags.String(), "imported variable 'x' from source 'base'") {
		t.Fatalf("expected imported warning for base.x, got: %s", diags.String())
	}
}

func TestWarnUnusedParamContributors_SelfRebindMarksImportedUsed(t *testing.T) {
	sp := func(off int) diag.Span {
		return diag.NewSpan("in.jbs", diag.NewPos(off, 1, off+1), diag.NewPos(off+1, 1, off+2))
	}
	assigns := map[string]localAssignMeta{
		"x": {
			Expr: ast.BinaryExpr{
				Left:  ast.IdentExpr{Name: "x", Span: sp(1)},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: sp(1)},
				Span:  sp(1),
			},
			Span: sp(1),
		},
	}
	imported := map[string]importedContribution{
		"x": {Source: "base", SourceVar: "x", Span: sp(2)},
	}
	diags := &diag.Diagnostics{}
	warnUnusedParamContributors(assigns, []string{"x"}, imported, []string{"x"}, []string{"x"}, diags)
	if countDiagCode(diags, "W312") != 0 {
		t.Fatalf("did not expect W312 for self-rebind, got: %s", diags.String())
	}
}
