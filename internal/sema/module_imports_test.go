package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

func TestSelectiveImportsRejectNonExportedAndLocalSymbols(t *testing.T) {
	span := diag.NewSpan("entry.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	depRef := imports.ModuleRef{ID: "dep", Label: "dep.jbs"}
	child := emptyModuleScope()
	child.LocalExportsByName["ok"] = &GlobalVar{Name: "ok", Value: eval.Int(1), Span: span}
	child.Program = ast.Program{File: depRef.Label, Stmts: []ast.Stmt{
		ast.GlobalAssign{Name: "declared", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span},
		ast.DoBlock{Name: "step", Span: span},
		ast.AnalyseBlock{StepName: "report", Span: span},
	}}
	info := &imports.ModuleInfo{
		Program: ast.Program{File: "entry.jbs", Stmts: []ast.Stmt{
			ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "dep", Span: span}, Alias: "alias", Span: span},
			ast.UseStmt{Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "blank", Span: span}, Alias: " ", Span: span},
			ast.DoBlock{Name: "local_step", Span: span},
			ast.UseStmt{
				Names:  []string{"ok", "alias", "local_step", "step", "report", "missing", "declared"},
				Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "dep", Span: span},
				Span:   span,
			},
		}},
		Uses: []imports.ResolvedUse{
			{Kind: imports.UseNamespace, Alias: "alias", Source: depRef, Span: span, Index: 0},
			{Kind: imports.UseNamespace, Alias: " ", Source: depRef, Span: span, Index: 1},
			{Kind: imports.UseSelective, Names: []string{"ok", "alias", "local_step", "step", "report", "missing", "declared"}, Source: depRef, Span: span, Index: 3},
		},
	}

	diags := &diag.Diagnostics{}
	prep := prepareModuleBindings(info, map[int]*moduleScope{3: child}, diags)
	if got := prep.AcceptedImports[projectedImportDecisionKey{Index: 3, Name: "ok"}]; got == nil || got.SourceGlobal != child.LocalExportsByName["ok"] {
		t.Fatalf("expected accepted import for ok, got %#v", got)
	}
	if _, exists := prep.AcceptedImports[projectedImportDecisionKey{Index: 3, Name: "declared"}]; exists {
		t.Fatalf("did not expect declared global without export to be accepted")
	}
	if len(prep.LocalVisibleNames) != 0 {
		t.Fatalf("unexpected visible names: %#v", prep.LocalVisibleNames)
	}
	gotCodes := make([]string, 0, len(diags.Items))
	for _, item := range diags.Items {
		gotCodes = append(gotCodes, item.Code)
	}
	wantCodes := []string{string(diag.CodeE534), string(diag.CodeE534), string(diag.CodeE533), string(diag.CodeE533), string(diag.CodeE532)}
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("diagnostic codes=%#v, want %#v; diagnostics:\n%s", gotCodes, wantCodes, diags.String())
	}
}

func TestSelectiveImportsDetectNestedLocalSymbolCollisions(t *testing.T) {
	span := diag.NewSpan("symbols.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	prog := ast.Program{Stmts: []ast.Stmt{
		ast.GlobalAssign{Name: "top", Op: ast.AssignEq, Expr: numberExpr(span, 1), Span: span},
		ast.DoBlock{Name: "run", Span: span},
		ast.AnalyseBlock{StepName: "summary", Span: span},
		ast.IfStmt{
			Cond: ast.BoolExpr{Value: true, Span: span},
			Then: []ast.Stmt{
				ast.GlobalAssign{Name: "then_value", Op: ast.AssignEq, Expr: numberExpr(span, 2), Span: span},
			},
			Elifs: []ast.ElifBranch{
				{
					Cond: ast.BoolExpr{Value: false, Span: span},
					Body: []ast.Stmt{
						ast.DoBlock{Name: "elif_step", Span: span},
					},
					Span: span,
				},
			},
			Else: []ast.Stmt{
				ast.AnalyseBlock{StepName: "else_report", Span: span},
			},
			Span: span,
		},
		ast.ForStmt{
			Target:   "item",
			Iterable: ast.TupleExpr{Items: []ast.Expr{numberExpr(span, 1)}, Span: span},
			Body: []ast.Stmt{
				ast.GlobalAssign{Name: "for_body", Op: ast.AssignEq, Expr: ast.IdentExpr{Name: "item", Span: span}, Span: span},
			},
			Span: span,
		},
		ast.WhileStmt{
			Cond: ast.BoolExpr{Value: false, Span: span},
			Body: []ast.Stmt{
				ast.DoBlock{Name: "while_step", Span: span},
			},
			Span: span,
		},
	}}

	cases := []struct {
		name string
		want localSymbolKind
	}{
		{name: "", want: localSymbolNone},
		{name: " ", want: localSymbolNone},
		{name: "top", want: localSymbolGlobal},
		{name: "run", want: localSymbolDo},
		{name: "summary", want: localSymbolAnalyse},
		{name: "then_value", want: localSymbolGlobal},
		{name: "elif_step", want: localSymbolDo},
		{name: "else_report", want: localSymbolAnalyse},
		{name: "item", want: localSymbolGlobal},
		{name: "for_body", want: localSymbolGlobal},
		{name: "while_step", want: localSymbolDo},
		{name: "missing", want: localSymbolNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := moduleLocalSymbolKind(prog, tc.name); got != tc.want {
				t.Fatalf("moduleLocalSymbolKind(%q)=%v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
