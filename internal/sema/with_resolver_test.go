package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func testBinding(name string, shape BindingShape, order []string, vars map[string][]eval.Value) *GlobalBinding {
	return &GlobalBinding{
		Name:  name,
		Shape: shape,
		Order: order,
		Vars:  vars,
	}
}

func TestBindingResolverExpandWithItemsInStepContext(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"named": testBinding("named", BindingTable, []string{"x", "y"}, map[string][]eval.Value{
				"x": {eval.Int(1)},
				"y": {eval.Int(2)},
			}),
			"fallback": testBinding("fallback", BindingScalar, []string{"f"}, map[string][]eval.Value{
				"f": {eval.String("ok")},
			}),
		},
	}
	items := []ast.WithItem{
		{Rejected: true, Name: "named", Span: span},
		{Name: "named", Span: span},
		{Name: "named", Alias: "alias_name", Span: span},
		{Name: "x", From: "named", Alias: "xx", Span: span},
		{Name: "fallback", From: "named", Span: span},
		{Name: "nope", From: "named", Span: span},
		{Name: "missing", Span: span},
		{SourceExpr: "named", SourceSlice: []string{"x"}, CombAlias: "slice_alias", Span: span},
		{SourceExpr: "named", SourceSlice: []string{"missing"}, Span: span},
	}

	expanded, issues := resolver.ExpandWithItems(items, ResolveOptions{
		Context:                   ImportIntoStep,
		EnableMixedSourceFallback: true,
	})

	if len(expanded) != 5 {
		t.Fatalf("expected 5 expanded items, got %d: %#v", len(expanded), expanded)
	}
	if !expanded[0].Full || expanded[0].Source != "named" || expanded[0].SourceExpr != "named" {
		t.Fatalf("unexpected full import expansion: %#v", expanded[0])
	}
	if !expanded[1].Full || expanded[1].SourceExpr != "alias_name" {
		t.Fatalf("expected aliased full import source expression, got %#v", expanded[1])
	}
	if expanded[2].Full || len(expanded[2].Vars) != 1 || expanded[2].Vars[0].Visible != "xx" || expanded[2].Vars[0].SourceVar != "x" {
		t.Fatalf("unexpected single-var import expansion: %#v", expanded[2])
	}
	if !expanded[3].Full || expanded[3].Source != "fallback" || expanded[3].SourceExpr != "fallback" {
		t.Fatalf("expected mixed-source fallback full import, got %#v", expanded[3])
	}
	if expanded[4].Source != "named" || expanded[4].Full || expanded[4].SourceExpr != "named" || expanded[4].CombAlias != "slice_alias" {
		t.Fatalf("unexpected source-slice expansion metadata: %#v", expanded[4])
	}
	if !reflect.DeepEqual(expanded[4].SliceOrder, []string{"x"}) {
		t.Fatalf("unexpected source-slice order: %#v", expanded[4].SliceOrder)
	}

	gotKinds := make([]ResolveIssueKind, 0, len(issues))
	for _, issue := range issues {
		gotKinds = append(gotKinds, issue.Kind)
	}
	wantKinds := []ResolveIssueKind{
		IssueUnknownVar,
		IssueUnknownSource,
		IssueUnknownVar,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("unexpected resolve issues: got=%#v want=%#v", gotKinds, wantKinds)
	}
}

func TestBindingResolverExpandWithItemsInAnalyseContext(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 10, 1), diag.NewPos(1, 10, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"named": testBinding("named", BindingScalar, []string{"value"}, map[string][]eval.Value{
				"value": {eval.String("ok")},
			}),
			"table": testBinding("table", BindingTable, []string{"x"}, map[string][]eval.Value{
				"x": {eval.Int(1)},
			}),
		},
	}
	items := []ast.WithItem{
		{Name: "table", Span: span},
		{SourceExpr: "table", SourceSlice: []string{"x"}, Span: span},
		{Name: "table", From: "named", Span: span},
	}

	expanded, issues := resolver.ExpandWithItems(items, ResolveOptions{
		Context:                   ImportIntoAnalyse,
		EnableMixedSourceFallback: true,
	})

	if len(expanded) != 0 {
		t.Fatalf("expected no expanded items in analyse context for disallowed bindings, got %#v", expanded)
	}
	gotKinds := make([]ResolveIssueKind, 0, len(issues))
	for _, issue := range issues {
		gotKinds = append(gotKinds, issue.Kind)
	}
	wantKinds := []ResolveIssueKind{
		IssueDisallowedBinding,
		IssueDisallowedBinding,
		IssueDisallowedBinding,
		IssueUnknownVar,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("unexpected analyse-context issues: got=%#v want=%#v", gotKinds, wantKinds)
	}
}

func TestBindingResolverResolveBindingAndExpandFullBinding(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 20, 1), diag.NewPos(1, 20, 2))
	ordered := testBinding("ordered", BindingScalar, []string{"b", "a"}, map[string][]eval.Value{
		"a": {eval.String("a")},
		"b": {eval.String("b")},
	})
	scalar := testBinding("scalar", BindingScalar, []string{"value"}, map[string][]eval.Value{
		"value": {eval.String("ok")},
	})
	table := testBinding("table", BindingTable, []string{"x"}, map[string][]eval.Value{
		"x": {eval.Int(1)},
	})
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"ordered": ordered,
			"scalar":  scalar,
			"table":   table,
		},
		Globals: map[string]eval.Value{
			"fn": eval.Function(&eval.FunctionValue{}),
		},
		Namespaces: map[string]*Namespace{
			"lib": {Name: "lib", Members: []string{"lib.fn"}},
		},
	}

	if got, issue := resolver.resolveBinding("missing", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoStep}); got != nil || issue == nil || issue.Kind != IssueUnknownSource {
		t.Fatalf("expected unknown-source issue, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("table", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoSubmitUse}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected disallowed-binding issue, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("scalar", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoAnalyse}); got != scalar || issue != nil {
		t.Fatalf("expected scalar binding to resolve for analyse context, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("fn", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoStep}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected function-valued source to report disallowed-binding, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("lib", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoStep}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected namespace source to report disallowed-binding, got binding=%#v issue=%#v", got, issue)
	}

	expanded := expandFullBinding(ast.WithItem{Name: "ordered", Alias: "alias_name", Span: span}, ordered)
	if !expanded.Full || expanded.Source != "ordered" || expanded.SourceExpr != "alias_name" {
		t.Fatalf("unexpected expanded full binding metadata: %#v", expanded)
	}
	if !reflect.DeepEqual(expanded.Vars, []ExpandedWithVar{
		{Visible: "b", SourceVar: "b"},
		{Visible: "a", SourceVar: "a"},
	}) {
		t.Fatalf("expected vars in binding order [b,a], got %#v", expanded.Vars)
	}

	sortedExpanded := expandFullBinding(ast.WithItem{Name: "table", Span: span}, &GlobalBinding{
		Name:  "table",
		Shape: BindingTable,
		Vars: map[string][]eval.Value{
			"z": {eval.Int(1)},
			"m": {eval.Int(2)},
		},
	})
	if !reflect.DeepEqual(sortedExpanded.Vars, []ExpandedWithVar{
		{Visible: "m", SourceVar: "m"},
		{Visible: "z", SourceVar: "z"},
	}) {
		t.Fatalf("expected vars sorted by name when no order is provided, got %#v", sortedExpanded.Vars)
	}
}
