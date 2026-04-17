package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/imports"
)

func numberExpr(span diag.Span, value int64) ast.NumberExpr {
	return ast.NumberExpr{Int: true, IntValue: value, Raw: "1", Span: span}
}

func TestBuildEntryNamespaceUnitAndRootUnit(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	childRef := imports.ModuleRef{ID: "child", Label: "child.jbs"}
	aRef := imports.ModuleRef{ID: "a", Label: "a.jbs"}
	zRef := imports.ModuleRef{ID: "z", Label: "z.jbs"}
	loadRes := &imports.LoadResult{
		Aliases: map[string]imports.ModuleRef{
			"z": zRef,
			"a": aRef,
		},
		Modules: map[string]*imports.ModuleInfo{
			childRef.ID: {
				Ref: childRef,
				Program: ast.Program{
					File: childRef.Label,
					Stmts: []ast.Stmt{
						ast.GlobalAssign{Name: "child_value", Op: ast.AssignEq, Expr: numberExpr(span, 3), Span: span},
						ast.SubmitBlock{Name: "child_submit", Span: span},
					},
				},
				Aliases: map[string]imports.ModuleRef{},
			},
			aRef.ID: {
				Ref: aRef,
				Program: ast.Program{
					File: aRef.Label,
					Stmts: []ast.Stmt{
						ast.GlobalAssign{Name: "a_value", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span},
						ast.DoBlock{Name: "run", Span: span},
					},
				},
				Aliases: map[string]imports.ModuleRef{
					"child": childRef,
				},
			},
			zRef.ID: {
				Ref: zRef,
				Program: ast.Program{
					File: zRef.Label,
					Stmts: []ast.Stmt{
						ast.GlobalAssign{Name: "z_value", Op: ast.AssignEq, Expr: numberExpr(span, 2), Span: span},
						ast.DoBlock{Name: "run", Span: span},
					},
				},
				Aliases: map[string]imports.ModuleRef{},
			},
		},
	}

	nilUnit, nilEnv := buildEntryNamespaceUnit(nil, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{})
	if len(nilUnit.Bindings) != 0 || len(nilEnv) != 0 {
		t.Fatalf("expected nil load result to produce empty namespace unit, got unit=%#v env=%#v", nilUnit, nilEnv)
	}

	diags := &diag.Diagnostics{}
	unit, env := buildEntryNamespaceUnit(loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !reflect.DeepEqual(unit.StepOrder, []string{"a.child.child_submit", "a.run", "z.run"}) {
		t.Fatalf("unexpected prefixed step order: %#v", unit.StepOrder)
	}
	if _, ok := unit.BindingsByName["a.a_value"]; !ok {
		t.Fatalf("expected prefixed binding a.a_value, got %#v", unit.BindingsByName)
	}
	if _, ok := unit.BindingsByName["a.child.child_value"]; !ok {
		t.Fatalf("expected nested prefixed binding a.child.child_value, got %#v", unit.BindingsByName)
	}
	if _, ok := unit.BindingsByName["z.z_value"]; !ok {
		t.Fatalf("expected prefixed binding z.z_value, got %#v", unit.BindingsByName)
	}
	if unit.Namespaces["a"] == nil || unit.Namespaces["a.child"] == nil || unit.Namespaces["z"] == nil {
		t.Fatalf("expected namespace entries for aliased modules, got %#v", unit.Namespaces)
	}
	if !eval.Equal(unit.Env["a.a_value"], eval.Int(1)) || !eval.Equal(unit.Env["a.child.child_value"], eval.Int(3)) || !eval.Equal(unit.Env["z.z_value"], eval.Int(2)) {
		t.Fatalf("unexpected unit env values: %#v", unit.Env)
	}
	env["a.a_value"] = eval.Int(99)
	if !eval.Equal(unit.Env["a.a_value"], eval.Int(1)) {
		t.Fatalf("expected returned env clone to be independent from unit env, got unit=%#v env=%#v", unit.Env, env)
	}

	cache := map[string]*moduleUnit{}
	root0 := buildModuleRootUnit(aRef, loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{}, cache)
	root0.Bindings[0].Name = "mutated"
	root0.StepOrder[0] = "mutated"
	root0.Env["a_value"] = eval.Int(99)
	root1 := buildModuleRootUnit(aRef, loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{}, cache)
	if _, ok := root1.BindingsByName["a_value"]; !ok {
		t.Fatalf("expected cached root unit clone to preserve original binding names, got %#v", root1.BindingsByName)
	}
	if !reflect.DeepEqual(root1.StepOrder, []string{"child.child_submit", "run"}) {
		t.Fatalf("expected cached root unit clone to preserve step order, got %#v", root1.StepOrder)
	}
	if !eval.Equal(root1.Env["a_value"], eval.Int(1)) {
		t.Fatalf("expected cached root unit clone to preserve env values, got %#v", root1.Env)
	}
}

func TestMergeValueEnvAndMergeBindingValues(t *testing.T) {
	merged := mergeValueEnv(
		map[string]eval.Value{
			"a": eval.Int(1),
			"x": eval.String("base"),
		},
		map[string]eval.Value{
			"b": eval.Int(2),
			"x": eval.String("extra"),
		},
	)
	if !eval.Equal(merged["a"], eval.Int(1)) || !eval.Equal(merged["b"], eval.Int(2)) || !eval.Equal(merged["x"], eval.String("extra")) {
		t.Fatalf("unexpected merged env: %#v", merged)
	}

	env := map[string]eval.Value{"pre": eval.String("keep")}
	mergeBindingValues(nil, map[string]*GlobalBinding{
		"x": {Value: eval.Int(1)},
	})
	mergeBindingValues(env, map[string]*GlobalBinding{
		"x":   {Value: eval.Int(1)},
		"nil": nil,
	})
	if !eval.Equal(env["pre"], eval.String("keep")) || !eval.Equal(env["x"], eval.Int(1)) {
		t.Fatalf("unexpected env after merging bindings: %#v", env)
	}
	if _, ok := env["nil"]; ok {
		t.Fatalf("did not expect nil binding to be merged, got %#v", env["nil"])
	}
}
