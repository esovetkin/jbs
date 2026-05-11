package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func indexExprForTest(base ast.Expr, items ...ast.Expr) ast.IndexExpr {
	return ast.IndexExpr{Base: base, Items: items, Span: spanAt(1400, 1)}
}

func listSelectorForTest(items ...ast.Expr) ast.ListExpr {
	return ast.ListExpr{Items: items, Span: spanAt(1400, 5)}
}

func tupleSelectorForTest(items ...ast.Expr) ast.TupleExpr {
	return ast.TupleExpr{Items: items, Span: spanAt(1400, 5)}
}

func TestSequenceIndexScalarInteger(t *testing.T) {
	env := map[string]Value{
		"xs": List([]Value{Int(10), Int(20), Int(30)}),
		"ys": Tuple([]Value{String("a"), String("b")}),
	}
	cases := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "list first",
			expr: indexExprForTest(ident("xs"), intExpr(0)),
			want: Int(10),
		},
		{
			name: "list middle",
			expr: indexExprForTest(ident("xs"), intExpr(1)),
			want: Int(20),
		},
		{
			name: "list negative",
			expr: indexExprForTest(ident("xs"), intExpr(-1)),
			want: Int(30),
		},
		{
			name: "tuple scalar",
			expr: indexExprForTest(ident("ys"), intExpr(1)),
			want: String("b"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestSequenceIndexScalarClonesNestedValue(t *testing.T) {
	base := List([]Value{List([]Value{Int(1)})})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(indexExprForTest(ident("xs"), intExpr(0)), map[string]Value{"xs": base}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	got.L[0] = Int(99)
	if base.L[0].L[0].I != 1 {
		t.Fatalf("scalar index did not clone nested value")
	}
}

func TestSequenceIndexIntegerSelector(t *testing.T) {
	env := map[string]Value{
		"xs": List([]Value{Int(10), Int(20), Int(30)}),
		"ys": Tuple([]Value{String("a"), String("b"), String("c")}),
	}
	cases := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "list gather with negative index",
			expr: indexExprForTest(ident("xs"), listSelectorForTest(intExpr(0), intExpr(-1))),
			want: List([]Value{Int(10), Int(30)}),
		},
		{
			name: "tuple gather preserves tuple",
			expr: indexExprForTest(ident("ys"), listSelectorForTest(intExpr(2), intExpr(0))),
			want: Tuple([]Value{String("c"), String("a")}),
		},
		{
			name: "tuple selector works",
			expr: indexExprForTest(ident("xs"), tupleSelectorForTest(intExpr(1), intExpr(0))),
			want: List([]Value{Int(20), Int(10)}),
		},
		{
			name: "duplicate indexes",
			expr: indexExprForTest(ident("xs"), listSelectorForTest(intExpr(0), intExpr(0), intExpr(-1))),
			want: List([]Value{Int(10), Int(10), Int(30)}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestSequenceIndexEmptySelectorReturnsEmptyResult(t *testing.T) {
	cases := []struct {
		name string
		base Value
		want Kind
	}{
		{name: "list", base: List([]Value{Int(1)}), want: KindList},
		{name: "tuple", base: Tuple([]Value{Int(1)}), want: KindTuple},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(indexExprForTest(ident("xs"), listSelectorForTest()), map[string]Value{"xs": tc.base}, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if got.Kind != tc.want || len(got.L) != 0 {
				t.Fatalf("unexpected empty selector result: %#v", got)
			}
		})
	}
}

func TestSequenceIndexGatherClonesNestedValues(t *testing.T) {
	base := List([]Value{List([]Value{Int(1)}), List([]Value{Int(2)})})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(indexExprForTest(ident("xs"), listSelectorForTest(intExpr(0), intExpr(1))), map[string]Value{"xs": base}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	got.L[0].L[0] = Int(99)
	if base.L[0].L[0].I != 1 {
		t.Fatalf("gather index did not clone nested value")
	}
}

func TestSequenceIndexBooleanMask(t *testing.T) {
	env := map[string]Value{
		"xs": List([]Value{Int(1), Int(2), Int(3), Int(4)}),
		"zs": List([]Value{Int(1), Int(2), Int(3), Int(4), Int(5)}),
		"ys": Tuple([]Value{String("a"), String("b"), String("c")}),
	}
	cases := []struct {
		name      string
		expr      ast.Expr
		want      Value
		wantWarns int
	}{
		{
			name: "exact length list mask",
			expr: indexExprForTest(ident("xs"), listSelectorForTest(boolExpr(true), boolExpr(false), boolExpr(true), boolExpr(false))),
			want: List([]Value{Int(1), Int(3)}),
		},
		{
			name: "even broadcast",
			expr: indexExprForTest(ident("xs"), listSelectorForTest(boolExpr(true), boolExpr(false))),
			want: List([]Value{Int(1), Int(3)}),
		},
		{
			name:      "uneven broadcast warning",
			expr:      indexExprForTest(ident("zs"), listSelectorForTest(boolExpr(true), boolExpr(false))),
			want:      List([]Value{Int(1), Int(3), Int(5)}),
			wantWarns: 1,
		},
		{
			name:      "mask longer than sequence",
			expr:      indexExprForTest(ident("ys"), listSelectorForTest(boolExpr(false), boolExpr(true), boolExpr(true), boolExpr(false))),
			want:      Tuple([]Value{String("b"), String("c")}),
			wantWarns: 1,
		},
		{
			name: "tuple mask preserves tuple",
			expr: indexExprForTest(ident("ys"), listSelectorForTest(boolExpr(true), boolExpr(false), boolExpr(true))),
			want: Tuple([]Value{String("a"), String("c")}),
		},
		{
			name: "all false returns empty base kind",
			expr: indexExprForTest(ident("ys"), listSelectorForTest(boolExpr(false), boolExpr(false), boolExpr(false))),
			want: Tuple(nil),
		},
		{
			name: "tuple selector mask",
			expr: indexExprForTest(ident("xs"), tupleSelectorForTest(boolExpr(false), boolExpr(true))),
			want: List([]Value{Int(2), Int(4)}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
			if warns := diagCount(diags, "W101"); warns != tc.wantWarns {
				t.Fatalf("unexpected warning count: got=%d want=%d diagnostics=%s", warns, tc.wantWarns, diags.String())
			}
			if tc.wantWarns > 0 && !hasDiagMessage(diags, "length mismatch in sequence mask") {
				t.Fatalf("expected sequence mask warning, got: %s", diags.String())
			}
		})
	}
}

func TestSequenceIndexMaskClonesNestedValues(t *testing.T) {
	base := List([]Value{List([]Value{Int(1)}), List([]Value{Int(2)})})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(indexExprForTest(ident("xs"), listSelectorForTest(boolExpr(true), boolExpr(false))), map[string]Value{"xs": base}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	got.L[0].L[0] = Int(99)
	if base.L[0].L[0].I != 1 {
		t.Fatalf("mask index did not clone nested value")
	}
}

func TestSequenceIndexDiagnostics(t *testing.T) {
	span := spanAt(1401, 1)
	tableValue := CombValue(&Comb{Order: []string{"x"}, Rows: []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}}})
	env := map[string]Value{
		"xs":    List([]Value{Int(1), Int(2)}),
		"empty": List(nil),
		"nil":   Null(),
		"d":     DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)}}),
		"c":     tableValue,
		"fn":    Function(&FunctionValue{}),
		"n":     Int(1),
	}
	cases := []struct {
		name string
		expr ast.Expr
	}{
		{name: "empty bracket selectors", expr: ast.IndexExpr{Base: ident("xs"), Span: span}},
		{name: "multiple bracket selectors", expr: indexExprForTest(ident("xs"), intExpr(0), intExpr(1))},
		{name: "float scalar selector", expr: indexExprForTest(ident("xs"), ast.NumberExpr{FloatValue: 1.0, Span: span})},
		{name: "string scalar selector", expr: indexExprForTest(ident("xs"), stringExpr("x"))},
		{name: "bool scalar selector", expr: indexExprForTest(ident("xs"), boolExpr(true))},
		{name: "null scalar selector", expr: indexExprForTest(ident("xs"), ident("nil"))},
		{name: "dict scalar selector", expr: indexExprForTest(ident("xs"), ident("d"))},
		{name: "table scalar selector", expr: indexExprForTest(ident("xs"), ident("c"))},
		{name: "function scalar selector", expr: indexExprForTest(ident("xs"), ident("fn"))},
		{name: "positive out of range", expr: indexExprForTest(ident("xs"), intExpr(2))},
		{name: "negative out of range", expr: indexExprForTest(ident("xs"), intExpr(-3))},
		{name: "empty sequence scalar", expr: indexExprForTest(ident("empty"), intExpr(0))},
		{name: "gather out of range", expr: indexExprForTest(ident("xs"), listSelectorForTest(intExpr(0), intExpr(2)))},
		{name: "mixed int bool selector", expr: indexExprForTest(ident("xs"), listSelectorForTest(intExpr(0), boolExpr(true)))},
		{name: "mixed bool int selector", expr: indexExprForTest(ident("xs"), listSelectorForTest(boolExpr(true), intExpr(1)))},
		{name: "unsupported selector item", expr: indexExprForTest(ident("xs"), listSelectorForTest(ast.NumberExpr{FloatValue: 1.0, Span: span}))},
		{name: "unsupported base", expr: indexExprForTest(ident("n"), intExpr(0))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 {
				t.Fatalf("expected E106, got: %s", diags.String())
			}
			if tc.name == "unsupported base" && !strings.Contains(diags.String(), "list, tuple, dictionary, or table base") {
				t.Fatalf("unsupported base diagnostic did not mention accepted base kinds: %s", diags.String())
			}
		})
	}
}

func TestSequenceIndexDoesNotChangeDictionaryBoolKeys(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		indexExprForTest(ident("d"), boolExpr(true)),
		map[string]Value{"d": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyBool, B: true}, Value: String("ok")}})},
		diags,
		ExprOptions{},
	)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, String("ok")) {
		t.Fatalf("unexpected dictionary bool-key result: %#v", got)
	}
}
