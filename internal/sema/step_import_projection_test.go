package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestImportsFromStepPlan(t *testing.T) {
	if got := importsFromStepPlan(nil); len(got) != 0 {
		t.Fatalf("expected empty imports for nil plan, got %#v", got)
	}

	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	plan := &StepScopePlan{
		Effective: map[string]VisibleBinding{
			"a": {Name: "a", SourceVar: "", Source: "srcA", Span: span},
			"b": {Name: "b", SourceVar: "srcB", Source: "srcB", Span: span},
		},
	}
	got := importsFromStepPlan(plan)
	if got["a"][0].SourceVar != "a" || got["a"][0].Source != "srcA" {
		t.Fatalf("expected source-var fallback for a, got %#v", got["a"])
	}
	if got["b"][0].SourceVar != "srcB" || got["b"][0].Source != "srcB" {
		t.Fatalf("unexpected imported var metadata for b, got %#v", got["b"])
	}
}

func TestExplicitImportsAndVisibleSpansFromStepPlan(t *testing.T) {
	span0 := diag.NewSpan("srcA.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	span1 := diag.NewSpan("srcB.jbs", diag.NewPos(1, 2, 1), diag.NewPos(1, 2, 2))
	fallback := diag.NewSpan("step.jbs", diag.NewPos(2, 3, 1), diag.NewPos(2, 3, 2))
	bindings := map[string]*GlobalBinding{
		"srcA": bindingWithOrigins("srcA", []string{"a", "b"}, map[string][]eval.Value{
			"a": {eval.Int(1)},
			"b": {eval.Int(2)},
		}, map[string]diag.Span{
			"a": span0,
		}),
		"srcB": bindingWithOrigins("srcB", nil, map[string][]eval.Value{
			"c": {eval.Int(3)},
		}, map[string]diag.Span{
			"c": span1,
		}),
	}
	plan := &StepScopePlan{
		ExplicitDelta: []ScopeImport{
			{Source: "srcA", Full: true, Span: fallback},
			{Source: "srcB", Visible: "alias_c", SourceVar: "c", Span: fallback},
			{Source: "missing", Full: true, Span: fallback},
		},
		Effective: map[string]VisibleBinding{
			"a":       {Name: "a", Source: "srcA", Span: fallback},
			"alias_c": {Name: "alias_c", Source: "srcB", SourceVar: "c", Span: fallback},
			"z":       {Name: "z", Source: "missing", Span: fallback},
		},
	}

	explicit := explicitImportsFromStepPlan(plan, bindings)
	if !reflect.DeepEqual(explicit["a"], []importedVar{{Name: "a", SourceVar: "a", Source: "srcA", Span: fallback}}) {
		t.Fatalf("unexpected full explicit import for a: %#v", explicit["a"])
	}
	if !reflect.DeepEqual(explicit["b"], []importedVar{{Name: "b", SourceVar: "b", Source: "srcA", Span: fallback}}) {
		t.Fatalf("unexpected full explicit import for b: %#v", explicit["b"])
	}
	if !reflect.DeepEqual(explicit["alias_c"], []importedVar{{Name: "alias_c", SourceVar: "c", Source: "srcB", Span: fallback}}) {
		t.Fatalf("unexpected explicit alias import: %#v", explicit["alias_c"])
	}

	spans := visibleSpansFromStepPlan(plan, bindings)
	if spans["a"] != span0 {
		t.Fatalf("expected source origin span for a, got %#v", spans["a"])
	}
	if spans["alias_c"] != span1 {
		t.Fatalf("expected source origin span for alias_c, got %#v", spans["alias_c"])
	}
	if spans["z"] != fallback {
		t.Fatalf("expected fallback span for missing source, got %#v", spans["z"])
	}
}

func TestAddEnvFromStepPlan(t *testing.T) {
	env := map[string]eval.Value{"pre": eval.String("keep")}
	addEnvFromStepPlan(env, nil, nil)
	if !eval.Equal(env["pre"], eval.String("keep")) {
		t.Fatalf("nil plan should not mutate env, got %#v", env)
	}

	plan := &StepScopePlan{
		Effective: map[string]VisibleBinding{
			"a": {Name: "a", Source: "srcA", Span: diag.Span{}},
			"b": {Name: "b", Source: "srcA", SourceVar: "srcB", Span: diag.Span{}},
			"c": {Name: "c", Source: "srcA", SourceVar: "missing", Span: diag.Span{}},
			"d": {Name: "d", Source: "missing", Span: diag.Span{}},
		},
	}
	bindings := map[string]*GlobalBinding{
		"srcA": bindingWithOrigins("srcA", nil, map[string][]eval.Value{
			"a":    {eval.Int(1), eval.Int(2)},
			"srcB": {eval.String("one")},
		}, nil),
	}

	addEnvFromStepPlan(env, plan, bindings)
	if !eval.Equal(env["a"], eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("expected list value for a, got %#v", env["a"])
	}
	if !eval.Equal(env["b"], eval.String("one")) {
		t.Fatalf("expected scalar value for b, got %#v", env["b"])
	}
	if _, ok := env["c"]; ok {
		t.Fatalf("did not expect missing source var to be added, got %#v", env["c"])
	}
	if _, ok := env["d"]; ok {
		t.Fatalf("did not expect missing source to be added, got %#v", env["d"])
	}
}

func TestResolveImportedVars(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 10))
	bindings := map[string]*GlobalBinding{
		"p": bindingWithOrigins("p", []string{"a", "b"}, map[string][]eval.Value{
			"a": {eval.Int(1)},
			"b": {eval.Int(2)},
		}, nil),
		"x": bindingWithOrigins("x", []string{"v"}, map[string][]eval.Value{
			"v": {eval.String("fallback")},
		}, nil),
	}
	items := []ast.WithItem{
		{Source: "p", Span: span},
		{Source: "p", Span: span},
		{Source: "p", Selectors: []string{"a"}, Span: span},
		{Source: "missing", Selectors: []string{"z"}, Span: span},
		{Source: "missing_full", Span: span},
		{Source: "p", Selectors: []string{"b", "missing"}, Span: span},
	}

	got := resolveImportedVars(items, bindings)
	if len(got["a"]) != 1 {
		t.Fatalf("expected dedup for repeated import of a, got %#v", got["a"])
	}
	if len(got["b"]) != 1 || got["b"][0].SourceVar != "b" || got["b"][0].Source != "p" {
		t.Fatalf("expected source-slice/full import for b, got %#v", got["b"])
	}
	if _, ok := got["z"]; ok {
		t.Fatalf("did not expect unknown source to be imported, got %#v", got["z"])
	}
	if _, ok := got["missing_full"]; ok {
		t.Fatalf("did not expect unknown full import to be imported, got %#v", got["missing_full"])
	}
}
