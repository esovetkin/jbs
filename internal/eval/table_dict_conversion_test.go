package eval

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestDictFromTableConversion(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: String("a")}}},
			{Values: map[string]Cell{"x": {Value: Int(2)}, "y": {Value: String("b")}}},
		},
	})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("dict"), posArg(ident("cases"))), map[string]Value{"cases": cases}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: List([]Value{Int(1), Int(2)})},
		{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: List([]Value{String("a"), String("b")})},
	})
	if !Equal(got, want) {
		t.Fatalf("unexpected dict(table) result: got=%#v want=%#v", got, want)
	}
	if !slices.Equal(got.D.Order, []DictKey{{Kind: DictKeyString, S: "x"}, {Kind: DictKeyString, S: "y"}}) {
		t.Fatalf("unexpected dictionary order: %#v", got.D.Order)
	}

	zeroRows := CombValue(&Comb{Order: []string{"x"}, Rows: nil})
	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callExpr(ident("dict"), posArg(ident("empty"))), map[string]Value{"empty": zeroRows}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected zero-row diagnostics: %s", diags.String())
	}
	want = DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: List(nil)}})
	if !Equal(got, want) {
		t.Fatalf("unexpected zero-row dict(table) result: got=%#v want=%#v", got, want)
	}

	empty := CombValue(&Comb{})
	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callExpr(ident("dict"), posArg(ident("empty"))), map[string]Value{"empty": empty}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected empty-table diagnostics: %s", diags.String())
	}
	if got.Kind != KindDict || dictLen(got.D) != 0 {
		t.Fatalf("expected empty dictionary from empty table, got %#v", got)
	}

	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callExpr(ident("dict"), posArg(intExpr(1))), nil, diags, ExprOptions{})
	if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
		t.Fatalf("expected dict(non-table) diagnostic, got value=%#v diags=%s", got, diags.String())
	}

	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callExpr(ident("dict"), posArg(ident("cases")), namedArg("extra", intExpr(1))), map[string]Value{"cases": cases}, diags, ExprOptions{})
	if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
		t.Fatalf("expected mixed dict(table, name=value) diagnostic, got value=%#v diags=%s", got, diags.String())
	}
}

func TestTableFromDictConversion(t *testing.T) {
	dict := DictValue([]DictEntry{
		{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: List([]Value{Int(1), Int(2)})},
		{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: Tuple([]Value{String("a"), String("b")})},
	})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("table"), posArg(ident("d"))), map[string]Value{"d": dict}, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected table(dict) diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"x", "y"}) {
		t.Fatalf("unexpected table(dict) columns: %#v", got)
	}
	if len(got.C.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %#v", got.C.Rows)
	}
	if !Equal(got.C.Rows[0].Values["x"].Value, Int(1)) || !Equal(got.C.Rows[1].Values["y"].Value, String("b")) {
		t.Fatalf("unexpected table(dict) rows: %#v", got.C.Rows)
	}

	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(callExpr(ident("table"), posArg(callExpr(ident("dict")))), nil, diags, ExprOptions{})
	if diags.HasErrors() {
		t.Fatalf("unexpected table(dict()) diagnostics: %s", diags.String())
	}
	if !IsComb(got) || len(got.C.Order) != 0 || len(got.C.Rows) != 0 {
		t.Fatalf("expected empty table from dict(), got %#v", got)
	}
}

func TestTableFromDictDiagnostics(t *testing.T) {
	tableValue := CombValue(&Comb{Order: []string{"y"}, Rows: []Row{{Values: map[string]Cell{"y": {Value: Int(1)}}}}})
	tests := []struct {
		name string
		dict Value
	}{
		{
			name: "non-string key",
			dict: DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyInt, I: 1}, Value: List([]Value{Int(1)})}}),
		},
		{
			name: "invalid column key",
			dict: DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "1x"}, Value: List([]Value{Int(1)})}}),
		},
		{
			name: "table-valued column",
			dict: DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: tableValue}}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(ident("table"), posArg(ident("d"))), map[string]Value{"d": tc.dict}, diags, ExprOptions{})
			if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
				t.Fatalf("expected table(dict) diagnostic, got value=%#v diags=%s", got, diags.String())
			}
		})
	}

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(
		callExpr(ident("table"), posArg(ident("d")), namedArg("y", intExpr(1))),
		map[string]Value{"d": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: List([]Value{Int(1)})}})},
		diags,
		ExprOptions{},
	)
	if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
		t.Fatalf("expected mixed table(dict, y=1) diagnostic, got value=%#v diags=%s", got, diags.String())
	}
}

func TestTableBroadcastsColumns(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		env      map[string]Value
		wantRows int
		wantWarn int
		wantErr  bool
	}{
		{
			name: "named clean divisible broadcast",
			expr: callExpr(ident("table"), namedArg("x", ident("xs")), namedArg("y", ident("ys"))),
			env: map[string]Value{
				"xs": intList(5),
				"ys": intList(10),
			},
			wantRows: 10,
		},
		{
			name: "named non-divisible broadcast warning",
			expr: callExpr(ident("table"), namedArg("x", ident("xs")), namedArg("y", ident("ys"))),
			env: map[string]Value{
				"xs": intList(3),
				"ys": intList(10),
			},
			wantRows: 10,
			wantWarn: 1,
		},
		{
			name: "dictionary non-divisible broadcast warning",
			expr: callExpr(ident("table"), posArg(ident("d"))),
			env: map[string]Value{
				"d": DictValue([]DictEntry{
					{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: intList(3)},
					{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: intList(10)},
				}),
			},
			wantRows: 10,
			wantWarn: 1,
		},
		{
			name: "empty column cannot broadcast",
			expr: callExpr(ident("table"), namedArg("x", ident("xs")), namedArg("y", ident("ys"))),
			env: map[string]Value{
				"xs": List(nil),
				"ys": intList(10),
			},
			wantErr: true,
		},
		{
			name: "all empty columns preserve order",
			expr: callExpr(ident("table"), namedArg("x", ident("xs")), namedArg("y", ident("ys"))),
			env: map[string]Value{
				"xs": List(nil),
				"ys": Tuple(nil),
			},
			wantRows: 0,
		},
		{
			name: "scalar broadcasts cleanly",
			expr: callExpr(ident("table"), namedArg("x", intExpr(1)), namedArg("y", ident("ys"))),
			env: map[string]Value{
				"ys": intList(3),
			},
			wantRows: 3,
		},
		{
			name: "t alias broadcasts through table path",
			expr: callExpr(ident("t"), namedArg("x", ident("xs")), namedArg("y", ident("ys"))),
			env: map[string]Value{
				"xs": intList(2),
				"ys": intList(4),
			},
			wantRows: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, ExprOptions{})
			if tc.wantErr {
				if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
					t.Fatalf("expected E106, got value=%#v diags=%s", got, diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if warns := diagCount(diags, "W101"); warns != tc.wantWarn {
				t.Fatalf("expected %d W101 diagnostics, got %d: %s", tc.wantWarn, warns, diags.String())
			}
			if !IsComb(got) || len(got.C.Rows) != tc.wantRows {
				t.Fatalf("expected %d table rows, got %#v", tc.wantRows, got)
			}
			if len(got.C.Order) == 2 && !slices.Equal(got.C.Order, []string{"x", "y"}) {
				t.Fatalf("unexpected column order: %#v", got.C.Order)
			}
		})
	}
}

func intList(n int) Value {
	values := make([]Value, 0, n)
	for i := 0; i < n; i++ {
		values = append(values, Int(int64(i)))
	}
	return List(values)
}
