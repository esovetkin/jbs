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
		Value: eval.CombValue(&eval.Comb{Order: order}),
		Shape: shape,
		Order: order,
		Vars:  vars,
	}
}

func TestBindingResolverResolveDoWithItems(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	table := eval.CombValue(&eval.Comb{
		Order: []string{"x", "y"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(1)}, "y": {Value: eval.Int(2)}}},
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(3)}, "y": {Value: eval.Int(4)}}},
		},
	})
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"named": {
				Name:  "named",
				Value: table,
				Shape: BindingTable,
				Order: []string{"x", "y"},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1), eval.Int(3)},
					"y": {eval.Int(2), eval.Int(4)},
				},
			},
			"series": {
				Name:  "series",
				Value: eval.List([]eval.Value{eval.Int(1), eval.List([]eval.Value{eval.Int(2)})}),
				Shape: BindingScalar,
				Order: []string{"series"},
				Vars:  map[string][]eval.Value{"series": {eval.Int(1), eval.List([]eval.Value{eval.Int(2)})}},
			},
		},
		Globals: map[string]eval.Value{"sel": eval.String("x")},
	}
	items := []ast.WithItem{
		withIdentItem("named", span),
		withIndexStringItem("named", []string{"x"}, span),
		{Expr: ast.IndexExpr{Base: ast.IdentExpr{Name: "named", Span: span}, Items: []ast.Expr{ast.IdentExpr{Name: "sel", Span: span}}, Span: span}, Span: span},
		withIdentItem("series", span),
		withIdentItem("missing", span),
		withIndexStringItem("named", []string{"missing"}, span),
	}

	diags := &diag.Diagnostics{}
	expanded := resolver.ResolveDoWithItems(items, diags)

	if len(expanded) != 4 {
		t.Fatalf("expected 4 expanded items, got %d: %#v", len(expanded), expanded)
	}
	if !expanded[0].Full || expanded[0].Source != "named" || expanded[0].RowCount != 2 {
		t.Fatalf("unexpected full table expansion: %#v", expanded[0])
	}
	if expanded[1].Full || len(expanded[1].Vars) != 1 || expanded[1].Vars[0] != (ExpandedWithVar{Visible: "x", SourceVar: "x"}) {
		t.Fatalf("unexpected projected expansion: %#v", expanded[1])
	}
	if expanded[2].Full || len(expanded[2].Vars) != 1 || expanded[2].Vars[0].Visible != "x" {
		t.Fatalf("unexpected variable selector expansion: %#v", expanded[2])
	}
	if !expanded[3].Full || expanded[3].VarsByName["series"][1].Kind != eval.KindString {
		t.Fatalf("expected non-scalar series element to be stringified, got %#v", expanded[3])
	}
	if countDiagCode(diags, "W314") != 1 {
		t.Fatalf("expected one W314 warning, got %d: %s", countDiagCode(diags, "W314"), diags.String())
	}
	if countDiagCode(diags, "E020") != 1 || countDiagCode(diags, "E021") != 1 {
		t.Fatalf("expected unknown source and unknown variable diagnostics, got: %s", diags.String())
	}
}

func TestBindingResolverResolveAnalyseWithItems(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 10, 1), diag.NewPos(1, 10, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"pattern": {
				Name:  "pattern",
				Value: eval.String("%d"),
				Shape: BindingScalar,
				Order: []string{"pattern"},
				Vars:  map[string][]eval.Value{"pattern": {eval.String("%d")}},
			},
			"series": {
				Name:  "series",
				Value: eval.List([]eval.Value{eval.String("%d")}),
				Shape: BindingScalar,
				Order: []string{"series"},
				Vars:  map[string][]eval.Value{"series": {eval.String("%d")}},
			},
			"table": {
				Name:  "table",
				Value: eval.CombValue(&eval.Comb{Order: []string{"x"}}),
				Shape: BindingTable,
				Order: []string{"x"},
				Vars:  map[string][]eval.Value{"x": {eval.String("%d")}},
			},
		},
	}
	items := []ast.WithItem{
		withIdentItem("pattern", span),
		withIdentItem("series", span),
		withIndexStringItem("table", []string{"x"}, span),
		withIdentItem("missing", span),
	}

	diags := &diag.Diagnostics{}
	imports, issues := resolver.ResolveAnalyseWithItems(items, diags)
	emitWithIssues(diags, analyseWithDiagPolicy(), issues)

	if imported, ok := imports["pattern"]; !ok || imported.Source != "pattern" || imported.SourceVar != "pattern" {
		t.Fatalf("expected pattern import, got %#v", imports)
	}
	if len(imports) != 1 {
		t.Fatalf("expected only one analyse import, got %#v", imports)
	}
	gotKinds := make([]ResolveIssueKind, 0, len(issues))
	for _, issue := range issues {
		gotKinds = append(gotKinds, issue.Kind)
	}
	wantKinds := []ResolveIssueKind{
		IssueDisallowedBinding,
		IssueUnsupportedExpression,
		IssueUnknownSource,
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("unexpected analyse issues: got=%#v want=%#v", gotKinds, wantKinds)
	}
	if countDiagCode(diags, "E420") != 2 || countDiagCode(diags, "E020") != 1 {
		t.Fatalf("unexpected analyse diagnostics: %s", diags.String())
	}
}

func TestBindingResolverResolveBinding(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 20, 1), diag.NewPos(1, 20, 2))
	scalar := &GlobalBinding{Name: "scalar", Value: eval.String("ok"), Shape: BindingScalar, Order: []string{"scalar"}, Vars: map[string][]eval.Value{"scalar": {eval.String("ok")}}}
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{"scalar": scalar},
		Globals:  map[string]eval.Value{"fn": eval.Function(&eval.FunctionValue{})},
		Namespaces: map[string]*Namespace{
			"lib": {Name: "lib", Members: []string{"lib.fn"}},
		},
	}

	if got, issue := resolver.resolveBinding("missing", ast.WithItem{Span: span}); got != nil || issue == nil || issue.Kind != IssueUnknownSource {
		t.Fatalf("expected unknown-source issue, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("scalar", ast.WithItem{Span: span}); got != scalar || issue != nil {
		t.Fatalf("expected scalar binding to resolve, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("fn", ast.WithItem{Span: span}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected function-valued source to report disallowed-binding, got binding=%#v issue=%#v", got, issue)
	}
	if got, issue := resolver.resolveBinding("lib", ast.WithItem{Span: span}); got != nil || issue == nil || issue.Kind != IssueDisallowedBinding {
		t.Fatalf("expected namespace source to report disallowed-binding, got binding=%#v issue=%#v", got, issue)
	}
}
