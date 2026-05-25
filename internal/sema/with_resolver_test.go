package sema

import (
	"reflect"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

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

func TestBindingResolverResolveDoWithAliases(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"x": {
				Name:  "x",
				Value: eval.List([]eval.Value{eval.Int(1), eval.Int(2)}),
				Shape: BindingScalar,
				Order: []string{"x"},
				Vars:  map[string][]eval.Value{"x": {eval.Int(1), eval.Int(2)}},
			},
			"cases": {
				Name:  "cases",
				Value: tableValueFromVars([]string{"long", "other"}, map[string][]eval.Value{"long": {eval.String("a")}, "other": {eval.String("b")}}),
				Shape: BindingTable,
				Order: []string{"long", "other"},
				Vars:  map[string][]eval.Value{"long": {eval.String("a")}, "other": {eval.String("b")}},
			},
		},
	}

	diags := &diag.Diagnostics{}
	expanded := resolver.ResolveDoWithItems([]ast.WithItem{
		withIdentAliasItem("x", "y", span),
		withIndexStringAliasItem("cases", []string{"long"}, "short", span),
		withIndexStringAliasItem("cases", []string{"long", "other"}, "bad", span),
		withIdentAliasItem("x", "lib.x", span),
	}, diags)

	if len(expanded) != 2 {
		t.Fatalf("expected two valid aliased expansions, got %#v", expanded)
	}
	if got := expanded[0].Vars; len(got) != 1 || got[0] != (ExpandedWithVar{Visible: "y", SourceVar: "x"}) {
		t.Fatalf("unexpected scalar alias expansion: %#v", expanded[0])
	}
	if got := expanded[1].Vars; len(got) != 1 || got[0] != (ExpandedWithVar{Visible: "short", SourceVar: "long"}) {
		t.Fatalf("unexpected projection alias expansion: %#v", expanded[1])
	}
	if countDiagCode(diags, string(diag.CodeE023)) != 2 {
		t.Fatalf("expected invalid alias diagnostics, got: %s", diags.String())
	}
}

func TestBindingResolverRejectsReservedDoWithVisibleNames(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	aliasSpan := diag.NewSpan("with.jbs", diag.NewPos(10, 1, 11), diag.NewPos(22, 1, 23))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"JBS_WORK_DIR": {
				Name:  "JBS_WORK_DIR",
				Value: eval.String("/tmp/wrong"),
				Shape: BindingScalar,
				Order: []string{"JBS_WORK_DIR"},
				Vars:  map[string][]eval.Value{"JBS_WORK_DIR": {eval.String("/tmp/wrong")}},
			},
			"x": {
				Name:  "x",
				Value: eval.String("/tmp/wrong"),
				Shape: BindingScalar,
				Order: []string{"x"},
				Vars:  map[string][]eval.Value{"x": {eval.String("/tmp/wrong")}},
			},
			"cases": {
				Name: "cases",
				Value: tableValueFromVars([]string{"JBS_WORK_DIR", "ok"}, map[string][]eval.Value{
					"JBS_WORK_DIR": {eval.String("/tmp/wrong")},
					"ok":           {eval.String("safe")},
				}),
				Shape: BindingTable,
				Order: []string{"JBS_WORK_DIR", "ok"},
				Vars: map[string][]eval.Value{
					"JBS_WORK_DIR": {eval.String("/tmp/wrong")},
					"ok":           {eval.String("safe")},
				},
			},
		},
	}

	diags := &diag.Diagnostics{}
	aliasItem := withIdentAliasItem("x", "JBS_WORK_DIR", span)
	aliasItem.AliasSpan = aliasSpan
	expanded := resolver.ResolveDoWithItems([]ast.WithItem{
		withIdentItem("JBS_WORK_DIR", span),
		aliasItem,
		withIdentItem("cases", span),
		withIndexStringItem("cases", []string{"JBS_WORK_DIR"}, span),
		withIdentAliasItem("x", "JBS_WORK_DIR_EXTRA", span),
	}, diags)

	if len(expanded) != 1 {
		t.Fatalf("expected only near-miss alias to expand, got %#v", expanded)
	}
	if got := expanded[0].Vars; len(got) != 1 || got[0] != (ExpandedWithVar{Visible: "JBS_WORK_DIR_EXTRA", SourceVar: "x"}) {
		t.Fatalf("unexpected near-miss expansion: %#v", expanded[0])
	}
	if countDiagCode(diags, string(diag.CodeE023)) != 4 {
		t.Fatalf("expected four reserved-name diagnostics, got %s", diags.String())
	}
	if !strings.Contains(diags.String(), "JBS_WORK_DIR") || !strings.Contains(diags.String(), "reserved for JBS runtime metadata") {
		t.Fatalf("missing reserved-name details: %s", diags.String())
	}
	if len(diags.Items) < 2 || diags.Items[1].Span != aliasSpan {
		t.Fatalf("alias diagnostic span = %#v, want %#v", diags.Items, aliasSpan)
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

func TestBindingResolverResolveAnalyseWithAlias(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 10, 1), diag.NewPos(1, 10, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"pattern": {
				Name:  "pattern",
				Value: eval.String("%d"),
				Shape: BindingScalar,
				Order: []string{"pattern"},
				Vars:  map[string][]eval.Value{"pattern": {eval.String("%d")}},
			},
		},
	}

	diags := &diag.Diagnostics{}
	imports, issues := resolver.ResolveAnalyseWithItems([]ast.WithItem{
		withIdentAliasItem("pattern", "pat", span),
	}, diags)
	if len(issues) != 0 || diags.HasErrors() {
		t.Fatalf("unexpected analyse alias diagnostics: issues=%#v diags=%s", issues, diags.String())
	}
	if _, ok := imports["pattern"]; ok {
		t.Fatalf("original name should not be visible when aliased: %#v", imports)
	}
	if got := imports["pat"]; got.Source != "pattern" || got.SourceVar != "pattern" {
		t.Fatalf("unexpected analyse alias import: %#v", imports)
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

func TestDoWithSkipsUnsupportedWithItemsAfterDiagnostic(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"ok": {
				Name:  "ok",
				Value: eval.Int(1),
				Shape: BindingScalar,
				Order: []string{"ok"},
				Vars:  map[string][]eval.Value{"ok": {eval.Int(1)}},
			},
		},
	}
	items := []ast.WithItem{
		{Expr: ast.StringExpr{Value: "bad", Span: span}, Span: span},
		withIdentItem("ok", span),
	}

	diags := &diag.Diagnostics{}
	expanded := resolver.ResolveDoWithItems(items, diags)
	if len(expanded) != 1 || expanded[0].Source != "ok" {
		t.Fatalf("expected only valid item to expand, got %#v", expanded)
	}
	if countDiagCode(diags, string(diag.CodeE023)) != 1 {
		t.Fatalf("expected unsupported-expression diagnostic, got %s", diags.String())
	}
}

func TestAnalyseWithAllowsDistinctBindingsWithSameSourceColumn(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 2, 1), diag.NewPos(1, 2, 2))
	resolver := BindingResolver{
		Bindings: map[string]*GlobalBinding{
			"first": {
				Name:  "first",
				Value: eval.String("%d"),
				Shape: BindingScalar,
				Order: []string{"same"},
				Vars:  map[string][]eval.Value{"same": {eval.String("%d")}},
			},
			"second": {
				Name:  "second",
				Value: eval.String("%f"),
				Shape: BindingScalar,
				Order: []string{"same"},
				Vars:  map[string][]eval.Value{"same": {eval.String("%f")}},
			},
		},
	}

	diags := &diag.Diagnostics{}
	imports, issues := resolver.ResolveAnalyseWithItems([]ast.WithItem{
		withIdentItem("first", span),
		withIdentItem("second", span),
		withIdentItem("first", span),
	}, diags)
	if len(issues) != 0 {
		t.Fatalf("did not expect resolve issues, got %#v", issues)
	}
	if countDiagCode(diags, string(diag.CodeE214)) != 0 {
		t.Fatalf("did not expect analyse conflict diagnostic, got %s", diags.String())
	}
	if got := imports["first"]; got.Source != "first" || got.SourceVar != "same" {
		t.Fatalf("expected first import to remain, got %#v", imports)
	}
	if got := imports["second"]; got.Source != "second" || got.SourceVar != "same" {
		t.Fatalf("expected second import to remain, got %#v", imports)
	}
}

func TestDoWithRejectsInvalidProjectionReferences(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 3, 1), diag.NewPos(1, 3, 2))
	resolver := BindingResolver{Globals: map[string]eval.Value{"n": eval.Int(1)}}
	cases := []ast.WithItem{
		{Span: span},
		{Expr: ast.StringExpr{Value: "bad", Span: span}, Span: span},
		{Expr: ast.IndexExpr{Base: ast.StringExpr{Value: "bad", Span: span}, Items: []ast.Expr{ast.StringExpr{Value: "x", Span: span}}, Span: span}, Span: span},
		{Expr: ast.IndexExpr{Base: ast.IdentExpr{Name: "table", Span: span}, Span: span}, Span: span},
		{Expr: ast.IndexExpr{Base: ast.IdentExpr{Name: "table", Span: span}, Items: []ast.Expr{ast.IdentExpr{Name: "n", Span: span}}, Span: span}, Span: span},
	}

	diags := &diag.Diagnostics{}
	for _, item := range cases {
		if ref, ok := resolver.resolveDoWithRef(item, diags); ok {
			t.Fatalf("expected invalid ref, got %#v", ref)
		}
	}
	if countDiagCode(diags, string(diag.CodeE023)) != 4 {
		t.Fatalf("expected four projection diagnostics, got %s", diags.String())
	}
}

func TestDoWithRejectsScalarProjection(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 4, 1), diag.NewPos(1, 4, 2))
	scalar := &GlobalBinding{
		Name:  "scalar",
		Value: eval.Int(1),
		Shape: BindingScalar,
		Order: []string{"scalar"},
		Vars:  map[string][]eval.Value{"scalar": {eval.Int(1)}},
	}

	diags := &diag.Diagnostics{}
	resolver := BindingResolver{}
	if _, ok := resolver.expandDoBinding(0, ast.WithItem{Span: span}, withRef{Source: "scalar", Columns: []string{"x"}, Span: span}, scalar, diags); ok {
		t.Fatalf("scalar projection should not expand")
	}
	if countDiagCode(diags, string(diag.CodeE420)) != 1 {
		t.Fatalf("expected scalar projection diagnostic, got %s", diags.String())
	}
}

func TestDoWithNormalizesDictionaryBindingsAndRejectsUnsupportedValues(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 5, 1), diag.NewPos(1, 5, 2))
	diags := &diag.Diagnostics{}
	validDict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a"}, Value: eval.Tuple([]eval.Value{eval.Int(1), eval.Int(2)})},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "b"}, Value: eval.List([]eval.Value{eval.String("x"), eval.String("y")})},
	})
	norm, ok := normalizeDoWithBinding("dict", &GlobalBinding{Name: "dict", Value: validDict}, span, diags)
	if !ok || norm.Shape != BindingTable || norm.RowCount != 2 || !reflect.DeepEqual(norm.Order, []string{"a", "b"}) {
		t.Fatalf("unexpected dict normalization: %#v ok=%v diags=%s", norm, ok, diags.String())
	}

	invalidDict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyInt, I: 1}, Value: eval.Int(1)},
	})
	if _, ok := normalizeDoWithBinding("bad_dict", &GlobalBinding{Name: "bad_dict", Value: invalidDict}, span, diags); ok {
		t.Fatalf("invalid dict key should not normalize")
	}
	if _, ok := normalizeDoWithBinding("fn", &GlobalBinding{Name: "fn", Value: eval.Function(&eval.FunctionValue{})}, span, diags); ok {
		t.Fatalf("function binding should not normalize")
	}
	if countDiagCode(diags, string(diag.CodeE420)) != 1 {
		t.Fatalf("expected unsupported-value diagnostic, got %s", diags.String())
	}
}
