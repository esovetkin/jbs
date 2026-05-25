package eval

import (
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestUniqueDuplicatedSequences(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "unique list",
			expr: callExpr(ident("unique"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(1), intExpr(3), intExpr(2)))),
			want: List([]Value{Int(1), Int(2), Int(3)}),
		},
		{
			name: "duplicated list",
			expr: callExpr(ident("duplicated"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(1), intExpr(3), intExpr(2)))),
			want: List([]Value{Bool(false), Bool(false), Bool(true), Bool(false), Bool(true)}),
		},
		{
			name: "unique tuple preserves tuple",
			expr: callExpr(ident("unique"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(1)))),
			want: Tuple([]Value{Int(1), Int(2)}),
		},
		{
			name: "duplicated tuple returns list",
			expr: callExpr(ident("duplicated"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(1)))),
			want: List([]Value{Bool(false), Bool(false), Bool(true)}),
		},
		{
			name: "numeric equality",
			expr: callExpr(ident("unique"), posArg(listExpr(intExpr(1), floatExpr(1), intExpr(2)))),
			want: List([]Value{Int(1), Int(2)}),
		},
		{
			name: "nested values",
			expr: callExpr(ident("unique"), posArg(listExpr(
				listExpr(intExpr(1)),
				listExpr(intExpr(1)),
				listExpr(intExpr(2)),
			))),
			want: List([]Value{
				List([]Value{Int(1)}),
				List([]Value{Int(2)}),
			}),
		},
		{
			name: "named values",
			expr: callExpr(ident("unique"), namedArg("values", listExpr(intExpr(1), intExpr(1), intExpr(2)))),
			want: List([]Value{Int(1), Int(2)}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestUniqueMatchesNegatedDuplicatedMaskForSequences(t *testing.T) {
	values := List([]Value{Int(1), Int(2), Int(1), Int(3), Int(2)})
	env := map[string]Value{"x": values}
	diags := &diag.Diagnostics{}
	unique := EvalExprWithOptions(callExpr(ident("unique"), posArg(ident("x"))), env, diags, ExprOptions{})
	filtered := EvalExprWithOptions(indexExprForTest(ident("x"), ast.UnaryExpr{
		Op:   "!",
		Expr: callExpr(ident("duplicated"), posArg(ident("x"))),
	}), env, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(unique, filtered) {
		t.Fatalf("unique(x)=%#v but x[!duplicated(x)]=%#v", unique, filtered)
	}
}

func TestUniqueDuplicatedFunctionValues(t *testing.T) {
	fn, ok := BuiltinFunctionValue("int")
	if !ok {
		t.Fatal("missing int builtin function value")
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("unique"), posArg(listExpr(ident("int"), ident("int")))), nil, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, List([]Value{fn})) {
		t.Fatalf("unexpected function-value unique result: %#v", got)
	}
}

func TestUniqueDuplicatedBuiltinFunctionValues(t *testing.T) {
	for _, name := range []string{"unique", "duplicated"} {
		value, ok := BuiltinFunctionValue(name)
		if !ok || value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != name {
			t.Fatalf("expected builtin function value for %q, got ok=%v value=%#v", name, ok, value)
		}
	}

	frame := NewRootFrame(nil)
	assignBuiltinFunction(t, frame, "uniq", "unique")
	assignBuiltinFunction(t, frame, "dups", "duplicated")
	diags := &diag.Diagnostics{}
	unique := EvalExprWithOptions(callExpr(ident("uniq"), posArg(listExpr(intExpr(1), intExpr(1), intExpr(2)))), nil, diags, ExprOptions{Frame: frame})
	duplicated := EvalExprWithOptions(callExpr(ident("dups"), posArg(listExpr(intExpr(1), intExpr(1), intExpr(2)))), nil, diags, ExprOptions{Frame: frame})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(unique, List([]Value{Int(1), Int(2)})) || !Equal(duplicated, List([]Value{Bool(false), Bool(true), Bool(false)})) {
		t.Fatalf("unexpected first-class results: unique=%#v duplicated=%#v", unique, duplicated)
	}

	mapped := EvalExprWithOptions(callExpr(ident("map"),
		posArg(ident("unique")),
		posArg(listExpr(
			listExpr(intExpr(1), intExpr(1)),
			listExpr(intExpr(2), intExpr(2)),
		)),
	), nil, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics after map: %s", diags.String())
	}
	if !Equal(mapped, List([]Value{List([]Value{Int(1)}), List([]Value{Int(2)})})) {
		t.Fatalf("unexpected map(unique, ...) result: %#v", mapped)
	}
}

func TestUniqueDuplicatedTables(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(2)}, "b": {Value: String("y")}}},
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("x")}}},
			{Values: map[string]Cell{"a": {Value: Int(1)}, "b": {Value: String("z")}}},
		},
	})
	env := map[string]Value{"cases": cases}
	diags := &diag.Diagnostics{}
	unique := EvalExprWithOptions(callExpr(ident("unique"), posArg(ident("cases"))), env, diags, ExprOptions{})
	duplicated := EvalExprWithOptions(callExpr(ident("duplicated"), posArg(ident("cases"))), env, diags, ExprOptions{})
	filtered := EvalExprWithOptions(indexExprForTest(ident("cases"), ast.UnaryExpr{
		Op:   "!",
		Expr: callExpr(ident("duplicated"), posArg(ident("cases"))),
	}), env, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(duplicated, List([]Value{Bool(false), Bool(false), Bool(true), Bool(false)})) {
		t.Fatalf("unexpected table duplicated mask: %#v", duplicated)
	}
	if !Equal(unique, filtered) {
		t.Fatalf("unique(table)=%#v but table[!duplicated(table)]=%#v", unique, filtered)
	}
	if !IsComb(unique) || !slices.Equal(unique.C.Order, []string{"a", "b"}) || len(unique.C.Rows) != 3 {
		t.Fatalf("unexpected unique table: %#v", unique)
	}
	wantA := []Value{Int(1), Int(2), Int(1)}
	wantB := []Value{String("x"), String("y"), String("z")}
	for i := range wantA {
		if !Equal(unique.C.Rows[i].Values["a"].Value, wantA[i]) || !Equal(unique.C.Rows[i].Values["b"].Value, wantB[i]) {
			t.Fatalf("unexpected unique row %d: %#v", i, unique.C.Rows[i])
		}
	}
}

func TestUniqueTablePreservesSchemaAndProjectionIdentity(t *testing.T) {
	empty := CombValue(&Comb{Order: []string{"x", "y"}, Rows: nil})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("unique"), posArg(ident("empty"))), map[string]Value{"empty": empty}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected empty-table diagnostics: %s", diags.String())
	}
	if !IsComb(got) || len(got.C.Rows) != 0 || !slices.Equal(got.C.Order, []string{"x", "y"}) {
		t.Fatalf("unique empty table did not preserve schema: %#v", got)
	}

	z := projectionIdentityGrid(t)
	unique := EvalExprWithOptions(callExpr(ident("unique"), posArg(ident("z"))), map[string]Value{"z": z}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected projection-table diagnostics: %s", diags.String())
	}
	if !IsComb(unique) || len(unique.C.Rows) != 12 {
		t.Fatalf("unique should collapse visible duplicate rows to 12 rows, got %#v", unique)
	}
	assertProjectionValues(t, unique, []string{"x"}, "x", []Value{Int(1), Int(2), Int(3)})
	assertProjectionRowCount(t, unique, []string{"y", "z"}, 4)
}

func TestUniqueDuplicatedDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		env      map[string]Value
		wantText string
	}{
		{
			name:     "unique missing values",
			expr:     callExpr(ident("unique")),
			wantText: "unique() expects arguments",
		},
		{
			name:     "duplicated too many args",
			expr:     callExpr(ident("duplicated"), posArg(listExpr(intExpr(1))), posArg(listExpr(intExpr(2)))),
			wantText: "duplicated() received too many positional arguments",
		},
		{
			name:     "unique unknown named",
			expr:     callExpr(ident("unique"), posArg(listExpr(intExpr(1))), namedArg("value", listExpr(intExpr(1)))),
			wantText: "unknown named argument 'value' for unique()",
		},
		{
			name:     "unique rejects scalar",
			expr:     callExpr(ident("unique"), posArg(intExpr(1))),
			wantText: "unique() expects list/tuple/table as first argument",
		},
		{
			name:     "duplicated rejects dict",
			expr:     callExpr(ident("duplicated"), posArg(ast.DictExpr{})),
			wantText: "duplicated() expects list/tuple/table as first argument",
		},
		{
			name:     "unique rejects malformed table",
			expr:     callExpr(ident("unique"), posArg(ident("bad"))),
			env:      map[string]Value{"bad": CombValue(nil)},
			wantText: "unique() received a malformed table value",
		},
		{
			name: "duplicated rejects malformed row",
			expr: callExpr(ident("duplicated"), posArg(ident("bad"))),
			env: map[string]Value{"bad": CombValue(&Comb{
				Order: []string{"x"},
				Rows:  []Row{{Values: map[string]Cell{}}},
			})},
			wantText: "duplicated() could not read table column 'x'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected E106 containing %q, got: %s", tc.wantText, diags.String())
			}
		})
	}
}
