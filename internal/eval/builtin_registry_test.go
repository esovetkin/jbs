package eval

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestBuiltinValueCallDispatchesFunctionValues(t *testing.T) {
	span := spanAt(1800, 1)

	t.Run("env", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalBuiltinValueCall("env", []CallValueArg{{Value: String("A"), Span: span}}, nil, span, diags, ExprOptions{
			Environ: fixedEnviron("A=value"),
		}, newEvalCtx(nil))
		if !Equal(got, String("value")) {
			t.Fatalf("unexpected env dispatch result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("get", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		dict := DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(7)}})
		got := evalBuiltinValueCall("get", []CallValueArg{
			{Value: dict, Span: span},
			{Value: String("x"), Span: span},
			{Value: Int(0), Span: span},
		}, nil, span, diags, ExprOptions{}, newEvalCtx(nil))
		if !Equal(got, Int(7)) {
			t.Fatalf("unexpected get dispatch result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("map", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		intFn, ok := BuiltinFunctionValue("int")
		if !ok {
			t.Fatal("missing int builtin function value")
		}
		got := evalBuiltinValueCall("map", []CallValueArg{
			{Value: intFn, Span: span},
			{Value: List([]Value{String("1"), String("2")}), Span: span},
		}, nil, span, diags, ExprOptions{}, newEvalCtx(nil))
		if !Equal(got, List([]Value{Int(1), Int(2)})) {
			t.Fatalf("unexpected map dispatch result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("reduce", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		add := Function(&FunctionValue{
			Params: []ast.FuncParam{{Name: "a"}, {Name: "b"}},
			Body: []ast.FuncBodyStmt{exprStmt(ast.BinaryExpr{
				Left:  ident("a"),
				Op:    "+",
				Right: ident("b"),
			})},
		})
		got := evalBuiltinValueCall("reduce", []CallValueArg{
			{Value: add, Span: span},
			{Value: List([]Value{Int(1), Int(2), Int(3)}), Span: span},
		}, nil, span, diags, ExprOptions{}, newEvalCtx(nil))
		if !Equal(got, Int(6)) {
			t.Fatalf("unexpected reduce dispatch result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("rows", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		table := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{{Values: map[string]Cell{
				"x": {Value: Int(1), Origin: span},
			}}},
		})
		got := evalBuiltinValueCall("rows", []CallValueArg{{Value: table, Span: span}}, nil, span, diags, ExprOptions{}, newEvalCtx(nil))
		want := List([]Value{
			DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)}}),
		})
		if !Equal(got, want) {
			t.Fatalf("unexpected rows dispatch result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestBuiltinRegistryExposesFunctionValuesForEveryBuiltin(t *testing.T) {
	for _, name := range BuiltinCallNames() {
		if !IsBuiltinCallName(name) {
			t.Fatalf("BuiltinCallNames returned non-builtin %q", name)
		}
		value, ok := BuiltinFunctionValue(name)
		if !ok {
			t.Fatalf("missing builtin function value for %q", name)
		}
		if value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != name {
			t.Fatalf("bad builtin function value for %q: %#v", name, value)
		}
	}
}

func TestBuiltinFunctionValuesMatchDirectCalls(t *testing.T) {
	tests := []struct {
		name string
		args []ast.CallArg
		opts ExprOptions
	}{
		{name: "int", args: []ast.CallArg{posArg(stringExpr("7"))}},
		{name: "len", args: []ast.CallArg{posArg(listExpr(intExpr(1), intExpr(2)))}},
		{name: "sum", args: []ast.CallArg{posArg(listExpr(intExpr(1), intExpr(2)))}},
		{name: "head", args: []ast.CallArg{posArg(listExpr(intExpr(1), intExpr(2), intExpr(3)))}},
		{name: "unique", args: []ast.CallArg{posArg(listExpr(intExpr(1), intExpr(1), intExpr(2)))}},
		{name: "duplicated", args: []ast.CallArg{posArg(listExpr(intExpr(1), intExpr(1), intExpr(2)))}},
		{name: "dict", args: []ast.CallArg{namedArg("x", intExpr(1))}},
		{name: "table", args: []ast.CallArg{namedArg("x", listExpr(intExpr(1), intExpr(2)))}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directDiags := &diag.Diagnostics{}
			gotDirect := EvalExprWithOptions(callExpr(ident(tc.name), tc.args...), nil, directDiags, tc.opts)
			if directDiags.HasErrors() {
				t.Fatalf("unexpected direct diagnostics: %s", directDiags.String())
			}

			frame := NewRootFrame(nil)
			assignBuiltinFunction(t, frame, "f", tc.name)
			opts := tc.opts
			opts.Frame = frame
			fnDiags := &diag.Diagnostics{}
			gotFn := EvalExprWithOptions(callExpr(ident("f"), tc.args...), nil, fnDiags, opts)
			if fnDiags.HasErrors() {
				t.Fatalf("unexpected function-value diagnostics: %s", fnDiags.String())
			}

			if !Equal(gotDirect, gotFn) {
				t.Fatalf("%s mismatch: direct=%#v function=%#v", tc.name, gotDirect, gotFn)
			}
		})
	}
}

func TestBuiltinRegistryKeepsSyntaxSensitiveDirectCalls(t *testing.T) {
	t.Run("names namespace syntax", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(
			callExpr(ident("names"), posArg(ident("lib"))),
			nil,
			diags,
			ExprOptions{Names: NewNameCatalog([]string{"lib"}, map[string][]string{"lib": {"x", "y"}})},
		)
		if !Equal(got, List([]Value{String("x"), String("y")})) {
			t.Fatalf("unexpected namespace names: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("names function value", func(t *testing.T) {
		frame := NewRootFrame(nil)
		assignBuiltinFunction(t, frame, "name_fn", "names")
		tableValue := CombValue(&Comb{
			Order: []string{"x", "y"},
			Rows: []Row{{Values: map[string]Cell{
				"x": {Value: Int(1)},
				"y": {Value: String("a")},
			}}},
		})
		dictValue := DictValue([]DictEntry{
			{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)},
			{Key: DictKey{Kind: DictKeyString, S: "y"}, Value: Int(2)},
		})
		frame.AssignLocal("grid", tableValue, diag.Span{})
		frame.AssignLocal("settings", dictValue, diag.Span{})

		diags := &diag.Diagnostics{}
		opts := ExprOptions{Frame: frame, Names: NewNameCatalog([]string{"grid", "settings"}, nil)}
		tableNames := EvalExprWithOptions(callExpr(ident("name_fn"), posArg(ident("grid"))), nil, diags, opts)
		if !Equal(tableNames, List([]Value{String("x"), String("y")})) {
			t.Fatalf("unexpected table names: %#v", tableNames)
		}
		dictNames := EvalExprWithOptions(callExpr(ident("name_fn"), posArg(ident("settings"))), nil, diags, opts)
		if !Equal(dictNames, List([]Value{String("x"), String("y")})) {
			t.Fatalf("unexpected dictionary names: %#v", dictNames)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("delete direct identifier", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{"x": Int(1)})
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("delete"), posArg(ident("x"))), nil, diags, ExprOptions{Frame: frame})
		if got.Kind != KindNull {
			t.Fatalf("delete() should return null, got %#v", got)
		}
		if _, ok := frame.LookupCell("x"); ok {
			t.Fatalf("delete(x) did not remove x")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("delete function value string", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{"x": Int(1)})
		assignBuiltinFunction(t, frame, "remove", "delete")
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("remove"), posArg(stringExpr("x"))), nil, diags, ExprOptions{Frame: frame})
		if got.Kind != KindNull {
			t.Fatalf("delete function value should return null, got %#v", got)
		}
		if _, ok := frame.LookupCell("x"); ok {
			t.Fatalf("remove(\"x\") did not remove x")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestBuiltinRegistryAllowsRangeRevInAllContexts(t *testing.T) {
	contexts := []struct {
		name string
		opts ExprOptions
	}{
		{name: "default"},
		{name: "binding", opts: ExprOptions{Context: EvalCtxBindingAssign}},
		{name: "scalar global", opts: ExprOptions{Context: EvalCtxScalarGlobalAssign}},
		{name: "analyse", opts: ExprOptions{Context: EvalCtxAnalyseAssign}},
	}
	for _, tc := range contexts {
		t.Run("range "+tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(ident("range"), posArg(intExpr(3))), nil, diags, tc.opts)
			if !Equal(got, List([]Value{Int(0), Int(1), Int(2)})) {
				t.Fatalf("unexpected range result: %#v", got)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
		t.Run("rev "+tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(callExpr(ident("rev"), posArg(listExpr(intExpr(1), intExpr(2)))), nil, diags, tc.opts)
			if !Equal(got, List([]Value{Int(2), Int(1)})) {
				t.Fatalf("unexpected rev result: %#v", got)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
		})
	}
}

func TestBuiltinRangeRevFunctionValuesHaveNoContextRestriction(t *testing.T) {
	frame := NewRootFrame(nil)
	assignBuiltinFunction(t, frame, "r", "range")
	assignBuiltinFunction(t, frame, "reverse", "rev")

	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("r"), posArg(intExpr(2))), nil, diags, ExprOptions{Frame: frame})
	if !Equal(got, List([]Value{Int(0), Int(1)})) {
		t.Fatalf("unexpected range function result: %#v", got)
	}
	got = EvalExprWithOptions(callExpr(ident("reverse"), posArg(listExpr(intExpr(1), intExpr(2)))), nil, diags, ExprOptions{Frame: frame})
	if !Equal(got, List([]Value{Int(2), Int(1)})) {
		t.Fatalf("unexpected rev function result: %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestBuiltinRegistryRejectsUnknownBuiltinValueCall(t *testing.T) {
	span := spanAt(1801, 1)

	diags := &diag.Diagnostics{}
	got := evalBuiltinValueCall("missing", nil, nil, span, diags, ExprOptions{}, newEvalCtx(nil))
	if got.Kind != KindNull || diagCount(diags, "E199") != 1 {
		t.Fatalf("expected unknown builtin diagnostic, got value=%#v diags=%s", got, diags.String())
	}
}

func TestBindRangeBuiltinValuesRejectsTrailingPositionalAfterNamed(t *testing.T) {
	span := spanAt(1802, 1)
	diags := &diag.Diagnostics{}
	values, ok := bindRangeBuiltinValues([]CallValueArg{
		{Name: "stop", Value: Int(3), Span: span},
		{Value: Int(4), Span: span},
	}, span, diags)
	if ok || values != nil {
		t.Fatalf("named range with trailing positional should fail, got values=%#v ok=%v", values, ok)
	}
	if diagCount(diags, "E106") != 1 {
		t.Fatalf("expected E106, got: %s", diags.String())
	}
}
