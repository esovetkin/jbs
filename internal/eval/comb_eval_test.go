package eval

import (
	"slices"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestEvalCombinationNilExpr(t *testing.T) {
	diags := &diag.Diagnostics{}
	rows := EvalCombination(nil, map[string][]Value{"a": {Int(1)}}, map[string]diag.Span{}, diags)
	if rows != nil {
		t.Fatalf("expected nil rows for nil expression, got %#v", rows)
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == "E113" {
			found = true
			if d.Message != "missing combination expression" {
				t.Fatalf("unexpected E113 message: %q", d.Message)
			}
			if !d.Span.IsZero() {
				t.Fatalf("expected zero span for missing expression, got %v", d.Span)
			}
		}
	}
	if !found {
		t.Fatalf("expected E113 for nil expression, got: %s", diags.String())
	}
}

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

func TestZipBroadcastNoWarningWhenDivisible(t *testing.T) {
	expr := ast.CombBinary{
		Left:   ast.CombIdent{Name: "a"},
		Op:     "+",
		OpSpan: diag.NewSpan("in.jbs", diag.NewPos(10, 1, 10), diag.NewPos(11, 1, 11)),
		Right:  ast.CombIdent{Name: "b"},
	}

	diagsA := &diag.Diagnostics{}
	rowsA := EvalCombination(expr, map[string][]Value{
		"a": {Int(1), Int(2), Int(3), Int(4)},
		"b": {String("x")},
	}, map[string]diag.Span{}, diagsA)
	if len(rowsA) != 4 {
		t.Fatalf("expected 4 rows for 4+1, got %d", len(rowsA))
	}
	for _, d := range diagsA.Items {
		if d.Code == "W101" {
			t.Fatalf("did not expect W101 for 4+1, got: %s", diagsA.String())
		}
	}

	diagsB := &diag.Diagnostics{}
	rowsB := EvalCombination(expr, map[string][]Value{
		"a": {Int(1), Int(2), Int(3), Int(4)},
		"b": {String("x"), String("y")},
	}, map[string]diag.Span{}, diagsB)
	if len(rowsB) != 4 {
		t.Fatalf("expected 4 rows for 4+2, got %d", len(rowsB))
	}
	for _, d := range diagsB.Items {
		if d.Code == "W101" {
			t.Fatalf("did not expect W101 for 4+2, got: %s", diagsB.String())
		}
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

func TestEvalCombProductEmptyInput(t *testing.T) {
	expr := ast.CombBinary{
		Left:  ast.CombIdent{Name: "a"},
		Op:    "*",
		Right: ast.CombIdent{Name: "b"},
	}

	tests := []struct {
		name   string
		series map[string][]Value
	}{
		{
			name: "left empty",
			series: map[string][]Value{
				"a": {},
				"b": {Int(1)},
			},
		},
		{
			name: "right empty",
			series: map[string][]Value{
				"a": {Int(1)},
				"b": {},
			},
		},
	}

	for _, tt := range tests {
		diags := &diag.Diagnostics{}
		rows := evalComb(expr, tt.series, map[string]diag.Span{}, CombEvalOptions{}, diags)
		if rows != nil {
			t.Fatalf("%s: expected nil rows for empty product side, got %#v", tt.name, rows)
		}
		if len(diags.Items) != 0 {
			t.Fatalf("%s: did not expect diagnostics, got %s", tt.name, diags.String())
		}
	}
}

func TestEvalCombProductCartesianProduct(t *testing.T) {
	expr := ast.CombBinary{
		Left:  ast.CombIdent{Name: "a"},
		Op:    "*",
		Right: ast.CombIdent{Name: "b"},
	}
	diags := &diag.Diagnostics{}

	rows := evalComb(expr, map[string][]Value{
		"a": {Int(1), Int(2)},
		"b": {String("x"), String("y")},
	}, map[string]diag.Span{}, CombEvalOptions{}, diags)

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	if len(diags.Items) != 0 {
		t.Fatalf("did not expect diagnostics, got: %s", diags.String())
	}

	got := [][2]string{
		{rows[0].Values["a"].Value.String(), rows[0].Values["b"].Value.String()},
		{rows[1].Values["a"].Value.String(), rows[1].Values["b"].Value.String()},
		{rows[2].Values["a"].Value.String(), rows[2].Values["b"].Value.String()},
		{rows[3].Values["a"].Value.String(), rows[3].Values["b"].Value.String()},
	}
	want := [][2]string{
		{"1", "x"},
		{"1", "y"},
		{"2", "x"},
		{"2", "y"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d: expected %v, got %v", i, want[i], got[i])
		}
	}
}

func TestEvalCombProductMergeConflictsAreSkipped(t *testing.T) {
	expr := ast.CombBinary{
		Left:   ast.CombIdent{Name: "x"},
		Op:     "*",
		OpSpan: diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(1, 2, 2)),
		Right:  ast.CombIdent{Name: "x"},
	}
	diags := &diag.Diagnostics{}

	rows := evalComb(expr, map[string][]Value{
		"x": {Int(1), Int(2)},
	}, map[string]diag.Span{}, CombEvalOptions{}, diags)

	if len(rows) != 2 {
		t.Fatalf("expected 2 merged rows after skipping conflicts, got %d", len(rows))
	}
	if rows[0].Values["x"].Value.I != 1 {
		t.Fatalf("expected first kept row x=1, got %#v", rows[0].Values["x"].Value)
	}
	if rows[1].Values["x"].Value.I != 2 {
		t.Fatalf("expected second kept row x=2, got %#v", rows[1].Values["x"].Value)
	}

	conflicts := 0
	for _, d := range diags.Items {
		if d.Code == "E042" {
			conflicts++
		}
	}
	if conflicts != 2 {
		t.Fatalf("expected exactly 2 E042 conflicts, got %d diagnostics: %s", conflicts, diags.String())
	}
}

func TestEvalCombIdentUsesProvidedOrigin(t *testing.T) {
	exprSpan := diag.NewSpan("expr.jbs", diag.NewPos(10, 2, 5), diag.NewPos(11, 2, 6))
	originSpan := diag.NewSpan("origin.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	expr := ast.CombIdent{Name: "a", Span: exprSpan}
	diags := &diag.Diagnostics{}

	rows := evalComb(expr, map[string][]Value{"a": {Int(1), Int(2)}}, map[string]diag.Span{"a": originSpan}, CombEvalOptions{}, diags)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if len(diags.Items) != 0 {
		t.Fatalf("did not expect diagnostics, got: %s", diags.String())
	}
	for i, row := range rows {
		cell, ok := row.Values["a"]
		if !ok {
			t.Fatalf("row %d missing key a", i)
		}
		if cell.Origin != originSpan {
			t.Fatalf("row %d expected origin %v, got %v", i, originSpan, cell.Origin)
		}
	}
}

func TestEvalCombIdentFallsBackToExpressionSpan(t *testing.T) {
	exprSpan := diag.NewSpan("expr.jbs", diag.NewPos(30, 4, 7), diag.NewPos(31, 4, 8))
	expr := ast.CombIdent{Name: "a", Span: exprSpan}

	tests := []struct {
		name    string
		origins map[string]diag.Span
	}{
		{name: "missing origin", origins: map[string]diag.Span{}},
		{name: "zero origin", origins: map[string]diag.Span{"a": {}}},
	}

	for _, tt := range tests {
		diags := &diag.Diagnostics{}
		rows := evalComb(expr, map[string][]Value{"a": {Int(1)}}, tt.origins, CombEvalOptions{}, diags)
		if len(rows) != 1 {
			t.Fatalf("%s: expected 1 row, got %d", tt.name, len(rows))
		}
		if len(diags.Items) != 0 {
			t.Fatalf("%s: did not expect diagnostics, got: %s", tt.name, diags.String())
		}
		got := rows[0].Values["a"].Origin
		if got != exprSpan {
			t.Fatalf("%s: expected fallback origin %v, got %v", tt.name, exprSpan, got)
		}
	}
}

func TestEvalCombUnknownIdentifier(t *testing.T) {
	exprSpan := diag.NewSpan("in.jbs", diag.NewPos(50, 8, 3), diag.NewPos(51, 8, 4))
	expr := ast.CombIdent{Name: "missing", Span: exprSpan}
	diags := &diag.Diagnostics{}

	rows := evalComb(expr, map[string][]Value{}, map[string]diag.Span{}, CombEvalOptions{}, diags)
	if rows != nil {
		t.Fatalf("expected nil rows for unknown identifier, got %#v", rows)
	}

	found := false
	for _, d := range diags.Items {
		if d.Code == "E111" {
			found = true
			if d.Span != exprSpan {
				t.Fatalf("expected E111 span %v, got %v", exprSpan, d.Span)
			}
		}
	}
	if !found {
		t.Fatalf("expected E111, got: %s", diags.String())
	}
}

func TestEvalCombUnsupportedOperator(t *testing.T) {
	opSpan := diag.NewSpan("in.jbs", diag.NewPos(70, 12, 9), diag.NewPos(71, 12, 10))
	expr := ast.CombBinary{
		Left:   ast.CombIdent{Name: "a"},
		Op:     "/",
		OpSpan: opSpan,
		Right:  ast.CombIdent{Name: "b"},
	}
	diags := &diag.Diagnostics{}

	rows := evalComb(expr, map[string][]Value{
		"a": {Int(1)},
		"b": {Int(2)},
	}, map[string]diag.Span{}, CombEvalOptions{}, diags)
	if rows != nil {
		t.Fatalf("expected nil rows for unsupported operator, got %#v", rows)
	}

	found := false
	for _, d := range diags.Items {
		if d.Code == "E112" {
			found = true
			if d.Span != opSpan {
				t.Fatalf("expected E112 span %v, got %v", opSpan, d.Span)
			}
		}
	}
	if !found {
		t.Fatalf("expected E112, got: %s", diags.String())
	}
}

func TestRowVariableNames(t *testing.T) {
	if got := RowVariableNames(nil); got != nil {
		t.Fatalf("expected nil for empty rows, got %#v", got)
	}

	rows := []Row{
		{Values: map[string]Cell{
			"b": {Value: Int(1)},
			"a": {Value: String("x")},
		}},
		{Values: map[string]Cell{
			"c": {Value: Bool(true)},
			"a": {Value: String("y")},
		}},
		{Values: map[string]Cell{}},
	}

	got := RowVariableNames(rows)
	want := []string{"a", "b", "c"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
