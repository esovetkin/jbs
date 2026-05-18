package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

func TestBuildModuleGlobalPlanNilInfo(t *testing.T) {
	plan := buildModuleGlobalPlan(nil, nil, nil, map[string]eval.Value{"builtin": eval.Int(1)}, &diag.Diagnostics{})
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(plan.Steps) != 0 || len(plan.StepByName) != 0 || len(plan.LocalVisibleNames) != 0 {
		t.Fatalf("expected empty plan for nil module info, got %#v", plan)
	}
}

func TestBuildModuleGlobalPlanControlFlowAndImports(t *testing.T) {
	span := diag.NewSpan("entry.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	nsRef := imports.ModuleRef{ID: "ns", Label: "ns.jbs"}
	depRef := imports.ModuleRef{ID: "dep", Label: "dep.jbs"}
	child := emptyModuleScope()
	child.LocalExportsByName["imported"] = &GlobalVar{Name: "imported", Value: eval.Int(7), Span: span}
	child.Program = ast.Program{File: depRef.Label}
	namespaceScope := emptyModuleScope()
	namespaceScope.Ref = nsRef

	thenAssign := ast.GlobalAssign{Name: "then_value", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span}
	nestedIfAssign := ast.GlobalAssign{Name: "nested_if_value", Op: ast.AssignEq, Expr: numberExpr(span, 2), Span: span}
	nestedForAssign := ast.GlobalAssign{Name: "nested_for_value", Op: ast.AssignEq, Expr: ast.IdentExpr{Name: "j", Span: span}, Span: span}
	elseAssign := ast.GlobalAssign{Name: "else_value", Op: ast.AssignEq, Expr: numberExpr(span, 3), Span: span}
	topForAssign := ast.GlobalAssign{Name: "loop_value", Op: ast.AssignEq, Expr: ast.IdentExpr{Name: "i", Span: span}, Span: span}
	finalAssign := ast.GlobalAssign{Name: "final_value", Op: ast.AssignEq, Expr: numberExpr(span, 4), Span: span}
	info := &imports.ModuleInfo{
		Ref:     imports.ModuleRef{ID: "entry", Label: "entry.jbs"},
		BaseDir: "/modules/entry",
		Program: ast.Program{File: "entry.jbs", Stmts: []ast.Stmt{
			ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "ns", Span: span}, Alias: "ns", Span: span},
			ast.UseStmt{Names: []string{"imported"}, Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "dep", Span: span}, Span: span},
			ast.IfStmt{
				Cond: ast.BoolExpr{Value: true, Span: span},
				Then: []ast.Stmt{
					thenAssign,
					ast.IfStmt{
						Cond: ast.BoolExpr{Value: false, Span: span},
						Then: []ast.Stmt{
							nestedIfAssign,
						},
						Span: span,
					},
					ast.ForStmt{
						Target:   "j",
						Iterable: ast.TupleExpr{Items: []ast.Expr{numberExpr(span, 1)}, Span: span},
						Body: []ast.Stmt{
							nestedForAssign,
						},
						Span: span,
					},
					ast.WhileStmt{
						Cond: ast.BoolExpr{Value: false, Span: span},
						Body: []ast.Stmt{
							ast.ContinueStmt{Span: span},
						},
						Span: span,
					},
				},
				Elifs: []ast.ElifBranch{
					{
						Cond: ast.BoolExpr{Value: false, Span: span},
						Body: []ast.Stmt{
							ast.ExprStmt{Expr: ast.IdentExpr{Name: "then_value", Span: span}, Span: span},
						},
						Span: span,
					},
				},
				Else: []ast.Stmt{elseAssign},
				Span: span,
			},
			ast.ForStmt{
				Target:   "i",
				Iterable: ast.TupleExpr{Items: []ast.Expr{numberExpr(span, 1), numberExpr(span, 2)}, Span: span},
				Body: []ast.Stmt{
					topForAssign,
					ast.BreakStmt{Span: span},
				},
				Span: span,
			},
			ast.WhileStmt{
				Cond: ast.BoolExpr{Value: false, Span: span},
				Body: []ast.Stmt{
					ast.ContinueStmt{Span: span},
				},
				Span: span,
			},
			ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "unused", Span: span}, Span: span},
			finalAssign,
		}},
		Uses: []imports.ResolvedUse{
			{Kind: imports.UseNamespace, Alias: "ns", Source: nsRef, Span: span, Index: 0},
			{Kind: imports.UseSelective, Names: []string{"imported"}, Source: depRef, Span: span, Index: 1},
		},
	}

	diags := &diag.Diagnostics{}
	plan := buildModuleGlobalPlan(
		info,
		map[int]*moduleScope{1: child},
		map[int]*moduleScope{0: namespaceScope},
		map[string]eval.Value{"seed": eval.Int(1)},
		diags,
	)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	gotKinds := make([]globalInputKind, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		gotKinds = append(gotKinds, step.Kind)
	}
	wantKinds := []globalInputKind{
		globalInputNamespaceImport,
		globalInputProjectedImport,
		globalInputIf,
		globalInputFor,
		globalInputWhile,
		globalInputAssign,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("plan kinds=%#v, want %#v", gotKinds, wantKinds)
	}
	if plan.Steps[0].NamespaceScope != namespaceScope {
		t.Fatalf("namespace import scope=%#v, want %#v", plan.Steps[0].NamespaceScope, namespaceScope)
	}
	if plan.Steps[1].Import == nil || plan.Steps[1].Import.SourceGlobal != child.LocalExportsByName["imported"] {
		t.Fatalf("projected import=%#v, want child export", plan.Steps[1].Import)
	}
	ifStep := plan.Steps[2]
	if len(ifStep.Then) != 4 || len(ifStep.Elifs) != 1 || len(ifStep.Else) != 1 {
		t.Fatalf("unexpected if children: then=%#v elifs=%#v else=%#v", ifStep.Then, ifStep.Elifs, ifStep.Else)
	}
	if ifStep.Then[0].Kind != globalInputAssign || ifStep.Then[0].Name != "then_value" || ifStep.Then[0].Index != ifStep.Index {
		t.Fatalf("unexpected nested assignment step: %#v", ifStep.Then[0])
	}
	if ifStep.Then[1].Kind != globalInputIf || ifStep.Then[1].Index != ifStep.Index || len(ifStep.Then[1].Then) != 1 {
		t.Fatalf("unexpected nested if step: %#v", ifStep.Then[1])
	}
	if ifStep.Then[2].Kind != globalInputFor || ifStep.Then[2].Name != "j" || ifStep.Then[2].Index != ifStep.Index || len(ifStep.Then[2].Body) != 1 {
		t.Fatalf("unexpected nested for step: %#v", ifStep.Then[2])
	}
	if ifStep.Then[3].Kind != globalInputWhile || ifStep.Then[3].Index != ifStep.Index || len(ifStep.Then[3].Body) != 1 {
		t.Fatalf("unexpected nested while step: %#v", ifStep.Then[3])
	}
	if ifStep.Elifs[0].Body[0].Kind != globalInputExpr || ifStep.Elifs[0].Body[0].Index != ifStep.Index {
		t.Fatalf("unexpected elif body: %#v", ifStep.Elifs[0].Body)
	}
	if ifStep.Else[0].Kind != globalInputAssign || ifStep.Else[0].Name != "else_value" || ifStep.Else[0].Index != ifStep.Index {
		t.Fatalf("unexpected else body: %#v", ifStep.Else)
	}
	forStep := plan.Steps[3]
	if forStep.Name != "i" || len(forStep.Body) != 2 || forStep.Body[0].Index != forStep.Index || forStep.Body[1].Kind != globalInputBreak {
		t.Fatalf("unexpected top-level for step: %#v", forStep)
	}
	whileStep := plan.Steps[4]
	if len(whileStep.Body) != 1 || whileStep.Body[0].Kind != globalInputContinue || whileStep.Body[0].Index != whileStep.Index {
		t.Fatalf("unexpected top-level while step: %#v", whileStep)
	}
	if plan.StepByName["imported"] != plan.Steps[1].ID || plan.StepByName["i"] != forStep.ID || plan.StepByName["j"] != ifStep.Then[2].ID || plan.StepByName["final_value"] != plan.Steps[5].ID {
		t.Fatalf("unexpected StepByName map: %#v", plan.StepByName)
	}
	if !reflect.DeepEqual(plan.LocalVisibleNames, []string{"then_value", "nested_if_value", "j", "nested_for_value", "else_value", "i", "loop_value", "final_value"}) {
		t.Fatalf("unexpected local visible names: %#v", plan.LocalVisibleNames)
	}
	for _, step := range plan.Steps {
		if step.Names == nil {
			t.Fatalf("expected name catalog for step %#v", step)
		}
	}
}

func TestAppendModuleGlobalPlanStepsSkipsUnavailableImports(t *testing.T) {
	span := diag.NewSpan("entry.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	plan := &globalPlan{Steps: make([]globalInputStep, 0), StepByName: make(map[string]int)}
	useByIndex := map[int]imports.ResolvedUse{
		0: {Kind: imports.UseSelective, Names: []string{"missing"}, Source: imports.ModuleRef{ID: "dep", Label: "dep.jbs"}, Span: span, Index: 0},
	}
	appendModuleGlobalPlanSteps(
		plan,
		[]ast.Stmt{ast.UseStmt{Names: []string{"missing"}, Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "dep", Span: span}, Span: span}},
		"/modules/entry",
		globalPlanContext{},
		useByIndex,
		moduleBindingPrep{AcceptedImports: map[projectedImportDecisionKey]*projectedImport{}},
		nil,
	)
	if len(plan.Steps) != 0 || len(plan.StepByName) != 0 {
		t.Fatalf("expected missing projected import to be skipped, got plan %#v", plan)
	}
}

func TestAppendModuleGlobalPlanStepsSkipsImportsInsideControlBodies(t *testing.T) {
	span := diag.NewSpan("entry.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	plan := &globalPlan{Steps: make([]globalInputStep, 0), StepByName: make(map[string]int)}
	useByIndex := map[int]imports.ResolvedUse{
		0: {Kind: imports.UseNamespace, Alias: "dep", Source: imports.ModuleRef{ID: "dep", Label: "dep.jbs"}, Span: span, Index: 0},
	}
	steps := appendModuleGlobalPlanSteps(
		plan,
		[]ast.Stmt{
			ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "dep", Span: span}, Alias: "dep", Span: span},
			ast.GlobalAssign{Name: "after", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span},
		},
		"/modules/entry",
		globalPlanContext{InControlBody: true, OriginIndex: 42},
		useByIndex,
		moduleBindingPrep{AcceptedImports: map[projectedImportDecisionKey]*projectedImport{}},
		nil,
	)
	if len(plan.Steps) != 0 {
		t.Fatalf("did not expect nested steps to be appended to root plan: %#v", plan.Steps)
	}
	if len(steps) != 1 || steps[0].Kind != globalInputAssign || steps[0].Name != "after" || steps[0].Index != 42 {
		t.Fatalf("unexpected nested steps: %#v", steps)
	}
}
