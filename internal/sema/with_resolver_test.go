package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
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
		{Source: "named", Span: span},
		{Source: "named", Selectors: []string{"x"}, Span: span},
		{Source: "fallback", Span: span},
		{Source: "missing", Span: span},
		{Source: "named", Selectors: []string{"missing"}, Span: span},
	}

	expanded, issues := resolver.ExpandWithItems(items, ResolveOptions{Context: ImportIntoStep})

	if len(expanded) != 3 {
		t.Fatalf("expected 3 expanded items, got %d: %#v", len(expanded), expanded)
	}
	if !expanded[0].Full || expanded[0].Source != "named" {
		t.Fatalf("unexpected full import expansion: %#v", expanded[0])
	}
	if expanded[1].Full || expanded[1].Source != "named" || len(expanded[1].Vars) != 1 || expanded[1].Vars[0] != (ExpandedWithVar{Visible: "x", SourceVar: "x"}) {
		t.Fatalf("unexpected projected import expansion: %#v", expanded[1])
	}
	if !expanded[2].Full || expanded[2].Source != "fallback" {
		t.Fatalf("unexpected scalar full import expansion: %#v", expanded[2])
	}

	gotKinds := make([]ResolveIssueKind, 0, len(issues))
	for _, issue := range issues {
		gotKinds = append(gotKinds, issue.Kind)
	}
	wantKinds := []ResolveIssueKind{
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
		{Source: "table", Span: span},
		{Source: "table", Selectors: []string{"x"}, Span: span},
		{Source: "named", Selectors: []string{"missing"}, Span: span},
	}

	expanded, issues := resolver.ExpandWithItems(items, ResolveOptions{Context: ImportIntoAnalyse})

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
		IssueUnknownVar,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("unexpected analyse-context issues: got=%#v want=%#v", gotKinds, wantKinds)
	}
	if issues[0].DisallowedReason != DisallowedBindingAnalyseTable || issues[1].DisallowedReason != DisallowedBindingAnalyseTable {
		t.Fatalf("expected table disallowed reasons, got %#v", issues)
	}
}

func TestBindingResolverReportsAnalyseDisallowedReasons(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"table": testBinding("table", BindingTable, []string{"x"}, map[string][]eval.Value{
				"x": {eval.String("a")},
			}),
			"multi": testBinding("multi", BindingScalar, []string{"x", "y"}, map[string][]eval.Value{
				"x": {eval.String("a")},
				"y": {eval.String("b")},
			}),
			"num": testBinding("num", BindingScalar, []string{"num"}, map[string][]eval.Value{
				"num": {eval.Int(1)},
			}),
		},
		Globals: map[string]eval.Value{
			"fn": eval.Function(&eval.FunctionValue{}),
		},
	}
	tests := []struct {
		name    string
		reason  DisallowedBindingReason
		columns int
		kind    eval.Kind
	}{
		{name: "table", reason: DisallowedBindingAnalyseTable, columns: 1, kind: eval.KindString},
		{name: "multi", reason: DisallowedBindingAnalyseMultiColumn, columns: 2},
		{name: "num", reason: DisallowedBindingAnalyseNonString, columns: 1, kind: eval.KindInt},
		{name: "fn", reason: DisallowedBindingNotData},
	}
	for _, tt := range tests {
		_, issue := resolver.resolveBinding(tt.name, ast.WithItem{Source: tt.name, Span: span}, ResolveOptions{Context: ImportIntoAnalyse})
		if issue == nil {
			t.Fatalf("%s: expected disallowed-binding issue", tt.name)
		}
		if issue.Kind != IssueDisallowedBinding || issue.DisallowedReason != tt.reason {
			t.Fatalf("%s: expected reason %v, got %#v", tt.name, tt.reason, issue)
		}
		if issue.DisallowedColumns != tt.columns {
			t.Fatalf("%s: expected %d columns, got %d", tt.name, tt.columns, issue.DisallowedColumns)
		}
		if issue.DisallowedValueKind != tt.kind {
			t.Fatalf("%s: expected value kind %q, got %q", tt.name, tt.kind, issue.DisallowedValueKind)
		}
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
	if got, issue := resolver.resolveBinding("scalar", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoAnalyse}); got != scalar || issue != nil {
		t.Fatalf("expected scalar binding to resolve for analyse context, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("fn", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoStep}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected function-valued source to report disallowed-binding, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("lib", ast.WithItem{Span: span}, ResolveOptions{Context: ImportIntoStep}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected namespace source to report disallowed-binding, got binding=%#v issue=%#v", got, issue)
	}

	expanded := expandFullBinding(ast.WithItem{Source: "ordered", Span: span}, ordered)
	if !expanded.Full || expanded.Source != "ordered" {
		t.Fatalf("unexpected expanded full binding metadata: %#v", expanded)
	}
	if !reflect.DeepEqual(expanded.Vars, []ExpandedWithVar{
		{Visible: "b", SourceVar: "b"},
		{Visible: "a", SourceVar: "a"},
	}) {
		t.Fatalf("expected vars in binding order [b,a], got %#v", expanded.Vars)
	}

	sortedExpanded := expandFullBinding(ast.WithItem{Source: "table", Span: span}, &GlobalBinding{
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
