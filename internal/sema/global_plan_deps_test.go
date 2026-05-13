package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestUniqueSortedBindingVersionKeys(t *testing.T) {
	keys := []BindingVersionKey{
		{Public: "z", Version: "2"},
		{},
		{Public: "a", Version: "2"},
		{Public: "a", Version: "1"},
		{Public: "a", Version: "2"},
	}
	want := []BindingVersionKey{
		{Public: "a", Version: "1"},
		{Public: "a", Version: "2"},
		{Public: "z", Version: "2"},
	}
	if got := uniqueSortedBindingVersionKeys(keys); !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueSortedBindingVersionKeys()=%#v, want %#v", got, want)
	}
}

func TestPreviousBindingDependencySnapshot(t *testing.T) {
	oldSelfKey := BindingVersionKey{Public: "x", Version: "old-x"}
	depKey := BindingVersionKey{Public: "base", Version: "base-1"}
	engine := &globalSeqEngine{
		currentBindings: map[string]*GlobalBinding{
			"x": {Name: "x", PublicName: "x", VersionID: oldSelfKey.Version},
		},
		globalVars: map[string]*GlobalVar{
			"x": {
				Name:          "x",
				DependsOn:     []string{"x", "base", "base"},
				DependsOnKeys: []BindingVersionKey{depKey, depKey, {}},
			},
			"base": {
				Name:      "base",
				VersionID: depKey.Version,
			},
		},
	}

	got := engine.previousBindingDependencySnapshot("x")
	if !reflect.DeepEqual(got.Names, []string{"base"}) {
		t.Fatalf("snapshot names=%#v, want [base]", got.Names)
	}
	wantKeys := []BindingVersionKey{depKey, oldSelfKey}
	if !reflect.DeepEqual(got.Keys, wantKeys) {
		t.Fatalf("snapshot keys=%#v, want %#v", got.Keys, wantKeys)
	}
}

func TestExprEvalTimeReadsName(t *testing.T) {
	sp := diag.Span{}
	cases := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{
			name: "direct ident",
			expr: ast.IdentExpr{Name: "x", Span: sp},
			want: true,
		},
		{
			name: "qualified namespace",
			expr: ast.QualifiedIdentExpr{Namespace: "x", Name: "value", Span: sp},
			want: true,
		},
		{
			name: "call argument inspected",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "f", Span: sp},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "x", Span: sp}),
				Span:   sp,
			},
			want: true,
		},
		{
			name: "opaque function call without self argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "f", Span: sp},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "y", Span: sp}),
				Span:   sp,
			},
			want: false,
		},
		{
			name: "function body skipped",
			expr: ast.FunctionExpr{
				Body: []ast.FuncBodyStmt{
					ast.ReturnStmt{Expr: ast.IdentExpr{Name: "x", Span: sp}, Span: sp},
				},
				Span: sp,
			},
			want: false,
		},
		{
			name: "function default inspected",
			expr: ast.FunctionExpr{
				Params: []ast.FuncParam{
					{Name: "v", Default: ast.IdentExpr{Name: "x", Span: sp}, Span: sp},
				},
				Span: sp,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exprEvalTimeReadsName(tc.expr, "x"); got != tc.want {
				t.Fatalf("exprEvalTimeReadsName()=%v, want %v", got, tc.want)
			}
		})
	}
}
