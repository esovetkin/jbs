package eval

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestNewNameCatalogAndCombNames(t *testing.T) {
	catalog := NewNameCatalog(
		[]string{"z", "a", "a", "m"},
		map[string][]string{
			"mod":       {"y", "x", "x"},
			"mod.child": nil,
		},
	)
	if !reflect.DeepEqual(catalog.Visible, []string{"a", "m", "z"}) {
		t.Fatalf("unexpected visible names: %#v", catalog.Visible)
	}
	if got := catalog.Namespaces["mod"].Members; !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("unexpected namespace members: %#v", got)
	}
	if got := catalog.Namespaces["mod.child"].Members; len(got) != 0 {
		t.Fatalf("expected empty nested namespace member list, got %#v", got)
	}

	withOrder := CombValue(&Comb{
		Order: []string{"y", "x", "y"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: Int(2)}}},
		},
	})
	if got := CombNames(withOrder); !reflect.DeepEqual(got, []string{"y", "x"}) {
		t.Fatalf("unexpected comb names with order: %#v", got)
	}

	withoutOrder := CombValue(&Comb{
		Rows: []Row{
			{Values: map[string]Cell{"z": {Value: Int(1)}, "a": {Value: Int(2)}}},
		},
	})
	if got := CombNames(withoutOrder); !reflect.DeepEqual(got, []string{"a", "z"}) {
		t.Fatalf("unexpected comb names without order: %#v", got)
	}
}

func TestEvalNamesCall(t *testing.T) {
	span := spanAt(900, 1)
	comb := CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: Int(2)}}},
			{Values: map[string]Cell{"x": {Value: Int(3)}, "y": {Value: Int(4)}}},
		},
	})
	opts := ExprOptions{
		Context: EvalCtxBindingAssign,
		Names: NewNameCatalog(
			[]string{"z", "x", "jbs_name"},
			map[string][]string{
				"lib":       {"systemname", "wnodes"},
				"lib.inner": {"value"},
			},
		),
	}

	tests := []struct {
		name     string
		expr     ast.Expr
		env      map[string]Value
		opts     ExprOptions
		want     Value
		wantCode string
	}{
		{
			name: "zero arg visible names",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Span:   span,
			},
			opts: opts,
			want: List([]Value{String("jbs_name"), String("x"), String("z")}),
		},
		{
			name: "namespace argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "lib", Span: span}),
				Span:   span,
			},
			opts: opts,
			want: List([]Value{String("systemname"), String("wnodes")}),
		},
		{
			name: "nested namespace argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args:   ast.PosCallArgs(ast.QualifiedIdentExpr{Namespace: "lib", Name: "inner", Span: span}),
				Span:   span,
			},
			opts: opts,
			want: List([]Value{String("value")}),
		},
		{
			name: "comb argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "params", Span: span}),
				Span:   span,
			},
			env:  map[string]Value{"params": comb},
			opts: opts,
			want: List([]Value{String("x"), String("y")}),
		},
		{
			name: "named values argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args: []ast.CallArg{
					namedArg("values", ast.ListExpr{Items: []ast.Expr{ast.IdentExpr{Name: "params", Span: span}}, Span: span}),
				},
				Span: span,
			},
			env:  map[string]Value{"params": comb},
			opts: opts,
			want: List([]Value{String("x"), String("y")}),
		},
		{
			name: "projected comb argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args: ast.PosCallArgs(
					ast.IndexExpr{
						Base:  ast.IdentExpr{Name: "params", Span: span},
						Items: []ast.Expr{ast.StringExpr{Value: "x", Span: span}},
						Span:  span,
					},
				),
				Span: span,
			},
			env:  map[string]Value{"params": comb},
			opts: opts,
			want: List([]Value{String("x")}),
		},
		{
			name: "invalid arity",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args: ast.PosCallArgs(
					ast.IdentExpr{Name: "a", Span: span},
					ast.IdentExpr{Name: "b", Span: span},
				),
				Span: span,
			},
			opts:     opts,
			wantCode: "E106",
		},
		{
			name: "invalid scalar argument",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}),
				Span:   span,
			},
			opts:     opts,
			wantCode: "E106",
		},
		{
			name: "missing catalog",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Span:   span,
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "missing variable keeps E100 root cause",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "names", Span: span},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "missing", Span: span}),
				Span:   span,
			},
			opts:     opts,
			wantCode: "E100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, tc.opts)
			if tc.wantCode != "" {
				if diagCount(diags, tc.wantCode) == 0 {
					t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected names result: got=%#v want=%#v", got, tc.want)
			}
		})
	}
}
