package eval

import (
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func listExpr(items ...ast.Expr) ast.ListExpr {
	return ast.ListExpr{Items: items}
}

func tupleExpr(items ...ast.Expr) ast.TupleExpr {
	return ast.TupleExpr{Items: items}
}

func floatExpr(v float64) ast.NumberExpr {
	return ast.NumberExpr{FloatValue: v}
}

func TestMapCallSupportsListsTuplesDefaultsClosuresAndComposition(t *testing.T) {
	t.Run("list result", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		inc := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "+",
			Right: intExpr(1),
		}))
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(inc),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(2), Int(3), Int(4)})
		if !Equal(got, want) {
			t.Fatalf("unexpected map list result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("tuple result preserves tuple kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		double := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "*",
			Right: intExpr(2),
		}))
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(double),
			posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		want := Tuple([]Value{Int(2), Int(4), Int(6)})
		if !Equal(got, want) || got.Kind != KindTuple {
			t.Fatalf("unexpected map tuple result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("built-in callback converts list values", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(ident("int")),
			posArg(listExpr(ast.StringExpr{Value: "1"}, ast.StringExpr{Value: "2"})),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(1), Int(2)})
		if !Equal(got, want) {
			t.Fatalf("unexpected map(int, ...) result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("built-in callback preserves tuple kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(ident("str")),
			posArg(tupleExpr(intExpr(1), intExpr(2))),
		), nil, diags, ExprOptions{})
		want := Tuple([]Value{String("1"), String("2")})
		if !Equal(got, want) || got.Kind != KindTuple {
			t.Fatalf("unexpected map(str, tuple) result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("shadowed built-in callback uses user function", func(t *testing.T) {
		frame := NewRootFrame(nil)
		defineFunctionInFrame(t, frame, "int", fnExpr(
			[]ast.FuncParam{{Name: "x"}},
			exprStmt(intExpr(42)),
		))
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(ident("int")),
			posArg(listExpr(ast.StringExpr{Value: "1"})),
		), nil, diags, ExprOptions{Frame: frame})
		want := List([]Value{Int(42)})
		if !Equal(got, want) {
			t.Fatalf("unexpected shadowed map result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("callback defaults work", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		addDefault := fnExpr(
			[]ast.FuncParam{{Name: "x"}, {Name: "delta", Default: intExpr(1)}},
			exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: ident("delta")}),
		)
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(addDefault),
			posArg(listExpr(intExpr(1), intExpr(2))),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(2), Int(3)})
		if !Equal(got, want) {
			t.Fatalf("unexpected defaulted map result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("closure callback reads captured value", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		makeAdder := fnExpr(
			[]ast.FuncParam{{Name: "delta"}},
			exprStmt(fnExpr(
				[]ast.FuncParam{{Name: "x"}},
				exprStmt(ast.BinaryExpr{Left: ident("x"), Op: "+", Right: ident("delta")}),
			)),
		)
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(callExpr(makeAdder, posArg(intExpr(10)))),
			posArg(listExpr(intExpr(1), intExpr(2))),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(11), Int(12)})
		if !Equal(got, want) {
			t.Fatalf("unexpected closure map result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("nested map and reduce compose", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		inc := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("x"),
			Op:    "+",
			Right: intExpr(1),
		}))
		sum2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(sum2),
			posArg(callExpr(ident("map"),
				posArg(inc),
				posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
			)),
		), nil, diags, ExprOptions{})
		if !Equal(got, Int(9)) {
			t.Fatalf("unexpected composed reduce(map(...)) result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestFilterFunctionOnListTupleAndBuiltins(t *testing.T) {
	t.Run("list predicate", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		keepLarge := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.CompareExpr{
			Left:  ident("x"),
			Op:    ">",
			Right: intExpr(2),
		}))
		got := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4))),
			posArg(keepLarge),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(3), Int(4)})
		if !Equal(got, want) {
			t.Fatalf("unexpected filter list result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("tuple predicate preserves tuple kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		notTwo := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ast.CompareExpr{
			Left:  ident("x"),
			Op:    "!=",
			Right: intExpr(2),
		}))
		got := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3))),
			posArg(notTwo),
		), nil, diags, ExprOptions{})
		want := Tuple([]Value{Int(1), Int(3)})
		if !Equal(got, want) || got.Kind != KindTuple {
			t.Fatalf("unexpected filter tuple result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("empty inputs do not call predicate", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		failing := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("missing_name")))
		listGot := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(listExpr()),
			posArg(failing),
		), nil, diags, ExprOptions{})
		tupleGot := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(tupleExpr()),
			posArg(failing),
		), nil, diags, ExprOptions{})
		if !Equal(listGot, List(nil)) || !Equal(tupleGot, Tuple(nil)) || tupleGot.Kind != KindTuple {
			t.Fatalf("unexpected empty filter results: list=%#v tuple=%#v", listGot, tupleGot)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("built-in predicate", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(listExpr(intExpr(0), intExpr(1), stringExpr(""), stringExpr("x"))),
			posArg(ident("bool")),
		), nil, diags, ExprOptions{})
		want := List([]Value{Int(1), String("x")})
		if !Equal(got, want) {
			t.Fatalf("unexpected filter(bool, ...) result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("filter as function value", func(t *testing.T) {
		frame := NewRootFrame(nil)
		filterValue, ok := BuiltinFunctionValue("filter")
		if !ok {
			t.Fatalf("missing filter built-in function value")
		}
		frame.AssignLocal("f", filterValue, diag.Span{})

		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("f"),
			posArg(listExpr(intExpr(0), intExpr(1), intExpr(2))),
			posArg(ident("bool")),
		), nil, diags, ExprOptions{Frame: frame})
		want := List([]Value{Int(1), Int(2)})
		if !Equal(got, want) {
			t.Fatalf("unexpected first-class filter result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestFilterFunctionOnTable(t *testing.T) {
	cases := CombValue(&Comb{
		Order: []string{"id", "group"},
		Rows: []Row{
			{Values: map[string]Cell{"id": {Value: Int(1)}, "group": {Value: String("a")}}},
			{Values: map[string]Cell{"id": {Value: Int(2)}, "group": {Value: String("b")}}},
			{Values: map[string]Cell{"id": {Value: Int(3)}, "group": {Value: String("a")}}},
		},
	})
	rowGroup := ast.IndexExpr{Base: ident("row"), Items: []ast.Expr{stringExpr("group")}}
	predicate := fnExpr([]ast.FuncParam{{Name: "row"}},
		exprStmt(callExpr(ident("print"), posArg(ident("row")))),
		exprStmt(ast.CompareExpr{Left: rowGroup, Op: "==", Right: stringExpr("a")}),
	)
	events := make([]PrintEvent, 0)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("filter"),
		posArg(ident("cases")),
		posArg(predicate),
	), map[string]Value{"cases": cases}, diags, ExprOptions{
		Print: func(event PrintEvent) {
			events = append(events, event)
		},
	})

	want := CombValue(&Comb{
		Order: []string{"id", "group"},
		Rows: []Row{
			{Values: map[string]Cell{"id": {Value: Int(1)}, "group": {Value: String("a")}}},
			{Values: map[string]Cell{"id": {Value: Int(3)}, "group": {Value: String("a")}}},
		},
	})
	if !Equal(got, want) {
		t.Fatalf("unexpected filtered table: got=%#v want=%#v", got, want)
	}
	if len(events) != 3 {
		t.Fatalf("expected predicate to see three row dictionaries, got %#v", events)
	}
	for _, event := range events {
		if len(event.Values) != 1 || event.Values[0].Kind != KindDict {
			t.Fatalf("expected printed row dictionary, got %#v", event.Values)
		}
		if !slices.Equal(event.Values[0].D.Order, []DictKey{{Kind: DictKeyString, S: "id"}, {Kind: DictKeyString, S: "group"}}) {
			t.Fatalf("unexpected row dictionary order: %#v", event.Values[0].D.Order)
		}
	}
	got.C.Rows[0].Values["id"] = Cell{Value: Int(99)}
	if cases.C.Rows[0].Values["id"].Value.I != 1 {
		t.Fatalf("expected filtered rows to be cloned")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestFilterFunctionOnZeroRowTable(t *testing.T) {
	cases := CombValue(&Comb{Order: []string{"id", "group"}, Rows: nil})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("filter"),
		posArg(ident("cases")),
		posArg(fnExpr([]ast.FuncParam{{Name: "row"}}, exprStmt(ident("missing_name")))),
	), map[string]Value{"cases": cases}, diags, ExprOptions{})
	if !IsComb(got) || len(got.C.Rows) != 0 || !slices.Equal(got.C.Order, []string{"id", "group"}) {
		t.Fatalf("unexpected zero-row table result: %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestFilterPredicateTruthinessAndFailFast(t *testing.T) {
	t.Run("truthiness warning", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(listExpr(stringExpr(""), stringExpr("x"), stringExpr("y"))),
			posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x")))),
		), nil, diags, ExprOptions{})
		want := List([]Value{String("x"), String("y")})
		if !Equal(got, want) {
			t.Fatalf("unexpected truthy filter result: got=%#v want=%#v", got, want)
		}
		if diagCount(diags, "W101") != 1 || !strings.Contains(diags.String(), "filter() cast non-boolean predicate result via truthiness") {
			t.Fatalf("expected one truthiness warning, got: %s", diags.String())
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
	})

	t.Run("predicate errors stop iteration", func(t *testing.T) {
		events := make([]PrintEvent, 0)
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
			posArg(fnExpr([]ast.FuncParam{{Name: "x"}},
				exprStmt(callExpr(ident("print"), posArg(ident("x")))),
				exprStmt(ident("missing_name")),
			)),
		), nil, diags, ExprOptions{
			Print: func(event PrintEvent) {
				events = append(events, event)
			},
		})
		if got.Kind != KindNull {
			t.Fatalf("expected null on predicate error, got %#v", got)
		}
		if diagCount(diags, "E100") != 1 {
			t.Fatalf("expected one E100, got: %s", diags.String())
		}
		if len(events) != 1 {
			t.Fatalf("expected fail-fast after one predicate call, got events=%#v", events)
		}
	})
}

func TestFilterFunctionDiagnosticsAndShadowing(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		wantText string
	}{
		{
			name:     "wrong arity zero",
			expr:     callExpr(ident("filter")),
			wantText: "filter() expects exactly two arguments",
		},
		{
			name:     "wrong arity one",
			expr:     callExpr(ident("filter"), posArg(listExpr(intExpr(1)))),
			wantText: "filter() expects exactly two arguments",
		},
		{
			name: "named argument",
			expr: callExpr(ident("filter"),
				namedArg("values", listExpr(intExpr(1))),
				posArg(ident("bool")),
			),
			wantText: "filter() does not accept named arguments",
		},
		{
			name: "bad target",
			expr: callExpr(ident("filter"),
				posArg(intExpr(1)),
				posArg(ident("bool")),
			),
			wantText: "filter() expects list/tuple/table as first argument",
		},
		{
			name: "bad predicate",
			expr: callExpr(ident("filter"),
				posArg(listExpr(intExpr(1))),
				posArg(intExpr(1)),
			),
			wantText: "filter() expects function value as second argument",
		},
		{
			name: "old mask form rejected",
			expr: callExpr(ident("filter"),
				posArg(listExpr(intExpr(1), intExpr(2))),
				posArg(listExpr(boolExpr(true), boolExpr(false))),
			),
			wantText: "filter() expects function value as second argument",
		},
		{
			name: "malformed table",
			expr: callExpr(ident("filter"),
				posArg(ident("cases")),
				posArg(fnExpr([]ast.FuncParam{{Name: "row"}}, exprStmt(boolExpr(true)))),
			),
			wantText: "filter() could not read table column 'y'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			env := map[string]Value{}
			if tc.name == "malformed table" {
				env["cases"] = CombValue(&Comb{
					Order: []string{"x", "y"},
					Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
				})
			}
			got := EvalExprWithOptions(tc.expr, env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected E106 containing %q, got: %s", tc.wantText, diags.String())
			}
		})
	}

	t.Run("user function shadows filter builtin", func(t *testing.T) {
		frame := NewRootFrame(nil)
		defineFunctionInFrame(t, frame, "filter", fnExpr(
			[]ast.FuncParam{{Name: "values"}, {Name: "fn"}},
			exprStmt(listExpr(intExpr(42))),
		))
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("filter"),
			posArg(listExpr(intExpr(1), intExpr(2))),
			posArg(ident("bool")),
		), nil, diags, ExprOptions{Frame: frame})
		want := List([]Value{Int(42)})
		if !Equal(got, want) {
			t.Fatalf("expected shadowed filter to win: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestSumProdBuiltins(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		want     Value
		wantKind Kind
	}{
		{
			name: "sum int list",
			expr: callExpr(ident("sum"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3)))),
			want: Int(6),
		},
		{
			name: "sum int tuple",
			expr: callExpr(ident("sum"), posArg(tupleExpr(intExpr(1), intExpr(2), intExpr(3)))),
			want: Int(6),
		},
		{
			name:     "sum mixed numeric list",
			expr:     callExpr(ident("sum"), posArg(listExpr(intExpr(1), floatExpr(2.5)))),
			want:     Float(3.5),
			wantKind: KindFloat,
		},
		{
			name: "sum string tuple",
			expr: callExpr(ident("sum"), posArg(tupleExpr(ast.StringExpr{Value: "a"}, ast.StringExpr{Value: "b"}, ast.StringExpr{Value: "c"}))),
			want: String("abc"),
		},
		{
			name: "sum singleton",
			expr: callExpr(ident("sum"), posArg(listExpr(ast.StringExpr{Value: "x"}))),
			want: String("x"),
		},
		{
			name: "prod int list",
			expr: callExpr(ident("prod"), posArg(listExpr(intExpr(2), intExpr(3), intExpr(4)))),
			want: Int(24),
		},
		{
			name: "prod int tuple",
			expr: callExpr(ident("prod"), posArg(tupleExpr(intExpr(2), intExpr(3), intExpr(4)))),
			want: Int(24),
		},
		{
			name:     "prod mixed numeric list",
			expr:     callExpr(ident("prod"), posArg(listExpr(intExpr(2), floatExpr(1.5)))),
			want:     Float(3.0),
			wantKind: KindFloat,
		},
		{
			name: "prod string repeat",
			expr: callExpr(ident("prod"), posArg(listExpr(ast.StringExpr{Value: "a"}, intExpr(3)))),
			want: String("aaa"),
		},
		{
			name: "prod singleton tuple",
			expr: callExpr(ident("prod"), posArg(tupleExpr(ast.StringExpr{Value: "x"}))),
			want: String("x"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected result: got=%#v want=%#v", got, tc.want)
			}
			if tc.wantKind != "" && got.Kind != tc.wantKind {
				t.Fatalf("unexpected result kind: got=%s want=%s", got.Kind, tc.wantKind)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestSumProdBuiltinFunctionValues(t *testing.T) {
	t.Run("builtin function values exist", func(t *testing.T) {
		for _, name := range []string{"sum", "prod"} {
			value, ok := BuiltinFunctionValue(name)
			if !ok || value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != name {
				t.Fatalf("expected builtin function value for %q, got ok=%v value=%#v", name, ok, value)
			}
		}
	})

	t.Run("assigned function values can be called", func(t *testing.T) {
		frame := NewRootFrame(nil)
		sumValue, _ := BuiltinFunctionValue("sum")
		prodValue, _ := BuiltinFunctionValue("prod")
		frame.AssignLocal("sum_fn", sumValue, diag.Span{})
		frame.AssignLocal("prod_fn", prodValue, diag.Span{})
		diags := &diag.Diagnostics{}
		sumGot := EvalExprWithOptions(callExpr(ident("sum_fn"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3)))), nil, diags, ExprOptions{Frame: frame})
		prodGot := EvalExprWithOptions(callExpr(ident("prod_fn"), posArg(listExpr(intExpr(2), intExpr(3), intExpr(4)))), nil, diags, ExprOptions{Frame: frame})
		if !Equal(sumGot, Int(6)) || !Equal(prodGot, Int(24)) {
			t.Fatalf("unexpected assigned builtin results: sum=%#v prod=%#v", sumGot, prodGot)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("map can use fold builtins", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		sumGot := EvalExprWithOptions(callExpr(ident("map"),
			posArg(ident("sum")),
			posArg(listExpr(
				listExpr(intExpr(1), intExpr(2)),
				listExpr(intExpr(3), intExpr(4)),
			)),
		), nil, diags, ExprOptions{})
		prodGot := EvalExprWithOptions(callExpr(ident("map"),
			posArg(ident("prod")),
			posArg(listExpr(
				tupleExpr(intExpr(2), intExpr(3)),
				tupleExpr(intExpr(4), intExpr(5)),
			)),
		), nil, diags, ExprOptions{})
		if !Equal(sumGot, List([]Value{Int(3), Int(7)})) {
			t.Fatalf("unexpected map(sum, ...) result: %#v", sumGot)
		}
		if !Equal(prodGot, List([]Value{Int(6), Int(20)})) {
			t.Fatalf("unexpected map(prod, ...) result: %#v", prodGot)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("containers and str preserve function values", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		listValue := EvalExprWithOptions(listExpr(ident("sum"), ident("prod")), nil, diags, ExprOptions{})
		if listValue.Kind != KindList || len(listValue.L) != 2 || listValue.L[0].Kind != KindFunction || listValue.L[1].Kind != KindFunction {
			t.Fatalf("expected list of function values, got %#v", listValue)
		}
		strValue := EvalExprWithOptions(callExpr(ident("str"), posArg(ident("sum"))), nil, diags, ExprOptions{})
		if !Equal(strValue, String("<function>")) {
			t.Fatalf("unexpected str(sum) result: %#v", strValue)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestSumProdBuiltinsReportErrors(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		wantCode string
		wantText string
	}{
		{
			name:     "sum wrong arity zero",
			expr:     callExpr(ident("sum")),
			wantCode: "E106",
			wantText: "sum() expects exactly one argument",
		},
		{
			name:     "sum wrong arity two",
			expr:     callExpr(ident("sum"), posArg(listExpr(intExpr(1))), posArg(listExpr(intExpr(2)))),
			wantCode: "E106",
			wantText: "sum() expects exactly one argument",
		},
		{
			name: "sum rejects named argument",
			expr: ast.CallExpr{
				Callee: ident("sum"),
				Args:   []ast.CallArg{namedArg("values", listExpr(intExpr(1), intExpr(2)))},
			},
			wantCode: "E106",
			wantText: "sum() does not accept named arguments",
		},
		{
			name:     "sum rejects scalar",
			expr:     callExpr(ident("sum"), posArg(intExpr(1))),
			wantCode: "E106",
			wantText: "sum() expects list or tuple as first argument",
		},
		{
			name:     "sum rejects empty",
			expr:     callExpr(ident("sum"), posArg(listExpr())),
			wantCode: "E106",
			wantText: "sum() cannot operate on an empty list/tuple",
		},
		{
			name:     "sum impossible operator",
			expr:     callExpr(ident("sum"), posArg(listExpr(fnExpr(nil, exprStmt(intExpr(1))), intExpr(2)))),
			wantCode: "E106",
			wantText: "operator '+' does not accept function values",
		},
		{
			name:     "prod wrong arity zero",
			expr:     callExpr(ident("prod")),
			wantCode: "E106",
			wantText: "prod() expects exactly one argument",
		},
		{
			name:     "prod wrong arity two",
			expr:     callExpr(ident("prod"), posArg(listExpr(intExpr(1))), posArg(listExpr(intExpr(2)))),
			wantCode: "E106",
			wantText: "prod() expects exactly one argument",
		},
		{
			name: "prod rejects named argument",
			expr: ast.CallExpr{
				Callee: ident("prod"),
				Args:   []ast.CallArg{namedArg("values", listExpr(intExpr(1), intExpr(2)))},
			},
			wantCode: "E106",
			wantText: "prod() does not accept named arguments",
		},
		{
			name:     "prod rejects scalar",
			expr:     callExpr(ident("prod"), posArg(intExpr(1))),
			wantCode: "E106",
			wantText: "prod() expects list or tuple as first argument",
		},
		{
			name:     "prod rejects empty",
			expr:     callExpr(ident("prod"), posArg(tupleExpr())),
			wantCode: "E106",
			wantText: "prod() cannot operate on an empty list/tuple",
		},
		{
			name:     "prod impossible operator",
			expr:     callExpr(ident("prod"), posArg(listExpr(ast.StringExpr{Value: "a"}, ast.StringExpr{Value: "b"}))),
			wantCode: "E105",
			wantText: "string '*' requires integer repeat count",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 || !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected %s containing %q, got: %s", tc.wantCode, tc.wantText, diags.String())
			}
		})
	}
}

func TestSumProdBuiltinsFailFastOnOperatorErrors(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("prod"),
		posArg(listExpr(ast.StringExpr{Value: "a"}, ast.StringExpr{Value: "b"}, ast.StringExpr{Value: "c"})),
	), nil, diags, ExprOptions{})
	if got.Kind != KindNull {
		t.Fatalf("expected null result on error, got %#v", got)
	}
	if count := diagCount(diags, "E105"); count != 1 {
		t.Fatalf("expected exactly one E105, got %d: %s", count, diags.String())
	}
}

func TestSumProdBuiltinsCanBeShadowed(t *testing.T) {
	t.Run("user functions shadow builtins", func(t *testing.T) {
		frame := NewRootFrame(nil)
		defineFunctionInFrame(t, frame, "sum", fnExpr(
			[]ast.FuncParam{{Name: "values"}},
			exprStmt(intExpr(42)),
		))
		defineFunctionInFrame(t, frame, "prod", fnExpr(
			[]ast.FuncParam{{Name: "values"}},
			exprStmt(intExpr(99)),
		))

		diags := &diag.Diagnostics{}
		sumGot := EvalExprWithOptions(callExpr(ident("sum"), posArg(listExpr(intExpr(1), intExpr(2), intExpr(3)))), nil, diags, ExprOptions{Frame: frame})
		prodGot := EvalExprWithOptions(callExpr(ident("prod"), posArg(listExpr(intExpr(2), intExpr(3), intExpr(4)))), nil, diags, ExprOptions{Frame: frame})
		if !Equal(sumGot, Int(42)) || !Equal(prodGot, Int(99)) {
			t.Fatalf("expected shadowing functions to win, got sum=%#v prod=%#v", sumGot, prodGot)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("data globals shadow builtin names", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(ident("sum"), map[string]Value{"sum": Int(12)}, diags, ExprOptions{})
		if !Equal(got, Int(12)) {
			t.Fatalf("expected data global named sum to shadow builtin, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestReduceCallSupportsListsTuplesAndSingletons(t *testing.T) {
	t.Run("list reduction", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		sum2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(sum2),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3), intExpr(4))),
		), nil, diags, ExprOptions{})
		if !Equal(got, Int(10)) {
			t.Fatalf("unexpected reduce list result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("tuple reduction", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		cat2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(cat2),
			posArg(tupleExpr(ast.StringExpr{Value: "a"}, ast.StringExpr{Value: "b"}, ast.StringExpr{Value: "c"})),
		), nil, diags, ExprOptions{})
		if !Equal(got, String("abc")) {
			t.Fatalf("unexpected reduce tuple result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("singleton sequence returns item unchanged", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		sum2 := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ast.BinaryExpr{
			Left:  ident("acc"),
			Op:    "+",
			Right: ident("x"),
		}))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(sum2),
			posArg(listExpr(intExpr(7))),
		), nil, diags, ExprOptions{})
		if !Equal(got, Int(7)) {
			t.Fatalf("unexpected singleton reduce result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestHigherOrderBuiltinsReportErrors(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expr
		wantCode string
	}{
		{
			name:     "map wrong arity",
			expr:     callExpr(ident("map"), posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x"))))),
			wantCode: "E106",
		},
		{
			name:     "reduce wrong arity",
			expr:     callExpr(ident("reduce"), posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x"))))),
			wantCode: "E106",
		},
		{
			name: "map rejects named builtin args",
			expr: ast.CallExpr{
				Callee: ident("map"),
				Args: []ast.CallArg{
					namedArg("fn", fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x")))),
					posArg(listExpr(intExpr(1))),
				},
			},
			wantCode: "E106",
		},
		{
			name: "reduce rejects named builtin args",
			expr: ast.CallExpr{
				Callee: ident("reduce"),
				Args: []ast.CallArg{
					posArg(fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("acc")))),
					namedArg("values", listExpr(intExpr(1))),
				},
			},
			wantCode: "E106",
		},
		{
			name:     "map first arg must be function",
			expr:     callExpr(ident("map"), posArg(intExpr(1)), posArg(listExpr(intExpr(1)))),
			wantCode: "E106",
		},
		{
			name:     "reduce first arg must be function",
			expr:     callExpr(ident("reduce"), posArg(intExpr(1)), posArg(listExpr(intExpr(1)))),
			wantCode: "E106",
		},
		{
			name: "map second arg must be list or tuple",
			expr: callExpr(ident("map"),
				posArg(fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("x")))),
				posArg(intExpr(1)),
			),
			wantCode: "E106",
		},
		{
			name: "reduce second arg must be list or tuple",
			expr: callExpr(ident("reduce"),
				posArg(fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("acc")))),
				posArg(intExpr(1)),
			),
			wantCode: "E106",
		},
		{
			name: "reduce empty input rejected",
			expr: callExpr(ident("reduce"),
				posArg(fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("acc")))),
				posArg(listExpr()),
			),
			wantCode: "E106",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
		})
	}
}

func TestHigherOrderBuiltinsFailFastOnCallbackErrors(t *testing.T) {
	t.Run("map aborts on first callback error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		bad := fnExpr([]ast.FuncParam{{Name: "x"}}, exprStmt(ident("missing")))
		got := EvalExprWithOptions(callExpr(ident("map"),
			posArg(bad),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null result, got %#v", got)
		}
		if count := diagCount(diags, "E100"); count != 1 {
			t.Fatalf("expected exactly one E100, got %d: %s", count, diags.String())
		}
	})

	t.Run("reduce aborts on first callback error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		bad := fnExpr([]ast.FuncParam{{Name: "acc"}, {Name: "x"}}, exprStmt(ident("missing")))
		got := EvalExprWithOptions(callExpr(ident("reduce"),
			posArg(bad),
			posArg(listExpr(intExpr(1), intExpr(2), intExpr(3))),
		), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null result, got %#v", got)
		}
		if count := diagCount(diags, "E100"); count != 1 {
			t.Fatalf("expected exactly one E100, got %d: %s", count, diags.String())
		}
	})
}
