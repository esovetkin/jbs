package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestGlobalExprDependenciesAndCollector(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))

	if got := globalExprDependencies(nil, "self"); got != nil {
		t.Fatalf("expected nil dependencies for nil expression, got %#v", got)
	}

	expr := ast.BinaryExpr{
		Left: ast.IdentExpr{Name: "self", Span: span},
		Op:   "+",
		Right: ast.TupleExpr{
			Items: []ast.Expr{
				ast.CallExpr{
					Callee: ast.IdentExpr{Name: "callee_ignored", Span: span},
					Args: []ast.Expr{
						ast.ListExpr{
							Items: []ast.Expr{
								ast.IdentExpr{Name: "b", Span: span},
								ast.QualifiedIdentExpr{Namespace: "ns", Name: "q", Span: span},
							},
							Span: span,
						},
					},
					Span: span,
				},
				ast.UnaryExpr{
					Op: "-",
					Expr: ast.CompareExpr{
						Left: ast.ConvertExpr{
							Target: "str",
							Expr: ast.MemberExpr{
								Base: ast.IndexExpr{
									Base:  ast.IdentExpr{Name: "g", Span: span},
									Items: []ast.Expr{ast.IdentExpr{Name: "selector_ignored", Span: span}},
									Span:  span,
								},
								Name: "member_ignored",
								Span: span,
							},
							Span: span,
						},
						Op: "==",
						Right: ast.ConditionalExpr{
							Then: ast.IdentExpr{Name: "d", Span: span},
							Cond: ast.BoolExpr{Value: true, Span: span},
							Else: ast.ModeExpr{
								Mode: "python",
								Expr: ast.AliasExpr{
									Expr: ast.IndexExpr{
										Base: ast.IdentExpr{Name: "f", Span: span},
										Items: []ast.Expr{
											ast.IdentExpr{Name: "index_ignored", Span: span},
										},
										Span: span,
									},
									Alias: "alias",
									Span:  span,
								},
								Span: span,
							},
							Span: span,
						},
						Span: span,
					},
					Span: span,
				},
			},
			Span: span,
		},
		Span: span,
	}

	out := map[string]struct{}{}
	collectGlobalExprDeps(expr, out)
	gotCollected := []string{"b", "d", "f", "g", "ns", "self"}
	for _, name := range gotCollected {
		if _, ok := out[name]; !ok {
			t.Fatalf("expected collected dependency %q, got %#v", name, out)
		}
	}
	if _, ok := out["callee_ignored"]; ok {
		t.Fatalf("did not expect call callee to be collected, got %#v", out)
	}
	if _, ok := out["index_ignored"]; ok {
		t.Fatalf("did not expect index item to be collected, got %#v", out)
	}
	if _, ok := out["selector_ignored"]; ok {
		t.Fatalf("did not expect member selector to be collected, got %#v", out)
	}
	if _, ok := out["member_ignored"]; ok {
		t.Fatalf("did not expect member name to be collected, got %#v", out)
	}

	gotDeps := globalExprDependencies(expr, "self")
	wantDeps := []string{"b", "d", "f", "g", "ns"}
	if !reflect.DeepEqual(gotDeps, wantDeps) {
		t.Fatalf("unexpected global dependencies: got=%#v want=%#v", gotDeps, wantDeps)
	}
}

func TestCompileUserGlobalsSkipsBuiltinsAllowsReassignAndTracksDeps(t *testing.T) {
	span := diag.NewSpan("globals.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	prog := ast.Program{
		File: "globals.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "builtin",
				Op:   ast.AssignEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 9, Raw: "9", Span: span},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "x",
				Op:   ast.AssignEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "x",
				Op:   ast.AssignPlusEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 2, Raw: "2", Span: span},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "y",
				Op:   ast.AssignEq,
				Expr: ast.BinaryExpr{
					Left:  ast.IdentExpr{Name: "x", Span: span},
					Op:    "+",
					Right: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
					Span:  span,
				},
				Span: span,
			},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	out, order := compileUserGlobals(prog, map[string]eval.Value{"builtin": eval.Int(7)}, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if _, ok := out["builtin"]; ok {
		t.Fatalf("expected builtin assignment to be skipped, got %#v", out["builtin"])
	}
	if !reflect.DeepEqual(order, []string{"x", "y"}) {
		t.Fatalf("unexpected global order: %#v", order)
	}
	if !eval.Equal(out["x"].Value, eval.Int(3)) {
		t.Fatalf("expected reassigned x value 3, got %#v", out["x"].Value)
	}
	if out["x"].Mode != "" {
		t.Fatalf("expected empty mode for x, got %q", out["x"].Mode)
	}
	if out["x"].DependsOn != nil {
		t.Fatalf("expected self reference to be dropped from x dependencies, got %#v", out["x"].DependsOn)
	}
	if !reflect.DeepEqual(out["y"].DependsOn, []string{"x"}) {
		t.Fatalf("expected y to depend on x, got %#v", out["y"].DependsOn)
	}
}

func TestExecGlobalPlanCollectsTopLevelExprResults(t *testing.T) {
	span := diag.NewSpan("exprs.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	prog := ast.Program{
		File: "exprs.jbs",
		Stmts: []ast.Stmt{
			ast.GlobalAssign{
				Name: "x",
				Op:   ast.AssignEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
				Span: span,
			},
			ast.ExprStmt{
				Expr: ast.IdentExpr{Name: "x", Span: span},
				Span: span,
			},
			ast.GlobalAssign{
				Name: "x",
				Op:   ast.AssignPlusEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
				Span: span,
			},
			ast.ExprStmt{
				Expr: ast.IdentExpr{Name: "x", Span: span},
				Span: span,
			},
		},
	}

	diags := &diag.Diagnostics{}
	exec := execGlobalPlan(buildGlobalPlan(prog), nil, nil, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	gotGlobals, order := globalVarsFromExec(exec)
	if !reflect.DeepEqual(order, []string{"x"}) {
		t.Fatalf("unexpected global order: %#v", order)
	}
	if len(gotGlobals) != 1 {
		t.Fatalf("expected only x to become a global, got %#v", gotGlobals)
	}
	if gotGlobals["x"] == nil || !eval.Equal(gotGlobals["x"].Value, eval.Int(2)) {
		t.Fatalf("expected x=2 after reassignment, got %#v", gotGlobals["x"])
	}
	if len(exec.TopLevelExprs) != 2 {
		t.Fatalf("expected two top-level expr results, got %#v", exec.TopLevelExprs)
	}
	if exec.TopLevelExprs[0].Index != 1 || exec.TopLevelExprs[1].Index != 3 {
		t.Fatalf("unexpected expr result indices: %#v", exec.TopLevelExprs)
	}
	if !eval.Equal(exec.TopLevelExprs[0].Value, eval.Int(1)) || !eval.Equal(exec.TopLevelExprs[1].Value, eval.Int(2)) {
		t.Fatalf("unexpected expr result values: %#v", exec.TopLevelExprs)
	}
}

func TestGlobalVarSeriesBindingFromGlobalVarAndCloneCombRows(t *testing.T) {
	span := diag.NewSpan("globals.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))

	scalarOrder, scalarVars := globalVarSeries("x", eval.Int(7))
	if !reflect.DeepEqual(scalarOrder, []string{"x"}) {
		t.Fatalf("unexpected scalar order: %#v", scalarOrder)
	}
	if !reflect.DeepEqual(scalarVars, map[string][]eval.Value{"x": {eval.Int(7)}}) {
		t.Fatalf("unexpected scalar vars: %#v", scalarVars)
	}

	combRows := []eval.Row{
		{
			Values: map[string]eval.Cell{
				"a": {Value: eval.Int(1)},
				"b": {Value: eval.String("x"), Origin: span},
			},
		},
		{
			Values: map[string]eval.Cell{
				"a": {Value: eval.Int(2)},
				"b": {Value: eval.String("y")},
			},
		},
	}
	combValue := eval.CombValue(&eval.Comb{
		Order: []string{"a", "b"},
		Rows:  combRows,
	})
	combOrder, combVars := globalVarSeries("grid", combValue)
	if !reflect.DeepEqual(combOrder, []string{"a", "b"}) {
		t.Fatalf("unexpected comb order: %#v", combOrder)
	}
	if !reflect.DeepEqual(combVars["a"], []eval.Value{eval.Int(1), eval.Int(2)}) {
		t.Fatalf("unexpected comb column a: %#v", combVars["a"])
	}
	if !reflect.DeepEqual(combVars["b"], []eval.Value{eval.String("x"), eval.String("y")}) {
		t.Fatalf("unexpected comb column b: %#v", combVars["b"])
	}

	scalarBinding := bindingFromGlobalVar("x", &GlobalVar{
		Name:      "x",
		Value:     eval.String("shell-value"),
		Mode:      "shell",
		Span:      span,
		Order:     []string{"x"},
		Vars:      map[string][]eval.Value{"x": {eval.String("shell-value")}},
		DependsOn: []string{"dep"},
	})
	if scalarBinding.Shape != BindingScalar || !scalarBinding.SyntheticGlobal {
		t.Fatalf("unexpected scalar binding metadata: %#v", scalarBinding)
	}
	if scalarBinding.Modes["x"] != "shell" {
		t.Fatalf("expected single-column mode to be preserved, got %#v", scalarBinding.Modes)
	}
	if len(scalarBinding.Rows) != 1 || !eval.Equal(scalarBinding.Rows[0].Values["x"].Value, eval.String("shell-value")) {
		t.Fatalf("unexpected scalar binding rows: %#v", scalarBinding.Rows)
	}
	if !reflect.DeepEqual(scalarBinding.DependsOn, []string{"dep"}) {
		t.Fatalf("unexpected scalar binding dependencies: %#v", scalarBinding.DependsOn)
	}

	tableBinding := bindingFromGlobalVar("grid", &GlobalVar{
		Name:  "grid",
		Value: combValue,
		Span:  span,
		Order: combOrder,
		Vars:  combVars,
	})
	if tableBinding.Shape != BindingTable || len(tableBinding.Rows) != 2 {
		t.Fatalf("unexpected table binding metadata: %#v", tableBinding)
	}
	if tableBinding.Rows[0].Values["a"].Origin != span || tableBinding.Rows[1].Values["b"].Origin != span {
		t.Fatalf("expected zero origins to be filled with fallback span, got %#v", tableBinding.Rows)
	}

	clonedRows := cloneCombRows(combRows, span)
	if clonedRows[0].Values["a"].Origin != span || clonedRows[1].Values["b"].Origin != span {
		t.Fatalf("expected fallback origin fill in cloned rows, got %#v", clonedRows)
	}
	clonedRows[0].Values["a"] = eval.Cell{Value: eval.Int(99), Origin: span}
	if combRows[0].Values["a"].Value.I != 1 {
		t.Fatalf("expected cloneCombRows to deep-copy row maps, got original %#v", combRows[0].Values["a"])
	}

	importedScalar := globalVarFromImportedBinding("renamed", scalarBinding, span)
	if importedScalar == nil || importedScalar.Name != "renamed" || importedScalar.Mode != "shell" {
		t.Fatalf("unexpected imported scalar global var: %#v", importedScalar)
	}
	if !reflect.DeepEqual(importedScalar.Order, []string{"renamed"}) {
		t.Fatalf("expected renamed scalar order, got %#v", importedScalar.Order)
	}
	if !reflect.DeepEqual(importedScalar.Vars, map[string][]eval.Value{"renamed": {eval.String("shell-value")}}) {
		t.Fatalf("unexpected imported scalar vars: %#v", importedScalar.Vars)
	}

	importedTable := globalVarFromImportedBinding("grid_copy", tableBinding, span)
	if importedTable == nil || importedTable.Name != "grid_copy" {
		t.Fatalf("unexpected imported table global var: %#v", importedTable)
	}
	if !reflect.DeepEqual(importedTable.Order, []string{"a", "b"}) {
		t.Fatalf("expected imported table order to preserve comb columns, got %#v", importedTable.Order)
	}
	if !reflect.DeepEqual(importedTable.Vars["a"], []eval.Value{eval.Int(1), eval.Int(2)}) || !reflect.DeepEqual(importedTable.Vars["b"], []eval.Value{eval.String("x"), eval.String("y")}) {
		t.Fatalf("unexpected imported table vars: %#v", importedTable.Vars)
	}
}

func TestGlobalExprReadNames(t *testing.T) {
	span := diag.NewSpan("reads.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	expr := ast.BinaryExpr{
		Left: ast.QualifiedIdentExpr{Namespace: "lib", Name: "value", Span: span},
		Op:   "+",
		Right: ast.IndexExpr{
			Base: ast.MemberExpr{
				Base: ast.QualifiedIdentExpr{Namespace: "jobs", Name: "x", Span: span},
				Name: "member_ignored",
				Span: span,
			},
			Items: []ast.Expr{
				ast.IdentExpr{Name: "ignored", Span: span},
			},
			Span: span,
		},
		Span: span,
	}
	got := globalExprReadNames(expr)
	want := []string{"lib", "lib.value", "jobs", "jobs.x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected read names: got=%#v want=%#v", got, want)
	}
}

func TestCompileUserGlobalsPlannerSemantics(t *testing.T) {
	span := diag.NewSpan("planner.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))

	t.Run("unique_forward_initializer", func(t *testing.T) {
		prog := ast.Program{
			File: "planner.jbs",
			Stmts: []ast.Stmt{
				ast.GlobalAssign{
					Name: "x",
					Expr: ast.BinaryExpr{
						Left:  ast.IdentExpr{Name: "y", Span: span},
						Op:    "+",
						Right: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
						Span:  span,
					},
					Span: span,
				},
				ast.GlobalAssign{
					Name: "y",
					Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
					Span: span,
				},
			},
			Span: span,
		}
		diags := &diag.Diagnostics{}
		out, order := compileUserGlobals(prog, nil, diags)
		if len(diags.Items) != 0 {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if !reflect.DeepEqual(order, []string{"x", "y"}) {
			t.Fatalf("unexpected global order: %#v", order)
		}
		if !eval.Equal(out["x"].Value, eval.Int(2)) || !eval.Equal(out["y"].Value, eval.Int(1)) {
			t.Fatalf("unexpected planned values: x=%#v y=%#v", out["x"], out["y"])
		}
	})

	t.Run("ambiguous_forward_read", func(t *testing.T) {
		prog := ast.Program{
			File: "planner.jbs",
			Stmts: []ast.Stmt{
				ast.GlobalAssign{
					Name: "x",
					Expr: ast.BinaryExpr{
						Left:  ast.IdentExpr{Name: "y", Span: span},
						Op:    "+",
						Right: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
						Span:  span,
					},
					Span: span,
				},
				ast.GlobalAssign{
					Name: "y",
					Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
					Span: span,
				},
				ast.GlobalAssign{
					Name: "y",
					Expr: ast.NumberExpr{Int: true, IntValue: 2, Raw: "2", Span: span},
					Span: span,
				},
			},
			Span: span,
		}
		diags := &diag.Diagnostics{}
		out, order := compileUserGlobals(prog, nil, diags)
		if countDiagCode(diags, "E100") == 0 {
			t.Fatalf("expected unresolved forward read diagnostics, got: %s", diags.String())
		}
		if !reflect.DeepEqual(order, []string{"x", "y"}) {
			t.Fatalf("unexpected global order: %#v", order)
		}
		if !eval.Equal(out["y"].Value, eval.Int(2)) {
			t.Fatalf("expected last y assignment to win, got %#v", out["y"])
		}
	})

	t.Run("first_compound_write_stays_invalid", func(t *testing.T) {
		prog := ast.Program{
			File: "planner.jbs",
			Stmts: []ast.Stmt{
				ast.GlobalAssign{
					Name: "x",
					Op:   ast.AssignPlusEq,
					Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
					Span: span,
				},
				ast.GlobalAssign{
					Name: "x",
					Expr: ast.NumberExpr{Int: true, IntValue: 2, Raw: "2", Span: span},
					Span: span,
				},
			},
			Span: span,
		}
		diags := &diag.Diagnostics{}
		out, _ := compileUserGlobals(prog, nil, diags)
		if countDiagCode(diags, "E100") == 0 {
			t.Fatalf("expected invalid first compound write to stay unresolved, got: %s", diags.String())
		}
		if !eval.Equal(out["x"].Value, eval.Int(2)) {
			t.Fatalf("expected final direct assignment to win, got %#v", out["x"])
		}
	})
}
