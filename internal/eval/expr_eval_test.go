package eval

import (
	"math"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
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

func TestEvalExprWithCtxModeExprPassthrough(t *testing.T) {
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
	got := evalExprWithCtx(
		ast.ModeExpr{
			Mode: "shell",
			Expr: ast.BinaryExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: 2},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 3},
				Span:  spanAt(84, 1),
			},
			Span: spanAt(84, 1),
		},
		map[string]Value{},
		diags,
		ExprOptions{},
		ctx,
	)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != 5 {
		t.Fatalf("expected passthrough evaluated value 5, got %#v", got)
	}
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
		name string
		op   string
		l    Value
		r    Value
		want bool
	}{
		{name: "eq-int-float", op: "==", l: Int(2), r: Float(2.0), want: true},
		{name: "ne-string", op: "!=", l: String("a"), r: String("b"), want: true},
		{name: "lt-string", op: "<", l: String("alpha"), r: String("beta"), want: true},
		{name: "le-string", op: "<=", l: String("beta"), r: String("beta"), want: true},
		{name: "ge-string", op: ">=", l: String("beta"), r: String("alpha"), want: true},
		{name: "gt-float-int", op: ">", l: Float(2.5), r: Int(2), want: true},
		{name: "ge-int-float", op: ">=", l: Int(3), r: Float(3.0), want: true},
		{name: "le-int-float", op: "<=", l: Int(3), r: Float(3.0), want: true},
		{name: "lt-int-int-false", op: "<", l: Int(7), r: Int(5), want: false},
		{name: "eq-bool", op: "==", l: Bool(true), r: Bool(true), want: true},
		{name: "ne-bool", op: "!=", l: Bool(true), r: Bool(false), want: true},
		{
			name: "eq-list",
			op:   "==",
			l:    List([]Value{Int(1), String("x")}),
			r:    List([]Value{Int(1), String("x")}),
			want: true,
		},
		{
			name: "ne-tuple",
			op:   "!=",
			l:    Tuple([]Value{Int(1), Int(2)}),
			r:    Tuple([]Value{Int(1), Int(3)}),
			want: true,
		},
		{
			name: "eq-list-vs-tuple-false",
			op:   "==",
			l:    List([]Value{Int(1), Int(2)}),
			r:    Tuple([]Value{Int(1), Int(2)}),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalCompare(tc.op, tc.l, tc.r, spanAt(20, 1), diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got.Kind != KindBool || got.B != tc.want {
				t.Fatalf("unexpected compare result: %#v (want bool=%v)", got, tc.want)
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
		ParamAssignmentTupleArithmetic: true,
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
		ParamAssignmentTupleArithmetic: true,
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
		ParamAssignmentTupleArithmetic: true,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Kind != KindTuple || len(got.L) != 0 {
		t.Fatalf("expected empty tuple for zero repetition, got %#v", got)
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
				ParamAssignmentTupleArithmetic: true,
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

func TestEvalTupleAndListConversions(t *testing.T) {
	diags := &diag.Diagnostics{}
	tupleFromList := EvalExpr(ast.ConvertExpr{
		Target: "tuple",
		Expr: ast.ListExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 1},
				ast.NumberExpr{Int: true, IntValue: 2},
			},
		},
	}, map[string]Value{}, diags)
	if tupleFromList.Kind != KindTuple || len(tupleFromList.L) != 2 {
		t.Fatalf("expected tuple from list conversion, got %#v", tupleFromList)
	}

	listFromTuple := EvalExpr(ast.ConvertExpr{
		Target: "list",
		Expr: ast.TupleExpr{
			Items: []ast.Expr{
				ast.NumberExpr{Int: true, IntValue: 3},
				ast.NumberExpr{Int: true, IntValue: 4},
			},
		},
	}, map[string]Value{}, diags)
	if listFromTuple.Kind != KindList || len(listFromTuple.L) != 2 {
		t.Fatalf("expected list from tuple conversion, got %#v", listFromTuple)
	}

	singletonTuple := EvalExpr(ast.ConvertExpr{
		Target: "tuple",
		Expr:   ast.NumberExpr{Int: true, IntValue: 9},
	}, map[string]Value{}, diags)
	if singletonTuple.Kind != KindTuple || len(singletonTuple.L) != 1 || singletonTuple.L[0].I != 9 {
		t.Fatalf("expected singleton tuple conversion, got %#v", singletonTuple)
	}
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
			name:   "unknown target passthrough",
			target: "identity",
			input:  Float(1.5),
			want:   Float(1.5),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evalConvert(tc.target, tc.input)
			if !Equal(got, tc.want) {
				t.Fatalf("expected %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestEvalConvertClonesSequenceValues(t *testing.T) {
	srcList := List([]Value{Int(1), Int(2)})
	convertedTuple := evalConvert("tuple", srcList)
	srcList.L[0] = Int(99)
	if convertedTuple.Kind != KindTuple || len(convertedTuple.L) != 2 {
		t.Fatalf("unexpected converted tuple: %#v", convertedTuple)
	}
	if convertedTuple.L[0].I != 1 {
		t.Fatalf("expected converted tuple to be independent clone, got %#v", convertedTuple)
	}

	srcTuple := Tuple([]Value{String("a"), String("b")})
	convertedList := evalConvert("list", srcTuple)
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
		{name: "and true true", op: "and", l: Bool(true), r: Bool(true), want: true},
		{name: "and true false", op: "and", l: Bool(true), r: Bool(false), want: false},
		{name: "or false false", op: "or", l: Bool(false), r: Bool(false), want: false},
		{name: "or false true", op: "or", l: Bool(false), r: Bool(true), want: true},
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

func TestEvalBinaryLogicalOperatorTypeError(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := evalBinary("and", Bool(true), Int(1), spanAt(61, 1), diags, ExprOptions{}, &evalCtx{overflowWarned: map[string]struct{}{}})
	if got.Kind != KindNull {
		t.Fatalf("expected null result on logical type error, got %#v", got)
	}
	if count := diagCount(diags, "E104"); count != 1 {
		t.Fatalf("expected one E104, got %d: %s", count, diags.String())
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
