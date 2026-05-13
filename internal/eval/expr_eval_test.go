package eval

import (
	"math"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestEvalExprWithCtxNilExpr(t *testing.T) {
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
	got := evalExprWithCtx(nil, map[string]Value{}, diags, ExprOptions{}, ctx)
	if got.Kind != KindNull {
		t.Fatalf("expected null for nil expression, got %#v", got)
	}
	if len(diags.Items) != 0 {
		t.Fatalf("did not expect diagnostics, got: %s", diags.String())
	}
}

func TestEvalExprWithCtxIdentifierResolution(t *testing.T) {
	env := map[string]Value{
		"x":      Int(7),
		"ns.var": String("ok"),
	}
	tests := []struct {
		name     string
		expr     ast.Expr
		want     Value
		diagCode string
	}{
		{
			name: "ident found",
			expr: ast.IdentExpr{Name: "x", Span: spanAt(80, 1)},
			want: Int(7),
		},
		{
			name: "qualified found",
			expr: ast.QualifiedIdentExpr{Namespace: "ns", Name: "var", Span: spanAt(81, 1)},
			want: String("ok"),
		},
		{
			name:     "ident missing",
			expr:     ast.IdentExpr{Name: "missing", Span: spanAt(82, 1)},
			want:     Null(),
			diagCode: "E100",
		},
		{
			name:     "qualified missing",
			expr:     ast.QualifiedIdentExpr{Namespace: "ns", Name: "missing", Span: spanAt(83, 1)},
			want:     Null(),
			diagCode: "E100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
			got := evalExprWithCtx(tc.expr, env, diags, ExprOptions{}, ctx)
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
			if tc.diagCode == "" {
				if diags.HasErrors() {
					t.Fatalf("unexpected errors: %s", diags.String())
				}
				return
			}
			if count := diagCount(diags, tc.diagCode); count != 1 {
				t.Fatalf("expected one %s, got %d: %s", tc.diagCode, count, diags.String())
			}
		})
	}
}

func TestQualifiedCombNamespaceDispatch(t *testing.T) {
	comb := CombValue(&Comb{
		Order: []string{"x"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}}},
			{Values: map[string]Cell{"x": {Value: Int(2)}}},
		},
	})
	tests := []struct {
		name     string
		env      map[string]Value
		expr     ast.Expr
		wantKind Kind
		wantLen  int
		diagCode string
	}{
		{
			name: "comb namespace column lookup",
			env:  map[string]Value{"m": comb},
			expr: ast.QualifiedIdentExpr{Namespace: "m", Name: "x", Span: spanAt(88, 1)},
			// multi-row column lookup returns list
			wantKind: KindList,
			wantLen:  2,
		},
		{
			name:     "comb namespace unknown column",
			env:      map[string]Value{"m": comb},
			expr:     ast.QualifiedIdentExpr{Namespace: "m", Name: "y", Span: spanAt(89, 1)},
			wantKind: KindNull,
			diagCode: "E100",
		},
		{
			name:     "non-comb namespace",
			env:      map[string]Value{"m": Int(1)},
			expr:     ast.QualifiedIdentExpr{Namespace: "m", Name: "x", Span: spanAt(90, 1)},
			wantKind: KindNull,
			diagCode: "E106",
		},
		{
			name:     "legacy dotted fallback",
			env:      map[string]Value{"m.x": String("ok")},
			expr:     ast.QualifiedIdentExpr{Namespace: "m", Name: "x", Span: spanAt(91, 1)},
			wantKind: KindString,
			diagCode: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, ExprOptions{Context: EvalCtxBindingAssign})
			if got.Kind != tc.wantKind {
				t.Fatalf("unexpected kind: got=%s want=%s value=%#v", got.Kind, tc.wantKind, got)
			}
			if tc.wantLen > 0 && len(got.L) != tc.wantLen {
				t.Fatalf("unexpected list length: got=%d want=%d", len(got.L), tc.wantLen)
			}
			if tc.diagCode != "" && diagCount(diags, tc.diagCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.diagCode, diags.String())
			}
		})
	}
}

func TestIndexExprCombProjectionErrors(t *testing.T) {
	comb := CombValue(&Comb{
		Order: []string{"x"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}}},
		},
	})
	tests := []struct {
		name     string
		env      map[string]Value
		expr     ast.Expr
		diagCode string
	}{
		{
			name: "non comb base",
			env:  map[string]Value{"m": Int(1)},
			expr: ast.IndexExpr{
				Base:  ast.IdentExpr{Name: "m", Span: spanAt(92, 1)},
				Items: []ast.Expr{ast.StringExpr{Value: "x", Span: spanAt(92, 3)}},
				Span:  spanAt(92, 1),
			},
			diagCode: "E106",
		},
		{
			name: "empty selectors",
			env:  map[string]Value{"m": comb},
			expr: ast.IndexExpr{
				Base:  ast.IdentExpr{Name: "m", Span: spanAt(93, 1)},
				Items: nil,
				Span:  spanAt(93, 1),
			},
			diagCode: "E106",
		},
		{
			name: "invalid selector expression kind",
			env:  map[string]Value{"m": comb},
			expr: ast.IndexExpr{
				Base:  ast.IdentExpr{Name: "m", Span: spanAt(94, 1)},
				Items: []ast.Expr{ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(94, 3)}},
				Span:  spanAt(94, 1),
			},
			diagCode: "E106",
		},
		{
			name: "unknown projected column",
			env:  map[string]Value{"m": comb},
			expr: ast.IndexExpr{
				Base:  ast.IdentExpr{Name: "m", Span: spanAt(95, 1)},
				Items: []ast.Expr{ast.StringExpr{Value: "y", Span: spanAt(95, 3)}},
				Span:  spanAt(95, 1),
			},
			diagCode: "E106",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, ExprOptions{Context: EvalCtxBindingAssign})
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if diagCount(diags, tc.diagCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.diagCode, diags.String())
			}
		})
	}
}

func TestMemberExprCombAccess(t *testing.T) {
	t.Run("projected multi-row member returns list", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		comb := CombValue(&Comb{
			Order: []string{"x", "y"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: Int(10)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}, "y": {Value: Int(20)}}},
			},
		})
		got := EvalExprWithOptions(
			ast.MemberExpr{
				Base: ast.IndexExpr{
					Base:  ast.IdentExpr{Name: "p0", Span: spanAt(96, 1)},
					Items: []ast.Expr{ast.StringExpr{Value: "x", Span: spanAt(96, 4)}},
					Span:  spanAt(96, 1),
				},
				Name: "x",
				Span: spanAt(96, 1),
			},
			map[string]Value{"p0": comb},
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
		)
		if got.Kind != KindList || len(got.L) != 2 || got.L[0].I != 1 || got.L[1].I != 2 {
			t.Fatalf("expected projected member list [1,2], got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("non comb member base reports E106", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			ast.MemberExpr{
				Base: ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(97, 1)},
				Name: "x",
				Span: spanAt(97, 1),
			},
			nil,
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
		)
		if got.Kind != KindNull {
			t.Fatalf("expected null result, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func TestCombBinarySupportsProjectedOperandsAndMemberAlias(t *testing.T) {
	env := map[string]Value{
		"p0": CombValue(&Comb{
			Order: []string{"x", "y"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: Int(50)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}, "y": {Value: Int(60)}}},
			},
		}),
		"p1": CombValue(&Comb{
			Order: []string{"x", "y"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(10)}, "y": {Value: Int(100)}}},
				{Values: map[string]Cell{"x": {Value: Int(20)}, "y": {Value: Int(200)}}},
			},
		}),
	}

	t.Run("projected comb operands combine without strict leaf errors", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			ast.BinaryExpr{
				Left: ast.IndexExpr{
					Base:  ast.IdentExpr{Name: "p0", Span: spanAt(98, 1)},
					Items: []ast.Expr{ast.StringExpr{Value: "x", Span: spanAt(98, 4)}},
					Span:  spanAt(98, 1),
				},
				Op: "+",
				Right: ast.IndexExpr{
					Base:  ast.IdentExpr{Name: "p1", Span: spanAt(98, 10)},
					Items: []ast.Expr{ast.StringExpr{Value: "y", Span: spanAt(98, 13)}},
					Span:  spanAt(98, 10),
				},
				Span: spanAt(98, 1),
			},
			env,
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
		)
		if !IsComb(got) || got.C == nil {
			t.Fatalf("expected comb result, got %#v", got)
		}
		if !slices.Equal(got.C.Order, []string{"x", "y"}) {
			t.Fatalf("expected columns [x y], got %#v", got.C.Order)
		}
		if len(got.C.Rows) != 2 {
			t.Fatalf("expected 2 rows, got %#v", got.C.Rows)
		}
		if got.C.Rows[0].Values["x"].Value.I != 1 || got.C.Rows[0].Values["y"].Value.I != 100 {
			t.Fatalf("unexpected first row values: %#v", got.C.Rows[0].Values)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("member alias combines with projected comb operand", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			ast.BinaryExpr{
				Left: ast.AliasExpr{
					Expr: ast.MemberExpr{
						Base: ast.IndexExpr{
							Base:  ast.IdentExpr{Name: "p0", Span: spanAt(99, 1)},
							Items: []ast.Expr{ast.StringExpr{Value: "x", Span: spanAt(99, 4)}},
							Span:  spanAt(99, 1),
						},
						Name: "x",
						Span: spanAt(99, 1),
					},
					Alias: "y",
					Span:  spanAt(99, 1),
				},
				Op: "+",
				Right: ast.IndexExpr{
					Base:  ast.IdentExpr{Name: "p1", Span: spanAt(99, 14)},
					Items: []ast.Expr{ast.StringExpr{Value: "x", Span: spanAt(99, 17)}},
					Span:  spanAt(99, 14),
				},
				Span: spanAt(99, 1),
			},
			env,
			diags,
			ExprOptions{Context: EvalCtxBindingAssign},
		)
		if !IsComb(got) || got.C == nil {
			t.Fatalf("expected comb result, got %#v", got)
		}
		if !slices.Equal(got.C.Order, []string{"x", "y"}) {
			t.Fatalf("expected columns [x y], got %#v", got.C.Order)
		}
		if len(got.C.Rows) != 2 {
			t.Fatalf("expected 2 rows, got %#v", got.C.Rows)
		}
		if got.C.Rows[0].Values["x"].Value.I != 10 || got.C.Rows[0].Values["y"].Value.I != 1 {
			t.Fatalf("unexpected first row values: %#v", got.C.Rows[0].Values)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestEvalExprWithCtxConditionalBranches(t *testing.T) {
	tests := []struct {
		name         string
		expr         ast.ConditionalExpr
		want         Value
		wantDiagE102 bool
	}{
		{
			name: "true branch",
			expr: ast.ConditionalExpr{
				Then: ast.StringExpr{Value: "then"},
				Cond: ast.BoolExpr{Value: true},
				Else: ast.StringExpr{Value: "else"},
			},
			want: String("then"),
		},
		{
			name: "false branch",
			expr: ast.ConditionalExpr{
				Then: ast.StringExpr{Value: "then"},
				Cond: ast.BoolExpr{Value: false},
				Else: ast.StringExpr{Value: "else"},
			},
			want: String("else"),
		},
		{
			name: "non bool condition falls back to then",
			expr: ast.ConditionalExpr{
				Then: ast.StringExpr{Value: "then"},
				Cond: ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(85, 1)},
				Else: ast.StringExpr{Value: "else"},
			},
			want:         String("then"),
			wantDiagE102: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
			got := evalExprWithCtx(tc.expr, map[string]Value{}, diags, ExprOptions{}, ctx)
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
			if tc.wantDiagE102 {
				if count := diagCount(diags, "E102"); count != 1 {
					t.Fatalf("expected one E102, got %d: %s", count, diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
		})
	}
}

func TestEvalExprWithCtxCompareExpr(t *testing.T) {
	tests := []struct {
		name         string
		expr         ast.CompareExpr
		want         Value
		wantDiagE110 bool
	}{
		{
			name: "numeric compare true",
			expr: ast.CompareExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: 5},
				Op:    ">",
				Right: ast.NumberExpr{Int: true, IntValue: 3},
				Span:  spanAt(87, 1),
			},
			want: Bool(true),
		},
		{
			name: "string compare false",
			expr: ast.CompareExpr{
				Left:  ast.StringExpr{Value: "alpha"},
				Op:    ">",
				Right: ast.StringExpr{Value: "beta"},
				Span:  spanAt(88, 1),
			},
			want: Bool(false),
		},
		{
			name: "unsupported compare reports E110",
			expr: ast.CompareExpr{
				Left:  ast.StringExpr{Value: "x"},
				Op:    "<",
				Right: ast.NumberExpr{Int: true, IntValue: 1},
				Span:  spanAt(89, 1),
			},
			want:         Bool(false),
			wantDiagE110: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
			got := evalExprWithCtx(tc.expr, map[string]Value{}, diags, ExprOptions{}, ctx)
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
			if tc.wantDiagE110 {
				if count := diagCount(diags, "E110"); count != 1 {
					t.Fatalf("expected one E110, got %d: %s", count, diags.String())
				}
				if diags.Items[0].Span != tc.expr.Span {
					t.Fatalf("expected E110 span %v, got %v", tc.expr.Span, diags.Items[0].Span)
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
		})
	}
}

func TestEvalExprWithCtxOverflowWarningDedupAcrossCalls(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
		Op:    "+",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(86, 1),
	}
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}

	got0 := evalExprWithCtx(expr, map[string]Value{}, diags, ExprOptions{}, ctx)
	got1 := evalExprWithCtx(expr, map[string]Value{}, diags, ExprOptions{}, ctx)

	if got0.Kind != KindInt || got1.Kind != KindInt || got0.I != math.MinInt64 || got1.I != math.MinInt64 {
		t.Fatalf("unexpected overflow results: first=%#v second=%#v", got0, got1)
	}
	if count := diagCount(diags, "W102"); count != 1 {
		t.Fatalf("expected one deduplicated W102 across shared context, got %d: %s", count, diags.String())
	}
}

func TestEvalVectorArithmetic(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.IdentExpr{Name: "x"},
		Op:   "+",
		Right: ast.NumberExpr{
			Int:      true,
			IntValue: 10,
		},
	}
	env := map[string]Value{
		"x": List([]Value{Int(1), Int(2), Int(3)}),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, env, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 3 || got.L[0].I != 11 || got.L[2].I != 13 {
		t.Fatalf("unexpected vector eval result: %#v", got)
	}
}

func TestEvalConditionalRequiresBool(t *testing.T) {
	expr := ast.ConditionalExpr{
		Then: ast.NumberExpr{Int: true, IntValue: 1},
		Cond: ast.NumberExpr{Int: true, IntValue: 2},
		Else: ast.NumberExpr{Int: true, IntValue: 0},
	}
	diags := &diag.Diagnostics{}
	_ = EvalExpr(expr, map[string]Value{}, diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E102" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E102, got: %s", diags.String())
	}
}

func TestEvalLargeIntegerLiteralExact(t *testing.T) {
	expr := ast.NumberExpr{Int: true, IntValue: 9007199254740993}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != 9007199254740993 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
}

func TestEvalIntOverflowAddWarns(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
		Op:    "+",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(1, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != math.MinInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntOverflowSubWarns(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MinInt64},
		Op:    "-",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(2, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != math.MaxInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntOverflowMulWarns(t *testing.T) {
	expr := ast.BinaryExpr{
		Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
		Op:    "*",
		Right: ast.NumberExpr{Int: true, IntValue: 2},
		Span:  spanAt(3, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != -2 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalIntOverflowUnaryWarns(t *testing.T) {
	expr := ast.UnaryExpr{
		Op:   "-",
		Expr: ast.NumberExpr{Int: true, IntValue: math.MinInt64},
		Span: spanAt(4, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != math.MinInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalUnaryScalarOperators(t *testing.T) {
	tests := []struct {
		name string
		op   string
		in   Value
		want Value
	}{
		{name: "plus int", op: "+", in: Int(5), want: Int(5)},
		{name: "minus int", op: "-", in: Int(5), want: Int(-5)},
		{name: "plus float", op: "+", in: Float(1.25), want: Float(1.25)},
		{name: "minus float", op: "-", in: Float(1.25), want: Float(-1.25)},
		{name: "bang true", op: "!", in: Bool(true), want: Bool(false)},
		{name: "bang false", op: "!", in: Bool(false), want: Bool(true)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalUnary(tc.op, tc.in, spanAt(4, 10), diags, &evalCtx{overflowWarned: map[string]struct{}{}})
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestEvalUnaryLogicalNotCastsAndVectorizes(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalUnary("!", List([]Value{Int(1), Int(0), String("")}), spanAt(4, 25), diags, &evalCtx{overflowWarned: map[string]struct{}{}})
	if got.Kind != KindList || len(got.L) != 3 {
		t.Fatalf("expected list result, got %#v", got)
	}
	want := []bool{false, true, true}
	for i, v := range want {
		if got.L[i].Kind != KindBool || got.L[i].B != v {
			t.Fatalf("unexpected vectorized ! result at %d: got=%#v want=%v", i, got.L[i], v)
		}
	}
	if count := diagCount(diags, "W101"); count != 1 {
		t.Fatalf("expected one W101 cast warning, got %d: %s", count, diags.String())
	}
}

func TestEvalUnarySequenceProducesList(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalUnary("-", Tuple([]Value{Int(1), Float(2.5)}), spanAt(4, 20), diags, &evalCtx{overflowWarned: map[string]struct{}{}})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 2 {
		t.Fatalf("expected list result for sequence unary, got %#v", got)
	}
	if got.L[0].Kind != KindInt || got.L[0].I != -1 {
		t.Fatalf("unexpected first unary sequence value: %#v", got.L[0])
	}
	if got.L[1].Kind != KindFloat || got.L[1].F != -2.5 {
		t.Fatalf("unexpected second unary sequence value: %#v", got.L[1])
	}
}

func TestEvalUnarySequenceNonNumericReportsE103(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalUnary("-", List([]Value{Int(1), String("x"), Bool(true)}), spanAt(4, 30), diags, &evalCtx{overflowWarned: map[string]struct{}{}})
	if got.Kind != KindList || len(got.L) != 3 {
		t.Fatalf("expected list result with element-wise unary eval, got %#v", got)
	}
	if got.L[0].Kind != KindInt || got.L[0].I != -1 {
		t.Fatalf("unexpected first unary sequence value: %#v", got.L[0])
	}
	if got.L[1].Kind != KindNull || got.L[2].Kind != KindNull {
		t.Fatalf("expected nulls for non-numeric unary elements, got %#v", got)
	}
	if count := diagCount(diags, "E103"); count != 2 {
		t.Fatalf("expected two E103 diagnostics, got %d: %s", count, diags.String())
	}
}

func TestEvalUnaryOverflowWarnDedupInSequence(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalUnary("-", List([]Value{Int(math.MinInt64), Int(math.MinInt64)}), spanAt(4, 40), diags, &evalCtx{overflowWarned: map[string]struct{}{}})
	if got.Kind != KindList || len(got.L) != 2 {
		t.Fatalf("expected list result, got %#v", got)
	}
	if got.L[0].Kind != KindInt || got.L[1].Kind != KindInt {
		t.Fatalf("expected int values after unary overflow case, got %#v", got)
	}
	if got.L[0].I != math.MinInt64 || got.L[1].I != math.MinInt64 {
		t.Fatalf("unexpected unary overflow values: %#v", got)
	}
	if count := diagCount(diags, "W102"); count != 1 {
		t.Fatalf("expected one deduplicated W102 warning, got %d: %s", count, diags.String())
	}
}

func TestEvalIntNoOverflowBoundariesNoWarning(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want int64
	}{
		{
			name: "max-plus-zero",
			expr: ast.BinaryExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 0},
				Span:  spanAt(5, 1),
			},
			want: math.MaxInt64,
		},
		{
			name: "min-plus-one",
			expr: ast.BinaryExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: math.MinInt64},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 1},
				Span:  spanAt(6, 1),
			},
			want: math.MinInt64 + 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExpr(tc.expr, map[string]Value{}, diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got.Kind != KindInt || got.I != tc.want {
				t.Fatalf("unexpected evaluated value: %#v", got)
			}
			if got := diagCount(diags, "W102"); got != 0 {
				t.Fatalf("did not expect W102 warning, got %d: %s", got, diags.String())
			}
		})
	}
}

func TestEvalIntOverflowVectorWarnDedup(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.ListExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
				ast.NumberExpr{Int: true, IntValue: math.MaxInt64},
			},
		},
		Op:    "+",
		Right: ast.NumberExpr{Int: true, IntValue: 1},
		Span:  spanAt(7, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 2 || got.L[0].I != math.MinInt64 || got.L[1].I != math.MinInt64 {
		t.Fatalf("unexpected evaluated value: %#v", got)
	}
	if got := diagCount(diags, "W102"); got != 1 {
		t.Fatalf("expected one deduplicated W102 warning, got %d: %s", got, diags.String())
	}
}

func TestEvalCompareSupportedOps(t *testing.T) {
	tests := []struct {
		name        string
		op          string
		l           Value
		r           Value
		wantBool    bool
		want        Value
		wantIsValue bool
	}{
		{name: "eq-int-float", op: "==", l: Int(2), r: Float(2.0), wantBool: true},
		{name: "ne-string", op: "!=", l: String("a"), r: String("b"), wantBool: true},
		{name: "lt-string", op: "<", l: String("alpha"), r: String("beta"), wantBool: true},
		{name: "le-string", op: "<=", l: String("beta"), r: String("beta"), wantBool: true},
		{name: "ge-string", op: ">=", l: String("beta"), r: String("alpha"), wantBool: true},
		{name: "gt-float-int", op: ">", l: Float(2.5), r: Int(2), wantBool: true},
		{name: "ge-int-float", op: ">=", l: Int(3), r: Float(3.0), wantBool: true},
		{name: "le-int-float", op: "<=", l: Int(3), r: Float(3.0), wantBool: true},
		{name: "lt-int-int-false", op: "<", l: Int(7), r: Int(5), wantBool: false},
		{name: "eq-bool", op: "==", l: Bool(true), r: Bool(true), wantBool: true},
		{name: "ne-bool", op: "!=", l: Bool(true), r: Bool(false), wantBool: true},
		{
			name:        "eq-list",
			op:          "==",
			l:           List([]Value{Int(1), String("x")}),
			r:           List([]Value{Int(1), String("x")}),
			want:        List([]Value{Bool(true), Bool(true)}),
			wantIsValue: true,
		},
		{
			name:        "ne-tuple",
			op:          "!=",
			l:           Tuple([]Value{Int(1), Int(2)}),
			r:           Tuple([]Value{Int(1), Int(3)}),
			want:        List([]Value{Bool(false), Bool(true)}),
			wantIsValue: true,
		},
		{
			name:        "eq-list-vs-tuple-false",
			op:          "==",
			l:           List([]Value{Int(1), Int(2)}),
			r:           Tuple([]Value{Int(1), Int(2)}),
			want:        List([]Value{Bool(true), Bool(true)}),
			wantIsValue: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalCompare(tc.op, tc.l, tc.r, spanAt(20, 1), diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if tc.wantIsValue {
				if !Equal(got, tc.want) {
					t.Fatalf("unexpected compare result: %#v (want %#v)", got, tc.want)
				}
				return
			}
			if got.Kind != KindBool || got.B != tc.wantBool {
				t.Fatalf("unexpected compare result: %#v (want bool=%v)", got, tc.wantBool)
			}
		})
	}
}

func TestEvalCompareUnsupportedReportsE110(t *testing.T) {
	tests := []struct {
		name string
		op   string
		l    Value
		r    Value
	}{
		{name: "type-mismatch-relational", op: "<", l: String("x"), r: Int(1)},
		{name: "unknown-op-on-numeric", op: "===", l: Int(1), r: Int(1)},
		{name: "unknown-op-on-string", op: "===", l: String("x"), r: String("x")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalCompare(tc.op, tc.l, tc.r, spanAt(21, 1), diags)
			if got.Kind != KindBool || got.B {
				t.Fatalf("unexpected compare fallback result: %#v", got)
			}
			if !diags.HasErrors() {
				t.Fatalf("expected error diagnostics, got none")
			}
			if count := diagCount(diags, "E110"); count != 1 {
				t.Fatalf("expected one E110, got %d: %s", count, diags.String())
			}
		})
	}
}

func TestEvalTupleConcatParamAssignmentMode(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1},
				ast.NumberExpr{Int: true, IntValue: 2},
				ast.NumberExpr{Int: true, IntValue: 3},
			},
		},
		Op: "+",
		Right: ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 4},
			},
		},
		Span: spanAt(30, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(expr, map[string]Value{}, diags, ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindTuple {
		t.Fatalf("expected tuple result, got %#v", got)
	}
	if len(got.L) != 4 || got.L[0].I != 1 || got.L[3].I != 4 {
		t.Fatalf("unexpected tuple concat result: %#v", got)
	}
}

func TestEvalTupleRepeatParamAssignmentMode(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1},
				ast.NumberExpr{Int: true, IntValue: 2},
				ast.NumberExpr{Int: true, IntValue: 3},
			},
		},
		Op:    "*",
		Right: ast.NumberExpr{Int: true, IntValue: 4},
		Span:  spanAt(31, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(expr, map[string]Value{}, diags, ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindTuple {
		t.Fatalf("expected tuple result, got %#v", got)
	}
	if len(got.L) != 12 {
		t.Fatalf("expected 12 repeated values, got %#v", got)
	}
	if got.L[0].I != 1 || got.L[3].I != 1 || got.L[11].I != 3 {
		t.Fatalf("unexpected tuple repeat result: %#v", got)
	}
}

func TestEvalTupleRepeatZeroCountParamAssignmentMode(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1},
				ast.NumberExpr{Int: true, IntValue: 2},
			},
		},
		Op:    "*",
		Right: ast.NumberExpr{Int: true, IntValue: 0},
		Span:  spanAt(31, 20),
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(expr, map[string]Value{}, diags, ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindTuple || len(got.L) != 0 {
		t.Fatalf("expected empty tuple for zero repetition, got %#v", got)
	}
}

func TestEvalTupleRepeatRejectsHugeOutputWithoutPanic(t *testing.T) {
	tests := []struct {
		name  string
		count int64
	}{
		{
			name:  "overflow",
			count: math.MaxInt64,
		},
		{
			name:  "over allocation budget",
			count: int64(maxRepeatOutputUnits/2 + 1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("repeat panicked: %v", r)
				}
			}()

			diags := &diag.Diagnostics{}
			got := evalParamTupleBinary(
				"*",
				Tuple([]Value{Int(1), Int(2)}),
				Int(tc.count),
				spanAt(91, 1),
				diags,
			)
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if count := diagCount(diags, "E106"); count != 1 {
				t.Fatalf("expected one E106, got %d: %s", count, diags.String())
			}
		})
	}
}

func TestEvalTupleArithmeticErrorsInParamMode(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{
			name: "tuple plus scalar",
			expr: ast.BinaryExpr{
				Left: ast.TupleExpr{
					Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
						ast.NumberExpr{Int: true, IntValue: 3},
					},
				},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 4},
				Span:  spanAt(32, 1),
			},
		},
		{
			name: "tuple multiply float",
			expr: ast.BinaryExpr{
				Left: ast.TupleExpr{
					Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					},
				},
				Op:    "*",
				Right: ast.NumberExpr{Raw: "1.5", FloatValue: 1.5},
				Span:  spanAt(33, 1),
			},
		},
		{
			name: "tuple multiply negative",
			expr: ast.BinaryExpr{
				Left: ast.TupleExpr{
					Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					},
				},
				Op:    "*",
				Right: ast.NumberExpr{Int: true, IntValue: -1},
				Span:  spanAt(34, 1),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			_ = EvalExprWithOptions(tc.expr, map[string]Value{}, diags, ExprOptions{
				GlobalAssignmentTupleArithmetic: true,
			})
			if count := diagCount(diags, "E106"); count == 0 {
				t.Fatalf("expected E106, got: %s", diags.String())
			}
		})
	}
}

func TestEvalTupleDefaultModeStaysVectorized(t *testing.T) {
	expr := ast.BinaryExpr{
		Left: ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1},
				ast.NumberExpr{Int: true, IntValue: 2},
				ast.NumberExpr{Int: true, IntValue: 3},
			},
		},
		Op:    "*",
		Right: ast.NumberExpr{Int: true, IntValue: 4},
		Span:  spanAt(35, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 3 {
		t.Fatalf("expected list vector result, got %#v", got)
	}
	if got.L[0].I != 4 || got.L[1].I != 8 || got.L[2].I != 12 {
		t.Fatalf("unexpected vector multiply result: %#v", got)
	}
}

func TestEvalTupleAndListConversionCalls(t *testing.T) {
	diags := &diag.Diagnostics{}
	tupleFromList := EvalExpr(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "tuple"},
		Args: ast.PosCallArgs(ast.ListExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1},
				ast.NumberExpr{Int: true, IntValue: 2},
			},
		}),
	}, map[string]Value{}, diags)
	if tupleFromList.Kind != KindTuple || len(tupleFromList.L) != 2 {
		t.Fatalf("expected tuple from list conversion, got %#v", tupleFromList)
	}

	listFromTuple := EvalExpr(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "list"},
		Args: ast.PosCallArgs(ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 3},
				ast.NumberExpr{Int: true, IntValue: 4},
			},
		}),
	}, map[string]Value{}, diags)
	if listFromTuple.Kind != KindList || len(listFromTuple.L) != 2 {
		t.Fatalf("expected list from tuple conversion, got %#v", listFromTuple)
	}

	singletonTuple := EvalExpr(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "tuple"},
		Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 9}),
	}, map[string]Value{}, diags)
	if singletonTuple.Kind != KindTuple || len(singletonTuple.L) != 1 || singletonTuple.L[0].I != 9 {
		t.Fatalf("expected singleton tuple conversion, got %#v", singletonTuple)
	}

	singletonList := EvalExpr(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "list"},
		Args:   ast.PosCallArgs(ast.StringExpr{Value: "x"}),
	}, map[string]Value{}, diags)
	if singletonList.Kind != KindList || len(singletonList.L) != 1 || singletonList.L[0].S != "x" {
		t.Fatalf("expected singleton list conversion, got %#v", singletonList)
	}

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestEvalTupleAndListRejectComb(t *testing.T) {
	comb := CombValue(&Comb{
		Order: []string{"x"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1), Origin: spanAt(200, 1)}}},
		},
	})
	env := map[string]Value{"m": comb}

	t.Run("tuple call rejects comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExpr(ast.CallExpr{
			Callee: ast.IdentExpr{Name: "tuple"},
			Args:   ast.PosCallArgs(ast.IdentExpr{Name: "m"}),
			Span:   spanAt(201, 1),
		}, env, diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null value for tuple(comb), got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected one E106, got: %s", diags.String())
		}
		if !strings.Contains(diags.String(), "tuple() does not accept table values") {
			t.Fatalf("expected tuple table rejection message, got: %s", diags.String())
		}
	})

	t.Run("list call rejects comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExpr(ast.CallExpr{
			Callee: ast.IdentExpr{Name: "list"},
			Args:   ast.PosCallArgs(ast.IdentExpr{Name: "m"}),
			Span:   spanAt(202, 1),
		}, env, diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null value for list(comb), got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected one E106, got: %s", diags.String())
		}
		if !strings.Contains(diags.String(), "list() does not accept table values") {
			t.Fatalf("expected list table rejection message, got: %s", diags.String())
		}
	})

}

func TestEvalConversionCalls(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		env      map[string]Value
		want     Value
		diagCode string
	}{
		{
			name: "bool from bool true",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.BoolExpr{Value: true}),
				Span:   spanAt(200, 1),
			},
			want: Bool(true),
		},
		{
			name: "bool from bool false",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.BoolExpr{Value: false}),
				Span:   spanAt(200, 10),
			},
			want: Bool(false),
		},
		{
			name: "bool from zero int",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 0}),
				Span:   spanAt(200, 20),
			},
			want: Bool(false),
		},
		{
			name: "bool from nonzero int",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 1}),
				Span:   spanAt(200, 30),
			},
			want: Bool(true),
		},
		{
			name: "bool from zero float",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.NumberExpr{FloatValue: 0.0}),
				Span:   spanAt(200, 40),
			},
			want: Bool(false),
		},
		{
			name: "bool from nonzero float",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.NumberExpr{FloatValue: 2.5}),
				Span:   spanAt(200, 50),
			},
			want: Bool(true),
		},
		{
			name: "bool from empty string",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: ""}),
				Span:   spanAt(200, 60),
			},
			want: Bool(false),
		},
		{
			name: "bool from non-empty string",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: "x"}),
				Span:   spanAt(200, 70),
			},
			want: Bool(true),
		},
		{
			name: "bool from null",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "n"}),
				Span:   spanAt(200, 80),
			},
			env:  map[string]Value{"n": Null()},
			want: Bool(false),
		},
		{
			name: "bool from empty list",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.ListExpr{}),
				Span:   spanAt(200, 90),
			},
			want: Bool(false),
		},
		{
			name: "bool from non-empty list",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.ListExpr{Items: []ast.Expr{ast.NumberExpr{Int: true, IntValue: 1}}}),
				Span:   spanAt(200, 100),
			},
			want: Bool(true),
		},
		{
			name: "bool from empty tuple",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.TupleExpr{}),
				Span:   spanAt(200, 110),
			},
			want: Bool(false),
		},
		{
			name: "bool from non-empty tuple",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.TupleExpr{Items: []ast.Expr{ast.NumberExpr{Int: true, IntValue: 1}}}),
				Span:   spanAt(200, 120),
			},
			want: Bool(true),
		},
		{
			name: "bool from empty table",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "m"}),
				Span:   spanAt(200, 130),
			},
			env: map[string]Value{
				"m": CombValue(&Comb{Order: []string{"x"}}),
			},
			want: Bool(false),
		},
		{
			name: "bool from non-empty table",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "m"}),
				Span:   spanAt(200, 140),
			},
			env: map[string]Value{
				"m": CombValue(&Comb{
					Order: []string{"x"},
					Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
				}),
			},
			want: Bool(true),
		},
		{
			name: "int from int",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 7}),
				Span:   spanAt(205, 1),
			},
			want: Int(7),
		},
		{
			name: "int from float truncates toward zero",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args:   ast.PosCallArgs(ast.UnaryExpr{Op: "-", Expr: ast.NumberExpr{Int: false, FloatValue: 7.9}, Span: spanAt(206, 5)}),
				Span:   spanAt(206, 1),
			},
			want: Int(-7),
		},
		{
			name: "int from bool",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args:   ast.PosCallArgs(ast.BoolExpr{Value: true}),
				Span:   spanAt(207, 1),
			},
			want: Int(1),
		},
		{
			name: "int from string",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: "42"}),
				Span:   spanAt(208, 1),
			},
			want: Int(42),
		},
		{
			name: "int rejects decimal string",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: "1.5"}),
				Span:   spanAt(209, 1),
			},
			want:     Null(),
			diagCode: "E106",
		},
		{
			name: "int rejects comb",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "m"}),
				Span:   spanAt(210, 1),
			},
			env: map[string]Value{
				"m": CombValue(&Comb{
					Order: []string{"x"},
					Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
				}),
			},
			want:     Null(),
			diagCode: "E106",
		},
		{
			name: "int rejects list",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
				),
				Span: spanAt(210, 20),
			},
			want:     Null(),
			diagCode: "E106",
		},
		{
			name: "float from int",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "float"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 7}),
				Span:   spanAt(211, 1),
			},
			want: Float(7.0),
		},
		{
			name: "float from bool",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "float"},
				Args:   ast.PosCallArgs(ast.BoolExpr{Value: false}),
				Span:   spanAt(212, 1),
			},
			want: Float(0.0),
		},
		{
			name: "float from exponent string",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "float"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: "1e3"}),
				Span:   spanAt(213, 1),
			},
			want: Float(1000.0),
		},
		{
			name: "float rejects non finite string",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "float"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: "NaN"}),
				Span:   spanAt(214, 1),
			},
			want:     Null(),
			diagCode: "E106",
		},
		{
			name: "float rejects tuple",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "float"},
				Args: ast.PosCallArgs(
					ast.TupleExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
				),
				Span: spanAt(214, 20),
			},
			want:     Null(),
			diagCode: "E106",
		},
		{
			name: "str from int",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "str"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 7}),
				Span:   spanAt(215, 1),
			},
			want: String("7"),
		},
		{
			name: "str from list uses whole value formatting",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "str"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
				),
				Span: spanAt(216, 1),
			},
			want: String("[1,2]"),
		},
		{
			name: "str from table",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "str"},
				Args:   ast.PosCallArgs(ast.IdentExpr{Name: "m"}),
				Span:   spanAt(217, 1),
			},
			env: map[string]Value{
				"m": CombValue(&Comb{
					Order: []string{"x"},
					Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
				}),
			},
			want: String("table(rows=1,cols=1)"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExpr(tc.expr, tc.env, diags)
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
			if tc.diagCode == "" {
				if diags.HasErrors() {
					t.Fatalf("unexpected diagnostics: %s", diags.String())
				}
				return
			}
			if diagCount(diags, tc.diagCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.diagCode, diags.String())
			}
		})
	}
}

func TestEvalUnaryConversionArityErrors(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{
			name: "bool no args",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Span:   spanAt(217, 1),
			},
		},
		{
			name: "bool two args",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "bool"},
				Args: ast.PosCallArgs(
					ast.NumberExpr{Int: true, IntValue: 1},
					ast.NumberExpr{Int: true, IntValue: 2},
				),
				Span: spanAt(217, 20),
			},
		},
		{
			name: "int no args",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "int"},
				Span:   spanAt(218, 1),
			},
		},
		{
			name: "float two args",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "float"},
				Args: ast.PosCallArgs(
					ast.NumberExpr{Int: true, IntValue: 1},
					ast.NumberExpr{Int: true, IntValue: 2},
				),
				Span: spanAt(219, 1),
			},
		},
		{
			name: "str two args",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "str"},
				Args: ast.PosCallArgs(
					ast.StringExpr{Value: "a"},
					ast.StringExpr{Value: "b"},
				),
				Span: spanAt(220, 1),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExpr(tc.expr, nil, diags)
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if diagCount(diags, "E106") != 1 {
				t.Fatalf("expected one E106, got: %s", diags.String())
			}
		})
	}
}

func TestEvalBoolConversionDoesNotWarnForExplicitTruthiness(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := EvalExpr(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "bool"},
		Args:   ast.PosCallArgs(ast.StringExpr{Value: "x"}),
		Span:   spanAt(221, 1),
	}, nil, diags)
	if got.Kind != KindBool || !got.B {
		t.Fatalf("expected true bool, got %#v", got)
	}
	if diagCount(diags, "W101") != 0 {
		t.Fatalf("explicit bool() should not warn, got: %s", diags.String())
	}
}

func TestEvalKernelCallsRangeRevTupleList(t *testing.T) {
	diags := &diag.Diagnostics{}
	opts := ExprOptions{Context: EvalCtxBindingAssign}

	rangeOne := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "range"},
		Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 5}),
	}, map[string]Value{}, diags, opts)
	if rangeOne.Kind != KindList || len(rangeOne.L) != 5 {
		t.Fatalf("expected range(5) list of len 5, got %#v", rangeOne)
	}
	for i, v := range rangeOne.L {
		if v.Kind != KindInt || v.I != int64(i) {
			t.Fatalf("unexpected range(5) value at %d: %#v", i, v)
		}
	}

	rangeStep := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "range"},
		Args: ast.PosCallArgs(
			ast.NumberExpr{Int: true, IntValue: 0},
			ast.NumberExpr{Int: true, IntValue: 10},
			ast.NumberExpr{Int: true, IntValue: 2},
		),
	}, map[string]Value{}, diags, opts)
	if rangeStep.Kind != KindList || len(rangeStep.L) != 5 {
		t.Fatalf("expected range(0,10,2) len 5, got %#v", rangeStep)
	}
	for i, want := range []int64{0, 2, 4, 6, 8} {
		if rangeStep.L[i].I != want {
			t.Fatalf("unexpected range(0,10,2) value at %d: %#v", i, rangeStep.L[i])
		}
	}
	rangeFloat := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "range"},
		Args: ast.PosCallArgs(
			ast.NumberExpr{Int: true, IntValue: 0},
			ast.NumberExpr{Int: false, FloatValue: 1.5},
			ast.NumberExpr{Int: false, FloatValue: 0.5},
		),
	}, map[string]Value{}, diags, opts)
	if rangeFloat.Kind != KindList || len(rangeFloat.L) != 3 {
		t.Fatalf("expected range(0,1.5,0.5) len 3, got %#v", rangeFloat)
	}
	for i, want := range []float64{0.0, 0.5, 1.0} {
		if rangeFloat.L[i].Kind != KindFloat || math.Abs(rangeFloat.L[i].F-want) > 1e-12 {
			t.Fatalf("unexpected range(0,1.5,0.5) value at %d: %#v", i, rangeFloat.L[i])
		}
	}
	rangeFloatSmallStep := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "range"},
		Args: ast.PosCallArgs(
			ast.NumberExpr{Int: true, IntValue: 0},
			ast.NumberExpr{Int: false, FloatValue: 1.5},
			ast.NumberExpr{Int: false, FloatValue: 0.01},
		),
	}, map[string]Value{}, diags, opts)
	if rangeFloatSmallStep.Kind != KindList || len(rangeFloatSmallStep.L) == 0 {
		t.Fatalf("expected non-empty range(0,1.5,0.01), got %#v", rangeFloatSmallStep)
	}
	last := rangeFloatSmallStep.L[len(rangeFloatSmallStep.L)-1]
	if last.Kind != KindFloat || !(last.F < 1.5) {
		t.Fatalf("expected last float value < 1.5, got %#v", last)
	}
	rangeNamed := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "range"},
		Args: []ast.CallArg{
			namedArg("start", ast.NumberExpr{Int: true, IntValue: 1}),
			namedArg("stop", ast.NumberExpr{Int: true, IntValue: 5}),
			namedArg("step", ast.NumberExpr{Int: true, IntValue: 2}),
		},
	}, map[string]Value{}, diags, opts)
	if !Equal(rangeNamed, List([]Value{Int(1), Int(3)})) {
		t.Fatalf("unexpected named range result: %#v", rangeNamed)
	}

	revList := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "rev"},
		Args: ast.PosCallArgs(
			ast.ListExpr{
				Items: []ast.Expr{
					ast.NumberExpr{Int: true, IntValue: 0},
					ast.NumberExpr{Int: true, IntValue: 1},
					ast.NumberExpr{Int: true, IntValue: 2},
				},
			},
		),
	}, map[string]Value{}, diags, opts)
	if revList.Kind != KindList || len(revList.L) != 3 {
		t.Fatalf("expected rev list len 3, got %#v", revList)
	}
	for i, want := range []int64{2, 1, 0} {
		if revList.L[i].I != want {
			t.Fatalf("unexpected rev value at %d: %#v", i, revList.L[i])
		}
	}
	revTuple := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "rev"},
		Args: ast.PosCallArgs(
			ast.TupleExpr{
				Items: []ast.Expr{
					ast.NumberExpr{Int: true, IntValue: 0},
					ast.NumberExpr{Int: true, IntValue: 1},
					ast.NumberExpr{Int: true, IntValue: 2},
				},
			},
		),
	}, map[string]Value{}, diags, opts)
	if revTuple.Kind != KindTuple || len(revTuple.L) != 3 {
		t.Fatalf("expected rev tuple len 3, got %#v", revTuple)
	}
	for i, want := range []int64{2, 1, 0} {
		if revTuple.L[i].I != want {
			t.Fatalf("unexpected rev tuple value at %d: %#v", i, revTuple.L[i])
		}
	}

	tupleVal := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "tuple"},
		Args: ast.PosCallArgs(
			ast.ListExpr{
				Items: []ast.Expr{
					ast.StringExpr{Value: "a"},
					ast.StringExpr{Value: "b"},
				},
			},
		),
	}, map[string]Value{}, diags, ExprOptions{})
	if tupleVal.Kind != KindTuple || len(tupleVal.L) != 2 {
		t.Fatalf("expected tuple conversion via call, got %#v", tupleVal)
	}

	listVal := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "list"},
		Args: ast.PosCallArgs(
			ast.TupleExpr{
				Items: []ast.Expr{
					ast.NumberExpr{Int: true, IntValue: 3},
					ast.NumberExpr{Int: true, IntValue: 4},
				},
			},
		),
	}, map[string]Value{}, diags, ExprOptions{})
	if listVal.Kind != KindList || len(listVal.L) != 2 {
		t.Fatalf("expected list conversion via call, got %#v", listVal)
	}
	lenNamed := EvalExprWithOptions(callExpr(ident("len"), namedArg("value", listExpr(intExpr(1), intExpr(2)))), nil, diags, ExprOptions{})
	if !Equal(lenNamed, Int(2)) {
		t.Fatalf("unexpected named len result: %#v", lenNamed)
	}

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestEvalKernelCallsRangeRevErrorsAndContext(t *testing.T) {
	tests := []struct {
		name       string
		expr       ast.Expr
		opts       ExprOptions
		wantCode   string
		wantNoErrs bool
	}{
		{
			name: "range arity error",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args:   nil,
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "range type error",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args:   ast.PosCallArgs(ast.StringExpr{Value: "x"}),
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "range two-arg float type error",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args: ast.PosCallArgs(
					ast.NumberExpr{Int: true, IntValue: 0},
					ast.NumberExpr{Int: false, FloatValue: 1.5},
				),
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "range step error",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args: ast.PosCallArgs(
					ast.NumberExpr{Int: true, IntValue: 0},
					ast.NumberExpr{Int: true, IntValue: 5},
					ast.NumberExpr{Int: true, IntValue: 0},
				),
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "range three-arg non-numeric type error",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args: ast.PosCallArgs(
					ast.NumberExpr{Int: true, IntValue: 0},
					ast.NumberExpr{Int: false, FloatValue: 1.5},
					ast.StringExpr{Value: "x"},
				),
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "rev non-sequence type error",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "rev"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 1}),
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name: "range context error outside top-level global assignment",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 3}),
			},
			opts:     ExprOptions{Context: EvalCtxScalarGlobalAssign},
			wantCode: "E199",
		},
		{
			name: "unknown function",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "unknown"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 1}),
			},
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E199",
		},
		{
			name: "range empty output for start greater than stop",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args: ast.PosCallArgs(
					ast.NumberExpr{Int: true, IntValue: 10},
					ast.NumberExpr{Int: true, IntValue: 1},
				),
			},
			opts:       ExprOptions{Context: EvalCtxBindingAssign},
			wantNoErrs: true,
		},
		{
			name: "range empty output for negative stop",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "range"},
				Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: -1}),
			},
			opts:       ExprOptions{Context: EvalCtxBindingAssign},
			wantNoErrs: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, map[string]Value{}, diags, tc.opts)
			if tc.wantNoErrs {
				if diags.HasErrors() {
					t.Fatalf("expected no errors, got: %s", diags.String())
				}
				if got.Kind != KindList || len(got.L) != 0 {
					t.Fatalf("expected empty list, got %#v", got)
				}
				return
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
		})
	}
}

func TestEvalCallUnsupportedCallee(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalCall(
		ast.NumberExpr{Int: true, IntValue: 7, Span: spanAt(73, 1)},
		nil,
		nil,
		spanAt(73, 1),
		diags,
		ExprOptions{Context: EvalCtxBindingAssign},
		&evalCtx{overflowWarned: map[string]struct{}{}},
	)
	if got.Kind != KindNull {
		t.Fatalf("expected null for unsupported callee, got %#v", got)
	}
	if diagCount(diags, "E199") != 1 {
		t.Fatalf("expected one E199, got: %s", diags.String())
	}
}

func TestBindingAssignTableBinarySupportsAliasOperand(t *testing.T) {
	env := map[string]Value{
		"x": Tuple([]Value{Int(1), Int(2)}),
		"a": CombValue(&Comb{
			Order: []string{"y"},
			Rows: []Row{
				{Values: map[string]Cell{"y": {Value: Int(3)}}},
				{Values: map[string]Cell{"y": {Value: Int(4)}}},
			},
		}),
	}
	expr := ast.BinaryExpr{
		Left: ast.AliasExpr{
			Expr:  ast.IdentExpr{Name: "x", Span: spanAt(86, 1)},
			Alias: "z",
			Span:  spanAt(86, 1),
		},
		Op:    "+",
		Right: ast.IdentExpr{Name: "a", Span: spanAt(86, 10)},
		Span:  spanAt(86, 1),
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(expr, env, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindComb || got.C == nil {
		t.Fatalf("expected comb result, got %#v", got)
	}
	if !slices.Equal(got.C.Order, []string{"y", "z"}) {
		t.Fatalf("expected columns [y z], got %#v", got.C.Order)
	}
}

func TestBindingAssignTableBinaryRejectsAliasOnTableOperand(t *testing.T) {
	env := map[string]Value{
		"x": Tuple([]Value{Int(1), Int(2)}),
		"a": CombValue(&Comb{
			Order: []string{"y"},
			Rows: []Row{
				{Values: map[string]Cell{"y": {Value: Int(3)}}},
				{Values: map[string]Cell{"y": {Value: Int(4)}}},
			},
		}),
	}
	expr := ast.BinaryExpr{
		Left: ast.AliasExpr{
			Expr:  ast.IdentExpr{Name: "a", Span: spanAt(87, 1)},
			Alias: "t",
			Span:  spanAt(87, 1),
		},
		Op:    "+",
		Right: ast.IdentExpr{Name: "x", Span: spanAt(87, 10)},
		Span:  spanAt(87, 1),
	}
	diags := &diag.Diagnostics{}
	_ = EvalExprWithOptions(expr, env, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diagCount(diags, "E106") == 0 {
		t.Fatalf("expected E106 for alias-on-table, got: %s", diags.String())
	}
}

func TestTableBuiltinsConstructMergeProductAndIndexProjection(t *testing.T) {
	span := spanAt(88, 1)
	env := map[string]Value{
		"ids":      Tuple([]Value{Int(1), Int(2)}),
		"labels":   Tuple([]Value{String("a"), String("b")}),
		"replicas": Tuple([]Value{Int(0), Int(1)}),
		"hosts":    Tuple([]Value{String("h0"), String("h1")}),
		"ports":    Tuple([]Value{Int(8080), Int(8081)}),
	}

	casesExpr := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "table", Span: span},
		Args: []ast.CallArg{
			{Name: "id", Expr: ast.IdentExpr{Name: "ids", Span: span}, Span: span},
			{Name: "label", Expr: ast.IdentExpr{Name: "labels", Span: span}, Span: span},
		},
		Span: span,
	}
	replicasExpr := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "table", Span: span},
		Args: []ast.CallArg{
			{Name: "replica", Expr: ast.IdentExpr{Name: "replicas", Span: span}, Span: span},
		},
		Span: span,
	}
	mergeExpr := ast.BinaryExpr{
		Left: ast.CallExpr{
			Callee: ast.IdentExpr{Name: "table", Span: span},
			Args: []ast.CallArg{
				{Name: "host", Expr: ast.IdentExpr{Name: "hosts", Span: span}, Span: span},
			},
			Span: span,
		},
		Op: "+",
		Right: ast.CallExpr{
			Callee: ast.IdentExpr{Name: "table", Span: span},
			Args: []ast.CallArg{
				{Name: "port", Expr: ast.IdentExpr{Name: "ports", Span: span}, Span: span},
			},
			Span: span,
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	cases := EvalExprWithOptions(casesExpr, env, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected table() diagnostics: %s", diags.String())
	}
	if !IsComb(cases) {
		t.Fatalf("expected table value from table(), got %#v", cases)
	}
	if !slices.Equal(cases.C.Order, []string{"id", "label"}) {
		t.Fatalf("unexpected table column order: %#v", cases.C.Order)
	}
	if len(cases.C.Rows) != 2 {
		t.Fatalf("expected 2 rows from table(), got %#v", cases.C.Rows)
	}

	diags = &diag.Diagnostics{}
	grid := EvalExprWithOptions(ast.BinaryExpr{
		Left:  ast.IdentExpr{Name: "cases", Span: span},
		Op:    "*",
		Right: replicasExpr,
		Span:  span,
	}, map[string]Value{"cases": cases, "replicas": env["replicas"]}, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected table product diagnostics: %s", diags.String())
	}
	if !IsComb(grid) || len(grid.C.Rows) != 4 {
		t.Fatalf("expected 4-row product table, got %#v", grid)
	}
	if !slices.Equal(grid.C.Order, []string{"id", "label", "replica"}) {
		t.Fatalf("unexpected product column order: %#v", grid.C.Order)
	}

	diags = &diag.Diagnostics{}
	view := EvalExprWithOptions(ast.IndexExpr{
		Base:  ast.IdentExpr{Name: "grid", Span: span},
		Items: []ast.Expr{stringExpr("id"), stringExpr("replica")},
		Span:  span,
	}, map[string]Value{"grid": grid}, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected table projection diagnostics: %s", diags.String())
	}
	if !IsComb(view) || !slices.Equal(view.C.Order, []string{"id", "replica"}) {
		t.Fatalf("unexpected projection result: %#v", view)
	}
	if len(view.C.Rows) != 4 {
		t.Fatalf("expected 4 selected rows, got %#v", view.C.Rows)
	}

	diags = &diag.Diagnostics{}
	merged := EvalExprWithOptions(mergeExpr, env, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected row-wise merge diagnostics: %s", diags.String())
	}
	if !IsComb(merged) || !slices.Equal(merged.C.Order, []string{"host", "port"}) {
		t.Fatalf("unexpected row-wise merge result: %#v", merged)
	}
	if len(merged.C.Rows) != 2 {
		t.Fatalf("expected 2 merged rows, got %#v", merged.C.Rows)
	}
}

func TestTableShortcutBuiltinConstructsTable(t *testing.T) {
	span := spanAt(89, 1)
	env := map[string]Value{
		"ids":    Tuple([]Value{Int(1), Int(2)}),
		"labels": Tuple([]Value{String("a"), String("b")}),
	}
	expr := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "t", Span: span},
		Args: []ast.CallArg{
			{Name: "id", Expr: ast.IdentExpr{Name: "ids", Span: span}, Span: span},
			{Name: "label", Expr: ast.IdentExpr{Name: "labels", Span: span}, Span: span},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(expr, env, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected t() diagnostics: %s", diags.String())
	}
	if !IsComb(got) {
		t.Fatalf("expected table value from t(), got %#v", got)
	}
	if !slices.Equal(got.C.Order, []string{"id", "label"}) {
		t.Fatalf("unexpected t() column order: %#v", got.C.Order)
	}
	if len(got.C.Rows) != 2 {
		t.Fatalf("expected 2 rows from t(), got %#v", got.C.Rows)
	}
}

func TestTableShortcutBuiltinShadowing(t *testing.T) {
	span := spanAt(90, 1)
	callT := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "t", Span: span},
		Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 2, Span: span}),
		Span:   span,
	}
	fn := EvalExprWithOptions(ast.FunctionExpr{
		Params: []ast.FuncParam{{Name: "x", Span: span}},
		Body: []ast.FuncBodyStmt{
			ast.ExprStmt{Expr: ast.BinaryExpr{
				Left:  ast.IdentExpr{Name: "x", Span: span},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 1, Span: span},
				Span:  span,
			}, Span: span},
		},
		Span: span,
	}, nil, &diag.Diagnostics{}, ExprOptions{})

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callT, map[string]Value{"t": fn}, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, Int(3)) {
		t.Fatalf("expected shadowed t function to run, got %#v", got)
	}

	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callT, map[string]Value{"t": Int(1)}, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if got.Kind != KindNull || diagCount(diags, "E199") == 0 {
		t.Fatalf("expected non-callable t binding to block table alias, got value=%#v diags=%s", got, diags.String())
	}
}

func TestEvalTupleAndListCallValidationBranches(t *testing.T) {
	t.Run("tuple arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalTupleCall([]Value{Int(1), Int(2)}, spanAt(74, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("tuple from tuple clones", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		src := Tuple([]Value{Int(1), Int(2)})
		got := evalTupleCall([]Value{src}, spanAt(75, 1), diags)
		if got.Kind != KindTuple || len(got.L) != 2 {
			t.Fatalf("expected tuple clone, got %#v", got)
		}
		src.L[0] = Int(9)
		if got.L[0].I != 1 {
			t.Fatalf("expected cloned tuple independent from source, got %#v", got)
		}
	})

	t.Run("list arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalListCall([]Value{}, spanAt(76, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("list scalar wraps", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalListCall([]Value{String("x")}, spanAt(77, 1), diags)
		if got.Kind != KindList || len(got.L) != 1 || got.L[0].Kind != KindString || got.L[0].S != "x" {
			t.Fatalf("expected scalar wrapped in list, got %#v", got)
		}
	})
}

func TestEvalRangeAndRevCornerBranches(t *testing.T) {
	t.Run("range null arg returns null without type diagnostic", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalRangeCall([]Value{Null()}, spanAt(78, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 0 {
			t.Fatalf("did not expect E106 for null short-circuit, got: %s", diags.String())
		}
	})

	t.Run("range overflow guard", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalRangeCall([]Value{Int(math.MaxInt64 - 1), Int(math.MaxInt64), Int(2)}, spanAt(79, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null on overflow guard, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106 overflow diagnostic, got: %s", diags.String())
		}
	})
	t.Run("range float non-progress guard", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalRangeCall([]Value{
			Float(10000000000000000),
			Float(10000000000000010),
			Float(1),
		}, spanAt(79, 10), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null for float non-progress guard, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106 non-progress diagnostic, got: %s", diags.String())
		}
	})

	t.Run("rev arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalRevCall([]Value{}, spanAt(80, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("rev null arg returns null without type diagnostic", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalRevCall([]Value{Null()}, spanAt(81, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 0 {
			t.Fatalf("did not expect E106 for null short-circuit, got: %s", diags.String())
		}
	})
}

func TestEvalConvert(t *testing.T) {
	tests := []struct {
		name   string
		target string
		input  Value
		want   Value
	}{
		{
			name:   "list to tuple",
			target: "tuple",
			input:  List([]Value{Int(1), Int(2)}),
			want:   Tuple([]Value{Int(1), Int(2)}),
		},
		{
			name:   "tuple to list",
			target: "list",
			input:  Tuple([]Value{String("a"), String("b")}),
			want:   List([]Value{String("a"), String("b")}),
		},
		{
			name:   "scalar to tuple singleton",
			target: "tuple",
			input:  Bool(true),
			want:   Tuple([]Value{Bool(true)}),
		},
		{
			name:   "scalar to list singleton",
			target: "list",
			input:  Int(9),
			want:   List([]Value{Int(9)}),
		},
		{
			name:   "empty list to tuple",
			target: "tuple",
			input:  List(nil),
			want:   Tuple(nil),
		},
		{
			name:   "empty tuple to list",
			target: "list",
			input:  Tuple(nil),
			want:   List(nil),
		},
		{
			name:   "string to int",
			target: "int",
			input:  String("42"),
			want:   Int(42),
		},
		{
			name:   "empty string to bool",
			target: "bool",
			input:  String(""),
			want:   Bool(false),
		},
		{
			name:   "non-empty string to bool",
			target: "bool",
			input:  String("x"),
			want:   Bool(true),
		},
		{
			name:   "zero int to bool",
			target: "bool",
			input:  Int(0),
			want:   Bool(false),
		},
		{
			name:   "non-empty tuple to bool",
			target: "bool",
			input:  Tuple([]Value{Int(1)}),
			want:   Bool(true),
		},
		{
			name:   "empty table to bool",
			target: "bool",
			input:  CombValue(&Comb{Order: []string{"x"}}),
			want:   Bool(false),
		},
		{
			name:   "bool to float",
			target: "float",
			input:  Bool(true),
			want:   Float(1.0),
		},
		{
			name:   "list to string",
			target: "str",
			input:  List([]Value{Int(1), Int(2)}),
			want:   String("[1,2]"),
		},
		{
			name:   "unknown target passthrough",
			target: "identity",
			input:  Float(1.5),
			want:   Float(1.5),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evalConvert(tc.target, tc.input, spanAt(70, 1), &diag.Diagnostics{})
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestEvalConvertRejectsInvalidScalarConversions(t *testing.T) {
	tests := []struct {
		name   string
		target string
		input  Value
	}{
		{
			name:   "int rejects malformed string",
			target: "int",
			input:  String("1.5"),
		},
		{
			name:   "float rejects tuple",
			target: "float",
			input:  Tuple([]Value{Int(1), Int(2)}),
		},
		{
			name:   "int rejects non finite float",
			target: "int",
			input:  Float(math.Inf(1)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalConvert(tc.target, tc.input, spanAt(73, 1), diags)
			if got.Kind != KindNull {
				t.Fatalf("expected null conversion result, got %#v", got)
			}
			if diagCount(diags, "E106") != 1 {
				t.Fatalf("expected one E106, got: %s", diags.String())
			}
		})
	}
}

func TestEvalConvertClonesSequenceValues(t *testing.T) {
	srcList := List([]Value{Int(1), Int(2)})
	convertedTuple := evalConvert("tuple", srcList, spanAt(71, 1), &diag.Diagnostics{})
	srcList.L[0] = Int(99)
	if convertedTuple.Kind != KindTuple || len(convertedTuple.L) != 2 {
		t.Fatalf("unexpected converted tuple: %#v", convertedTuple)
	}
	if convertedTuple.L[0].I != 1 {
		t.Fatalf("expected converted tuple to be independent clone, got %#v", convertedTuple)
	}

	srcTuple := Tuple([]Value{String("a"), String("b")})
	convertedList := evalConvert("list", srcTuple, spanAt(72, 1), &diag.Diagnostics{})
	srcTuple.L[1] = String("z")
	if convertedList.Kind != KindList || len(convertedList.L) != 2 {
		t.Fatalf("unexpected converted list: %#v", convertedList)
	}
	if convertedList.L[1].S != "b" {
		t.Fatalf("expected converted list to be independent clone, got %#v", convertedList)
	}
}

func TestEvalBinaryLogicalOperators(t *testing.T) {
	tests := []struct {
		name string
		op   string
		l    Value
		r    Value
		want bool
	}{
		{name: "amp true true", op: "&", l: Bool(true), r: Bool(true), want: true},
		{name: "amp true false", op: "&", l: Bool(true), r: Bool(false), want: false},
		{name: "pipe false false", op: "|", l: Bool(false), r: Bool(false), want: false},
		{name: "pipe false true", op: "|", l: Bool(false), r: Bool(true), want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalBinary(tc.op, tc.l, tc.r, spanAt(60, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got.Kind != KindBool || got.B != tc.want {
				t.Fatalf("unexpected boolean result: %#v", got)
			}
		})
	}
}

func TestLogicalOperatorAliasesParseAndEvaluateAsBooleans(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: "true & false", want: false},
		{src: "true && false", want: false},
		{src: "true and false", want: false},
		{src: "true | false", want: true},
		{src: "true || false", want: true},
		{src: "true or false", want: true},
	}
	for _, tc := range tests {
		got := evalParsedExprForTest(t, tc.src)
		if got.Kind != KindBool || got.B != tc.want {
			t.Fatalf("unexpected result for %q: %#v", tc.src, got)
		}
	}
}

func TestLogicalOperatorAliasesPreserveVectorBehavior(t *testing.T) {
	tests := []string{
		"[true, false] & [false, true]",
		"[true, false] && [false, true]",
		"[true, false] and [false, true]",
	}
	want := List([]Value{Bool(false), Bool(false)})
	for _, src := range tests {
		got := evalParsedExprForTest(t, src)
		if !Equal(got, want) {
			t.Fatalf("unexpected result for %q: %#v", src, got)
		}
	}

	tests = []string{
		"[true, false] | [false, true]",
		"[true, false] || [false, true]",
		"[true, false] or [false, true]",
	}
	want = List([]Value{Bool(true), Bool(true)})
	for _, src := range tests {
		got := evalParsedExprForTest(t, src)
		if !Equal(got, want) {
			t.Fatalf("unexpected result for %q: %#v", src, got)
		}
	}
}

func evalParsedExprForTest(t *testing.T, src string) Value {
	t.Helper()
	diags := &diag.Diagnostics{}
	expr, ok := parser.ParseStandaloneExpr("expr.jbs", src, diag.NewPos(0, 1, 1), diags)
	if !ok {
		t.Fatalf("expected standalone expression for %q", src)
	}
	if diags.HasErrors() {
		t.Fatalf("parse failed for %q: %s", src, diags.String())
	}
	got := EvalExpr(expr, map[string]Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("eval failed for %q: %s", src, diags.String())
	}
	return got
}

func TestEvalBinaryLogicalOperatorsCastAndBroadcast(t *testing.T) {
	t.Run("scalar cast", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalBinary("&", Int(1), String("x"), spanAt(61, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
		if got.Kind != KindBool || !got.B {
			t.Fatalf("expected true bool, got %#v", got)
		}
		if count := diagCount(diags, "W101"); count != 1 {
			t.Fatalf("expected one W101 cast warning, got %d: %s", count, diags.String())
		}
	})

	t.Run("vector cast and broadcast", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		left := Tuple([]Value{Int(1), Int(0), Int(2)})
		right := List([]Value{Bool(false)})
		got := evalBinary("|", left, right, spanAt(61, 8), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
		if got.Kind != KindList || len(got.L) != 3 {
			t.Fatalf("expected list of length 3, got %#v", got)
		}
		want := []bool{true, false, true}
		for i, v := range want {
			if got.L[i].Kind != KindBool || got.L[i].B != v {
				t.Fatalf("unexpected bool at %d: got=%#v want=%v", i, got.L[i], v)
			}
		}
		if count := diagCount(diags, "W101"); count != 2 {
			t.Fatalf("expected two W101 warnings (broadcast+cast), got %d: %s", count, diags.String())
		}
	})
}

func TestEvalBinaryLogicalNoWarningForPureBoolNoBroadcast(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalBinary("&", Bool(true), Bool(false), spanAt(61, 20), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
	if got.Kind != KindBool || got.B {
		t.Fatalf("unexpected bool result: %#v", got)
	}
	if len(diags.Items) != 0 {
		t.Fatalf("expected no diagnostics, got: %s", diags.String())
	}
}

func TestEvalBinaryStringOperations(t *testing.T) {
	type tc struct {
		name      string
		op        string
		l         Value
		r         Value
		want      Value
		wantError string
	}
	tests := []tc{
		{
			name: "concat string plus int",
			op:   "+",
			l:    String("ab"),
			r:    Int(3),
			want: String("ab3"),
		},
		{
			name: "repeat string times int",
			op:   "*",
			l:    String("ab"),
			r:    Int(3),
			want: String("ababab"),
		},
		{
			name: "repeat int times string",
			op:   "*",
			l:    Int(3),
			r:    String("ab"),
			want: String("ababab"),
		},
		{
			name: "repeat string times zero",
			op:   "*",
			l:    String("ab"),
			r:    Int(0),
			want: String(""),
		},
		{
			name:      "repeat string times negative",
			op:        "*",
			l:         String("ab"),
			r:         Int(-1),
			want:      Null(),
			wantError: "E105",
		},
		{
			name:      "repeat string times float",
			op:        "*",
			l:         String("ab"),
			r:         Float(2.5),
			want:      Null(),
			wantError: "E105",
		},
		{
			name:      "unsupported string operator",
			op:        "-",
			l:         String("ab"),
			r:         Int(1),
			want:      Null(),
			wantError: "E105",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalBinary(tc.op, tc.l, tc.r, spanAt(62, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
			if tc.wantError == "" {
				if diags.HasErrors() {
					t.Fatalf("unexpected errors: %s", diags.String())
				}
				return
			}
			if count := diagCount(diags, tc.wantError); count != 1 {
				t.Fatalf("expected one %s, got %d: %s", tc.wantError, count, diags.String())
			}
		})
	}
}

func TestEvalStringRepeatRejectsHugeOutputWithoutPanic(t *testing.T) {
	tests := []struct {
		name  string
		left  Value
		right Value
	}{
		{
			name:  "left string overflow",
			left:  String("ab"),
			right: Int(math.MaxInt64),
		},
		{
			name:  "right string overflow",
			left:  Int(math.MaxInt64),
			right: String("ab"),
		},
		{
			name:  "over allocation budget",
			left:  String("ab"),
			right: Int(int64(maxRepeatOutputUnits/2 + 1)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("repeat panicked: %v", r)
				}
			}()

			diags := &diag.Diagnostics{}
			got := evalBinary("*", tc.left, tc.right, spanAt(90, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if count := diagCount(diags, "E105"); count != 1 {
				t.Fatalf("expected one E105, got %d: %s", count, diags.String())
			}
		})
	}
}

func TestCheckedRepeatSize(t *testing.T) {
	tests := []struct {
		name        string
		elementSize int
		count       int64
		wantTotal   int
		wantOK      bool
	}{
		{name: "zero count", elementSize: 8, count: 0, wantTotal: 0, wantOK: true},
		{name: "zero element", elementSize: 0, count: 100, wantTotal: 0, wantOK: true},
		{name: "normal", elementSize: 2, count: 3, wantTotal: 6, wantOK: true},
		{name: "negative", elementSize: 2, count: -1, wantOK: false},
		{name: "overflow", elementSize: 2, count: math.MaxInt64, wantOK: false},
		{name: "budget", elementSize: 2, count: int64(maxRepeatOutputUnits/2 + 1), wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			total, _, ok := checkedRepeatSize(tc.elementSize, tc.count, diag.CodeE106, "test repetition", spanAt(92, 1), diags)
			if ok != tc.wantOK || total != tc.wantTotal {
				t.Fatalf("checkedRepeatSize() total=%d ok=%v, want total=%d ok=%v", total, ok, tc.wantTotal, tc.wantOK)
			}
		})
	}
}

func TestEvalBinaryNumericAndTypeErrors(t *testing.T) {
	tests := []struct {
		name     string
		op       string
		l        Value
		r        Value
		wantKind Kind
		wantInt  int64
		wantF    float64
		diagCode string
	}{
		{name: "numeric division", op: "/", l: Int(7), r: Int(2), wantKind: KindFloat, wantF: 3.5},
		{name: "numeric modulo", op: "%", l: Int(7), r: Int(3), wantKind: KindInt, wantInt: 1},
		{name: "division by zero", op: "/", l: Int(7), r: Int(0), wantKind: KindNull, diagCode: "E107"},
		{name: "modulo by zero", op: "%", l: Int(7), r: Int(0), wantKind: KindNull, diagCode: "E107"},
		{name: "modulo float operand", op: "%", l: Int(7), r: Float(2.0), wantKind: KindNull, diagCode: "E108"},
		{name: "unknown operator", op: "^", l: Int(2), r: Int(3), wantKind: KindNull, diagCode: "E109"},
		{name: "unsupported mixed types", op: "+", l: Bool(true), r: Int(1), wantKind: KindNull, diagCode: "E106"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalBinary(tc.op, tc.l, tc.r, spanAt(64, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
			if got.Kind != tc.wantKind {
				t.Fatalf("expected kind %s, got %#v", tc.wantKind, got)
			}
			if tc.wantKind == KindInt && got.I != tc.wantInt {
				t.Fatalf("expected int %d, got %#v", tc.wantInt, got)
			}
			if tc.wantKind == KindFloat && got.F != tc.wantF {
				t.Fatalf("expected float %v, got %#v", tc.wantF, got)
			}
			if tc.diagCode == "" {
				if diags.HasErrors() {
					t.Fatalf("unexpected errors: %s", diags.String())
				}
				return
			}
			if count := diagCount(diags, tc.diagCode); count != 1 {
				t.Fatalf("expected one %s, got %d: %s", tc.diagCode, count, diags.String())
			}
		})
	}
}

func TestEvalBinaryFloatArithmeticBranches(t *testing.T) {
	tests := []struct {
		name string
		op   string
		l    Value
		r    Value
		want float64
	}{
		{name: "float plus int", op: "+", l: Float(1.25), r: Int(2), want: 3.25},
		{name: "int minus float", op: "-", l: Int(5), r: Float(1.5), want: 3.5},
		{name: "float multiply int", op: "*", l: Float(2.5), r: Int(4), want: 10.0},
		{name: "float divide float", op: "/", l: Float(7.5), r: Float(2.5), want: 3.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalBinary(tc.op, tc.l, tc.r, spanAt(65, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got.Kind != KindFloat {
				t.Fatalf("expected float result kind, got %#v", got)
			}
			if got.F != tc.want {
				t.Fatalf("expected float %v, got %#v", tc.want, got)
			}
		})
	}
}

func TestMulInt64Checked(t *testing.T) {
	tests := []struct {
		name         string
		a            int64
		b            int64
		wantResult   int64
		wantOverflow bool
	}{
		{
			name:         "zero short-circuit",
			a:            0,
			b:            math.MaxInt64,
			wantResult:   0,
			wantOverflow: false,
		},
		{
			name:         "negative no overflow boundary equals 2^63",
			a:            math.MinInt64,
			b:            1,
			wantResult:   math.MinInt64,
			wantOverflow: false,
		},
		{
			name:         "negative overflow when magnitude exceeds 2^63 and hi is zero",
			a:            -4611686018427387905,
			b:            2,
			wantResult:   9223372036854775806,
			wantOverflow: true,
		},
		{
			name:         "positive no overflow max times one",
			a:            math.MaxInt64,
			b:            1,
			wantResult:   math.MaxInt64,
			wantOverflow: false,
		},
		{
			name:         "positive overflow when lo exceeds max int64",
			a:            math.MaxInt64,
			b:            2,
			wantResult:   -2,
			wantOverflow: true,
		},
		{
			name:         "hi non-zero overflow",
			a:            1 << 62,
			b:            4,
			wantResult:   0,
			wantOverflow: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotResult, gotOverflow := mulInt64Checked(tc.a, tc.b)
			if gotResult != tc.wantResult || gotOverflow != tc.wantOverflow {
				t.Fatalf(
					"mulInt64Checked(%d, %d) expected (%d, %v), got (%d, %v)",
					tc.a,
					tc.b,
					tc.wantResult,
					tc.wantOverflow,
					gotResult,
					gotOverflow,
				)
			}
		})
	}
}

func TestAbsInt64ToUint64Branches(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want uint64
	}{
		{
			name: "non-negative branch",
			in:   7,
			want: 7,
		},
		{
			name: "min-int64 special branch",
			in:   math.MinInt64,
			want: uint64(1) << 63,
		},
		{
			name: "negative regular branch",
			in:   -7,
			want: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := absInt64ToUint64(tc.in)
			if got != tc.want {
				t.Fatalf("absInt64ToUint64(%d) expected %d, got %d", tc.in, tc.want, got)
			}
		})
	}
}

func diagCount(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, d := range diags.Items {
		if d.Code == code {
			count++
		}
	}
	return count
}

func spanAt(line, col int) diag.Span {
	pos := diag.NewPos(0, line, col)
	return diag.NewSpan("eval_test.jbs", pos, pos)
}
