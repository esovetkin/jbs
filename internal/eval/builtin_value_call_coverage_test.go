package eval

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestBuiltinValueCallDispatchCoverage(t *testing.T) {
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
		want := RowList([]Value{
			DictValue([]DictEntry{{Key: DictKey{Kind: DictKeyString, S: "x"}, Value: Int(1)}}),
		}, []string{"x"})
		if !Equal(got, want) {
			t.Fatalf("unexpected rows dispatch result: got=%#v want=%#v", got, want)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestSimpleBuiltinValuesCoverage(t *testing.T) {
	span := spanAt(1801, 1)

	t.Run("unknown function", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		values, ok := simpleBuiltinValues("missing", nil, span, diags)
		if ok || values != nil {
			t.Fatalf("unknown builtin should fail, got values=%#v ok=%v", values, ok)
		}
		if diagCount(diags, "E199") != 1 {
			t.Fatalf("expected E199, got: %s", diags.String())
		}
	})

	t.Run("fallback positional values", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		values, ok := simpleBuiltinValues("env", []CallValueArg{
			{Value: String("A"), Span: span},
			{Value: String("fallback"), Span: span},
		}, span, diags)
		if !ok || len(values) != 2 || !Equal(values[0], String("A")) || !Equal(values[1], String("fallback")) {
			t.Fatalf("unexpected fallback values: values=%#v ok=%v", values, ok)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("fallback rejects named arguments", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		values, ok := simpleBuiltinValues("env", []CallValueArg{{Name: "name", Value: String("A"), Span: span}}, span, diags)
		if ok || values != nil {
			t.Fatalf("named fallback should fail, got values=%#v ok=%v", values, ok)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func TestBindRangeBuiltinValuesNamedErrorCoverage(t *testing.T) {
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
