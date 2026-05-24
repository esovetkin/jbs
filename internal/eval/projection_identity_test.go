package eval

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestCombProjectUsesProjectionIdentityNotValues(t *testing.T) {
	z := projectionIdentityGrid(t)

	assertProjectionValues(t, z, []string{"x"}, "x", []Value{Int(1), Int(2), Int(3), Int(1), Int(2), Int(3)})
	assertProjectionValues(t, z, []string{"y"}, "y", []Value{String("a"), String("a"), String("b"), String("b")})
	assertProjectionValues(t, z, []string{"z"}, "z", []Value{String("x"), String("y")})
	assertProjectionRowCount(t, z, []string{"y", "z"}, 8)

	dup, ok := CombProject(z, []string{"x", "x"})
	if !ok {
		t.Fatalf("duplicate-selector projection failed")
	}
	if !slices.Equal(dup.C.Order, []string{"x"}) || len(dup.C.Rows) != 6 {
		t.Fatalf("unexpected duplicate-selector projection: %#v", dup)
	}
}

func TestSelectUsesProjectionIdentity(t *testing.T) {
	z := projectionIdentityGrid(t)
	diags := &diag.Diagnostics{}
	got := evalSelectCall([]ast.CallArg{
		posArg(ident("z")),
		posArg(ident("x")),
	}, map[string]Value{"z": z}, spanAt(2300, 1), diags, ExprOptions{}, newEvalCtx(nil))
	if diags.HasErrors() {
		t.Fatalf("unexpected select diagnostics: %s", diags.String())
	}
	want, ok := CombProject(z, []string{"x"})
	if !ok {
		t.Fatalf("projection failed")
	}
	if !Equal(got, want) {
		t.Fatalf("select()=%#v want %#v", got, want)
	}
}

func TestProjectionIdentityAtTableCreationBoundaries(t *testing.T) {
	duplicates := tableForProjectionTest(t, "x", []Value{Int(1), Int(1)})
	assertProjectionValues(t, duplicates, []string{"x"}, "x", []Value{Int(1), Int(1)})

	cyclic := tableFromColumnsForProjectionTest(t, []CallValueArg{
		{Name: "x", Value: Tuple([]Value{Int(1)}), Span: spanAt(2301, 1)},
		{Name: "y", Value: Tuple([]Value{String("a"), String("b")}), Span: spanAt(2301, 4)},
	})
	assertProjectionValues(t, cyclic, []string{"x"}, "x", []Value{Int(1)})
	assertProjectionValues(t, cyclic, []string{"y"}, "y", []Value{String("a"), String("b")})

	rows := List([]Value{
		DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)}}),
		DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)}}),
	})
	rowTable := tableFromColumnsForProjectionTest(t, []CallValueArg{{Value: rows, Span: spanAt(2302, 1)}})
	assertProjectionValues(t, rowTable, []string{"x"}, "x", []Value{Int(1), Int(1)})

	parsed := &parsedDelimitedTable{
		Header: []string{"x"},
		Rows:   [][]string{{"1"}, {"1"}},
		Kinds:  []tableColumnKind{tableColumnInt},
	}
	diags := &diag.Diagnostics{}
	csvTable := tableToCombValue(parsed, spanAt(2303, 1), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected CSV diagnostics: %s", diags.String())
	}
	assertProjectionValues(t, csvTable, []string{"x"}, "x", []Value{Int(1), Int(1)})
}

func TestProjectionIdentityThroughTableOperations(t *testing.T) {
	base := tableForProjectionTest(t, "x", []Value{Int(1), Int(1)})
	renamed := renameTableValue(base, DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: String("y")},
	}), spanAt(2304, 1), spanAt(2304, 5), spanAt(2304, 1), &diag.Diagnostics{})
	assertProjectionValues(t, renamed, []string{"y"}, "y", []Value{Int(1), Int(1)})

	diags := &diag.Diagnostics{}
	bound := rbindTables([]CallValueArg{
		{Value: tableForProjectionTest(t, "x", []Value{Int(1)}), Span: spanAt(2305, 1)},
		{Value: tableForProjectionTest(t, "x", []Value{Int(1)}), Span: spanAt(2305, 5)},
	}, spanAt(2305, 1), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected rbind diagnostics: %s", diags.String())
	}
	assertProjectionValues(t, bound, []string{"x"}, "x", []Value{Int(1), Int(1)})

	sampled := sampleTable(tableForProjectionTest(t, "x", []Value{Int(1)}), 3, true, spanAt(2306, 1), &diag.Diagnostics{}, NewRandomStateWithSeed(1))
	assertProjectionValues(t, sampled, []string{"x"}, "x", []Value{Int(1), Int(1), Int(1)})

	head := headTailTable("head", base, 2)
	assertProjectionValues(t, head, []string{"x"}, "x", []Value{Int(1), Int(1)})
	tail := headTailTable("tail", base, 2)
	assertProjectionValues(t, tail, []string{"x"}, "x", []Value{Int(1), Int(1)})

	fnValue, ok := BuiltinFunctionValue("bool")
	if !ok || fnValue.Kind != KindFunction {
		t.Fatalf("expected bool builtin function")
	}
	filtered := evalFilterTable(base, fnValue.Fn, nil, spanAt(2307, 1), spanAt(2307, 1), &diag.Diagnostics{}, ExprOptions{}, newEvalCtx(nil))
	assertProjectionValues(t, filtered, []string{"x"}, "x", []Value{Int(1), Int(1)})
}

func TestRbindPreservesProjectionIdentityPerArgument(t *testing.T) {
	z := projectionIdentityGrid(t)
	z0 := productTablesForProjectionTest(t,
		tableForProjectionTest(t, "x", []Value{Int(7), Int(8), Int(9), Int(7), Int(8), Int(9)}),
		tableForProjectionTest(t, "y", []Value{String("c"), String("c"), String("d"), String("d")}),
		tableForProjectionTest(t, "z", []Value{String("u"), String("v")}),
	)

	assertProjectionRowCount(t, z, []string{"y", "z"}, 8)
	assertProjectionRowCount(t, z0, []string{"y", "z"}, 8)

	diags := &diag.Diagnostics{}
	bound := rbindTables([]CallValueArg{
		{Value: z, Span: spanAt(2320, 1)},
		{Value: z0, Span: spanAt(2320, 5)},
	}, spanAt(2320, 1), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected rbind diagnostics: %s", diags.String())
	}

	assertProjectionRowCount(t, bound, []string{"y", "z"}, 16)
	assertProjectionRowCount(t, bound, []string{"x"}, 12)
}

func TestRbindRepeatedTableArgumentsGetDistinctProjectionNamespaces(t *testing.T) {
	z := projectionIdentityGrid(t)

	diags := &diag.Diagnostics{}
	bound := rbindTables([]CallValueArg{
		{Value: z, Span: spanAt(2321, 1)},
		{Value: z, Span: spanAt(2321, 5)},
	}, spanAt(2321, 1), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected rbind diagnostics: %s", diags.String())
	}

	assertProjectionRowCount(t, bound, []string{"y", "z"}, 16)
	assertProjectionRowCount(t, bound, []string{"z"}, 4)
}

func TestRowsRoundTripClearsProjectionIdentity(t *testing.T) {
	original := projectionIdentityGrid(t)
	assertProjectionValues(t, original, []string{"x"}, "x", []Value{Int(1), Int(2), Int(3), Int(1), Int(2), Int(3)})
	assertProjectionRowCount(t, original, []string{"y", "z"}, 8)

	rows := rowsFromTable(original, spanAt(2308, 1), &diag.Diagnostics{})
	roundTrip := tableFromColumnsForProjectionTest(t, []CallValueArg{{Value: rows, Span: spanAt(2308, 8)}})

	if !Equal(roundTrip, original) {
		t.Fatalf("round-trip changed visible rows: got=%#v want %#v", roundTrip, original)
	}
	assertProjectionRowCount(t, roundTrip, []string{"x"}, 48)
	assertProjectionRowCount(t, roundTrip, []string{"y"}, 48)
	assertProjectionRowCount(t, roundTrip, []string{"y", "z"}, 48)
}

func projectionIdentityGrid(t *testing.T) Value {
	t.Helper()
	x := tableForProjectionTest(t, "x", []Value{Int(1), Int(2), Int(3), Int(1), Int(2), Int(3)})
	y := tableForProjectionTest(t, "y", []Value{String("a"), String("a"), String("b"), String("b")})
	z := tableForProjectionTest(t, "z", []Value{String("x"), String("y")})
	return productTablesForProjectionTest(t, x, y, z)
}

func tableForProjectionTest(t *testing.T, name string, values []Value) Value {
	t.Helper()
	return tableFromColumnsForProjectionTest(t, []CallValueArg{{Name: name, Value: Tuple(values), Span: spanAt(2310, 1)}})
}

func tableFromColumnsForProjectionTest(t *testing.T, args []CallValueArg) Value {
	t.Helper()
	diags := &diag.Diagnostics{}
	got := evalTableValueCall(args, spanAt(2311, 1), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected table diagnostics: %s", diags.String())
	}
	if !IsComb(got) {
		t.Fatalf("expected table value, got %#v", got)
	}
	return got
}

func productTablesForProjectionTest(t *testing.T, tables ...Value) Value {
	t.Helper()
	if len(tables) == 0 {
		t.Fatalf("missing tables")
	}
	order := append([]string(nil), tables[0].C.Order...)
	rows := cloneRows(tables[0].C.Rows)
	diags := &diag.Diagnostics{}
	for _, table := range tables[1:] {
		order = mergeRowOrders(order, table.C.Order)
		rows = productRows(rows, table.C.Rows, ast.CombBinary{Op: "*", OpSpan: spanAt(2312, 1), Span: spanAt(2312, 1)}, diags)
		if diags.HasErrors() {
			t.Fatalf("unexpected product diagnostics: %s", diags.String())
		}
	}
	return tableValueFromOrderedRows(order, rows)
}

func assertProjectionValues(t *testing.T, table Value, cols []string, col string, want []Value) {
	t.Helper()
	projected, ok := CombProject(table, cols)
	if !ok {
		t.Fatalf("projection %v failed", cols)
	}
	if len(projected.C.Rows) != len(want) {
		t.Fatalf("projection %v row count=%d want %d: %#v", cols, len(projected.C.Rows), len(want), projected.C.Rows)
	}
	for i, wantValue := range want {
		got := projected.C.Rows[i].Values[col].Value
		if !Equal(got, wantValue) {
			t.Fatalf("projection %v row %d=%#v want %#v", cols, i, got, wantValue)
		}
	}
}

func assertProjectionRowCount(t *testing.T, table Value, cols []string, want int) {
	t.Helper()
	projected, ok := CombProject(table, cols)
	if !ok {
		t.Fatalf("projection %v failed", cols)
	}
	if len(projected.C.Rows) != want {
		t.Fatalf("projection %v row count=%d want %d", cols, len(projected.C.Rows), want)
	}
}
