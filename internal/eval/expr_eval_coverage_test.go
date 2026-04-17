package eval

import (
	"slices"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestEvalExprWithCtxQualifiedCombScalarAliasAndIndexCoverage(t *testing.T) {
	t.Run("qualified comb single-row returns scalar", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		comb := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(7)}}},
			},
		})
		got := EvalExprWithOptions(
			ast.QualifiedIdentExpr{Namespace: "m", Name: "x", Span: spanAt(500, 1)},
			map[string]Value{"m": comb},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
		)
		if got.Kind != KindInt || got.I != 7 {
			t.Fatalf("expected int scalar 7, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("alias outside comb reports E106", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			ast.AliasExpr{
				Expr:  ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(501, 1)},
				Alias: "v",
				Span:  spanAt(501, 1),
			},
			map[string]Value{},
			diags,
			ExprOptions{},
		)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("index with qualified selector projects comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		comb := CombValue(&Comb{
			Order: []string{"ns.x", "y"},
			Rows: []Row{
				{Values: map[string]Cell{
					"ns.x": {Value: Int(1)},
					"y":    {Value: Int(10)},
				}},
			},
		})
		got := EvalExprWithOptions(
			ast.IndexExpr{
				Base: ast.IdentExpr{Name: "m", Span: spanAt(502, 1)},
				Items: []ast.Expr{
					ast.QualifiedIdentExpr{Namespace: "ns", Name: "x", Span: spanAt(502, 4)},
				},
				Span: spanAt(502, 1),
			},
			map[string]Value{"m": comb},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
		)
		if !IsComb(got) || got.C == nil {
			t.Fatalf("expected comb projection result, got %#v", got)
		}
		if !slices.Equal(got.C.Order, []string{"ns.x"}) {
			t.Fatalf("unexpected comb order after projection: %#v", got.C.Order)
		}
		if len(got.C.Rows) != 1 || got.C.Rows[0].Values["ns.x"].Value.I != 1 {
			t.Fatalf("unexpected projected rows: %#v", got.C.Rows)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestEvalExprWithCtxCombBinaryPaths(t *testing.T) {
	makeComb := func(name string, vals ...int64) Value {
		rows := make([]Row, 0, len(vals))
		for _, v := range vals {
			rows = append(rows, Row{Values: map[string]Cell{
				name: {Value: Int(v)},
			}})
		}
		return CombValue(&Comb{Order: []string{name}, Rows: rows})
	}
	env := map[string]Value{
		"a": makeComb("a", 1, 2),
		"b": makeComb("b", 10, 20),
	}

	t.Run("comb zip via +", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExpr(
			ast.BinaryExpr{
				Left:  ast.IdentExpr{Name: "a", Span: spanAt(503, 1)},
				Op:    "+",
				Right: ast.IdentExpr{Name: "b", Span: spanAt(503, 5)},
				Span:  spanAt(503, 3),
			},
			env,
			diags,
		)
		if !IsComb(got) || CombRowCount(got) != 2 {
			t.Fatalf("expected zipped comb with 2 rows, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("comb product via *", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExpr(
			ast.BinaryExpr{
				Left:  ast.IdentExpr{Name: "a", Span: spanAt(504, 1)},
				Op:    "*",
				Right: ast.IdentExpr{Name: "b", Span: spanAt(504, 5)},
				Span:  spanAt(504, 3),
			},
			env,
			diags,
		)
		if !IsComb(got) || CombRowCount(got) != 4 {
			t.Fatalf("expected product comb with 4 rows, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestExprNeedsCombBinaryAdditionalBranches(t *testing.T) {
	env := map[string]Value{
		"c": CombValue(&Comb{
			Order: []string{"x"},
			Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
		}),
	}

	tests := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{
			name: "qualified unknown returns false",
			expr: ast.QualifiedIdentExpr{Namespace: "missing", Name: "x", Span: spanAt(505, 1)},
			want: false,
		},
		{
			name: "call with comb in callee tree",
			expr: ast.CallExpr{
				Callee: ast.UnaryExpr{Op: "+", Expr: ast.IdentExpr{Name: "c", Span: spanAt(506, 1)}, Span: spanAt(506, 1)},
				Args:   []ast.Expr{ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(506, 3)}},
				Span:   spanAt(506, 1),
			},
			want: true,
		},
		{
			name: "index detects comb in selectors",
			expr: ast.IndexExpr{
				Base: ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(507, 1)},
				Items: []ast.Expr{
					ast.IdentExpr{Name: "c", Span: spanAt(507, 3)},
				},
				Span: spanAt(507, 1),
			},
			want: true,
		},
		{
			name: "index no comb returns false",
			expr: ast.IndexExpr{
				Base: ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(508, 1)},
				Items: []ast.Expr{
					ast.NumberExpr{Int: true, IntValue: 2, Span: spanAt(508, 3)},
				},
				Span: spanAt(508, 1),
			},
			want: false,
		},
		{
			name: "tuple no comb returns false",
			expr: ast.TupleExpr{Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(509, 1)},
			}},
			want: false,
		},
	}
	for _, tc := range tests {
		if got := exprNeedsCombBinary(tc.expr, env); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestEvalCombCallAndNamedRowsCoverage(t *testing.T) {
	span := spanAt(510, 1)
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}

	t.Run("comb arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalCombCall(
			[]ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1, Span: span},
				ast.NumberExpr{Int: true, IntValue: 2, Span: span},
			},
			map[string]Value{},
			span,
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
			ctx,
		)
		if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected comb arity error, got value=%#v diags=%s", got, diags.String())
		}
	})

	t.Run("named rows nil expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(nil, map[string]Value{}, diags, ExprOptions{}, ctx)
		if ok || rows != nil {
			t.Fatalf("expected nil,false for nil expr, got rows=%#v ok=%v", rows, ok)
		}
	})

	t.Run("named rows duplicate identifiers", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(
			ast.BinaryExpr{
				Left:  ast.IdentExpr{Name: "x", Span: span},
				Op:    "+",
				Right: ast.IdentExpr{Name: "x", Span: span},
				Span:  span,
			},
			map[string]Value{"x": List([]Value{Int(1), Int(2)})},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
			ctx,
		)
		if ok || rows != nil || diagCount(diags, "E036") == 0 {
			t.Fatalf("expected duplicate-name error, got rows=%#v ok=%v diags=%s", rows, ok, diags.String())
		}
	})

	t.Run("named rows unknown identifier", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(ast.IdentExpr{Name: "missing", Span: span}, map[string]Value{}, diags, ExprOptions{}, ctx)
		if ok || rows != nil || diagCount(diags, "E100") == 0 {
			t.Fatalf("expected unknown identifier error, got rows=%#v ok=%v diags=%s", rows, ok, diags.String())
		}
	})

	t.Run("named rows qualified identifier", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(
			ast.QualifiedIdentExpr{Namespace: "ns", Name: "v", Span: span},
			map[string]Value{"ns.v": Int(9)},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
			ctx,
		)
		if !ok || len(rows) != 1 {
			t.Fatalf("expected qualified named rows, got rows=%#v ok=%v", rows, ok)
		}
		if rows[0].Values["ns.v"].Value.I != 9 {
			t.Fatalf("expected ns.v=9 row value, got %#v", rows[0].Values)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("named rows comb call leaf success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(
			ast.CallExpr{
				Callee: ast.IdentExpr{Name: "comb", Span: span},
				Args: []ast.Expr{
					ast.AliasExpr{
						Expr:  ast.NumberExpr{Int: true, IntValue: 1, Span: span},
						Alias: "k",
						Span:  span,
					},
				},
				Span: span,
			},
			map[string]Value{},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
			ctx,
		)
		if !ok || len(rows) != 1 {
			t.Fatalf("expected comb leaf rows, got rows=%#v ok=%v", rows, ok)
		}
		if rows[0].Values["k"].Value.I != 1 {
			t.Fatalf("expected alias column k=1, got %#v", rows[0].Values)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("named rows comb call leaf non-comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(
			ast.CallExpr{
				Callee: ast.IdentExpr{Name: "comb", Span: span},
				Args: []ast.Expr{
					ast.NumberExpr{Int: true, IntValue: 1, Span: span},
				},
				Span: span,
			},
			map[string]Value{},
			diags,
			ExprOptions{},
			ctx,
		)
		if ok || rows != nil || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected comb leaf non-comb error, got rows=%#v ok=%v diags=%s", rows, ok, diags.String())
		}
	})

	t.Run("named rows alias requires non-empty and non-comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rows, ok := evalExprAsNamedCombRows(
			ast.AliasExpr{
				Expr:  ast.NumberExpr{Int: true, IntValue: 1, Span: span},
				Alias: "",
				Span:  span,
			},
			map[string]Value{},
			diags,
			ExprOptions{},
			ctx,
		)
		if ok || rows != nil || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected empty alias error, got rows=%#v ok=%v diags=%s", rows, ok, diags.String())
		}

		diags = &diag.Diagnostics{}
		rows, ok = evalExprAsNamedCombRows(
			ast.AliasExpr{
				Expr:  ast.IdentExpr{Name: "c", Span: span},
				Alias: "k",
				Span:  span,
			},
			map[string]Value{
				"c": CombValue(&Comb{
					Order: []string{"x"},
					Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
				}),
			},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
			ctx,
		)
		if ok || rows != nil || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected alias-on-comb error, got rows=%#v ok=%v diags=%s", rows, ok, diags.String())
		}
	})
}

func TestEvalBinaryVectorAndCompareCoverage(t *testing.T) {
	span := spanAt(520, 1)
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}

	t.Run("comb operator unsupported", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		comb := CombValue(&Comb{
			Order: []string{"x"},
			Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
		})
		got := evalBinary("-", comb, comb, span, diags, ExprOptions{}, ctx)
		if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106 for unsupported comb operator, got value=%#v diags=%s", got, diags.String())
		}
	})

	t.Run("comb plus supported", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		left := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}}},
			},
		})
		right := CombValue(&Comb{
			Order: []string{"y"},
			Rows: []Row{
				{Values: map[string]Cell{"y": {Value: Int(10)}}},
				{Values: map[string]Cell{"y": {Value: Int(20)}}},
			},
		})
		got := evalBinary("+", left, right, span, diags, ExprOptions{}, ctx)
		if !IsComb(got) || got.C == nil {
			t.Fatalf("expected comb result, got %#v", got)
		}
		if CombRowCount(got) != 2 {
			t.Fatalf("expected zipped comb with 2 rows, got %#v", got.C.Rows)
		}
		if got.C.Rows[0].Values["x"].Value.I != 1 || got.C.Rows[0].Values["y"].Value.I != 10 {
			t.Fatalf("unexpected first row values: %#v", got.C.Rows[0].Values)
		}
		if got.C.Rows[1].Values["x"].Value.I != 2 || got.C.Rows[1].Values["y"].Value.I != 20 {
			t.Fatalf("unexpected second row values: %#v", got.C.Rows[1].Values)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("comb product supported", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		left := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}}},
			},
		})
		right := CombValue(&Comb{
			Order: []string{"y"},
			Rows: []Row{
				{Values: map[string]Cell{"y": {Value: Int(10)}}},
				{Values: map[string]Cell{"y": {Value: Int(20)}}},
			},
		})
		got := evalBinary("*", left, right, span, diags, ExprOptions{}, ctx)
		if !IsComb(got) || got.C == nil {
			t.Fatalf("expected comb result, got %#v", got)
		}
		if CombRowCount(got) != 4 {
			t.Fatalf("expected product comb with 4 rows, got %#v", got.C.Rows)
		}
		if got.C.Rows[0].Values["x"].Value.I != 1 || got.C.Rows[0].Values["y"].Value.I != 10 {
			t.Fatalf("unexpected first row values: %#v", got.C.Rows[0].Values)
		}
		if got.C.Rows[3].Values["x"].Value.I != 2 || got.C.Rows[3].Values["y"].Value.I != 20 {
			t.Fatalf("unexpected last row values: %#v", got.C.Rows[3].Values)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("tuple arithmetic unsupported operator", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalParamTupleBinary("-", Tuple([]Value{Int(1)}), Tuple([]Value{Int(2)}), span, diags)
		if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected tuple-op E106, got value=%#v diags=%s", got, diags.String())
		}
	})

	t.Run("vector binary empty input", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalVectorBinary("+", List(nil), List([]Value{Int(1)}), span, diags, ExprOptions{}, ctx)
		if got.Kind != KindList || len(got.L) != 0 {
			t.Fatalf("expected empty list, got %#v", got)
		}
	})

	t.Run("vector binary right longer", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalVectorBinary("+", List([]Value{Int(1)}), List([]Value{Int(2), Int(3)}), span, diags, ExprOptions{}, ctx)
		if got.Kind != KindList || len(got.L) != 2 {
			t.Fatalf("expected two output items, got %#v", got)
		}
		if got.L[0].I != 3 || got.L[1].I != 4 {
			t.Fatalf("unexpected vector-binary result: %#v", got.L)
		}
	})

	t.Run("compare sequence empty and broadcast", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalCompare("==", List(nil), List([]Value{Int(1)}), span, diags)
		if got.Kind != KindList || len(got.L) != 0 {
			t.Fatalf("expected empty comparison list, got %#v", got)
		}
		got = evalCompare("<", List([]Value{Int(1)}), List([]Value{Int(2), Int(0)}), span, diags)
		if got.Kind != KindList || len(got.L) != 2 {
			t.Fatalf("expected broadcast comparison list of len 2, got %#v", got)
		}
		if !got.L[0].B || got.L[1].B {
			t.Fatalf("unexpected comparison broadcast values: %#v", got.L)
		}
	})

	t.Run("any with all false values returns false", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("any", []Value{List([]Value{Bool(false), Bool(false)})}, span, diags)
		if got.Kind != KindBool || got.B {
			t.Fatalf("expected any(all-false)=false, got %#v", got)
		}
	})
}
