package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
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
analyse run {
  n = "N: %d" in "out.log"
  (n)
}
	`
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 5 {
		t.Fatalf("expected 5 top-level statements, got %d (%#v)", len(prog.Stmts), prog.Stmts)
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

func TestParseProgramTreatsLetAndParamBlocksAsGenericInvalidExpressions(t *testing.T) {
	tests := []string{
		"let defaults {\n  queue = \"batch\"\n}\n",
		"param cases {\n  x = (1, 2)\n  x\n}\n",
	}

	for _, src := range tests {
		diags := &diag.Diagnostics{}
		prog := Parse("in.jbs", src, diags)
		if len(prog.Stmts) != 1 {
			t.Fatalf("expected one statement for %q, got %#v", src, prog.Stmts)
		}
		if !hasDiag(diags, "E061") {
			t.Fatalf("expected generic trailing-token diagnostic for %q, got: %s", src, diags.String())
		}
		if got := len(diags.Items); got != 1 {
			t.Fatalf("expected exactly one diagnostic for %q, got %d: %s", src, got, diags.String())
		}
	}
}

func TestParseProgramRecoversAfterFormerKeywordShapedBlocks(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
param cases {
  x = (1, 2)
  x
}
let defaults {
  queue = "batch"
}
do run {
  echo ok
}
`
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 3 {
		t.Fatalf("expected three top-level statements, got %#v", prog.Stmts)
	}
	count := 0
	for _, item := range diags.Items {
		if item.Code == "E061" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected two generic expression diagnostics, got %d: %s", count, diags.String())
	}
	if _, ok := prog.Stmts[2].(ast.DoBlock); !ok {
		t.Fatalf("expected parser to recover and keep trailing do block, got %#v", prog.Stmts[2])
	}
}

func TestParseProgramDoesNotTreatLetOrParamAssignmentsAsLegacyBlocks(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := "let = 1\nparam = 2\n"
	prog := Parse("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two assignments, got %#v", prog.Stmts)
	}
	if _, ok := prog.Stmts[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected first statement to stay a global assignment, got %#v", prog.Stmts[0])
	}
	if _, ok := prog.Stmts[1].(ast.GlobalAssign); !ok {
		t.Fatalf("expected second statement to stay a global assignment, got %#v", prog.Stmts[1])
	}
}
