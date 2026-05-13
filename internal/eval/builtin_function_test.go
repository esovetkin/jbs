package eval

import (
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func assignBuiltinFunction(t *testing.T, frame *Frame, localName, builtinName string) {
	t.Helper()
	value, ok := BuiltinFunctionValue(builtinName)
	if !ok {
		t.Fatalf("missing builtin function %q", builtinName)
	}
	frame.AssignLocal(localName, value, diag.Span{})
}

func TestBuiltinIdentifierEvaluatesToFunctionValue(t *testing.T) {
	diags := &diag.Diagnostics{}
	value := EvalExprWithOptions(ident("int"), nil, diags, ExprOptions{})
	if value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != "int" {
		t.Fatalf("expected int to evaluate to builtin function value, got %#v", value)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}

	frame := NewRootFrame(nil)
	frame.AssignLocal("f", value, diag.Span{})
	diags = &diag.Diagnostics{}
	got := EvalExprWithOptions(callExpr(ident("f"), posArg(ast.StringExpr{Value: "7"})), nil, diags, ExprOptions{Frame: frame})
	if !Equal(got, Int(7)) {
		t.Fatalf("expected f(\"7\") to return 7, got %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestBuiltinIdentifierPreservesLocalAssignmentRules(t *testing.T) {
	t.Run("unassigned local shadows builtin", func(t *testing.T) {
		fn := fnExpr(nil,
			exprStmt(ident("int")),
			localAssign("int", intExpr(1)),
		)
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(fn), nil, diags, ExprOptions{})
		if got.Kind != KindNull {
			t.Fatalf("expected null for unassigned local, got %#v", got)
		}
		if diagCount(diags, "E100") != 1 || !strings.Contains(diags.String(), "local variable 'int' is used before assignment") {
			t.Fatalf("expected unassigned-local diagnostic, got: %s", diags.String())
		}
	})

	t.Run("assigned local shadows builtin", func(t *testing.T) {
		frame := NewRootFrame(nil)
		defineFunctionInFrame(t, frame, "int", fnExpr(
			[]ast.FuncParam{{Name: "x"}},
			exprStmt(intExpr(42)),
		))
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("int"), posArg(ast.StringExpr{Value: "7"})), nil, diags, ExprOptions{Frame: frame})
		if !Equal(got, Int(42)) {
			t.Fatalf("expected shadowing user function result, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func TestBuiltinFunctionValueBehaviorOutsideCalls(t *testing.T) {
	diags := &diag.Diagnostics{}
	listValue := EvalExprWithOptions(callExpr(ident("list"), posArg(ident("int"))), nil, diags, ExprOptions{})
	if listValue.Kind != KindList || len(listValue.L) != 1 || listValue.L[0].Kind != KindFunction {
		t.Fatalf("expected list(int) to preserve builtin function value, got %#v", listValue)
	}
	tupleValue := EvalExprWithOptions(callExpr(ident("tuple"), posArg(ident("int"))), nil, diags, ExprOptions{})
	if tupleValue.Kind != KindTuple || len(tupleValue.L) != 1 || tupleValue.L[0].Kind != KindFunction {
		t.Fatalf("expected tuple(int) to preserve builtin function value, got %#v", tupleValue)
	}
	strValue := EvalExprWithOptions(callExpr(ident("str"), posArg(ident("int"))), nil, diags, ExprOptions{})
	if strValue.Kind != KindString || strValue.S != "<function>" {
		t.Fatalf("expected str(int) == <function>, got %#v", strValue)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestSpecialBuiltinFunctionValues(t *testing.T) {
	t.Run("table and dict and update", func(t *testing.T) {
		frame := NewRootFrame(nil)
		assignBuiltinFunction(t, frame, "make_table", "table")
		assignBuiltinFunction(t, frame, "make_dict", "dict")
		assignBuiltinFunction(t, frame, "patch", "update")

		diags := &diag.Diagnostics{}
		tableValue := EvalExprWithOptions(callExpr(ident("make_table"),
			namedArg("x", listExpr(intExpr(1), intExpr(2))),
		), nil, diags, ExprOptions{Frame: frame})
		if !IsComb(tableValue) || CombRowCount(tableValue) != 2 {
			t.Fatalf("expected table from builtin value, got %#v", tableValue)
		}

		dictValue := EvalExprWithOptions(callExpr(ident("make_dict"), namedArg("x", intExpr(1))), nil, diags, ExprOptions{Frame: frame})
		if dictValue.Kind != KindDict || dictValue.D == nil {
			t.Fatalf("expected dict from builtin value, got %#v", dictValue)
		}

		frame.AssignLocal("base", dictValue, diag.Span{})
		updated := EvalExprWithOptions(callExpr(ident("patch"),
			posArg(ident("base")),
			namedArg("y", intExpr(2)),
		), nil, diags, ExprOptions{Frame: frame})
		if updated.Kind != KindDict || dictLen(updated.D) != 2 {
			t.Fatalf("expected updated dict with two entries, got %#v", updated)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("select and names", func(t *testing.T) {
		frame := NewRootFrame(nil)
		assignBuiltinFunction(t, frame, "project", "select")
		assignBuiltinFunction(t, frame, "name_fn", "names")
		grid := CombValue(&Comb{
			Order: []string{"x", "y"},
			Rows: []Row{{Values: map[string]Cell{
				"x": {Value: Int(1)},
				"y": {Value: String("a")},
			}}},
		})
		frame.AssignLocal("grid", grid, diag.Span{})
		opts := ExprOptions{
			Frame: frame,
			Names: NewNameCatalog([]string{"grid", "name_fn", "project"}, nil),
		}

		diags := &diag.Diagnostics{}
		projected := EvalExprWithOptions(callExpr(ident("project"),
			posArg(ident("grid")),
			posArg(ast.StringExpr{Value: "x"}),
		), nil, diags, opts)
		if !IsComb(projected) || !slices.Equal(CombNames(projected), []string{"x"}) {
			t.Fatalf("expected selected x column, got %#v", projected)
		}

		scopeNames := EvalExprWithOptions(callExpr(ident("name_fn")), nil, diags, opts)
		if !listValueContains(scopeNames, "grid") || !listValueContains(scopeNames, "project") {
			t.Fatalf("expected scope names from names function value, got %#v", scopeNames)
		}
		tableNames := EvalExprWithOptions(callExpr(ident("name_fn"), posArg(ident("grid"))), nil, diags, opts)
		if !Equal(tableNames, List([]Value{String("x"), String("y")})) {
			t.Fatalf("expected table column names, got %#v", tableNames)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("delete uses caller frame", func(t *testing.T) {
		frame := NewRootFrame(map[string]Value{"x": Int(1)})
		assignBuiltinFunction(t, frame, "remove", "delete")

		diags := &diag.Diagnostics{}
		_ = EvalExprWithOptions(callExpr(ident("remove"), posArg(ast.StringExpr{Value: "x"})), nil, diags, ExprOptions{Frame: frame})
		if _, ok := frame.LookupCell("x"); ok {
			t.Fatalf("expected remove(\"x\") to delete caller-frame local")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}

		diags = &diag.Diagnostics{}
		_ = EvalExprWithOptions(callExpr(ident("remove"), posArg(ast.StringExpr{Value: "int"})), nil, diags, ExprOptions{Frame: frame})
		if diagCount(diags, "E106") != 1 || !strings.Contains(diags.String(), "cannot delete built-in function 'int'") {
			t.Fatalf("expected protected builtin delete diagnostic, got: %s", diags.String())
		}
	})

	t.Run("shell function value runs", func(t *testing.T) {
		frame := NewRootFrame(nil)
		assignBuiltinFunction(t, frame, "sh", "shell")
		called := false
		runner := func(ShellCommand) ([]byte, error) {
			called = true
			return []byte("hi\n"), nil
		}

		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(callExpr(ident("sh"), posArg(ast.StringExpr{Value: "printf hi"})), nil, diags, ExprOptions{
			Frame:       frame,
			ShellRunner: runner,
		})
		if !called || got.Kind != KindString || got.S != "hi" {
			t.Fatalf("expected shell function value to run once and return hi, called=%v got=%#v", called, got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})
}

func listValueContains(value Value, want string) bool {
	if value.Kind != KindList {
		return false
	}
	for _, item := range value.L {
		if item.Kind == KindString && item.S == want {
			return true
		}
	}
	return false
}
