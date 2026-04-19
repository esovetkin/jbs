package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestParseProgramDispatchAndSpan(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
jbs_name = "bench"
x = (1, 2)
x
do run {
  echo hi
}
submit s {
  account = "a"
  queue = "q"
  args_exec = "-lc hostname"
}
analyse run {
  n = "N: %d" in "out.log"
  (n)
}
	`
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 6 {
		t.Fatalf("expected 6 top-level statements, got %d (%#v)", len(prog.Stmts), prog.Stmts)
	}
	if prog.File != "in.jbs" {
		t.Fatalf("unexpected program file: %q", prog.File)
	}
	if prog.Span.Start.Offset >= prog.Span.End.Offset {
		t.Fatalf("expected non-empty merged program span, got %+v", prog.Span)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid program: %s", diags.String())
	}
}

func TestParseProgramReportsExpressionErrorsForMalformedTopLevelInput(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := "@\nunknownblock x\n"
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two expression statements for malformed source, got %#v", prog.Stmts)
	}
	if !hasDiag(diags, "E058") {
		t.Fatalf("expected E058 for invalid expression token, got: %s", diags.String())
	}
	if !hasDiag(diags, "E061") {
		t.Fatalf("expected E061 for trailing tokens in malformed expr line, got: %s", diags.String())
	}
}

func TestParseProgramWithFunctionSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
f = function(x, y = 1) {
  x + y
}
function(a) {
  return a
}(1)
`
	prog := Parse("functions.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two top-level statements, got %d", len(prog.Stmts))
	}
	assign, ok := prog.Stmts[0].(ast.GlobalAssign)
	if !ok {
		t.Fatalf("expected first stmt to be global assign, got %#v", prog.Stmts[0])
	}
	if _, ok := assign.Expr.(ast.FunctionExpr); !ok {
		t.Fatalf("expected function literal rhs, got %#v", assign.Expr)
	}
	exprStmt, ok := prog.Stmts[1].(ast.ExprStmt)
	if !ok {
		t.Fatalf("expected second stmt to be expr stmt, got %#v", prog.Stmts[1])
	}
	call, ok := exprStmt.Expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression, got %#v", exprStmt.Expr)
	}
	if _, ok := call.Callee.(ast.FunctionExpr); !ok {
		t.Fatalf("expected function literal callee, got %#v", call.Callee)
	}
}
