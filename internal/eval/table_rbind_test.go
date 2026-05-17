package eval

import (
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestRbindTables(t *testing.T) {
	left := CombValue(&Comb{
		Order: []string{"id", "label"},
		Rows: []Row{
			{Values: map[string]Cell{"id": {Value: Int(1)}, "label": {Value: String("a")}}},
			{Values: map[string]Cell{"id": {Value: Int(2)}, "label": {Value: String("b")}}},
		},
	})
	right := CombValue(&Comb{
		Order: []string{"label", "id"},
		Rows: []Row{
			{Values: map[string]Cell{"label": {Value: String("c")}, "id": {Value: Int(3)}}},
		},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("rbind"), posArg(ident("left")), posArg(ident("right"))),
		map[string]Value{"left": left, "right": right},
		diags,
		ExprOptions{},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"id", "label"}) || len(got.C.Rows) != 3 {
		t.Fatalf("unexpected rbind table: %#v", got)
	}
	if !Equal(got.C.Rows[0].Values["id"].Value, Int(1)) ||
		!Equal(got.C.Rows[1].Values["label"].Value, String("b")) ||
		!Equal(got.C.Rows[2].Values["id"].Value, Int(3)) ||
		!Equal(got.C.Rows[2].Values["label"].Value, String("c")) {
		t.Fatalf("unexpected rbind rows: %#v", got.C.Rows)
	}
}

func TestRbindSingleTableClonesValuesAndPreservesMetadata(t *testing.T) {
	origin := spanAt(1900, 1)
	source := CombValue(&Comb{
		Order: []string{"items"},
		Rows: []Row{{Values: map[string]Cell{
			"items": {Value: List([]Value{Int(1)}), Origin: origin, Assigned: true},
		}}},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rbind"), posArg(ident("source"))), map[string]Value{"source": source}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, source) {
		t.Fatalf("unexpected single-table rbind: got=%#v want=%#v", got, source)
	}
	cell := got.C.Rows[0].Values["items"]
	if cell.Origin != origin || !cell.Assigned {
		t.Fatalf("metadata was not preserved: %#v", cell)
	}
	sourceCell := source.C.Rows[0].Values["items"]
	sourceCell.Value.L[0] = Int(99)
	source.C.Rows[0].Values["items"] = sourceCell
	if !Equal(cell.Value, List([]Value{Int(1)})) {
		t.Fatalf("rbind cell value was not deep-cloned: %#v", cell.Value)
	}
}

func TestRbindCallSpreadAndNamedVarargs(t *testing.T) {
	left := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	right := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(2)}}}},
	})
	tables := List([]Value{left, right})
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{name: "spread", expr: callExpr(ident("rbind"), posSpreadArg(ident("tables")))},
		{name: "named varargs", expr: callExpr(ident("rbind"), namedArg("tables", ident("tables")))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, map[string]Value{"tables": tables}, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !IsComb(got) || len(got.C.Rows) != 2 ||
				!Equal(got.C.Rows[0].Values["x"].Value, Int(1)) ||
				!Equal(got.C.Rows[1].Values["x"].Value, Int(2)) {
				t.Fatalf("unexpected rbind result: %#v", got)
			}
		})
	}
}

func TestRbindZeroRowAndZeroColumnTables(t *testing.T) {
	empty := CombValue(&Comb{Order: []string{"a", "b"}, Rows: nil})
	data := CombValue(&Comb{
		Order: []string{"b", "a"},
		Rows: []Row{{Values: map[string]Cell{
			"b": {Value: Int(2)},
			"a": {Value: Int(1)},
		}}},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rbind"), posArg(ident("empty")), posArg(ident("data"))), map[string]Value{"empty": empty, "data": data}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"a", "b"}) || len(got.C.Rows) != 1 {
		t.Fatalf("unexpected zero-row rbind result: %#v", got)
	}
	if !Equal(got.C.Rows[0].Values["a"].Value, Int(1)) || !Equal(got.C.Rows[0].Values["b"].Value, Int(2)) {
		t.Fatalf("unexpected zero-row appended row: %#v", got.C.Rows[0])
	}

	zeroColsA := CombValue(&Comb{Rows: []Row{{Values: map[string]Cell{}}, {Values: map[string]Cell{}}}})
	zeroColsB := CombValue(&Comb{Rows: []Row{{Values: map[string]Cell{}}}})
	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callExpr(ident("rbind"), posArg(ident("a")), posArg(ident("b"))), map[string]Value{"a": zeroColsA, "b": zeroColsB}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected zero-column diagnostics: %s", diags.String())
	}
	if !IsComb(got) || len(got.C.Order) != 0 || len(got.C.Rows) != 3 {
		t.Fatalf("unexpected zero-column rbind result: %#v", got)
	}
	for _, row := range got.C.Rows {
		if len(row.Values) != 0 {
			t.Fatalf("expected empty row values, got %#v", row.Values)
		}
	}
}

func TestRbindDiagnostics(t *testing.T) {
	base := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	other := CombValue(&Comb{
		Order: []string{"y"},
		Rows:  []Row{{Values: map[string]Cell{"y": {Value: Int(2)}}}},
	})
	malformed := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{}}},
	})

	tests := []struct {
		name    string
		expr    ast.Expr
		env     map[string]Value
		message string
	}{
		{
			name:    "no arguments",
			expr:    callExpr(ident("rbind")),
			message: "expects at least 1 positional arguments",
		},
		{
			name:    "non-table argument",
			expr:    callExpr(ident("rbind"), posArg(intExpr(1))),
			message: "argument 1 must be a table value",
		},
		{
			name:    "unknown named argument",
			expr:    callExpr(ident("rbind"), posArg(ident("base")), namedArg("other", ident("base"))),
			env:     map[string]Value{"base": base},
			message: "unknown named argument 'other'",
		},
		{
			name:    "column mismatch",
			expr:    callExpr(ident("rbind"), posArg(ident("base")), posArg(ident("other"))),
			env:     map[string]Value{"base": base, "other": other},
			message: "columns do not match",
		},
		{
			name:    "malformed row",
			expr:    callExpr(ident("rbind"), posArg(ident("malformed"))),
			env:     map[string]Value{"malformed": malformed},
			message: "could not read table column 'x'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, ExprOptions{})
			if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
				t.Fatalf("expected E106 null result, got value=%#v diags=%s", got, diags.String())
			}
			if !strings.Contains(diags.String(), tc.message) {
				t.Fatalf("expected diagnostic containing %q, got: %s", tc.message, diags.String())
			}
		})
	}
}

func TestRbindBuiltinFunctionValue(t *testing.T) {
	fn, ok := BuiltinFunctionValue("rbind")
	if !ok {
		t.Fatalf("missing rbind built-in function value")
	}
	frame := NewRootFrame(nil)
	frame.AssignLocal("rb", fn, diag.Span{})
	left := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	right := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(2)}}}},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rb"), posArg(ident("left")), posArg(ident("right"))), map[string]Value{"left": left, "right": right}, diags, ExprOptions{Frame: frame})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || len(got.C.Rows) != 2 || !Equal(got.C.Rows[1].Values["x"].Value, Int(2)) {
		t.Fatalf("unexpected rbind function-value result: %#v", got)
	}
}
