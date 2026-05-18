package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestBindingResolverResolveDoWithItemsSkipsUnsupportedRefs(t *testing.T) {
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

func TestBindingResolverResolveAnalyseWithItemsKeepsDuplicateSource(t *testing.T) {
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

func TestWithResolverHelperEdges(t *testing.T) {
	if got := analyseDisallowedReasonForValue(eval.String("ok")); got != DisallowedBindingNone {
		t.Fatalf("string analyse value reason=%v, want none", got)
	}
	if got := analyseSourceVar("fallback", nil); got != "fallback" {
		t.Fatalf("nil binding source var=%q, want fallback", got)
	}
	if got := analyseSourceVar("missing", &GlobalBinding{Order: []string{"only"}, Vars: map[string][]eval.Value{"other": {eval.String("%d")}}}); got != "only" {
		t.Fatalf("single-column source var=%q, want only", got)
	}
	if got := analyseSourceVar("missing", &GlobalBinding{Order: []string{"a", "b"}, Vars: map[string][]eval.Value{"a": {eval.String("%d")}, "b": {eval.String("%f")}}}); got != "missing" {
		t.Fatalf("multi-column fallback source var=%q, want missing", got)
	}
	if got, ok := withBareName(ast.QualifiedIdentExpr{Namespace: "lib", Name: "x"}); !ok || got != "lib.x" {
		t.Fatalf("qualified bare name=%q ok=%v, want lib.x true", got, ok)
	}
	if got := sourceVarNameForScalar("source", nil); got != "source" {
		t.Fatalf("nil scalar source var=%q, want source", got)
	}
	if order, vars := varsFromTable(eval.Int(1)); order != nil || vars != nil {
		t.Fatalf("non-table varsFromTable=%#v %#v, want nil nil", order, vars)
	}
	if (BindingResolver{}).isExpressionVisibleOnly("") {
		t.Fatalf("empty name should not be expression-visible")
	}
	if (BindingResolver{Bindings: map[string]*GlobalBinding{"x": {Name: "x"}}}).isExpressionVisibleOnly("x") {
		t.Fatalf("bound name should not be expression-visible-only")
	}
}

func TestBindingResolverResolveDoWithRefInvalidProjections(t *testing.T) {
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

func TestBindingResolverExpandDoBindingEdges(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 4, 1), diag.NewPos(1, 4, 2))
	resolver := BindingResolver{}
	diags := &diag.Diagnostics{}
	if expansion, ok := resolver.expandDoBinding(0, ast.WithItem{Span: span}, withRef{Source: "missing", Span: span}, nil, diags); ok || expansion.Source != "" {
		t.Fatalf("nil binding should not expand, got %#v ok=%v", expansion, ok)
	}

	scalar := &GlobalBinding{
		Name:  "scalar",
		Value: eval.Int(1),
		Shape: BindingScalar,
		Order: []string{"scalar"},
		Vars:  map[string][]eval.Value{"scalar": {eval.Int(1)}},
	}
	if _, ok := resolver.expandDoBinding(0, ast.WithItem{Span: span}, withRef{Source: "scalar", Columns: []string{"x"}, Span: span}, scalar, diags); ok {
		t.Fatalf("scalar projection should not expand")
	}
	if countDiagCode(diags, string(diag.CodeE420)) != 1 {
		t.Fatalf("expected scalar projection diagnostic, got %s", diags.String())
	}

	emptyName := &GlobalBinding{Value: eval.Int(9)}
	expansion, ok := resolver.expandDoBinding(2, ast.WithItem{Span: span}, withRef{Span: span}, emptyName, diags)
	if !ok || expansion.Source != "" || expansion.DisplaySource != "" || expansion.SourceKey != (BindingVersionKey{}) {
		t.Fatalf("unexpected empty-source expansion: %#v ok=%v", expansion, ok)
	}
}

func TestNormalizeDoWithBindingDictAndUnsupportedEdges(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 5, 1), diag.NewPos(1, 5, 2))
	diags := &diag.Diagnostics{}
	if norm, ok := normalizeDoWithBinding("nil", nil, span, diags); ok || norm.RowCount != 0 {
		t.Fatalf("nil binding should not normalize, got %#v ok=%v", norm, ok)
	}

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

func TestVarsFromTableSkipsMissingColumnValues(t *testing.T) {
	value := eval.CombValue(&eval.Comb{
		Order: []string{"x", "missing"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(1)}}},
		},
	})

	order, vars := varsFromTable(value)
	if !reflect.DeepEqual(order, []string{"x", "missing"}) {
		t.Fatalf("unexpected order: %#v", order)
	}
	if _, exists := vars["missing"]; exists {
		t.Fatalf("missing column should not be present in vars: %#v", vars)
	}
	if got := vars["x"]; len(got) != 1 || !eval.Equal(got[0], eval.Int(1)) {
		t.Fatalf("unexpected x column values: %#v", got)
	}
}
