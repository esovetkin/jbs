package eval

import (
	"math"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestEvalLenCallBranches(t *testing.T) {
	t.Run("arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalLenCall([]Value{}, spanAt(300, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("list tuple string comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		if got := evalLenCall([]Value{List([]Value{Int(1), Int(2), Int(3)})}, spanAt(301, 1), diags); got.Kind != KindInt || got.I != 3 {
			t.Fatalf("unexpected len(list) result: %#v", got)
		}
		if got := evalLenCall([]Value{Tuple([]Value{Int(1), Int(2)})}, spanAt(302, 1), diags); got.Kind != KindInt || got.I != 2 {
			t.Fatalf("unexpected len(tuple) result: %#v", got)
		}
		if got := evalLenCall([]Value{String("aβ")}, spanAt(303, 1), diags); got.Kind != KindInt || got.I != 2 {
			t.Fatalf("unexpected len(string) result: %#v", got)
		}
		comb := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}}},
			},
		})
		if got := evalLenCall([]Value{comb}, spanAt(304, 1), diags); got.Kind != KindInt || got.I != 2 {
			t.Fatalf("unexpected len(comb) result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
	})

	t.Run("unsupported kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalLenCall([]Value{Bool(true)}, spanAt(305, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null for unsupported len target, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func TestEvalFilterCallBranches(t *testing.T) {
	t.Run("arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{List([]Value{Int(1)})}, spanAt(310, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("empty mask error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{
			List([]Value{Int(1), Int(2)}),
			List(nil),
		}, spanAt(311, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("list broadcast and cast warning", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{
			List([]Value{Int(1), Int(2), Int(3)}),
			List([]Value{Int(1)}),
		}, spanAt(312, 1), diags)
		if got.Kind != KindList || len(got.L) != 3 {
			t.Fatalf("expected full list due truthy broadcast mask, got %#v", got)
		}
		if diagCount(diags, "W101") != 1 {
			t.Fatalf("expected one W101 cast warning for divisible broadcast, got: %s", diags.String())
		}
		if hasDiagMessage(diags, "length mismatch in filter mask") {
			t.Fatalf("did not expect mismatch warning for divisible broadcast, got: %s", diags.String())
		}
	})

	t.Run("divisible broadcast has no mismatch warning", func(t *testing.T) {
		values := make([]Value, 0, 10)
		for i := int64(0); i < 10; i++ {
			values = append(values, Int(i))
		}
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{
			List(values),
			List([]Value{Bool(true), Bool(false)}),
		}, spanAt(312, 20), diags)
		if got.Kind != KindList || len(got.L) != 5 {
			t.Fatalf("expected five filtered values, got %#v", got)
		}
		want := []int64{0, 2, 4, 6, 8}
		for i, v := range got.L {
			if v.Kind != KindInt || v.I != want[i] {
				t.Fatalf("unexpected filtered value at %d: got=%#v want=%d", i, v, want[i])
			}
		}
		if hasDiagMessage(diags, "length mismatch in filter mask") {
			t.Fatalf("did not expect mismatch warning for divisible broadcast, got: %s", diags.String())
		}
		if diagCount(diags, "W101") != 0 {
			t.Fatalf("expected no W101 warnings for boolean divisible mask, got: %s", diags.String())
		}
	})

	t.Run("non-divisible broadcast emits mismatch warning", func(t *testing.T) {
		values := make([]Value, 0, 10)
		for i := int64(0); i < 10; i++ {
			values = append(values, Int(i))
		}
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{
			List(values),
			List([]Value{Bool(true), Bool(false), Bool(true)}),
		}, spanAt(312, 40), diags)
		if got.Kind != KindList || len(got.L) != 7 {
			t.Fatalf("expected seven filtered values, got %#v", got)
		}
		if !hasDiagMessage(diags, "length mismatch in filter mask") {
			t.Fatalf("expected mismatch warning for non-divisible broadcast, got: %s", diags.String())
		}
		if diagCount(diags, "W101") != 1 {
			t.Fatalf("expected one W101 mismatch warning, got: %s", diags.String())
		}
	})

	t.Run("tuple result preserves tuple kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{
			Tuple([]Value{Int(1), Int(2), Int(3)}),
			List([]Value{Bool(true), Bool(false), Bool(true)}),
		}, spanAt(313, 1), diags)
		if got.Kind != KindTuple || len(got.L) != 2 || got.L[0].I != 1 || got.L[1].I != 3 {
			t.Fatalf("unexpected filtered tuple: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
	})

	t.Run("comb nil payload short branch", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{
			CombValue(nil),
			Bool(true),
		}, spanAt(314, 1), diags)
		if got.Kind != KindComb || got.C == nil {
			t.Fatalf("expected comb result, got %#v", got)
		}
		if len(got.C.Rows) != 0 || len(got.C.Order) != 0 {
			t.Fatalf("expected empty comb payload, got %#v", got.C)
		}
	})

	t.Run("comb filter clones rows", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		base := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}}},
			},
		})
		got := evalFilterCall([]Value{
			base,
			List([]Value{Bool(false), Bool(true)}),
		}, spanAt(315, 1), diags)
		if !IsComb(got) || len(got.C.Rows) != 1 {
			t.Fatalf("expected one filtered comb row, got %#v", got)
		}
		got.C.Rows[0].Values["x"] = Cell{Value: Int(99)}
		if base.C.Rows[1].Values["x"].Value.I != 2 {
			t.Fatalf("expected filtered rows cloned from source")
		}
	})

	t.Run("unsupported target kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalFilterCall([]Value{Int(1), Bool(true)}, spanAt(316, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func hasDiagMessage(diags *diag.Diagnostics, needle string) bool {
	for _, item := range diags.Items {
		if strings.Contains(item.Message, needle) {
			return true
		}
	}
	return false
}

func TestEvalAllAnyCallBranches(t *testing.T) {
	t.Run("arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("all", []Value{}, spanAt(320, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("comb rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("any", []Value{CombValue(&Comb{})}, spanAt(321, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("empty list defaults", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		if got := evalAllAnyCall("all", []Value{List(nil)}, spanAt(322, 1), diags); got.Kind != KindBool || !got.B {
			t.Fatalf("expected all([])=true, got %#v", got)
		}
		if got := evalAllAnyCall("any", []Value{List(nil)}, spanAt(323, 1), diags); got.Kind != KindBool || got.B {
			t.Fatalf("expected any([])=false, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
	})

	t.Run("truthiness cast warning only once", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("all", []Value{List([]Value{Int(1), String("")})}, spanAt(324, 1), diags)
		if got.Kind != KindBool || got.B {
			t.Fatalf("expected all([1,\"\"])=false, got %#v", got)
		}
		if diagCount(diags, "W101") != 1 {
			t.Fatalf("expected one cast warning, got: %s", diags.String())
		}
	})
}

func TestExprEvalHelpersTruthyAndMask(t *testing.T) {
	comb := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	tests := []struct {
		name   string
		in     Value
		want   bool
		casted bool
	}{
		{name: "bool", in: Bool(true), want: true, casted: false},
		{name: "int", in: Int(0), want: false, casted: true},
		{name: "float", in: Float(2.0), want: true, casted: true},
		{name: "string", in: String(""), want: false, casted: true},
		{name: "null", in: Null(), want: false, casted: true},
		{name: "list", in: List([]Value{Int(1)}), want: true, casted: true},
		{name: "tuple", in: Tuple(nil), want: false, casted: true},
		{name: "comb", in: comb, want: true, casted: true},
		{name: "unknown", in: Value{Kind: Kind("mystery")}, want: true, casted: true},
	}
	for _, tc := range tests {
		got, casted := truthy(tc.in)
		if got != tc.want || casted != tc.casted {
			t.Fatalf("%s: expected (%v,%v), got (%v,%v)", tc.name, tc.want, tc.casted, got, casted)
		}
	}

	if got := toSeriesOrScalar(Int(7)); len(got) != 1 || got[0].I != 7 {
		t.Fatalf("unexpected scalar conversion: %#v", got)
	}
	seq := Tuple([]Value{Int(1), Int(2)})
	series := toSeriesOrScalar(seq)
	if len(series) != 2 {
		t.Fatalf("unexpected tuple conversion to series: %#v", series)
	}
	seq.L[0] = Int(99)
	if series[0].I != 1 {
		t.Fatalf("expected series clone independent from original")
	}

	diags := &diag.Diagnostics{}
	if got := broadcastMask([]Value{Bool(true)}, 0, spanAt(325, 1), diags); got != nil {
		t.Fatalf("expected nil mask for n<=0, got %#v", got)
	}
	if len(diags.Items) != 0 {
		t.Fatalf("did not expect diagnostics for n<=0, got: %s", diags.String())
	}
}

func TestBuiltinCallNames(t *testing.T) {
	names := BuiltinCallNames()
	for _, name := range []string{"range", "rev", "table", "t", "map", "reduce", "read_csv"} {
		if !slices.Contains(names, name) {
			t.Fatalf("BuiltinCallNames missing %q: %#v", name, names)
		}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("BuiltinCallNames must be sorted, got %#v", names)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			t.Fatalf("BuiltinCallNames contains duplicate %q: %#v", name, names)
		}
		seen[name] = struct{}{}
	}
	for _, name := range []string{"range", "table", "t"} {
		if !IsBuiltinCallName(name) {
			t.Fatalf("expected %q to be a builtin call name", name)
		}
	}
	for _, name := range []string{"shell", "python", "missing"} {
		if IsBuiltinCallName(name) {
			t.Fatalf("did not expect %q to be a builtin call name", name)
		}
	}
}

func TestEvalRangeFloatBranches(t *testing.T) {
	at := spanAt(340, 1)
	tests := []struct {
		name      string
		start     float64
		stop      float64
		step      float64
		wantKind  Kind
		wantLen   int
		wantCode  string
		wantError bool
	}{
		{
			name:      "reject non-finite input",
			start:     math.NaN(),
			stop:      1.0,
			step:      0.1,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:      "reject non-positive step",
			start:     0.0,
			stop:      1.0,
			step:      0.0,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:     "start greater or equal stop yields empty list",
			start:    2.0,
			stop:     2.0,
			step:     0.5,
			wantKind: KindList,
			wantLen:  0,
		},
		{
			name:      "step too small to make progress",
			start:     1e308,
			stop:      math.MaxFloat64,
			step:      1.0,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:      "overflow while generating values",
			start:     math.MaxFloat64 * 0.75,
			stop:      math.MaxFloat64,
			step:      math.MaxFloat64 * 0.75,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:     "valid float range",
			start:    0.0,
			stop:     1.5,
			step:     0.5,
			wantKind: KindList,
			wantLen:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalRangeFloat(tc.start, tc.stop, tc.step, at, diags)
			if got.Kind != tc.wantKind {
				t.Fatalf("unexpected kind: got=%s want=%s value=%#v", got.Kind, tc.wantKind, got)
			}
			if tc.wantKind == KindList && len(got.L) != tc.wantLen {
				t.Fatalf("unexpected list length: got=%d want=%d value=%#v", len(got.L), tc.wantLen, got)
			}
			if tc.wantError {
				if diagCount(diags, tc.wantCode) == 0 {
					t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
		})
	}
}

func TestBinaryNeedsRelaxedCombEvalCoverage(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{name: "nil", expr: nil, want: false},
		{name: "alias", expr: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "k"}, want: true},
		{name: "ident non alias", expr: ast.IdentExpr{Name: "n"}, want: false},
		{name: "qualified non alias", expr: ast.QualifiedIdentExpr{Namespace: "ns", Name: "col"}, want: false},
		{name: "binary recurse", expr: ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 2}, Alias: "c"}}, want: true},
		{name: "unary recurse", expr: ast.UnaryExpr{Op: "-", Expr: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}}, want: true},
		{name: "call recurse args", expr: ast.CallExpr{Callee: ast.IdentExpr{Name: "tuple"}, Args: ast.PosCallArgs(ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"})}, want: true},
		{name: "index recurse", expr: ast.IndexExpr{Base: ast.NumberExpr{Int: true, IntValue: 1}, Items: []ast.Expr{ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 2}, Alias: "x"}}}, want: true},
		{name: "member recurse", expr: ast.MemberExpr{Base: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}, Name: "x"}, want: true},
		{name: "list recurse", expr: ast.ListExpr{Items: []ast.Expr{ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}}}, want: true},
		{name: "tuple recurse", expr: ast.TupleExpr{Items: []ast.Expr{ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}}}, want: true},
		{name: "compare recurse", expr: ast.CompareExpr{Left: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}, Op: "==", Right: ast.NumberExpr{Int: true, IntValue: 1}}, want: true},
		{name: "conditional recurse", expr: ast.ConditionalExpr{Then: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}, Cond: ast.BoolExpr{Value: true}, Else: ast.NumberExpr{Int: true, IntValue: 0}}, want: true},
		{name: "default", expr: ast.NumberExpr{Int: true, IntValue: 1}, want: false},
	}
	for _, tc := range cases {
		if got := binaryNeedsRelaxedCombEval(tc.expr); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestCombRowsHelpersCoverage(t *testing.T) {
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
	env := map[string]Value{
		"a": Int(1),
	}

	if rows := combRowsFromBinaryOperand(nil, List([]Value{Int(1), Int(2)}), env, diags, ExprOptions{}, ctx); len(rows) != 2 {
		t.Fatalf("expected 2 rows for nil expr fallback, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.IdentExpr{Name: "a", Span: spanAt(330, 1)}, Int(3), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected one named row for ident, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.QualifiedIdentExpr{Namespace: "ns", Name: "x", Span: spanAt(331, 1)}, Int(4), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected one named row for qualified ident, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 5}, Alias: "z", Span: spanAt(332, 1)}, Int(0), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected one named row for alias helper, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.NumberExpr{Int: true, IntValue: 7, Span: spanAt(333, 1)}, Int(7), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected scalar fallback row, got %#v", rows)
	}

	base := []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}}
	combRows := combRowsFromValue(CombValue(&Comb{Order: []string{"x"}, Rows: base}), diag.Span{})
	if len(combRows) != 1 {
		t.Fatalf("expected cloned comb rows, got %#v", combRows)
	}
	combRows[0].Values["x"] = Cell{Value: Int(9)}
	if base[0].Values["x"].Value.I != 1 {
		t.Fatalf("expected combRowsFromValue to clone comb rows")
	}
}

func TestFirstDuplicatedColumnNameExtraBranches(t *testing.T) {
	if dup, ok := firstDuplicatedColumnName(nil, []Row{{Values: map[string]Cell{"a": {Value: Int(1)}}}}); ok || dup != "" {
		t.Fatalf("expected no duplicate for empty left, got %q", dup)
	}
	if dup, ok := firstDuplicatedColumnName([]Row{{Values: map[string]Cell{"a": {Value: Int(1)}}}}, nil); ok || dup != "" {
		t.Fatalf("expected no duplicate for empty right, got %q", dup)
	}
	if dup, ok := firstDuplicatedColumnName([]Row{{Values: map[string]Cell{}}}, []Row{{Values: map[string]Cell{"a": {Value: Int(1)}}}}); ok || dup != "" {
		t.Fatalf("expected no duplicate for empty left-name set, got %q", dup)
	}
	if dup, ok := firstDuplicatedColumnName(
		[]Row{{Values: map[string]Cell{"a": {Value: Int(1)}}}},
		[]Row{{Values: map[string]Cell{"a": {Value: Int(2)}}}},
	); !ok || dup != "a" {
		t.Fatalf("expected duplicate column 'a', got (%q,%v)", dup, ok)
	}
}

func TestEvalExprWithCtxDefaultUnsupportedNode(t *testing.T) {
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
	expr := &ast.StringExpr{Value: "x", Span: spanAt(340, 1)}
	got := evalExprWithCtx(expr, map[string]Value{}, diags, ExprOptions{}, ctx)
	if got.Kind != KindNull {
		t.Fatalf("expected null for unsupported pointer node, got %#v", got)
	}
	if diagCount(diags, "E199") != 1 {
		t.Fatalf("expected one E199, got: %s", diags.String())
	}
}

func TestEvalBuiltinCallsIntegration(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "len call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "len"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
				),
			},
			want: Int(2),
		},
		{
			name: "filter call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "filter"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
					ast.ListExpr{Items: []ast.Expr{
						ast.BoolExpr{Value: false},
						ast.BoolExpr{Value: true},
					}},
				),
			},
			want: List([]Value{Int(2)}),
		},
		{
			name: "all call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "all"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{ast.BoolExpr{Value: true}, ast.BoolExpr{Value: true}}},
				),
			},
			want: Bool(true),
		},
		{
			name: "any call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "any"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{ast.BoolExpr{Value: false}, ast.BoolExpr{Value: true}}},
				),
			},
			want: Bool(true),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExpr(tc.expr, map[string]Value{}, diags)
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected builtin-call result: got=%#v want=%#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
		})
	}
}
