package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestCollectShellLikeRefs(t *testing.T) {
	base := diag.NewPos(100, 10, 5)
	refs := collectShellLikeRefs(`echo $a ${b} $1 ${2} \$skip ${_ok}
$x ${bad`, base, "t.jbs")
	if len(refs) != 4 {
		t.Fatalf("expected 4 refs, got %d: %#v", len(refs), refs)
	}
	got := []string{refs[0].Name, refs[1].Name, refs[2].Name, refs[3].Name}
	want := []string{"a", "b", "_ok", "x"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ref %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestCollectExprStringRefs(t *testing.T) {
	p0 := diag.NewPos(0, 1, 1)
	p1 := diag.NewPos(20, 1, 21)
	p2 := diag.NewPos(21, 1, 22)
	p3 := diag.NewPos(35, 1, 36)
	left := ast.StringExpr{
		Value: "echo ${a}",
		Span:  diag.NewSpan("expr.jbs", p0, p1),
	}
	rightInner := ast.StringExpr{
		Value: "$b",
		Span:  diag.NewSpan("expr.jbs", p2, p3),
	}
	expr := ast.BinaryExpr{
		Left: left,
		Op:   "+",
		Right: ast.ModeExpr{
			Mode: "python",
			Expr: rightInner,
			Span: diag.NewSpan("expr.jbs", p2, p3),
		},
		Span: diag.NewSpan("expr.jbs", p0, p3),
	}
	refs := collectExprStringRefs(expr)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %#v", len(refs), refs)
	}
	if refs[0].Name != "a" || refs[1].Name != "b" {
		t.Fatalf("unexpected refs: %#v", refs)
	}
}

func TestResolveImportedVarsMixedFallback(t *testing.T) {
	sources := map[string]*ImportSource{
		"p1": {
			Name:  "p1",
			Kind:  SourceKindParam,
			Order: []string{"a", "x"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
				"x": {eval.Int(2)},
			},
		},
		"p2": {
			Name:  "p2",
			Kind:  SourceKindParam,
			Order: []string{"b"},
			Vars: map[string][]eval.Value{
				"b": {eval.String("v")},
			},
		},
	}
	items := []ast.WithItem{
		{Name: "x", From: "p1", Span: diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))},
		{Name: "p2", From: "p1", Span: diag.NewSpan("in.jbs", diag.NewPos(2, 1, 3), diag.NewPos(3, 1, 4))},
	}
	got := resolveImportedVars(items, sources)
	if len(got["x"]) != 1 || got["x"][0].Paramset != "p1" || got["x"][0].SourceVar != "x" {
		t.Fatalf("expected x imported from p1, got %#v", got["x"])
	}
	if len(got["b"]) != 1 || got["b"][0].Paramset != "p2" || got["b"][0].SourceVar != "b" {
		t.Fatalf("expected b imported from p2 fallback, got %#v", got["b"])
	}
}
