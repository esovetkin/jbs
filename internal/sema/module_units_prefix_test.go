package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestPrefixModuleScope(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	original := emptyModuleScope()
	binding := &GlobalBinding{
		Name:      "value",
		Value:     eval.Int(1),
		Shape:     BindingScalar,
		Vars:      map[string][]eval.Value{"value": {eval.Int(1)}},
		BaseVars:  map[string][]eval.Value{"value": {eval.Int(1)}},
		Order:     []string{"value"},
		Rows:      []eval.Row{{Values: map[string]eval.Cell{"value": {Value: eval.Int(1), Origin: span}}}},
		Span:      span,
		DependsOn: []string{"dep", ""},
		VersionID: "v1",
	}
	exported := &GlobalVar{Name: "value", Value: eval.Int(1), Span: span, Order: []string{"value"}, Vars: map[string][]eval.Value{"value": {eval.Int(1)}}, VersionID: "v1"}
	original.LocalExportsByName["value"] = exported
	original.ExportsByName["value"] = exported
	original.Bindings = []*GlobalBinding{binding}
	original.BindingsByName["value"] = binding
	original.Env["value"] = eval.Int(1)
	original.DoBlocks = []ast.DoBlock{{Name: "run", Span: span}}
	original.StepOrder = []string{"run"}
	original.Namespaces["inner"] = &Namespace{Name: "inner", Members: []string{"inner.value"}, Bindings: []string{"inner.value"}, Steps: []string{"inner.run"}}

	prefixed := prefixModuleScope(original, "mod")
	if _, ok := prefixed.ExportsByName["mod.value"]; !ok {
		t.Fatalf("expected prefixed export name, got %#v", prefixed.ExportsByName)
	}
	if _, ok := prefixed.BindingsByName["mod.value"]; !ok {
		t.Fatalf("expected prefixed binding name, got %#v", prefixed.BindingsByName)
	}
	if !reflect.DeepEqual(prefixed.Bindings[0].DependsOn, []string{"mod.dep"}) {
		t.Fatalf("expected prefixed dependency names, got %#v", prefixed.Bindings[0].DependsOn)
	}
	if prefixed.Bindings[0].PublicName != "mod.value" || prefixed.Bindings[0].VersionID != "v1" {
		t.Fatalf("expected public name prefix and version preservation, got %#v", prefixed.Bindings[0])
	}
	if prefixed.Namespaces["mod"] == nil || prefixed.Namespaces["mod.inner"] == nil {
		t.Fatalf("expected prefixed namespaces, got %#v", prefixed.Namespaces)
	}
	if !reflect.DeepEqual(prefixed.StepOrder, []string{"mod.run"}) {
		t.Fatalf("unexpected prefixed step order: %#v", prefixed.StepOrder)
	}
	if !eval.Equal(prefixed.Env["mod.value"], eval.Int(1)) {
		t.Fatalf("unexpected prefixed env: %#v", prefixed.Env)
	}
}

func TestPrefixModuleScopeRewritesWithItemRoots(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	original := emptyModuleScope()
	original.DoBlocks = []ast.DoBlock{{
		Name: "run",
		WithItems: []ast.WithItem{
			{
				Expr: ast.IndexExpr{
					Base:  ast.IdentExpr{Name: "cases", Span: span},
					Items: []ast.Expr{ast.StringExpr{Value: "x", Span: span}},
					Span:  span,
				},
				Alias: "short",
				Span:  span,
			},
			{
				Expr: ast.QualifiedIdentExpr{Namespace: "lib", Name: "value", Span: span},
				Span: span,
			},
		},
		Span: span,
	}}
	original.StepOrder = []string{"run"}

	prefixed := prefixModuleScope(original, "mod")
	if len(prefixed.DoBlocks) != 1 || len(prefixed.DoBlocks[0].WithItems) != 2 {
		t.Fatalf("unexpected prefixed do blocks: %#v", prefixed.DoBlocks)
	}
	first, ok := prefixed.DoBlocks[0].WithItems[0].Expr.(ast.IndexExpr)
	if !ok {
		t.Fatalf("expected first with item to remain an index expression, got %T", prefixed.DoBlocks[0].WithItems[0].Expr)
	}
	base, ok := first.Base.(ast.QualifiedIdentExpr)
	if !ok || base.Namespace != "mod" || base.Name != "cases" {
		t.Fatalf("unexpected prefixed index base: %#v", first.Base)
	}
	if _, ok := first.Items[0].(ast.StringExpr); !ok {
		t.Fatalf("expected projection key to remain unprefixed, got %#v", first.Items[0])
	}
	if prefixed.DoBlocks[0].WithItems[0].Alias != "short" {
		t.Fatalf("expected with alias to be preserved")
	}
	second, ok := prefixed.DoBlocks[0].WithItems[1].Expr.(ast.QualifiedIdentExpr)
	if !ok || second.Namespace != "mod.lib" || second.Name != "value" {
		t.Fatalf("unexpected prefixed qualified expression: %#v", prefixed.DoBlocks[0].WithItems[1].Expr)
	}
}

func TestPrefixWithExprRootLeavesNonRootExpressionsUnchanged(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	expr := ast.StringExpr{Value: "literal", Span: span}
	got := prefixWithExprRoot(expr, "mod")
	if got != expr {
		t.Fatalf("expected non-root expression to remain unchanged, got %#v", got)
	}
	if got := prefixWithItems(nil, "mod"); len(got) != 0 {
		t.Fatalf("expected empty with item list, got %#v", got)
	}
}

func TestMergeModuleScope(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	existingBinding := &GlobalBinding{Name: "a.value", Value: eval.Int(1), Vars: map[string][]eval.Value{"a.value": {eval.Int(1)}}, BaseVars: map[string][]eval.Value{"a.value": {eval.Int(1)}}, Order: []string{"a.value"}, Span: span}
	newBinding := &GlobalBinding{Name: "a.other", Value: eval.Int(2), Vars: map[string][]eval.Value{"a.other": {eval.Int(2)}}, BaseVars: map[string][]eval.Value{"a.other": {eval.Int(2)}}, Order: []string{"a.other"}, Span: span}
	existingExport := &GlobalVar{Name: "a.value", Value: eval.Int(1), Span: span, Order: []string{"a.value"}, Vars: map[string][]eval.Value{"a.value": {eval.Int(1)}}}
	newExport := &GlobalVar{Name: "a.other", Value: eval.Int(2), Span: span, Order: []string{"a.other"}, Vars: map[string][]eval.Value{"a.other": {eval.Int(2)}}}

	dst := emptyModuleScope()
	dst.ExportsByName["a.value"] = existingExport
	dst.Bindings = []*GlobalBinding{existingBinding}
	dst.BindingsByName["a.value"] = existingBinding
	dst.Env["a.value"] = eval.Int(1)
	dst.DoBlocks = []ast.DoBlock{{Name: "a.run", Span: span}}
	dst.StepOrder = []string{"a.run"}
	dst.Namespaces["a"] = &Namespace{Name: "a", Members: []string{"a.value"}, Bindings: []string{"a.value"}, Steps: []string{"a.run"}}

	src := emptyModuleScope()
	src.ExportsByName["a.value"] = existingExport
	src.ExportsByName["a.other"] = newExport
	src.Bindings = []*GlobalBinding{existingBinding, newBinding}
	src.BindingsByName["a.value"] = existingBinding
	src.BindingsByName["a.other"] = newBinding
	src.Env["a.other"] = eval.Int(2)
	src.DoBlocks = []ast.DoBlock{{Name: "a.run", Span: span}, {Name: "a.extra", Span: span}}
	src.StepOrder = []string{"a.run", "a.extra"}
	src.Namespaces["a"] = &Namespace{Name: "a", Members: []string{"a.value", "a.other"}, Bindings: []string{"a.value", "a.other"}, Steps: []string{"a.run", "a.extra"}}

	mergeModuleScope(dst, src)
	if len(dst.ExportsByName) != 2 || dst.ExportsByName["a.other"] == nil {
		t.Fatalf("expected mergeModuleScope to add exports, got %#v", dst.ExportsByName)
	}
	if len(dst.Bindings) != 2 || dst.BindingsByName["a.other"] == nil {
		t.Fatalf("expected mergeModuleScope to add only new bindings, got %#v", dst.BindingsByName)
	}
	if len(dst.DoBlocks) != 2 || dst.DoBlocks[1].Name != "a.extra" {
		t.Fatalf("expected mergeModuleScope to append only new do blocks, got %#v", dst.DoBlocks)
	}
	if !reflect.DeepEqual(dst.Namespaces["a"].Members, []string{"a.value", "a.other"}) || !reflect.DeepEqual(dst.Namespaces["a"].Bindings, []string{"a.value", "a.other"}) || !reflect.DeepEqual(dst.Namespaces["a"].Steps, []string{"a.run", "a.extra"}) {
		t.Fatalf("unexpected merged namespace: %#v", dst.Namespaces["a"])
	}
}
