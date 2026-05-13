package eval

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestRowsFromTableConversion(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: String("a")}}},
			{Values: map[string]Cell{"x": {Value: Int(2)}, "y": {Value: String("b")}}},
		},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rows"), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := List([]Value{
		DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)},
			{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: String("a")},
		}),
		DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(2)},
			{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: String("b")},
		}),
	})
	if !Equal(got, want) {
		t.Fatalf("unexpected rows(table) result: got=%#v want=%#v", got, want)
	}
	if !slices.Equal(got.L[0].D.Order, []DictKey{{Kind: DictKeyString, S: "x"}, {Kind: DictKeyString, S: "y"}}) {
		t.Fatalf("unexpected row dictionary order: %#v", got.L[0].D.Order)
	}
}

func TestRowsFromZeroRowTable(t *testing.T) {
	cases := CombValue(&Comb{Order: []string{"x", "y"}, Rows: nil})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rows"), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 0 {
		t.Fatalf("expected empty list, got %#v", got)
	}
}

func TestRowsFromZeroRowTableCanRoundTripThroughTable(t *testing.T) {
	cases := CombValue(&Comb{Order: []string{"x", "y"}, Rows: nil})
	diags := &diag.Diagnostics{}

	rows := EvalExprWithOptions(callExpr(ident("rows"), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected rows diagnostics: %s", diags.String())
	}
	rows = CloneValue(rows)
	got := EvalExprWithOptions(callExpr(ident("table"), posArg(ident("rows"))), map[string]Value{"rows": rows}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected table diagnostics: %s", diags.String())
	}
	if !Equal(got, cases) {
		t.Fatalf("unexpected zero-row table round-trip: got=%#v want=%#v", got, cases)
	}
}

func TestRowsSchemaIsHiddenFromVisibleListBehavior(t *testing.T) {
	cases := CombValue(&Comb{Order: []string{"x", "y"}, Rows: nil})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rows"), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	plain := List(nil)
	if got.String() != plain.String() {
		t.Fatalf("row schema changed visible string: got=%q want=%q", got.String(), plain.String())
	}
	if !Equal(got, plain) {
		t.Fatalf("row schema changed equality: got=%#v want=%#v", got, plain)
	}
	if StableValueKey(got) != StableValueKey(plain) {
		t.Fatalf("row schema changed stable key: got=%q want=%q", StableValueKey(got), StableValueKey(plain))
	}
}

func TestRowsFromTableClonesNestedValues(t *testing.T) {
	nested := List([]Value{Int(1)})
	cases := CombValue(&Comb{
		Order: []string{"items"},
		Rows:  []Row{{Values: map[string]Cell{"items": {Value: nested}}}},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(callExpr(ident("rows"), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	value := got.L[0].D.Entries[DictKey{Kind: DictKeyString, S: "items"}]
	value.L[0] = Int(99)
	if cases.C.Rows[0].Values["items"].Value.L[0].I != 1 {
		t.Fatalf("rows(table) did not clone nested cell values")
	}
}

func TestRowsDiagnostics(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	tests := []struct {
		name string
		call ast.Expr
	}{
		{
			name: "non-table argument",
			call: callExpr(ident("rows"), posArg(intExpr(1))),
		},
		{
			name: "missing argument",
			call: callExpr(ident("rows")),
		},
		{
			name: "extra argument",
			call: callExpr(ident("rows"), posArg(ident("cases")), posArg(intExpr(1))),
		},
		{
			name: "named argument",
			call: callExpr(ident("rows"), namedArg("table_value", ident("cases"))),
		},
		{
			name: "malformed table",
			call: callExpr(ident("rows"), posArg(ident("cases"))),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			env := map[string]Value{}
			if tc.name == "malformed table" || tc.name == "named argument" || tc.name == "extra argument" {
				env["cases"] = cases
			}
			got := EvalExprWithOptions(tc.call, env, diags, ExprOptions{})
			if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
				t.Fatalf("expected E106 diagnostic, got value=%#v diags=%s", got, diags.String())
			}
		})
	}
}

func TestBuiltinCallNamesIncludesRows(t *testing.T) {
	if !slices.Contains(BuiltinCallNames(), "rows") {
		t.Fatalf("BuiltinCallNames missing rows: %#v", BuiltinCallNames())
	}
	if !IsBuiltinCallName("rows") {
		t.Fatalf("expected rows to be a builtin call name")
	}
}
