package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func testSpan(line int) diag.Span {
	return diag.NewSpan("in.jbs", diag.NewPos(line, line, 1), diag.NewPos(line+1, line, 2))
}

func testParamset(name string, span diag.Span, vars map[string][]eval.Value, order []string) *Paramset {
	origins := make(map[string]diag.Span, len(vars))
	for k := range vars {
		origins[k] = span
	}
	return &Paramset{
		Name:    name,
		Block:   ast.ParamBlock{Name: name, Span: span},
		Vars:    vars,
		Origins: origins,
		Order:   order,
	}
}

func TestCollectStepDefinitionsKeepsFirstOccurrenceAndOrder(t *testing.T) {
	span1 := testSpan(1)
	span2 := testSpan(10)
	span3 := testSpan(20)

	res := &Result{
		Program: ast.Program{
			Stmts: []ast.Stmt{
				ast.DoBlock{Name: "step_a", Span: span1},
				ast.SubmitBlock{Name: "step_b", Span: span2},
				ast.DoBlock{Name: "step_a", Span: span3},
			},
		},
	}

	defs, order := collectStepDefinitions(res)
	if len(defs) != 2 {
		t.Fatalf("expected 2 step definitions, got %d", len(defs))
	}
	if len(order) != 2 || order[0] != "step_a" || order[1] != "step_b" {
		t.Fatalf("unexpected step order: %#v", order)
	}
	if defs["step_a"].Span != span1 {
		t.Fatalf("expected first definition span for step_a, got=%+v want=%+v", defs["step_a"].Span, span1)
	}
}

func TestStepDefinitionDepsReturnsCopy(t *testing.T) {
	defs := map[string]stepDefinition{
		"s0": {Name: "s0", After: []string{"a", "b"}},
	}
	deps := stepDefinitionDeps(defs)
	if len(deps["s0"]) != 2 || deps["s0"][0] != "a" || deps["s0"][1] != "b" {
		t.Fatalf("unexpected deps: %#v", deps)
	}
	deps["s0"][0] = "mutated"
	if defs["s0"].After[0] != "a" {
		t.Fatalf("expected original deps to stay unchanged, got=%#v", defs["s0"].After)
	}
}

func TestBuildStepImportPlansHandlesInheritanceAndConflicts(t *testing.T) {
	spanP0 := testSpan(30)
	spanP1 := testSpan(40)
	spanS0 := testSpan(50)
	spanS1 := testSpan(60)

	p0 := testParamset(
		"p0",
		spanP0,
		map[string][]eval.Value{
			"a": {eval.Int(1)},
			"b": {eval.String("b0")},
		},
		[]string{"a", "b"},
	)
	p1 := testParamset(
		"p1",
		spanP1,
		map[string][]eval.Value{
			"b": {eval.String("b1")},
			"c": {eval.String("c1")},
		},
		[]string{"b", "c"},
	)

	s0 := ast.DoBlock{
		Name: "s0",
		WithItems: []ast.WithItem{
			{Name: "p0", Span: spanS0},
		},
		Span: spanS0,
	}
	s1 := ast.DoBlock{
		Name:  "s1",
		After: []string{"s0", "s0"},
		WithItems: []ast.WithItem{
			{Name: "b", From: "p1", Span: spanS1},
			{Name: "c", From: "p1", Span: spanS1},
		},
		Span: spanS1,
	}

	res := &Result{
		Program: ast.Program{
			Stmts: []ast.Stmt{s0, s1},
		},
		Paramsets: []*Paramset{p0, p1},
		ParamByName: map[string]*Paramset{
			"p0": p0,
			"p1": p1,
		},
		LetNamespaces:      []*LetNamespace{},
		LetByName:          map[string]*LetNamespace{},
		ImportSourceByName: map[string]*ImportSource{},
	}
	buildImportSources(res)
	diags := &diag.Diagnostics{}
	buildStepImportPlans(res, diags)

	plan0 := res.StepImportByName["s0"]
	if plan0 == nil {
		t.Fatalf("missing plan for s0")
	}
	if len(plan0.ExplicitDelta) != 1 || !plan0.ExplicitDelta[0].Full || plan0.ExplicitDelta[0].Source != "p0" {
		t.Fatalf("expected full explicit import for s0 from p0, got %#v", plan0.ExplicitDelta)
	}
	if len(plan0.Effective) != 2 {
		t.Fatalf("expected 2 effective vars in s0, got %#v", plan0.Effective)
	}

	plan1 := res.StepImportByName["s1"]
	if plan1 == nil {
		t.Fatalf("missing plan for s1")
	}
	if len(plan1.InheritedSteps) != 1 || plan1.InheritedSteps[0] != "s0" {
		t.Fatalf("expected deduplicated inherited steps [s0], got %#v", plan1.InheritedSteps)
	}
	if len(plan1.ExplicitDelta) != 1 || plan1.ExplicitDelta[0].Visible != "c" || plan1.ExplicitDelta[0].Source != "p1" {
		t.Fatalf("expected only non-conflicting explicit import c from p1, got %#v", plan1.ExplicitDelta)
	}
	if got := plan1.Effective["a"].Paramset; got != "p0" {
		t.Fatalf("expected inherited a from p0, got %q", got)
	}
	if got := plan1.Effective["b"].Paramset; got != "p0" {
		t.Fatalf("expected inherited b from p0 after conflict pruning, got %q", got)
	}
	if got := plan1.Effective["c"].Paramset; got != "p1" {
		t.Fatalf("expected explicit c from p1, got %q", got)
	}
	if countDiagCode(diags, string(diag.CodeE214)) != 1 {
		t.Fatalf("expected one E214 conflict diagnostic, got: %s", diags.String())
	}
}
