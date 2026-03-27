package eval

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestZipBroadcastWarning(t *testing.T) {
	expr := ast.CombBinary{
		Left:   ast.CombIdent{Name: "a"},
		Op:     "+",
		OpSpan: diag.NewSpan("in.jbs", diag.NewPos(10, 1, 10), diag.NewPos(11, 1, 11)),
		Right:  ast.CombIdent{Name: "b"},
	}
	series := map[string][]Value{
		"a": {Int(1), Int(2)},
		"b": {String("x"), String("y"), String("z")},
	}
	origins := map[string]diag.Span{
		"a": diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2)),
		"b": diag.NewSpan("in.jbs", diag.NewPos(3, 1, 3), diag.NewPos(4, 1, 4)),
	}
	diags := &diag.Diagnostics{}
	rows := EvalCombination(expr, series, origins, diags)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[2].Values["a"].Value.I != 1 || rows[2].Values["b"].Value.S != "z" {
		t.Fatalf("unexpected broadcast result in row 3: %#v", rows[2].Values)
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == "W101" {
			found = true
			if d.Span.Start.Line == 0 {
				t.Fatalf("W101 missing source span")
			}
		}
	}
	if !found {
		t.Fatalf("expected W101 warning, got: %s", diags.String())
	}
}

func TestRepeatedIdentifierError(t *testing.T) {
	expr := ast.CombBinary{
		Left:   ast.CombIdent{Name: "a", Span: diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))},
		Op:     "+",
		OpSpan: diag.NewSpan("in.jbs", diag.NewPos(3, 1, 3), diag.NewPos(4, 1, 4)),
		Right:  ast.CombIdent{Name: "a", Span: diag.NewSpan("in.jbs", diag.NewPos(5, 1, 5), diag.NewPos(6, 1, 6))},
	}
	series := map[string][]Value{
		"a": {Int(1), Int(2)},
	}
	diags := &diag.Diagnostics{}
	_ = EvalCombination(expr, series, map[string]diag.Span{}, diags)

	found := false
	for _, d := range diags.Items {
		if d.Code == "E036" {
			found = true
			if len(d.Related) == 0 {
				t.Fatalf("expected related span for E036")
			}
		}
	}
	if !found {
		t.Fatalf("expected E036, got: %s", diags.String())
	}
}

func TestConflictMergeError(t *testing.T) {
	left := Row{Values: map[string]Cell{"x": {Value: Int(1), Origin: diag.NewSpan("l", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))}}}
	right := Row{Values: map[string]Cell{"x": {Value: Int(2), Origin: diag.NewSpan("r", diag.NewPos(3, 1, 3), diag.NewPos(4, 1, 4))}}}
	diags := &diag.Diagnostics{}
	_, ok := mergeRows(left, right, diag.NewSpan("in", diag.NewPos(7, 1, 7), diag.NewPos(8, 1, 8)), diags)
	if ok {
		t.Fatalf("expected merge to fail")
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == "E042" {
			found = true
			if len(d.Related) < 2 {
				t.Fatalf("expected conflict related spans")
			}
		}
	}
	if !found {
		t.Fatalf("expected E042, got: %s", diags.String())
	}
}
