package eval

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func stringExpr(value string) ast.StringExpr {
	return ast.StringExpr{Value: value}
}

func boolExpr(value bool) ast.BoolExpr {
	return ast.BoolExpr{Value: value}
}

func TestDictValueCloneEqualStringAndIteration(t *testing.T) {
	original := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "name"}, Value: String("case")},
		{Key: DictKey{Kind: DictKeyInt, I: 2}, Value: List([]Value{Int(1)})},
		{Key: DictKey{Kind: DictKeyBool, B: true}, Value: Bool(false)},
	})
	if original.Kind != KindDict || original.D == nil {
		t.Fatalf("expected dictionary value, got %#v", original)
	}
	if got := original.String(); got != "{name:case,2:[1],true:false}" {
		t.Fatalf("unexpected dictionary string: %q", got)
	}

	clone := CloneValue(original)
	clone.D.Set(DictKey{Kind: DictKeyString, S: "name"}, String("changed"))
	if original.D.Entries[DictKey{Kind: DictKeyString, S: "name"}].S != "case" {
		t.Fatalf("CloneValue did not deep-copy dictionary entries")
	}

	sameContent := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyBool, B: true}, Value: Bool(false)},
		{Key: DictKey{Kind: DictKeyInt, I: 2}, Value: List([]Value{Int(1)})},
		{Key: DictKey{Kind: DictKeyString, S: "name"}, Value: String("case")},
	})
	if !Equal(original, sameContent) {
		t.Fatalf("dictionary equality should ignore insertion order")
	}

	diags := &diag.Diagnostics{}
	keys, ok := IterableElements(original, spanAt(1300, 1), diags)
	if !ok || diags.HasErrors() {
		t.Fatalf("dictionary should be iterable by keys: %s", diags.String())
	}
	want := []Value{String("name"), Int(2), Bool(true)}
	if len(keys) != len(want) {
		t.Fatalf("unexpected key count: %#v", keys)
	}
	for i := range want {
		if !Equal(keys[i], want[i]) {
			t.Fatalf("unexpected key %d: got %#v want %#v", i, keys[i], want[i])
		}
	}
}

func TestEvalDictLiteralCallGetUpdateMergeAndIndex(t *testing.T) {
	span := spanAt(1301, 1)
	literal := ast.DictExpr{
		Entries: []ast.DictEntryExpr{
			{Key: ast.IdentExpr{Name: "k", Span: span}, Value: intExpr(1), Span: span},
			{Key: intExpr(7), Value: stringExpr("seven"), Span: span},
		},
		Span: span,
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(literal, map[string]Value{"k": String("name")}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindDict || got.D.Entries[DictKey{Kind: DictKeyString, S: "name"}].I != 1 {
		t.Fatalf("unexpected literal value: %#v", got)
	}

	base := EvalExprWithOptions(
		callExpr(ident("dict"), namedArg("a", intExpr(1)), namedArg("b", stringExpr("x"))),
		nil,
		diags,
		ExprOptions{},
	)
	updated := EvalExprWithOptions(
		callExpr(ident("update"), posArg(ident("d")), namedArg("b", intExpr(2)), namedArg("c", boolExpr(true))),
		map[string]Value{"d": base},
		diags,
		ExprOptions{},
	)
	updatedNamedBase := EvalExprWithOptions(
		callExpr(ident("update"), namedArg("dict", ident("d")), namedArg("c", intExpr(3))),
		map[string]Value{"d": base},
		diags,
		ExprOptions{},
	)
	if diags.HasErrors() {
		t.Fatalf("unexpected update diagnostics: %s", diags.String())
	}
	if base.D.Entries[DictKey{Kind: DictKeyString, S: "b"}].S != "x" {
		t.Fatalf("update mutated the original dictionary: %#v", base)
	}
	if updated.D.Entries[DictKey{Kind: DictKeyString, S: "b"}].I != 2 {
		t.Fatalf("update did not replace b: %#v", updated)
	}
	if updatedNamedBase.D.Entries[DictKey{Kind: DictKeyString, S: "c"}].I != 3 {
		t.Fatalf("update(dict=...) did not set c: %#v", updatedNamedBase)
	}

	spread := EvalExprWithOptions(
		callExpr(ident("dict"), kwSpreadArg(ident("entries"))),
		map[string]Value{"entries": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "c"}, Value: Int(3)}})},
		diags,
		ExprOptions{},
	)
	if spread.Kind != KindDict || spread.D.Entries[DictKey{Kind: DictKeyString, S: "c"}].I != 3 {
		t.Fatalf("unexpected dict(**entries) result: %#v", spread)
	}

	merged := EvalExprWithOptions(
		ast.BinaryExpr{Left: ident("left"), Op: "+", Right: ident("right"), Span: span},
		map[string]Value{"left": base, "right": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "b"}, Value: Int(9)}})},
		diags,
		ExprOptions{},
	)
	if merged.D.Entries[DictKey{Kind: DictKeyString, S: "b"}].I != 9 {
		t.Fatalf("dictionary merge did not prefer right-hand values: %#v", merged)
	}

	indexed := EvalExprWithOptions(
		ast.IndexExpr{Base: ident("d"), Items: []ast.Expr{stringExpr("b")}, Span: span},
		map[string]Value{"d": updated},
		diags,
		ExprOptions{},
	)
	if !Equal(indexed, Int(2)) {
		t.Fatalf("unexpected indexed value: %#v", indexed)
	}

	fallback := EvalExprWithOptions(
		callExpr(ident("get"), posArg(ident("d")), posArg(stringExpr("missing")), posArg(intExpr(44))),
		map[string]Value{"d": updated},
		diags,
		ExprOptions{},
	)
	if !Equal(fallback, Int(44)) {
		t.Fatalf("unexpected get fallback: %#v", fallback)
	}

	if got := EvalExprWithOptions(callExpr(ident("len"), posArg(ident("d"))), map[string]Value{"d": updated}, diags, ExprOptions{}); !Equal(got, Int(3)) {
		t.Fatalf("unexpected len(dict): %#v", got)
	}
	if got := EvalExprWithOptions(callExpr(ident("bool"), posArg(ast.DictExpr{})), nil, diags, ExprOptions{}); !Equal(got, Bool(false)) {
		t.Fatalf("empty dictionary should be false, got %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestDictDiagnostics(t *testing.T) {
	span := spanAt(1302, 1)
	cases := []ast.Expr{
		callExpr(ident("dict")),
		callExpr(ident("dict"), posArg(intExpr(1))),
		callExpr(ident("get"), posArg(ast.DictExpr{}), namedArg("key", stringExpr("x")), posArg(intExpr(0))),
		callExpr(ident("update"), posArg(ast.DictExpr{}), posArg(intExpr(1))),
		ast.DictExpr{Entries: []ast.DictEntryExpr{{Key: ast.ListExpr{Items: []ast.Expr{intExpr(1)}, Span: span}, Value: intExpr(1), Span: span}}, Span: span},
		ast.BinaryExpr{Left: ast.DictExpr{}, Op: "+", Right: intExpr(1), Span: span},
		ast.IndexExpr{Base: ast.DictExpr{}, Items: []ast.Expr{stringExpr("missing")}, Span: span},
	}
	for _, expr := range cases {
		diags := &diag.Diagnostics{}
		_ = EvalExprWithOptions(expr, nil, diags, ExprOptions{})
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106 for %#v, got %s", expr, diags.String())
		}
	}
}
