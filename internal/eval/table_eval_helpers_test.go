package eval

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestTableFromDictValueDirectHelper(t *testing.T) {
	span := spanAt(1300, 1)

	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got, ok := TableFromDictValue(DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: List([]Value{Int(1), Int(2)})},
			{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: String("a")},
		}), span, diags)
		if !ok || diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: ok=%v diags=%s", ok, diags.String())
		}
		if !IsComb(got) || !slices.Equal(got.C.Order, []string{"x", "y"}) || len(got.C.Rows) != 2 {
			t.Fatalf("unexpected table: %#v", got)
		}
		if !Equal(got.C.Rows[1].Values["x"].Value, Int(2)) || !Equal(got.C.Rows[1].Values["y"].Value, String("a")) {
			t.Fatalf("unexpected broadcast row: %#v", got.C.Rows[1])
		}
	})

	t.Run("non dictionary", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got, ok := TableFromDictValue(Int(1), span, diags)
		if ok || got.Kind != KindNull || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected table(dict) diagnostic, got ok=%v value=%#v diags=%s", ok, got, diags.String())
		}
	})

	t.Run("invalid dictionary key", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got, ok := TableFromDictValue(DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "bad.name"}, Value: Int(1)},
		}), span, diags)
		if ok || got.Kind != KindNull || diagCount(diags, "E106") == 0 {
			t.Fatalf("expected invalid-key diagnostic, got ok=%v value=%#v diags=%s", ok, got, diags.String())
		}
	})
}

func TestEvalProductCallSuccessCases(t *testing.T) {
	span := spanAt(1301, 1)
	left := CombValue(&Comb{
		Order: []string{"y", "x"},
		Rows: []Row{
			{Values: map[string]Cell{"y": {Value: Int(1)}, "x": {Value: String("a")}}},
			{Values: map[string]Cell{"y": {Value: Int(2)}, "x": {Value: String("b")}}},
		},
	})
	right := CombValue(&Comb{
		Order: []string{"z"},
		Rows: []Row{
			{Values: map[string]Cell{"z": {Value: Bool(false)}}},
			{Values: map[string]Cell{"z": {Value: Bool(true)}}},
		},
	})

	t.Run("single table returns cloned table", func(t *testing.T) {
		env := map[string]Value{"left": left}
		diags := &diag.Diagnostics{}
		got := evalProductCall([]ast.CallArg{posArg(ident("left"))}, env, span, diags, ExprOptions{}, newEvalCtx(NewRootFrame(env)))
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if !Equal(got, left) {
			t.Fatalf("single product should preserve table value: got=%#v want=%#v", got, left)
		}
		left.C.Rows[0].Values["x"] = Cell{Value: String("mutated")}
		if got.C.Rows[0].Values["x"].Value.S != "a" {
			t.Fatalf("single product result did not clone rows: %#v", got.C.Rows[0])
		}
		left.C.Rows[0].Values["x"] = Cell{Value: String("a")}
	})

	t.Run("multiple tables preserve operand order", func(t *testing.T) {
		env := map[string]Value{"left": left, "right": right}
		diags := &diag.Diagnostics{}
		got := evalProductCall(
			[]ast.CallArg{posArg(ident("left")), posArg(ident("right"))},
			env,
			span,
			diags,
			ExprOptions{},
			newEvalCtx(NewRootFrame(env)),
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if !IsComb(got) || !slices.Equal(got.C.Order, []string{"y", "x", "z"}) || len(got.C.Rows) != 4 {
			t.Fatalf("unexpected product table: %#v", got)
		}
		first := got.C.Rows[0].Values
		if !Equal(first["y"].Value, Int(1)) || !Equal(first["x"].Value, String("a")) || !Equal(first["z"].Value, Bool(false)) {
			t.Fatalf("unexpected first product row: %#v", first)
		}
	})

	t.Run("empty table keeps combined schema and no rows", func(t *testing.T) {
		empty := CombValue(&Comb{Order: []string{"empty"}, Rows: nil})
		env := map[string]Value{"empty": empty, "right": right}
		diags := &diag.Diagnostics{}
		got := evalProductCall(
			[]ast.CallArg{posArg(ident("empty")), posArg(ident("right"))},
			env,
			span,
			diags,
			ExprOptions{},
			newEvalCtx(NewRootFrame(env)),
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if !IsComb(got) || !slices.Equal(got.C.Order, []string{"empty", "z"}) || len(got.C.Rows) != 0 {
			t.Fatalf("unexpected empty product: %#v", got)
		}
	})

	t.Run("row merge diagnostics abort product", func(t *testing.T) {
		malformedLeft := CombValue(&Comb{
			Order: []string{"a"},
			Rows: []Row{{Values: map[string]Cell{
				"a":      {Value: Int(1)},
				"hidden": {Value: Int(1), Origin: span},
			}}},
		})
		malformedRight := CombValue(&Comb{
			Order: []string{"b"},
			Rows: []Row{{Values: map[string]Cell{
				"b":      {Value: Int(2)},
				"hidden": {Value: Int(2), Origin: span},
			}}},
		})
		env := map[string]Value{"left": malformedLeft, "right": malformedRight}
		diags := &diag.Diagnostics{}
		got := evalProductCall(
			[]ast.CallArg{posArg(ident("left")), posArg(ident("right"))},
			env,
			span,
			diags,
			ExprOptions{},
			newEvalCtx(NewRootFrame(env)),
		)
		if got.Kind != KindNull || diagCount(diags, "E042") == 0 {
			t.Fatalf("expected row-conflict diagnostic, got value=%#v diags=%s", got, diags.String())
		}
	})
}

func TestEvalProductCallPropagatesArgumentErrors(t *testing.T) {
	span := spanAt(1304, 1)
	diags := &diag.Diagnostics{}
	got := evalProductCall(nil, nil, span, diags, ExprOptions{}, newEvalCtx(NewRootFrame(nil)))
	if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
		t.Fatalf("expected product argument diagnostic, got value=%#v diags=%s", got, diags.String())
	}
}

func TestCloneTableValueRejectsNonTable(t *testing.T) {
	if got := cloneTableValue(Int(1)); got.Kind != KindNull {
		t.Fatalf("expected null for non-table clone, got %#v", got)
	}
}

func TestEvalSelectCallSuccessfulSelectors(t *testing.T) {
	span := spanAt(1302, 1)
	table := CombValue(&Comb{
		Order: []string{"x", "y"},
		Rows: []Row{
			{Values: map[string]Cell{"x": {Value: Int(1)}, "y": {Value: String("a")}}},
			{Values: map[string]Cell{"x": {Value: Int(2)}, "y": {Value: String("b")}}},
		},
	})

	t.Run("simple selector", func(t *testing.T) {
		env := map[string]Value{"cases": table}
		diags := &diag.Diagnostics{}
		got := evalSelectCall(
			[]ast.CallArg{posArg(ident("cases")), posArg(ident("y"))},
			env,
			span,
			diags,
			ExprOptions{},
			newEvalCtx(NewRootFrame(env)),
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if !IsComb(got) || !slices.Equal(got.C.Order, []string{"y"}) || len(got.C.Rows) != 2 {
			t.Fatalf("unexpected select result: %#v", got)
		}
		if !Equal(got.C.Rows[1].Values["y"].Value, String("b")) {
			t.Fatalf("unexpected selected row: %#v", got.C.Rows[1])
		}
	})

	t.Run("qualified table binding", func(t *testing.T) {
		env := map[string]Value{"mod.cases": table}
		diags := &diag.Diagnostics{}
		got := evalSelectCall(
			[]ast.CallArg{
				posArg(ast.QualifiedIdentExpr{Namespace: "mod", Name: "cases", Span: span}),
				posArg(ident("x")),
			},
			env,
			span,
			diags,
			ExprOptions{},
			newEvalCtx(NewRootFrame(env)),
		)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
		if !IsComb(got) || !slices.Equal(got.C.Order, []string{"x"}) || !Equal(got.C.Rows[0].Values["x"].Value, Int(1)) {
			t.Fatalf("unexpected qualified select result: %#v", got)
		}
	})
}

func TestEvalSelectCallValidationBranches(t *testing.T) {
	span := spanAt(1305, 1)
	table := CombValue(&Comb{Order: []string{"x"}, Rows: []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}}})

	tests := []struct {
		name string
		args []ast.CallArg
		env  map[string]Value
		ctx  *evalCtx
	}{
		{
			name: "too few args",
			args: []ast.CallArg{posArg(ident("cases"))},
			env:  map[string]Value{"cases": table},
			ctx:  newEvalCtx(NewRootFrame(map[string]Value{"cases": table})),
		},
		{
			name: "named arg",
			args: []ast.CallArg{posArg(ident("cases")), namedArg("x", ident("x"))},
			env:  map[string]Value{"cases": table},
			ctx:  newEvalCtx(NewRootFrame(map[string]Value{"cases": table})),
		},
		{
			name: "first arg not table",
			args: []ast.CallArg{posArg(ident("x")), posArg(ident("x"))},
			env:  map[string]Value{"x": Int(1)},
			ctx:  newEvalCtx(NewRootFrame(map[string]Value{"x": Int(1)})),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalSelectCall(tc.args, tc.env, span, diags, ExprOptions{}, tc.ctx)
			if got.Kind != KindNull || diagCount(diags, "E106") == 0 {
				t.Fatalf("expected select diagnostic, got value=%#v diags=%s", got, diags.String())
			}
		})
	}

	t.Run("recursion limit aborts table evaluation", func(t *testing.T) {
		env := map[string]Value{"cases": table}
		ctx := newEvalCtx(NewRootFrame(env))
		ctx.markRecursionLimitHit()
		diags := &diag.Diagnostics{}
		got := evalSelectCall([]ast.CallArg{posArg(ident("cases")), posArg(ident("x"))}, env, span, diags, ExprOptions{}, ctx)
		if got.Kind != KindNull {
			t.Fatalf("expected null after recursion-limit abort, got %#v", got)
		}
	})
}

func TestEvalPositionalTableArgsValidationAndSuccess(t *testing.T) {
	span := spanAt(1303, 1)
	tableA := CombValue(&Comb{Order: []string{"a"}, Rows: []Row{{Values: map[string]Cell{"a": {Value: Int(1)}}}}})
	tableB := CombValue(&Comb{Order: []string{"b"}, Rows: []Row{{Values: map[string]Cell{"b": {Value: Int(2)}}}}})

	tests := []struct {
		name     string
		args     []ast.CallArg
		env      map[string]Value
		ctx      *evalCtx
		wantOK   bool
		wantLen  int
		wantCode string
	}{
		{
			name:     "zero args",
			args:     nil,
			env:      nil,
			ctx:      newEvalCtx(NewRootFrame(nil)),
			wantOK:   false,
			wantCode: "E106",
		},
		{
			name:     "named arg",
			args:     []ast.CallArg{namedArg("table", ident("a"))},
			env:      map[string]Value{"a": tableA},
			ctx:      newEvalCtx(NewRootFrame(map[string]Value{"a": tableA})),
			wantOK:   false,
			wantCode: "E106",
		},
		{
			name:     "non table arg",
			args:     []ast.CallArg{posArg(ident("x"))},
			env:      map[string]Value{"x": Int(1)},
			ctx:      newEvalCtx(NewRootFrame(map[string]Value{"x": Int(1)})),
			wantOK:   false,
			wantCode: "E106",
		},
		{
			name:    "multi table success",
			args:    []ast.CallArg{posArg(ident("a")), posArg(ident("b"))},
			env:     map[string]Value{"a": tableA, "b": tableB},
			ctx:     newEvalCtx(NewRootFrame(map[string]Value{"a": tableA, "b": tableB})),
			wantOK:  true,
			wantLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got, ok := evalPositionalTableArgs("product", tc.args, tc.env, span, diags, ExprOptions{}, tc.ctx)
			if ok != tc.wantOK || len(got) != tc.wantLen {
				t.Fatalf("unexpected result: ok=%v len=%d tables=%#v", ok, len(got), got)
			}
			if tc.wantCode != "" && diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got %s", tc.wantCode, diags.String())
			}
			if tc.wantCode == "" && diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}

	t.Run("recursion limit aborts after expression evaluation", func(t *testing.T) {
		env := map[string]Value{"a": tableA}
		ctx := newEvalCtx(NewRootFrame(env))
		ctx.markRecursionLimitHit()
		diags := &diag.Diagnostics{}
		got, ok := evalPositionalTableArgs("product", []ast.CallArg{posArg(ident("a"))}, env, span, diags, ExprOptions{}, ctx)
		if ok || got != nil {
			t.Fatalf("expected recursion-limit abort, got ok=%v tables=%#v", ok, got)
		}
	})
}

func TestSelectorNameDirectCases(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
		ok   bool
	}{
		{name: "identifier", expr: ident("column_1"), want: "column_1", ok: true},
		{name: "qualified rejected", expr: ast.QualifiedIdentExpr{Namespace: "m", Name: "x"}, ok: false},
		{name: "invalid identifier", expr: ident("bad.name"), ok: false},
		{name: "non identifier", expr: intExpr(1), ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := selectorName(tc.expr)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("selectorName() = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
