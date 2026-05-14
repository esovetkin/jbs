package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestHeadTailSequences(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "head list default",
			expr: callExpr(ident("head"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4), intExpr(5), intExpr(6)))),
			want: List([]Value{Int(1), Int(2), Int(3), Int(4), Int(5)}),
		},
		{
			name: "tail list default",
			expr: callExpr(ident("tail"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4), intExpr(5), intExpr(6)))),
			want: List([]Value{Int(2), Int(3), Int(4), Int(5), Int(6)}),
		},
		{
			name: "head tuple default",
			expr: callExpr(ident("head"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4), intExpr(5), intExpr(6)))),
			want: Tuple([]Value{Int(1), Int(2), Int(3), Int(4), Int(5)}),
		},
		{
			name: "tail tuple default",
			expr: callExpr(ident("tail"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4), intExpr(5), intExpr(6)))),
			want: Tuple([]Value{Int(2), Int(3), Int(4), Int(5), Int(6)}),
		},
		{
			name: "head tuple named n",
			expr: callExpr(ident("head"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3))), namedArg("n", intExpr(2))),
			want: Tuple([]Value{Int(1), Int(2)}),
		},
		{
			name: "tail tuple positional n",
			expr: callExpr(ident("tail"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3))), posArg(intExpr(2))),
			want: Tuple([]Value{Int(2), Int(3)}),
		},
		{
			name: "head named values",
			expr: callExpr(ident("head"), namedArg("values", listExpr(intExpr(1), intExpr(2), intExpr(3))), namedArg("n", intExpr(2))),
			want: List([]Value{Int(1), Int(2)}),
		},
		{
			name: "tail named values",
			expr: callExpr(ident("tail"), namedArg("values", tupleExpr(intExpr(1), intExpr(2), intExpr(3))), namedArg("n", intExpr(2))),
			want: Tuple([]Value{Int(2), Int(3)}),
		},
		{
			name: "head larger than length",
			expr: callExpr(ident("head"), posArg(listExpr(intExpr(1), intExpr(2))), namedArg("n", intExpr(99))),
			want: List([]Value{Int(1), Int(2)}),
		},
		{
			name: "tail zero",
			expr: callExpr(ident("tail"), posArg(tupleExpr(intExpr(1), intExpr(2))), namedArg("n", intExpr(0))),
			want: Tuple(nil),
		},
		{
			name: "head empty list",
			expr: callExpr(ident("head"), posArg(listExpr())),
			want: List(nil),
		},
		{
			name: "tail empty tuple",
			expr: callExpr(ident("tail"), posArg(tupleExpr())),
			want: Tuple(nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestHeadTailTables(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"id", "label"},
		Rows: []Row{
			{Values: map[string]Cell{"id": {Value: Int(1)}, "label": {Value: String("a")}}},
			{Values: map[string]Cell{"id": {Value: Int(2)}, "label": {Value: String("b")}}},
			{Values: map[string]Cell{"id": {Value: Int(3)}, "label": {Value: String("c")}}},
		},
	})
	env := map[string]Value{"cases": cases}
	tests := []struct {
		name    string
		expr    ast.Expr
		wantIDs []int64
	}{
		{
			name:    "head rows",
			expr:    callExpr(ident("head"), posArg(ident("cases")), posArg(intExpr(2))),
			wantIDs: []int64{1, 2},
		},
		{
			name:    "tail rows",
			expr:    callExpr(ident("tail"), posArg(ident("cases")), posArg(intExpr(2))),
			wantIDs: []int64{2, 3},
		},
		{
			name:    "head all rows",
			expr:    callExpr(ident("head"), posArg(ident("cases")), posArg(intExpr(99))),
			wantIDs: []int64{1, 2, 3},
		},
		{
			name:    "head zero rows",
			expr:    callExpr(ident("head"), posArg(ident("cases")), posArg(intExpr(0))),
			wantIDs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !IsComb(got) {
				t.Fatalf("expected table result, got %#v", got)
			}
			if len(got.C.Order) != 2 || got.C.Order[0] != "id" || got.C.Order[1] != "label" {
				t.Fatalf("unexpected column order: %#v", got.C.Order)
			}
			if len(got.C.Rows) != len(tc.wantIDs) {
				t.Fatalf("unexpected row count: got=%d want=%d", len(got.C.Rows), len(tc.wantIDs))
			}
			for i, wantID := range tc.wantIDs {
				cell, ok := got.C.Rows[i].Values["id"]
				if !ok || cell.Value.Kind != KindInt || cell.Value.I != wantID {
					t.Fatalf("unexpected row %d id cell: %#v", i, cell)
				}
			}
		})
	}
}

func TestHeadTailZeroRowTablePreservesSchema(t *testing.T) {
	cases := CombValue(&Comb{Order: []string{"id", "label"}, Rows: nil})
	for _, name := range []string{"head", "tail"} {
		t.Run(name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(ident(name), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !IsComb(got) || len(got.C.Rows) != 0 || len(got.C.Order) != 2 || got.C.Order[0] != "id" || got.C.Order[1] != "label" {
				t.Fatalf("unexpected zero-row table result: %#v", got)
			}
		})
	}
}

func TestHeadTailClonesResults(t *testing.T) {
	t.Run("sequence values", func(t *testing.T) {
		values := List([]Value{List([]Value{Int(1)})})
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("head"), posArg(ident("values")), posArg(intExpr(1))), map[string]Value{"values": values}, diags, ExprOptions{})
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		got.L[0].L[0] = Int(99)
		if values.L[0].L[0].I != 1 {
			t.Fatalf("head() did not clone nested sequence values")
		}
	})

	t.Run("table rows", func(t *testing.T) {
		cases := CombValue(&Comb{
			Order: []string{"id"},
			Rows:  []Row{{Values: map[string]Cell{"id": {Value: Int(1)}}}},
		})
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("head"), posArg(ident("cases")), posArg(intExpr(1))), map[string]Value{"cases": cases}, diags, ExprOptions{})
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		got.C.Rows[0].Values["id"] = Cell{Value: Int(99)}
		if cases.C.Rows[0].Values["id"].Value.I != 1 {
			t.Fatalf("head() did not clone table rows")
		}
	})
}

func TestHeadTailDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		wantText string
	}{
		{
			name:     "head missing values",
			expr:     callExpr(ident("head")),
			wantText: "head() expects arguments",
		},
		{
			name:     "tail too many args",
			expr:     callExpr(ident("tail"), posArg(listExpr(intExpr(1))), posArg(intExpr(1)), posArg(intExpr(2))),
			wantText: "tail() received too many positional arguments",
		},
		{
			name:     "head unknown named",
			expr:     callExpr(ident("head"), posArg(listExpr(intExpr(1))), namedArg("count", intExpr(1))),
			wantText: "unknown named argument 'count' for head()",
		},
		{
			name:     "tail non-int n",
			expr:     callExpr(ident("tail"), posArg(listExpr(intExpr(1))), namedArg("n", ast.StringExpr{Value: "1"})),
			wantText: "tail() n argument must be an integer",
		},
		{
			name:     "head negative n",
			expr:     callExpr(ident("head"), posArg(listExpr(intExpr(1))), namedArg("n", intExpr(-1))),
			wantText: "head() n argument must be non-negative",
		},
		{
			name:     "tail rejects scalar",
			expr:     callExpr(ident("tail"), posArg(intExpr(1))),
			wantText: "tail() expects list/tuple/table as first argument",
		},
		{
			name:     "head rejects malformed table",
			expr:     callExpr(ident("head"), posArg(ident("bad"))),
			wantText: "head() received a malformed table value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			env := map[string]Value{"bad": CombValue(nil)}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected E106 containing %q, got: %s", tc.wantText, diags.String())
			}
		})
	}
}

func TestHeadTailBuiltinFunctionValues(t *testing.T) {
	for _, name := range []string{"head", "tail"} {
		value, ok := BuiltinFunctionValue(name)
		if !ok || value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != name {
			t.Fatalf("expected builtin function value for %q, got ok=%v value=%#v", name, ok, value)
		}
	}

	frame := NewRootFrame(nil)
	assignBuiltinFunction(t, frame, "first", "head")
	assignBuiltinFunction(t, frame, "last", "tail")
	diags := &diag.Diagnostics{}
	first := EvalExprWithOptions(callExpr(ident("first"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))), posArg(intExpr(2))), nil, diags, ExprOptions{Frame: frame})
	last := EvalExprWithOptions(callExpr(ident("last"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))), posArg(intExpr(2))), nil, diags, ExprOptions{Frame: frame})
	if !Equal(first, List([]Value{Int(1), Int(2)})) || !Equal(last, List([]Value{Int(2), Int(3)})) {
		t.Fatalf("unexpected first-class results: first=%#v last=%#v", first, last)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}

	mapped := EvalExprWithOptions(callExpr(ident("map"),
		posArg(ident("head")),
		posArg(listExpr(
			listExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4), intExpr(5), intExpr(6)),
			listExpr(intExpr(7), intExpr(8)),
		)),
	), nil, diags, ExprOptions{})
	if !Equal(mapped, List([]Value{
		List([]Value{Int(1), Int(2), Int(3), Int(4), Int(5)}),
		List([]Value{Int(7), Int(8)}),
	})) {
		t.Fatalf("unexpected map(head, ...) result: %#v", mapped)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}
