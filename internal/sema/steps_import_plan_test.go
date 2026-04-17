package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func bindingWithOrigins(name string, order []string, vars map[string][]eval.Value, origins map[string]diag.Span) *GlobalBinding {
	return &GlobalBinding{
		Name:    name,
		Shape:   BindingTable,
		Order:   order,
		Vars:    vars,
		Origins: origins,
	}
}

func TestCollectStepDefinitionsUsesFirstMatchingBlockAndCopiesSlices(t *testing.T) {
	span0 := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	span1 := diag.NewSpan("in.jbs", diag.NewPos(2, 2, 1), diag.NewPos(3, 2, 2))
	res := &Result{
		DoBlocks: []ast.DoBlock{
			{Name: "build", After: []string{"prep"}, WithItems: []ast.WithItem{{Name: "srcA", Span: span0}}, Span: span0},
			{Name: "build", After: []string{"shadow"}, WithItems: []ast.WithItem{{Name: "srcB", Span: span1}}, Span: span1},
		},
		Submits:   []ast.SubmitBlock{{Name: "submit", After: []string{"build"}, Span: span1}},
		StepOrder: []string{"submit", "build", "missing"},
	}

	defs, order := collectStepDefinitions(res)
	if !reflect.DeepEqual(order, []string{"submit", "build"}) {
		t.Fatalf("unexpected step-definition order: %#v", order)
	}
	if !reflect.DeepEqual(defs["build"].After, []string{"prep"}) {
		t.Fatalf("expected first matching build definition, got %#v", defs["build"])
	}
	if len(defs["build"].WithItems) != 1 || defs["build"].WithItems[0].Name != "srcA" {
		t.Fatalf("expected first matching build with-items, got %#v", defs["build"].WithItems)
	}

	defAfter := defs["build"].After
	defItems := defs["build"].WithItems
	defAfter[0] = "changed"
	defItems[0].Name = "changed"
	if res.DoBlocks[0].After[0] != "prep" {
		t.Fatalf("expected collectStepDefinitions to copy after slice, got %#v", res.DoBlocks[0].After)
	}
	if res.DoBlocks[0].WithItems[0].Name != "srcA" {
		t.Fatalf("expected collectStepDefinitions to copy with-items slice, got %#v", res.DoBlocks[0].WithItems)
	}
}

func TestStepDefinitionDepsReturnsCopy(t *testing.T) {
	defs := map[string]stepDefinition{
		"build": {
			Name:  "build",
			After: []string{"prep", "compile"},
		},
	}

	deps := stepDefinitionDeps(defs)
	if !reflect.DeepEqual(deps["build"], []string{"prep", "compile"}) {
		t.Fatalf("unexpected deps: %#v", deps)
	}
	deps["build"][0] = "changed"
	if defs["build"].After[0] != "prep" {
		t.Fatalf("expected copied dependency slice, got %#v", defs["build"].After)
	}
}

func TestBuildStepScopePlansHandlesInheritanceFallbackAndConflicts(t *testing.T) {
	spanA := diag.NewSpan("srcA.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	spanB := diag.NewSpan("srcB.jbs", diag.NewPos(2, 2, 1), diag.NewPos(3, 2, 2))
	spanF := diag.NewSpan("fallback.jbs", diag.NewPos(4, 3, 1), diag.NewPos(5, 3, 2))
	stepSpan := diag.NewSpan("steps.jbs", diag.NewPos(10, 10, 1), diag.NewPos(11, 10, 2))

	res := &Result{
		BindingsByName: map[string]*GlobalBinding{
			"srcA": bindingWithOrigins("srcA", []string{"shared", "a"}, map[string][]eval.Value{
				"shared": {eval.String("a-shared")},
				"a":      {eval.Int(1)},
			}, map[string]diag.Span{
				"shared": spanA,
				"a":      spanA,
			}),
			"srcB": bindingWithOrigins("srcB", []string{"shared", "b"}, map[string][]eval.Value{
				"shared": {eval.String("b-shared")},
				"b":      {eval.Int(2)},
			}, map[string]diag.Span{
				"shared": spanB,
				"b":      spanB,
			}),
			"fallback": bindingWithOrigins("fallback", []string{"f"}, map[string][]eval.Value{
				"f": {eval.String("ok")},
			}, map[string]diag.Span{
				"f": spanF,
			}),
		},
		DoBlocks: []ast.DoBlock{
			{Name: "first", WithItems: []ast.WithItem{{Name: "srcA", Span: stepSpan}}, Span: stepSpan},
			{Name: "second", WithItems: []ast.WithItem{{Name: "srcB", Span: stepSpan}}, Span: stepSpan},
			{Name: "mixed", WithItems: []ast.WithItem{{Name: "fallback", From: "srcA", Span: stepSpan}}, Span: stepSpan},
			{Name: "merge", After: []string{"first", "second", "first"}, Span: stepSpan},
			{Name: "explicitConflict", After: []string{"first"}, WithItems: []ast.WithItem{{Name: "srcB", Span: stepSpan}}, Span: stepSpan},
		},
		StepOrder: []string{"first", "second", "mixed", "merge", "explicitConflict"},
	}

	diags := &diag.Diagnostics{}
	buildStepScopePlans(res, diags)
	if res.StepScopeByName == nil {
		t.Fatalf("expected step scope plans to be populated")
	}
	if countDiagCode(diags, "E214") != 2 {
		t.Fatalf("expected two E214 conflicts, got %d: %s", countDiagCode(diags, "E214"), diags.String())
	}

	first := res.StepScopeByName["first"]
	if first == nil || len(first.ExplicitDelta) != 1 || !first.ExplicitDelta[0].Full || first.ExplicitDelta[0].Source != "srcA" {
		t.Fatalf("unexpected first-step plan: %#v", first)
	}
	if first.Effective["shared"].Source != "srcA" || first.Effective["shared"].Span != spanA {
		t.Fatalf("unexpected first-step visible binding: %#v", first.Effective["shared"])
	}

	mixed := res.StepScopeByName["mixed"]
	if mixed == nil || len(mixed.ExplicitDelta) != 1 || !mixed.ExplicitDelta[0].Full || mixed.ExplicitDelta[0].Source != "fallback" {
		t.Fatalf("expected mixed-source fallback to import full fallback binding, got %#v", mixed)
	}

	merge := res.StepScopeByName["merge"]
	if merge == nil {
		t.Fatalf("expected merge step plan")
	}
	if !reflect.DeepEqual(merge.InheritedSteps, []string{"first", "second"}) {
		t.Fatalf("expected deduped inherited step order, got %#v", merge.InheritedSteps)
	}
	if merge.Inherited["shared"].Source != "srcA" {
		t.Fatalf("expected inherited conflict to keep first source, got %#v", merge.Inherited["shared"])
	}
	if merge.Effective["b"].Source != "srcB" {
		t.Fatalf("expected non-conflicting inherited variable from second source, got %#v", merge.Effective["b"])
	}

	explicitConflict := res.StepScopeByName["explicitConflict"]
	if explicitConflict == nil {
		t.Fatalf("expected explicitConflict step plan")
	}
	if len(explicitConflict.ExplicitDelta) != 1 {
		t.Fatalf("expected only non-conflicting explicit import to remain, got %#v", explicitConflict.ExplicitDelta)
	}
	if explicitConflict.ExplicitDelta[0].Full || explicitConflict.ExplicitDelta[0].Source != "srcB" || explicitConflict.ExplicitDelta[0].Visible != "b" {
		t.Fatalf("unexpected explicit-conflict delta: %#v", explicitConflict.ExplicitDelta[0])
	}
	if explicitConflict.Effective["shared"].Source != "srcA" {
		t.Fatalf("expected inherited source to win explicit conflict, got %#v", explicitConflict.Effective["shared"])
	}
	if explicitConflict.Effective["b"].Source != "srcB" || explicitConflict.Effective["b"].Span != spanB {
		t.Fatalf("expected non-conflicting explicit source to remain, got %#v", explicitConflict.Effective["b"])
	}
}
