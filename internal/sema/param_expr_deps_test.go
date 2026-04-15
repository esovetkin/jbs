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

func TestCompareContributorID_AllBranches(t *testing.T) {
	tests := []struct {
		name string
		a    contributorID
		b    contributorID
		want int
	}{
		{
			name: "kind less",
			a:    makeLocalContributorID("x"),
			b:    makeImportedContributorID("x", "p", "x"),
			want: -1,
		},
		{
			name: "kind greater",
			a:    makeImportedContributorID("x", "p", "x"),
			b:    makeLocalContributorID("x"),
			want: 1,
		},
		{
			name: "visible less",
			a:    makeLocalContributorID("a"),
			b:    makeLocalContributorID("b"),
			want: -1,
		},
		{
			name: "visible greater",
			a:    makeLocalContributorID("b"),
			b:    makeLocalContributorID("a"),
			want: 1,
		},
		{
			name: "source less",
			a:    makeImportedContributorID("x", "a", "v"),
			b:    makeImportedContributorID("x", "b", "v"),
			want: -1,
		},
		{
			name: "source greater",
			a:    makeImportedContributorID("x", "b", "v"),
			b:    makeImportedContributorID("x", "a", "v"),
			want: 1,
		},
		{
			name: "source var less",
			a:    makeImportedContributorID("x", "p", "a"),
			b:    makeImportedContributorID("x", "p", "b"),
			want: -1,
		},
		{
			name: "source var greater",
			a:    makeImportedContributorID("x", "p", "b"),
			b:    makeImportedContributorID("x", "p", "a"),
			want: 1,
		},
		{
			name: "equal",
			a:    makeImportedContributorID("x", "p", "a"),
			b:    makeImportedContributorID("x", "p", "a"),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareContributorID(tt.a, tt.b)
			switch {
			case tt.want < 0 && got >= 0:
				t.Fatalf("expected negative compare, got %d", got)
			case tt.want > 0 && got <= 0:
				t.Fatalf("expected positive compare, got %d", got)
			case tt.want == 0 && got != 0:
				t.Fatalf("expected zero compare, got %d", got)
			}
		})
	}
}

func TestWarnUnusedParamContributors_AdditionalBranches(t *testing.T) {
	sp := func(off int) diag.Span {
		return diag.NewSpan("in.jbs", diag.NewPos(off, 1, off+1), diag.NewPos(off+1, 1, off+2))
	}

	t.Run("both empty returns early", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		warnUnusedParamContributors(nil, nil, nil, nil, []string{"x"}, diags)
		if len(diags.Items) != 0 {
			t.Fatalf("expected no diagnostics on empty inputs, got: %s", diags.String())
		}
	})

	t.Run("covers imported fallback and seed imported roots", func(t *testing.T) {
		assigns := map[string]localAssignMeta{
			// z not listed in order to exercise localByVisible map fallback from assigns iteration
			"z": {Expr: ast.StringExpr{Value: "z", Span: sp(1)}, Span: sp(1)},
			"y": {Expr: ast.StringExpr{Value: "y", Span: sp(3)}, Span: sp(3)},
		}
		imported := map[string]importedContribution{
			// "imp" absent from importedOrder to exercise importedByVisible fallback branch
			"imp": {Source: "base", SourceVar: "imp", Span: sp(2)},
		}
		diags := &diag.Diagnostics{}
		warnUnusedParamContributors(
			assigns,
			[]string{"ghost", "y"}, // y warning path; z still covers fallback insertion branch
			imported,
			[]string{"ghost_import"}, // missing import in imported map => skip branch
			[]string{"", "imp"},      // "" hits root-skip, imp hits imported-root reachability path
			diags,
		)
		if countDiagCode(diags, "W312") != 1 {
			t.Fatalf("expected exactly one W312 (for local y), got %d: %s", countDiagCode(diags, "W312"), diags.String())
		}
		if !containsWarningForVar(diags, "y") {
			t.Fatalf("expected local unused warning for y, got: %s", diags.String())
		}
		if strings.Contains(diags.String(), "imported variable 'imp' from source 'base'") {
			t.Fatalf("did not expect imported warning for seeded imported root, got: %s", diags.String())
		}
	})
}
