package sema

import (
	"reflect"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func bindingWithOrigins(name string, order []string, vars map[string][]eval.Value, origins map[string]diag.Span) *GlobalBinding {
	return &GlobalBinding{
		Name:    name,
		Value:   tableValueFromVars(order, vars),
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
			{Name: "build", After: []string{"prep"}, WithItems: []ast.WithItem{withIdentItem("srcA", span0)}, Span: span0},
			{Name: "build", After: []string{"shadow"}, WithItems: []ast.WithItem{withIdentItem("srcB", span1)}, Span: span1},
			{Name: "run", After: []string{"build"}, Span: span1},
		},
		StepOrder: []string{"run", "build", "missing"},
	}

	defs, order := collectStepDefinitions(res)
	if !reflect.DeepEqual(order, []string{"run", "build"}) {
		t.Fatalf("unexpected step-definition order: %#v", order)
	}
	if !reflect.DeepEqual(defs["build"].After, []string{"prep"}) {
		t.Fatalf("expected first matching build definition, got %#v", defs["build"])
	}
	if len(defs["build"].WithItems) != 1 || withItemIdentName(defs["build"].WithItems[0]) != "srcA" {
		t.Fatalf("expected first matching build with-items, got %#v", defs["build"].WithItems)
	}

	defAfter := defs["build"].After
	defItems := defs["build"].WithItems
	defAfter[0] = "changed"
	defItems[0] = withIdentItem("changed", span0)
	if res.DoBlocks[0].After[0] != "prep" {
		t.Fatalf("expected collectStepDefinitions to copy after slice, got %#v", res.DoBlocks[0].After)
	}
	if withItemIdentName(res.DoBlocks[0].WithItems[0]) != "srcA" {
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

func TestBuildStepScopePlansHandlesInheritanceAndConflicts(t *testing.T) {
	spanA := diag.NewSpan("srcA.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	spanB := diag.NewSpan("srcB.jbs", diag.NewPos(2, 2, 1), diag.NewPos(3, 2, 2))
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
		},
		DoBlocks: []ast.DoBlock{
			{Name: "first", WithItems: []ast.WithItem{withIdentItem("srcA", stepSpan)}, Span: stepSpan},
			{Name: "second", WithItems: []ast.WithItem{withIdentItem("srcB", stepSpan)}, Span: stepSpan},
			{Name: "projected", WithItems: []ast.WithItem{withIndexStringItem("srcA", []string{"a"}, stepSpan)}, Span: stepSpan},
			{Name: "merge", After: []string{"first", "second", "first"}, Span: stepSpan},
			{Name: "explicitConflict", After: []string{"first"}, WithItems: []ast.WithItem{withIdentItem("srcB", stepSpan)}, Span: stepSpan},
		},
		StepOrder: []string{"first", "second", "projected", "merge", "explicitConflict"},
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

	projected := res.StepScopeByName["projected"]
	if projected == nil || len(projected.ExplicitDelta) != 1 || projected.ExplicitDelta[0].Full || projected.ExplicitDelta[0].Source != "srcA" || projected.ExplicitDelta[0].Visible != "a" {
		t.Fatalf("expected projected import plan, got %#v", projected)
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
	if merge.Inherited["shared"].ViaStep != "first" {
		t.Fatalf("expected inherited binding provenance from first step, got %#v", merge.Inherited["shared"])
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
	if got := diags.String(); !containsAll(got,
		"inherited via `after` from predecessor steps 'first' and 'second'",
		"`with` import from 'srcB' collides with name inherited via `after first`",
	) {
		t.Fatalf("expected conflict messages to distinguish after/with origins, got: %s", got)
	}
}

func TestAnalyzeAllowsDuplicateInheritedVisibleNameForSameSourceVersion(t *testing.T) {
	src := `
cases = table(x = (1,2))

do step0 with cases["x"] { echo $x }

do step1 with cases["x"] { echo $x }

do step2
        after step0
        after step1
{
        echo $x
}
`
	_, diags := analyzeRefValidationSource(t, "same_source_conflict.jbs", src)
	if countDiagCode(diags, string(diag.CodeE214)) != 0 {
		t.Fatalf("did not expect E214 for duplicate same-source inherited x, got: %s", diags.String())
	}
}

func TestReboundWithAfterConflictUsesPublicSourceName(t *testing.T) {
	src := `
x = 1

do s with x {
    echo $x
}

x = 2

do t after s with x {
    echo $x
}
`
	_, diags := analyzeRefValidationSource(t, "rebound_with_after.jbs", src)
	if countDiagCode(diags, string(diag.CodeE214)) != 1 {
		t.Fatalf("expected one E214 conflict, got: %s", diags.String())
	}
	text := diags.String()
	if !strings.Contains(text, "`with` import from 'x' collides with name inherited via `after s`") {
		t.Fatalf("expected public source name in conflict, got: %s", text)
	}
	if strings.Contains(text, "_js"+"__") {
		t.Fatalf("legacy snapshot name leaked into diagnostic: %s", text)
	}
}

func TestAnalyzeAllowsDifferentInheritedVisibleNamesAcrossReboundPublicSource(t *testing.T) {
	src := `
cases = table(x = (1,2))

do step0 with cases["x"] { echo $x }

cases = table(y = (1,2))

do step1 with cases["y"] { echo $y }

do step2
        after step0
        after step1
{
        echo $x $y
}
`
	_, diags := analyzeRefValidationSource(t, "different_names_rebind.jbs", src)
	if countDiagCode(diags, string(diag.CodeE214)) != 0 {
		t.Fatalf("did not expect E214 for distinct inherited names x and y, got: %s", diags.String())
	}
}

func withItemIdentName(item ast.WithItem) string {
	ident, _ := item.Expr.(ast.IdentExpr)
	return ident.Name
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
