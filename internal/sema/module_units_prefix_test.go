package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestPrefixModuleUnitAndHelpers(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	original := &moduleUnit{
		Bindings: []*GlobalBinding{
			{
				Name:      "value",
				Value:     eval.Int(1),
				Shape:     BindingScalar,
				Order:     []string{"value"},
				Vars:      map[string][]eval.Value{"value": {eval.Int(1)}},
				BaseVars:  map[string][]eval.Value{"value": {eval.Int(1)}},
				Origins:   map[string]diag.Span{"value": span},
				Modes:     map[string]string{"value": "shell"},
				Rows:      []eval.Row{{Values: map[string]eval.Cell{"value": {Value: eval.Int(1), Origin: span}}}},
				Span:      span,
				DependsOn: []string{"dep", ""},
			},
		},
		BindingsByName: map[string]*GlobalBinding{},
		DoBlocks: []ast.DoBlock{
			{Name: "run", After: []string{"prep"}, WithItems: []ast.WithItem{{Name: "local", Span: span}, {Name: "x", From: "src", Span: span}, {SourceExpr: "expr", SourceSlice: []string{"a"}, Span: span}}, Span: span},
		},
		Submits: []ast.SubmitBlock{
			{Name: "submit", After: []string{"run"}, UseNames: []string{"helper"}, WithItems: []ast.WithItem{{Name: "y", From: "src", Span: span}}, Span: span},
		},
		StepOrder: []string{"run", "submit"},
		Namespaces: map[string]*Namespace{
			"inner": {Name: "inner", Bindings: []string{"value"}, Steps: []string{"run"}},
		},
		Env: map[string]eval.Value{"value": eval.Int(1)},
	}
	original.BindingsByName["value"] = original.Bindings[0]

	cloned := prefixModuleUnit(original, " ")
	cloned.Bindings[0].Name = "changed"
	cloned.StepOrder[0] = "changed"
	if original.Bindings[0].Name != "value" || original.StepOrder[0] != "run" {
		t.Fatalf("expected blank-prefix branch to return an independent clone, got original=%#v", original)
	}

	prefixed := prefixModuleUnit(original, "mod")
	if _, ok := prefixed.BindingsByName["mod.value"]; !ok {
		t.Fatalf("expected prefixed binding name, got %#v", prefixed.BindingsByName)
	}
	if !reflect.DeepEqual(prefixed.Bindings[0].DependsOn, []string{"mod.dep"}) {
		t.Fatalf("expected prefixed dependency names, got %#v", prefixed.Bindings[0].DependsOn)
	}
	if prefixed.DoBlocks[0].Name != "mod.run" || !reflect.DeepEqual(prefixed.DoBlocks[0].After, []string{"mod.prep"}) {
		t.Fatalf("unexpected prefixed do block: %#v", prefixed.DoBlocks[0])
	}
	if prefixed.Submits[0].Name != "mod.submit" || !reflect.DeepEqual(prefixed.Submits[0].UseNames, []string{"mod.helper"}) {
		t.Fatalf("unexpected prefixed submit block: %#v", prefixed.Submits[0])
	}
	if prefixed.DoBlocks[0].WithItems[0].Name != "mod.local" || prefixed.DoBlocks[0].WithItems[1].From != "mod.src" || prefixed.DoBlocks[0].WithItems[2].SourceExpr != "mod.expr" {
		t.Fatalf("unexpected prefixed with-items: %#v", prefixed.DoBlocks[0].WithItems)
	}
	if prefixed.Namespaces["mod"] == nil || prefixed.Namespaces["mod.inner"] == nil {
		t.Fatalf("expected prefixed namespaces, got %#v", prefixed.Namespaces)
	}
	if !containsStepName(prefixed.DoBlocks, "mod.run") || containsStepName(prefixed.DoBlocks, "missing") {
		t.Fatalf("unexpected containsStepName result for %#v", prefixed.DoBlocks)
	}
	if !containsSubmitName(prefixed.Submits, "mod.submit") || containsSubmitName(prefixed.Submits, "missing") {
		t.Fatalf("unexpected containsSubmitName result for %#v", prefixed.Submits)
	}
	if !reflect.DeepEqual(prefixNames("mod", []string{"a", "", "b"}), []string{"mod.a", "mod.b"}) {
		t.Fatalf("unexpected prefixed names")
	}
}

func TestMergeAndIntegrateModuleUnit(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	existingBinding := &GlobalBinding{Name: "a.value", Value: eval.Int(1), Shape: BindingScalar, Span: span}
	newBinding := &GlobalBinding{Name: "a.other", Value: eval.Int(2), Shape: BindingScalar, Span: span}
	dst := &moduleUnit{
		Bindings:       []*GlobalBinding{existingBinding},
		BindingsByName: map[string]*GlobalBinding{"a.value": existingBinding},
		DoBlocks:       []ast.DoBlock{{Name: "a.run", Span: span}},
		Submits:        []ast.SubmitBlock{{Name: "a.submit", Span: span}},
		StepOrder:      []string{"a.run", "a.submit"},
		Namespaces: map[string]*Namespace{
			"a": {Name: "a", Bindings: []string{"a.value"}, Steps: []string{"a.run"}},
		},
		Env: map[string]eval.Value{"a.value": eval.Int(1)},
	}
	src := &moduleUnit{
		Bindings:       []*GlobalBinding{existingBinding, newBinding},
		BindingsByName: map[string]*GlobalBinding{"a.value": existingBinding, "a.other": newBinding},
		DoBlocks:       []ast.DoBlock{{Name: "a.run", Span: span}, {Name: "a.extra", Span: span}},
		Submits:        []ast.SubmitBlock{{Name: "a.submit", Span: span}, {Name: "a.submit_extra", Span: span}},
		StepOrder:      []string{"a.run", "a.extra", "a.submit_extra"},
		Namespaces: map[string]*Namespace{
			"a": {Name: "a", Bindings: []string{"a.value", "a.other"}, Steps: []string{"a.run", "a.extra"}},
		},
		Env: map[string]eval.Value{"a.other": eval.Int(2)},
	}

	mergeModuleUnit(dst, src)
	if len(dst.Bindings) != 2 || dst.BindingsByName["a.other"] != newBinding {
		t.Fatalf("expected mergeModuleUnit to add only new bindings, got %#v", dst.BindingsByName)
	}
	if len(dst.DoBlocks) != 2 || dst.DoBlocks[1].Name != "a.extra" {
		t.Fatalf("expected mergeModuleUnit to append only new do blocks, got %#v", dst.DoBlocks)
	}
	if len(dst.Submits) != 2 || dst.Submits[1].Name != "a.submit_extra" {
		t.Fatalf("expected mergeModuleUnit to append only new submit blocks, got %#v", dst.Submits)
	}
	if !reflect.DeepEqual(dst.StepOrder, []string{"a.run", "a.submit", "a.extra", "a.submit_extra"}) {
		t.Fatalf("unexpected merged step order: %#v", dst.StepOrder)
	}
	if !reflect.DeepEqual(dst.Namespaces["a"].Bindings, []string{"a.value", "a.other"}) || !reflect.DeepEqual(dst.Namespaces["a"].Steps, []string{"a.run", "a.extra", "a.submit_extra"}) {
		t.Fatalf("unexpected merged namespace: %#v", dst.Namespaces["a"])
	}
	if !eval.Equal(dst.Env["a.other"], eval.Int(2)) {
		t.Fatalf("expected merged env value for new binding, got %#v", dst.Env)
	}

	res := &Result{
		Bindings:       []*GlobalBinding{existingBinding},
		BindingsByName: map[string]*GlobalBinding{"a.value": existingBinding},
		Namespaces:     map[string]*Namespace{"a": {Name: "a", Bindings: []string{"a.value"}, Steps: []string{"a.run"}}},
		DoBlocks:       []ast.DoBlock{{Name: "a.run", Span: span}},
		Submits:        []ast.SubmitBlock{{Name: "a.submit", Span: span}},
		StepOrder:      []string{"a.run", "a.submit"},
	}
	integrateModuleUnit(nil, src)
	integrateModuleUnit(res, nil)
	integrateModuleUnit(res, src)
	if len(res.Bindings) != 2 || res.BindingsByName["a.other"] != newBinding {
		t.Fatalf("expected integrateModuleUnit to add only new bindings, got %#v", res.BindingsByName)
	}
	if !reflect.DeepEqual(res.StepOrder, []string{"a.run", "a.submit", "a.extra", "a.submit_extra"}) {
		t.Fatalf("unexpected integrated step order: %#v", res.StepOrder)
	}
	if !reflect.DeepEqual(res.Namespaces["a"].Bindings, []string{"a.value", "a.other"}) || !reflect.DeepEqual(res.Namespaces["a"].Steps, []string{"a.run", "a.extra"}) {
		t.Fatalf("unexpected integrated namespace: %#v", res.Namespaces["a"])
	}
}
