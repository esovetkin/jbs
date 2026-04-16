package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestGlobalExprDependencies(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	expr := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "comb", Span: span},
		Args: []ast.Expr{
			ast.BinaryExpr{
				Left: ast.IdentExpr{Name: "a", Span: span},
				Op:   "*",
				Right: ast.IndexExpr{
					Base: ast.IdentExpr{Name: "params", Span: span},
					Items: []ast.Expr{
						ast.IdentExpr{Name: "x", Span: span},
					},
					Span: span,
				},
				Span: span,
			},
			ast.QualifiedIdentExpr{Namespace: "ns", Name: "k", Span: span},
		},
		Span: span,
	}
	got := globalExprDependencies(expr, "")
	want := []string{"a", "ns", "params"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deps: got=%v want=%v", got, want)
	}
}

func TestGlobalExprDependenciesDropsSelfReferenceFromAssignOpRewrite(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	rhs := ast.IdentExpr{Name: "b", Span: span}
	effective := assignmentExpr("a", ast.AssignPlusEq, rhs, span)
	got := globalExprDependencies(effective, "a")
	want := []string{"b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deps after self-drop: got=%v want=%v", got, want)
	}
}

func TestCompileUserGlobalsPopulatesDependsOn(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "x",
				Expr: ast.TupleExpr{
					Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
						ast.NumberExpr{Int: true, IntValue: 2, Raw: "2", Span: span},
					},
					Span: span,
				},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "a",
				Expr: ast.TupleExpr{
					Items: []ast.Expr{
						ast.StringExpr{Value: "a", Span: span},
						ast.StringExpr{Value: "b", Span: span},
					},
					Span: span,
				},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "params",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "comb", Span: span},
					Args: []ast.Expr{
						ast.BinaryExpr{
							Left:  ast.IdentExpr{Name: "a", Span: span},
							Op:    "*",
							Right: ast.IdentExpr{Name: "x", Span: span},
							Span:  span,
						},
					},
					Span: span,
				},
				Span: span,
			},
		},
	}

	diags := &diag.Diagnostics{}
	globals, _ := compileUserGlobals(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	params := globals["params"]
	if params == nil {
		t.Fatalf("expected params global to be compiled")
	}
	want := []string{"a", "x"}
	if !reflect.DeepEqual(params.DependsOn, want) {
		t.Fatalf("unexpected params deps: got=%v want=%v", params.DependsOn, want)
	}
}

func TestCollectGlobalExprDepsNilAndNodeCoverage(t *testing.T) {
	out := map[string]struct{}{}
	collectGlobalExprDeps(nil, out)
	if len(out) != 0 {
		t.Fatalf("expected nil expression to add no deps, got %#v", out)
	}

	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	expr := ast.ConditionalExpr{
		Then: ast.AliasExpr{
			Expr: ast.QualifiedIdentExpr{Namespace: "ns", Name: "k", Span: span},
			Span: span,
		},
		Cond: ast.CompareExpr{
			Left: ast.UnaryExpr{
				Op:   "!",
				Expr: ast.IdentExpr{Name: "x", Span: span},
				Span: span,
			},
			Op: "==",
			Right: ast.ConvertExpr{
				Target: "list",
				Expr: ast.IndexExpr{
					Base:  ast.IdentExpr{Name: "base", Span: span},
					Items: []ast.Expr{ast.IdentExpr{Name: "ignored_item", Span: span}},
					Span:  span,
				},
				Span: span,
			},
			Span: span,
		},
		Else: ast.ModeExpr{
			Mode: "python",
			Expr: ast.TupleExpr{
				Items: []ast.Expr{
					ast.ListExpr{
						Items: []ast.Expr{
							ast.CallExpr{
								Callee: ast.IdentExpr{Name: "comb", Span: span},
								Args:   []ast.Expr{ast.IdentExpr{Name: "y", Span: span}},
								Span:   span,
							},
						},
						Span: span,
					},
				},
				Span: span,
			},
			Span: span,
		},
		Span: span,
	}
	got := globalExprDependencies(expr, "")
	want := []string{"base", "ns", "x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dependency set: got=%v want=%v", got, want)
	}
}

func TestCompileUserGlobalsSkipsBuiltinsAndAllowsReassign(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	prog := ast.Program{
		File: "in.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{Name: "jbs_name", Expr: ast.StringExpr{Value: "x", Span: span}, Span: span},
			ast.GlobalAssign{Name: "x", Expr: ast.NumberExpr{Int: true, IntValue: 1, Span: span}, Span: span},
			ast.GlobalAssign{Name: "x", Expr: ast.NumberExpr{Int: true, IntValue: 2, Span: span}, Span: span},
		},
	}
	diags := &diag.Diagnostics{}
	globals, order := compileUserGlobals(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)

	if _, ok := globals["jbs_name"]; ok {
		t.Fatalf("expected builtin global assignment to be ignored")
	}
	if len(order) != 1 || order[0] != "x" {
		t.Fatalf("expected only x in user global order, got %#v", order)
	}
	if countDiagCode(diags, "W300") != 0 {
		t.Fatalf("did not expect W300 for global reassignment, got: %s", diags.String())
	}
}

func TestAddGlobalSourcesAndGlobalVarToParamsetBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	zeroOrigin := diag.Span{}

	combRows := []eval.Row{
		{
			Values: map[string]eval.Cell{
				"c": {Value: eval.Int(1), Origin: zeroOrigin},
			},
		},
	}

	globals := map[string]*GlobalVar{
		"snew": {
			Name:  "snew",
			Value: eval.Int(1),
			Mode:  "python",
			Span:  span,
			Order: []string{"snew"},
			Vars:  map[string][]eval.Value{"snew": {eval.Int(1)}},
		},
		"pcoll": {
			Name:  "pcoll",
			Value: eval.Int(2),
			Span:  span,
			Order: []string{"pcoll"},
			Vars:  map[string][]eval.Value{"pcoll": {eval.Int(2)}},
		},
		"lexists": {
			Name:  "lexists",
			Value: eval.Int(3),
			Span:  span,
			Order: []string{"lexists"},
			Vars:  map[string][]eval.Value{"lexists": {eval.Int(3)}},
		},
		"pnonscalar": {
			Name:  "pnonscalar",
			Value: eval.List([]eval.Value{eval.Int(1), eval.Int(2)}),
			Span:  span,
			Order: []string{"pnonscalar"},
			Vars:  map[string][]eval.Value{"pnonscalar": {eval.Int(1), eval.Int(2)}},
		},
		"lcoll": {
			Name:  "lcoll",
			Value: eval.List([]eval.Value{eval.Int(1)}),
			Span:  span,
			Order: []string{"lcoll"},
			Vars:  map[string][]eval.Value{"lcoll": {eval.Int(1)}},
		},
		"combv": {
			Name:  "combv",
			Value: eval.CombValue(&eval.Comb{Order: []string{"c"}, Rows: combRows}),
			Span:  span,
			Order: []string{}, // force globalVarToParamset default order path
			Vars:  map[string][]eval.Value{"combv": {eval.Int(1)}},
		},
	}

	res := &Result{
		LetNamespaces:      []*LetNamespace{},
		LetByName:          map[string]*LetNamespace{"lexists": {Name: "lexists"}, "lcoll": {Name: "lcoll"}},
		Paramsets:          []*Paramset{},
		ParamByName:        map[string]*Paramset{"pcoll": {Name: "pcoll"}, "pnonscalar": {Name: "pnonscalar"}},
		ImportSourceByName: map[string]*ImportSource{},
	}
	diags := &diag.Diagnostics{}
	addGlobalSources(res, globals, []string{"snew", "pcoll", "lexists", "pnonscalar", "lcoll", "combv"}, diags)

	if _, ok := res.LetByName["snew"]; !ok {
		t.Fatalf("expected scalar global snew to be lowered into let namespace")
	}
	if res.LetByName["snew"].Modes["snew"] != "python" {
		t.Fatalf("expected scalar let mode propagation for snew, got %#v", res.LetByName["snew"])
	}
	if _, ok := res.ParamByName["combv"]; !ok {
		t.Fatalf("expected non-scalar global combv to be lowered into synthetic paramset")
	}
	if countDiagCode(diags, "E210") == 0 {
		t.Fatalf("expected E210 for scalar global collision with paramset, got: %s", diags.String())
	}
	if countDiagCode(diags, "E400") == 0 {
		t.Fatalf("expected E400 for non-scalar global collision with let namespace, got: %s", diags.String())
	}

	// Ensure cloneCombRows filled zero cell origin with fallback span.
	combParam := res.ParamByName["combv"]
	if combParam == nil || len(combParam.Rows) != 1 {
		t.Fatalf("expected combv paramset rows, got %#v", combParam)
	}
	cell := combParam.Rows[0].Values["c"]
	if cell.Origin.IsZero() {
		t.Fatalf("expected comb row cell zero origin to be replaced with fallback span")
	}
}
