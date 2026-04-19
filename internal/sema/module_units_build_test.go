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
				Ref:     entryRef,
				BaseDir: "/mods/entry",
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
				Ref:     childRef,
				BaseDir: "/mods/child",
				Program: ast.Program{File: childRef.Label, Stmts: []ast.Stmt{
					ast.GlobalAssign{Name: "child_value", Op: ast.AssignEq, Expr: numberExpr(span, 3), Span: span},
					ast.SubmitBlock{Name: "child_submit", Span: span},
				}},
			},
			aRef.ID: {
				Ref:     aRef,
				BaseDir: "/mods/a",
				Program: ast.Program{File: aRef.Label, Stmts: []ast.Stmt{
					ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "child", Span: span}, Alias: "child", Span: span},
					ast.GlobalAssign{Name: "a_value", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span},
					ast.DoBlock{Name: "run", Span: span},
				}},
				Uses: []imports.ResolvedUse{{Kind: imports.UseNamespace, Alias: "child", Source: childRef, Span: span, Index: 0}},
			},
			zRef.ID: {
				Ref:     zRef,
				BaseDir: "/mods/z",
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
	if _, ok := scope.ExportsByName["a.a_value"]; !ok {
		t.Fatalf("expected prefixed export a.a_value, got %#v", scope.ExportsByName)
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
	if !reflect.DeepEqual(scope.BaseDirByFile, map[string]string{
		entryRef.Label: "/mods/entry",
		childRef.Label: "/mods/child",
		aRef.Label:     "/mods/a",
		zRef.Label:     "/mods/z",
	}) {
		t.Fatalf("unexpected base-dir mapping: %#v", scope.BaseDirByFile)
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
	if root1.BaseDirByFile[aRef.Label] != "/mods/a" || root1.BaseDirByFile[childRef.Label] != "/mods/child" {
		t.Fatalf("expected cached module clone to preserve base-dir mapping, got %#v", root1.BaseDirByFile)
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

func TestBuildEntryModuleScopeKeepsOnlyEntryTopLevelExprResults(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	entryRef := imports.ModuleRef{ID: "entry", Label: "entry.jbs"}
	childRef := imports.ModuleRef{ID: "child", Label: "child.jbs"}
	loadRes := &imports.LoadResult{
		Entry: entryRef,
		Modules: map[string]*imports.ModuleInfo{
			entryRef.ID: {
				Ref: entryRef,
				Program: ast.Program{File: entryRef.Label, Stmts: []ast.Stmt{
					ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "child", Span: span}, Alias: "child", Span: span},
					ast.ExprStmt{
						Expr: ast.QualifiedIdentExpr{Namespace: "child", Name: "value", Span: span},
						Span: span,
					},
				}},
				Uses: []imports.ResolvedUse{
					{Kind: imports.UseNamespace, Alias: "child", Source: childRef, Span: span, Index: 0},
				},
			},
			childRef.ID: {
				Ref: childRef,
				Program: ast.Program{File: childRef.Label, Stmts: []ast.Stmt{
					ast.GlobalAssign{Name: "value", Op: ast.AssignEq, Expr: numberExpr(span, 7), Span: span},
					ast.ExprStmt{Expr: ast.IdentExpr{Name: "value", Span: span}, Span: span},
				}},
			},
		},
	}

	diags := &diag.Diagnostics{}
	scope := buildEntryModuleScope(loadRes, map[string]eval.Value{"builtin": eval.Int(9)}, diags)
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(scope.TopLevelExprs) != 1 {
		t.Fatalf("expected only entry expr results, got %#v", scope.TopLevelExprs)
	}
	if !eval.Equal(scope.TopLevelExprs[0].Value, eval.Int(7)) {
		t.Fatalf("unexpected entry expr result: %#v", scope.TopLevelExprs[0])
	}
}

func TestCompileModuleHandlesFunctionValuedGlobals(t *testing.T) {
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
						ast.GlobalAssign{Name: "base", Op: ast.AssignEq, Expr: numberExpr(span, 40), Span: span},
						ast.GlobalAssign{
							Name: "mk",
							Op:   ast.AssignEq,
							Expr: ast.FunctionExpr{
								Params: []ast.FuncParam{{Name: "delta", Span: span}},
								Body: []ast.FuncBodyStmt{
									ast.ExprStmt{
										Expr: ast.FunctionExpr{
											Params: []ast.FuncParam{{Name: "x", Span: span}},
											Body: []ast.FuncBodyStmt{
												ast.ExprStmt{
													Expr: ast.BinaryExpr{
														Left: ast.BinaryExpr{
															Left:  ast.IdentExpr{Name: "x", Span: span},
															Op:    "+",
															Right: ast.IdentExpr{Name: "delta", Span: span},
															Span:  span,
														},
														Op:    "+",
														Right: ast.IdentExpr{Name: "base", Span: span},
														Span:  span,
													},
													Span: span,
												},
											},
											Span: span,
										},
										Span: span,
									},
								},
								Span: span,
							},
							Span: span,
						},
						ast.GlobalAssign{
							Name: "inc",
							Op:   ast.AssignEq,
							Expr: ast.CallExpr{
								Callee: ast.IdentExpr{Name: "mk", Span: span},
								Args:   ast.PosCallArgs(numberExpr(span, 1)),
								Span:   span,
							},
							Span: span,
						},
						ast.GlobalAssign{
							Name: "value",
							Op:   ast.AssignEq,
							Expr: ast.CallExpr{
								Callee: ast.IdentExpr{Name: "inc", Span: span},
								Args:   ast.PosCallArgs(numberExpr(span, 1)),
								Span:   span,
							},
							Span: span,
						},
					},
				},
			},
		},
	}

	diags := &diag.Diagnostics{}
	scope := compileModule(ref, loadRes, nil, diags, map[string]*moduleScope{}, map[string]bool{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if scope.GlobalVarByName["inc"] == nil || scope.GlobalVarByName["inc"].Value.Kind != eval.KindFunction {
		t.Fatalf("expected function-valued global inc in module scope, got %#v", scope.GlobalVarByName["inc"])
	}
	if scope.LocalExportsByName["inc"] == nil || scope.LocalExportsByName["inc"].Value.Kind != eval.KindFunction {
		t.Fatalf("expected function-valued export inc in module scope, got %#v", scope.LocalExportsByName["inc"])
	}
	if _, ok := scope.LocalBindingsByName["inc"]; ok {
		t.Fatalf("did not expect function-valued global inc to become a local binding")
	}
	if scope.LocalBindingsByName["value"] == nil || !eval.Equal(scope.LocalBindingsByName["value"].Value, eval.Int(42)) {
		t.Fatalf("expected value binding 42, got %#v", scope.LocalBindingsByName["value"])
	}
	if !reflect.DeepEqual(scope.LocalBindingsByName["value"].DependsOn, []string{"base", "inc", "mk"}) {
		t.Fatalf("unexpected module runtime deps for value: %#v", scope.LocalBindingsByName["value"])
	}
	if scope.Globals.Values["inc"].Kind != eval.KindFunction {
		t.Fatalf("expected function global inc to remain in scope.Globals, got %#v", scope.Globals.Values["inc"])
	}
}

func TestBuildEntryModuleScopeImportsFunctionExports(t *testing.T) {
	span := diag.NewSpan("mods.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	entryRef := imports.ModuleRef{ID: "entry", Label: "entry.jbs"}
	libRef := imports.ModuleRef{ID: "lib", Label: "lib.jbs"}
	loadRes := &imports.LoadResult{
		Entry: entryRef,
		Modules: map[string]*imports.ModuleInfo{
			entryRef.ID: {
				Ref:     entryRef,
				BaseDir: "/mods/entry",
				Program: ast.Program{File: entryRef.Label, Stmts: []ast.Stmt{
					ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "lib", Span: span}, Alias: "lib", Span: span},
					ast.UseStmt{Names: []string{"add"}, Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "lib", Span: span}, Span: span},
				}},
				Uses: []imports.ResolvedUse{
					{Kind: imports.UseNamespace, Alias: "lib", Source: libRef, Span: span, Index: 0},
					{Kind: imports.UseSelective, Names: []string{"add"}, Source: libRef, Span: span, Index: 1},
				},
			},
			libRef.ID: {
				Ref:     libRef,
				BaseDir: "/mods/lib",
				Program: ast.Program{File: libRef.Label, Stmts: []ast.Stmt{
					ast.GlobalAssign{
						Name: "base",
						Op:   ast.AssignEq,
						Expr: numberExpr(span, 40),
						Span: span,
					},
					ast.GlobalAssign{
						Name: "add",
						Op:   ast.AssignEq,
						Expr: ast.FunctionExpr{
							Params: []ast.FuncParam{{Name: "x", Span: span}, {Name: "y", Span: span}},
							Body: []ast.FuncBodyStmt{
								ast.ExprStmt{
									Expr: ast.BinaryExpr{
										Left: ast.BinaryExpr{
											Left:  ast.IdentExpr{Name: "x", Span: span},
											Op:    "+",
											Right: ast.IdentExpr{Name: "y", Span: span},
											Span:  span,
										},
										Op:    "+",
										Right: ast.IdentExpr{Name: "base", Span: span},
										Span:  span,
									},
									Span: span,
								},
							},
							Span: span,
						},
						Span: span,
					},
				}},
			},
		},
	}

	diags := &diag.Diagnostics{}
	scope := buildEntryModuleScope(loadRes, nil, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if scope.ExportsByName["add"] == nil || scope.ExportsByName["add"].Value.Kind != eval.KindFunction {
		t.Fatalf("expected selective imported function export add, got %#v", scope.ExportsByName["add"])
	}
	if scope.ExportsByName["lib.add"] == nil || scope.ExportsByName["lib.add"].Value.Kind != eval.KindFunction {
		t.Fatalf("expected namespaced imported function export lib.add, got %#v", scope.ExportsByName["lib.add"])
	}
	if scope.Namespaces["lib"] == nil || !reflect.DeepEqual(directNamespaceMembers("lib", scope.Namespaces["lib"]), []string{"add", "base"}) {
		t.Fatalf("expected namespace members for lib, got %#v", scope.Namespaces["lib"])
	}
	if call := eval.EvalExprWithOptions(
		ast.CallExpr{
			Callee: ast.QualifiedIdentExpr{Namespace: "lib", Name: "add", Span: span},
			Args:   ast.PosCallArgs(numberExpr(span, 1), numberExpr(span, 2)),
			Span:   span,
		},
		nil,
		diags,
		eval.ExprOptions{Context: eval.EvalCtxBindingAssign, Frame: eval.NewRootFrame(scope.Env)},
	); !eval.Equal(call, eval.Int(43)) {
		t.Fatalf("expected lib.add(1,2)=43, got %#v", call)
	}
	if call := eval.EvalExprWithOptions(
		ast.CallExpr{
			Callee: ast.IdentExpr{Name: "add", Span: span},
			Args:   ast.PosCallArgs(numberExpr(span, 1), numberExpr(span, 2)),
			Span:   span,
		},
		nil,
		diags,
		eval.ExprOptions{Context: eval.EvalCtxBindingAssign, Frame: eval.NewRootFrame(scope.Env)},
	); !eval.Equal(call, eval.Int(43)) {
		t.Fatalf("expected add(1,2)=43, got %#v", call)
	}
}
