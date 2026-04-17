package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
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
	}
	original.Bindings = []*GlobalBinding{binding}
	original.BindingsByName["value"] = binding
	original.Env["value"] = eval.Int(1)
	original.DoBlocks = []ast.DoBlock{{Name: "run", Span: span}}
	original.Submits = []ast.SubmitBlock{{Name: "submit_extra", Span: span}}
	original.StepOrder = []string{"run", "submit_extra"}
	original.Namespaces["inner"] = &Namespace{Name: "inner", Bindings: []string{"inner.value"}, Steps: []string{"inner.run"}}

	prefixed := prefixModuleScope(original, "mod")
	if _, ok := prefixed.BindingsByName["mod.value"]; !ok {
		t.Fatalf("expected prefixed binding name, got %#v", prefixed.BindingsByName)
	}
	if !reflect.DeepEqual(prefixed.Bindings[0].DependsOn, []string{"mod.dep"}) {
		t.Fatalf("expected prefixed dependency names, got %#v", prefixed.Bindings[0].DependsOn)
	}
	if prefixed.Namespaces["mod"] == nil || prefixed.Namespaces["mod.inner"] == nil {
		t.Fatalf("expected prefixed namespaces, got %#v", prefixed.Namespaces)
	}
	if !reflect.DeepEqual(prefixed.StepOrder, []string{"mod.run", "mod.submit_extra"}) {
		t.Fatalf("unexpected prefixed step order: %#v", prefixed.StepOrder)
	}
	if !eval.Equal(prefixed.Env["mod.value"], eval.Int(1)) {
		t.Fatalf("unexpected prefixed env: %#v", prefixed.Env)
	}
}

func TestMergeModuleScope(t *testing.T) {
	span := diag.NewSpan("scope.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	existingBinding := &GlobalBinding{Name: "a.value", Value: eval.Int(1), Vars: map[string][]eval.Value{"a.value": {eval.Int(1)}}, BaseVars: map[string][]eval.Value{"a.value": {eval.Int(1)}}, Order: []string{"a.value"}, Span: span}
	newBinding := &GlobalBinding{Name: "a.other", Value: eval.Int(2), Vars: map[string][]eval.Value{"a.other": {eval.Int(2)}}, BaseVars: map[string][]eval.Value{"a.other": {eval.Int(2)}}, Order: []string{"a.other"}, Span: span}

	dst := emptyModuleScope()
	dst.Bindings = []*GlobalBinding{existingBinding}
	dst.BindingsByName["a.value"] = existingBinding
	dst.Env["a.value"] = eval.Int(1)
	dst.DoBlocks = []ast.DoBlock{{Name: "a.run", Span: span}}
	dst.Submits = []ast.SubmitBlock{{Name: "a.submit_extra", Span: span}}
	dst.StepOrder = []string{"a.run", "a.submit_extra"}
	dst.Namespaces["a"] = &Namespace{Name: "a", Bindings: []string{"a.value"}, Steps: []string{"a.run", "a.submit_extra"}}

	src := emptyModuleScope()
	src.Bindings = []*GlobalBinding{existingBinding, newBinding}
	src.BindingsByName["a.value"] = existingBinding
	src.BindingsByName["a.other"] = newBinding
	src.Env["a.other"] = eval.Int(2)
	src.DoBlocks = []ast.DoBlock{{Name: "a.run", Span: span}, {Name: "a.extra", Span: span}}
	src.Submits = []ast.SubmitBlock{{Name: "a.submit_extra", Span: span}}
	src.StepOrder = []string{"a.run", "a.extra", "a.submit_extra"}
	src.Namespaces["a"] = &Namespace{Name: "a", Bindings: []string{"a.value", "a.other"}, Steps: []string{"a.run", "a.extra", "a.submit_extra"}}

	mergeModuleScope(dst, src)
	if len(dst.Bindings) != 2 || dst.BindingsByName["a.other"] == nil {
		t.Fatalf("expected mergeModuleScope to add only new bindings, got %#v", dst.BindingsByName)
	}
	if len(dst.DoBlocks) != 2 || dst.DoBlocks[1].Name != "a.extra" {
		t.Fatalf("expected mergeModuleScope to append only new do blocks, got %#v", dst.DoBlocks)
	}
	if !reflect.DeepEqual(dst.Namespaces["a"].Bindings, []string{"a.value", "a.other"}) || !reflect.DeepEqual(dst.Namespaces["a"].Steps, []string{"a.run", "a.submit_extra", "a.extra"}) {
		t.Fatalf("unexpected merged namespace: %#v", dst.Namespaces["a"])
	}
}
