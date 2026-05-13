package eval

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestRenameTableColumns(t *testing.T) {
	cases := renameTestTable()
	mapping := renameMapping("x", "id")
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping"))),
		map[string]Value{"cases": cases, "mapping": mapping},
		diags,
		ExprOptions{},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"id", "y"}) {
		t.Fatalf("unexpected renamed table: %#v", got)
	}
	if len(got.C.Rows) != 2 ||
		!Equal(got.C.Rows[0].Values["id"].Value, Int(1)) ||
		!Equal(got.C.Rows[0].Values["y"].Value, String("a")) ||
		!Equal(got.C.Rows[1].Values["id"].Value, Int(2)) ||
		!Equal(got.C.Rows[1].Values["y"].Value, String("b")) {
		t.Fatalf("unexpected renamed rows: %#v", got.C.Rows)
	}
}

func TestRenameMultipleTableColumns(t *testing.T) {
	cases := renameTestTable()
	mapping := renameMapping("x", "id", "y", "label")
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping"))),
		map[string]Value{"cases": cases, "mapping": mapping},
		diags,
		ExprOptions{},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"id", "label"}) {
		t.Fatalf("unexpected renamed table: %#v", got)
	}
	if !Equal(got.C.Rows[1].Values["id"].Value, Int(2)) || !Equal(got.C.Rows[1].Values["label"].Value, String("b")) {
		t.Fatalf("unexpected renamed row values: %#v", got.C.Rows)
	}
}

func TestRenameTableColumnsSwap(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"a", "b"},
		Rows: []Row{{Values: map[string]Cell{
			"a": {Value: Int(1)},
			"b": {Value: Int(2)},
		}}},
	})
	mapping := renameMapping("a", "b", "b", "a")
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping"))),
		map[string]Value{"cases": cases, "mapping": mapping},
		diags,
		ExprOptions{},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"b", "a"}) {
		t.Fatalf("unexpected swap order: %#v", got)
	}
	if !Equal(got.C.Rows[0].Values["b"].Value, Int(1)) || !Equal(got.C.Rows[0].Values["a"].Value, Int(2)) {
		t.Fatalf("unexpected swap values: %#v", got.C.Rows)
	}
}

func TestRenameTableNoopAndEmptyMapping(t *testing.T) {
	cases := renameTestTable()
	tests := []struct {
		name    string
		mapping Value
	}{
		{name: "empty mapping", mapping: DictValue(nil)},
		{name: "noop mapping", mapping: renameMapping("x", "x")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(
				callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping"))),
				map[string]Value{"cases": cases, "mapping": tc.mapping},
				diags,
				ExprOptions{},
			)

			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !IsComb(got) || !slices.Equal(got.C.Order, []string{"x", "y"}) || len(got.C.Rows) != 2 {
				t.Fatalf("unexpected unchanged table: %#v", got)
			}
		})
	}
}

func TestRenameZeroRowTable(t *testing.T) {
	cases := CombValue(&Comb{Order: []string{"x", "y"}, Rows: nil})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping"))),
		map[string]Value{"cases": cases, "mapping": renameMapping("x", "id")},
		diags,
		ExprOptions{},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"id", "y"}) || len(got.C.Rows) != 0 {
		t.Fatalf("unexpected zero-row rename: %#v", got)
	}
}

func TestRenameBuiltinFunctionValue(t *testing.T) {
	renameFn, ok := BuiltinFunctionValue("rename")
	if !ok {
		t.Fatalf("missing rename built-in function value")
	}
	frame := NewRootFrame(nil)
	frame.AssignLocal("r", renameFn, diag.Span{})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("r"), posArg(ident("cases")), posArg(ident("mapping"))),
		map[string]Value{"cases": renameTestTable(), "mapping": renameMapping("x", "id")},
		diags,
		ExprOptions{Frame: frame},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || !slices.Equal(got.C.Order, []string{"id", "y"}) {
		t.Fatalf("unexpected rename function-value result: %#v", got)
	}
}

func TestRenameDoesNotMutateInputAndPreservesCellMetadata(t *testing.T) {
	origin := spanAt(801, 1)
	cases := CombValue(&Comb{
		Order: []string{"x"},
		Rows: []Row{{Values: map[string]Cell{
			"x": {Value: List([]Value{Int(1)}), Origin: origin, Assigned: true},
		}}},
	})
	diags := &diag.Diagnostics{}

	got := EvalExprWithOptions(
		callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping"))),
		map[string]Value{"cases": cases, "mapping": renameMapping("x", "id")},
		diags,
		ExprOptions{},
	)

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !slices.Equal(cases.C.Order, []string{"x"}) {
		t.Fatalf("input table order was mutated: %#v", cases.C.Order)
	}
	if _, ok := cases.C.Rows[0].Values["x"]; !ok {
		t.Fatalf("input row was mutated: %#v", cases.C.Rows[0].Values)
	}
	cell := got.C.Rows[0].Values["id"]
	if cell.Origin != origin || !cell.Assigned {
		t.Fatalf("cell metadata was not preserved: %#v", cell)
	}
	originalCell := cases.C.Rows[0].Values["x"]
	originalCell.Value.L[0] = Int(99)
	cases.C.Rows[0].Values["x"] = originalCell
	if !Equal(cell.Value, List([]Value{Int(1)})) {
		t.Fatalf("renamed cell value was not cloned: %#v", cell.Value)
	}
}

func TestRenameDiagnostics(t *testing.T) {
	base := renameTestTable()
	tests := []struct {
		name     string
		expr     func() ast.Expr
		env      map[string]Value
		wantE106 int
	}{
		{
			name: "wrong arity",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")))
			},
			env:      map[string]Value{"cases": base},
			wantE106: 1,
		},
		{
			name: "named arg",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), namedArg("mapping", ident("mapping")))
			},
			env:      map[string]Value{"cases": base, "mapping": renameMapping("x", "id")},
			wantE106: 1,
		},
		{
			name: "first arg not table",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(intExpr(1)), posArg(ident("mapping")))
			},
			env:      map[string]Value{"mapping": renameMapping("x", "id")},
			wantE106: 1,
		},
		{
			name: "second arg not dict",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(intExpr(1)))
			},
			env:      map[string]Value{"cases": base},
			wantE106: 1,
		},
		{
			name: "non-string old key",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env: map[string]Value{
				"cases":   base,
				"mapping": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyInt, I: 1}, Value: String("id")}}),
			},
			wantE106: 1,
		},
		{
			name: "missing old column",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env:      map[string]Value{"cases": base, "mapping": renameMapping("missing", "id")},
			wantE106: 1,
		},
		{
			name: "non-string new value",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env: map[string]Value{
				"cases":   base,
				"mapping": DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)}}),
			},
			wantE106: 1,
		},
		{
			name: "invalid new name",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env:      map[string]Value{"cases": base, "mapping": renameMapping("x", "1id")},
			wantE106: 1,
		},
		{
			name: "collision with unchanged column",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env:      map[string]Value{"cases": base, "mapping": renameMapping("x", "y")},
			wantE106: 1,
		},
		{
			name: "collision between renamed columns",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env:      map[string]Value{"cases": base, "mapping": renameMapping("x", "z", "y", "z")},
			wantE106: 1,
		},
		{
			name: "malformed table missing cell",
			expr: func() ast.Expr {
				return callExpr(ident("rename"), posArg(ident("cases")), posArg(ident("mapping")))
			},
			env: map[string]Value{
				"cases": CombValue(&Comb{
					Order: []string{"x", "y"},
					Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
				}),
				"mapping": renameMapping("x", "id"),
			},
			wantE106: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr(), tc.env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if count := diagCount(diags, "E106"); count != tc.wantE106 {
				t.Fatalf("expected %d E106 diagnostic(s), got %d: %s", tc.wantE106, count, diags.String())
			}
		})
	}
}

func renameTestTable() Value {
	return CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: String("a")}}},
			{Values: map[string]Cell{"x": {Value: Int(2)}, "y": {Value: String("b")}}},
		},
	})
}

func renameMapping(pairs ...string) Value {
	entries := make([]DictEntry, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		entries = append(entries, DictEntry{
			Key:   DictKey{Kind: DictKeyString, S: pairs[i]},
			Value: String(pairs[i+1]),
		})
	}
	return DictValue(entries)
}
