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

func TestBuildEntryModuleScopeAndCompileModule(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	entryRef := imports.ModuleRef{ID: "entry", Label: "entry.jbs"}
	childRef := imports.ModuleRef{ID: "child", Label: "child.jbs"}
	aRef := imports.ModuleRef{ID: "a", Label: "a.jbs"}
	zRef := imports.ModuleRef{ID: "z", Label: "z.jbs"}
	loadRes := &imports.LoadResult{
		Entry: entryRef,
		Modules: map[string]*imports.ModuleInfo{
			entryRef.ID: {
				Ref: entryRef,
				Program: ast.Program{File: entryRef.Label, Stmts: []ast.Stmt{
					ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "a", Span: span}, Alias: "a", Span: span},
					ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "z", Span: span}, Alias: "z", Span: span},
				}},
				Uses: []imports.ResolvedUse{
					{Kind: imports.UseNamespace, Alias: "a", Source: aRef, Span: span, Index: 0},
					{Kind: imports.UseNamespace, Alias: "z", Source: zRef, Span: span, Index: 1},
				},
			},
			childRef.ID: {
				Ref: childRef,
				Program: ast.Program{File: childRef.Label, Stmts: []ast.Stmt{
					ast.GlobalAssign{Name: "child_value", Op: ast.AssignEq, Expr: numberExpr(span, 3), Span: span},
					ast.SubmitBlock{Name: "child_submit", Span: span},
				}},
			},
			aRef.ID: {
				Ref: aRef,
				Program: ast.Program{File: aRef.Label, Stmts: []ast.Stmt{
					ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "child", Span: span}, Alias: "child", Span: span},
					ast.GlobalAssign{Name: "a_value", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span},
					ast.DoBlock{Name: "run", Span: span},
				}},
				Uses: []imports.ResolvedUse{{Kind: imports.UseNamespace, Alias: "child", Source: childRef, Span: span, Index: 0}},
			},
			zRef.ID: {
				Ref: zRef,
				Program: ast.Program{File: zRef.Label, Stmts: []ast.Stmt{
					ast.GlobalAssign{Name: "z_value", Op: ast.AssignEq, Expr: numberExpr(span, 2), Span: span},
					ast.DoBlock{Name: "run", Span: span},
				}},
			},
		},
	}

	nilScope := buildEntryModuleScope(nil, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{})
	if len(nilScope.Bindings) != 0 || len(nilScope.Env) != 0 {
		t.Fatalf("expected nil load result to produce empty module scope, got %#v", nilScope)
	}

	diags := &diag.Diagnostics{}
	scope := buildEntryModuleScope(loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !reflect.DeepEqual(scope.StepOrder, []string{"a.child.child_submit", "a.run", "z.run"}) {
		t.Fatalf("unexpected prefixed step order: %#v", scope.StepOrder)
	}
	if _, ok := scope.BindingsByName["a.a_value"]; !ok {
		t.Fatalf("expected prefixed binding a.a_value, got %#v", scope.BindingsByName)
	}
	if _, ok := scope.BindingsByName["a.child.child_value"]; !ok {
		t.Fatalf("expected nested prefixed binding a.child.child_value, got %#v", scope.BindingsByName)
	}
	if _, ok := scope.BindingsByName["z.z_value"]; !ok {
		t.Fatalf("expected prefixed binding z.z_value, got %#v", scope.BindingsByName)
	}
	if scope.Namespaces["a"] == nil || scope.Namespaces["a.child"] == nil || scope.Namespaces["z"] == nil {
		t.Fatalf("expected namespace entries for imported modules, got %#v", scope.Namespaces)
	}
	if !eval.Equal(scope.Env["a.a_value"], eval.Int(1)) || !eval.Equal(scope.Env["a.child.child_value"], eval.Int(3)) || !eval.Equal(scope.Env["z.z_value"], eval.Int(2)) {
		t.Fatalf("unexpected scope env values: %#v", scope.Env)
	}

	cache := map[string]*moduleScope{}
	root0 := compileModule(aRef, loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{}, cache, map[string]bool{})
	root0.Bindings[0].Name = "mutated"
	root0.StepOrder[0] = "mutated"
	root0.Env["a_value"] = eval.Int(99)
	root1 := compileModule(aRef, loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{}, cache, map[string]bool{})
	if _, ok := root1.BindingsByName["a_value"]; !ok {
		t.Fatalf("expected cached module clone to preserve binding names, got %#v", root1.BindingsByName)
	}
	if !reflect.DeepEqual(root1.StepOrder, []string{"child.child_submit", "run"}) {
		t.Fatalf("expected cached module clone to preserve step order, got %#v", root1.StepOrder)
	}
	if !eval.Equal(root1.Env["a_value"], eval.Int(1)) {
		t.Fatalf("expected cached module clone to preserve env values, got %#v", root1.Env)
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

func TestCompileModuleUsesSharedGlobalPlan(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ref := imports.ModuleRef{ID: "dep", Label: "dep.jbs"}
	loadRes := &imports.LoadResult{
		Entry: ref,
		Modules: map[string]*imports.ModuleInfo{
			ref.ID: {
				Ref: ref,
				Program: ast.Program{
					File: ref.Label,
					Stmts: []ast.Stmt{
						ast.GlobalAssign{
							Name: "x",
							Expr: ast.BinaryExpr{
								Left:  ast.IdentExpr{Name: "y", Span: span},
								Op:    "+",
								Right: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
								Span:  span,
							},
							Span: span,
						},
						ast.GlobalAssign{
							Name: "y",
							Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
							Span: span,
						},
						ast.GlobalAssign{
							Name: "y",
							Op:   ast.AssignPlusEq,
							Expr: ast.NumberExpr{Int: true, IntValue: 2, Raw: "2", Span: span},
							Span: span,
						},
					},
				},
			},
		},
	}
	unit := compileModule(ref, loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, &diag.Diagnostics{}, map[string]*moduleScope{}, map[string]bool{})
	if !eval.Equal(unit.Env["x"], eval.Int(2)) || !eval.Equal(unit.Env["y"], eval.Int(3)) {
		t.Fatalf("unexpected planned module env: %#v", unit.Env)
	}
	if unit.LocalBindingsByName["x"] == nil || unit.LocalBindingsByName["y"] == nil {
		t.Fatalf("expected compiled local bindings from shared plan, got %#v", unit.LocalBindingsByName)
	}
	if !reflect.DeepEqual(unit.LocalBindingsByName["x"].DependsOn, []string{"y"}) {
		t.Fatalf("expected x dependency metadata to be preserved, got %#v", unit.LocalBindingsByName["x"])
	}
}
