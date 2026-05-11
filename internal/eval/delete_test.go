package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func deleteCall(names ...string) ast.CallExpr {
	args := make([]ast.CallArg, 0, len(names))
	for _, name := range names {
		args = append(args, posArg(ident(name)))
	}
	return callExpr(ident("delete"), args...)
}

func TestEvalDeleteRemovesCurrentFrameLocals(t *testing.T) {
	frame := NewRootFrame(map[string]Value{"x": Int(1), "y": Int(2)})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(deleteCall("x", "y"), nil, diags, ExprOptions{Frame: frame})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindNull {
		t.Fatalf("delete() should return null, got %#v", got)
	}
	if frame.HasLocal("x") || frame.HasLocal("y") {
		t.Fatalf("delete() did not remove local variables")
	}
}

func TestEvalDeleteUsesTopLevelHook(t *testing.T) {
	frame := NewRootFrame(map[string]Value{"x": Int(1)})
	deleted := make([]string, 0)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(deleteCall("x"), nil, diags, ExprOptions{
		Frame: frame,
		DeleteName: func(name string, at diag.Span, diags *diag.Diagnostics) bool {
			deleted = append(deleted, name)
			frame.DeleteLocal(name)
			return true
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindNull {
		t.Fatalf("delete() should return null, got %#v", got)
	}
	if len(deleted) != 1 || deleted[0] != "x" {
		t.Fatalf("delete() did not call top-level hook: %#v", deleted)
	}
	if frame.HasLocal("x") {
		t.Fatalf("top-level hook did not delete x")
	}
}

func TestEvalDeleteInvalidTargets(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.CallExpr
		env      map[string]Value
		wantCode string
		wantText string
	}{
		{
			name:     "zero args",
			expr:     callExpr(ident("delete")),
			wantCode: "E106",
			wantText: "expects at least one variable",
		},
		{
			name:     "string target",
			expr:     callExpr(ident("delete"), posArg(stringExpr("x"))),
			wantCode: "E106",
			wantText: "targets must be bare identifiers",
		},
		{
			name: "qualified target",
			expr: callExpr(ident("delete"), posArg(ast.QualifiedIdentExpr{
				Namespace: "lib",
				Name:      "x",
			})),
			wantCode: "E106",
			wantText: "targets must be bare identifiers",
		},
		{
			name:     "named target",
			expr:     callExpr(ident("delete"), namedArg("x", intExpr(1))),
			wantCode: "E106",
			wantText: "does not accept named arguments",
		},
		{
			name:     "duplicate target",
			expr:     deleteCall("x", "x"),
			env:      map[string]Value{"x": Int(1)},
			wantCode: "E106",
			wantText: "listed more than once",
		},
		{
			name:     "missing local",
			expr:     deleteCall("missing"),
			wantCode: "E100",
			wantText: "unknown local variable 'missing'",
		},
		{
			name:     "builtin function",
			expr:     deleteCall("range"),
			wantCode: "E106",
			wantText: "cannot delete built-in function 'range'",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, tc.env, diags, ExprOptions{})
			if got.Kind != KindNull {
				t.Fatalf("delete() should return null, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
			if !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected diagnostic containing %q, got: %s", tc.wantText, diags.String())
			}
		})
	}
}

func TestEvalDeleteCanRemoveLocalBuiltinShadow(t *testing.T) {
	frame := NewRootFrame(map[string]Value{"range": Int(99)})
	diags := &diag.Diagnostics{}
	_ = EvalExprWithOptions(deleteCall("range"), nil, diags, ExprOptions{Frame: frame})
	if diags.HasErrors() {
		t.Fatalf("unexpected delete diagnostics: %s", diags.String())
	}
	if frame.HasLocal("range") {
		t.Fatalf("delete() did not remove local shadow")
	}
	got := EvalExprWithOptions(callExpr(ident("range"), posArg(intExpr(2))), nil, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected range diagnostics after deleting shadow: %s", diags.String())
	}
	want := List([]Value{Int(0), Int(1)})
	if !Equal(got, want) {
		t.Fatalf("unexpected range result after deleting shadow: got=%#v want=%#v", got, want)
	}
}

func TestEvalDeleteDoesNotRemoveCapturedParent(t *testing.T) {
	parent := NewRootFrame(map[string]Value{"x": Int(1)})
	child := NewChildFrame(parent)
	diags := &diag.Diagnostics{}
	_ = EvalExprWithOptions(deleteCall("x"), nil, diags, ExprOptions{Frame: child})
	if diagCount(diags, "E100") != 1 {
		t.Fatalf("expected E100 for captured parent delete, got: %s", diags.String())
	}
	if !parent.HasLocal("x") {
		t.Fatalf("delete() removed captured parent variable")
	}
}

func TestBuiltinCallNamesIncludesDelete(t *testing.T) {
	if !IsBuiltinCallName("delete") {
		t.Fatalf("delete should be a builtin call name")
	}
}
