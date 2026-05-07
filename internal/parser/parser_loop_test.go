package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestParseTopLevelLoops(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("loop.jbs", `
for x in range(10) {
	if x == 5 {
		break
	}
	continue
}
while x < 3 {
	x += 1
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two statements, got %#v", prog.Stmts)
	}
	forStmt, ok := prog.Stmts[0].(ast.ForStmt)
	if !ok {
		t.Fatalf("expected ForStmt, got %#v", prog.Stmts[0])
	}
	if forStmt.Target != "x" || len(forStmt.Body) != 2 {
		t.Fatalf("unexpected for statement: %#v", forStmt)
	}
	if _, ok := forStmt.Body[0].(ast.IfStmt); !ok {
		t.Fatalf("expected nested if, got %#v", forStmt.Body[0])
	}
	if _, ok := forStmt.Body[1].(ast.ContinueStmt); !ok {
		t.Fatalf("expected continue, got %#v", forStmt.Body[1])
	}
	whileStmt, ok := prog.Stmts[1].(ast.WhileStmt)
	if !ok {
		t.Fatalf("expected WhileStmt, got %#v", prog.Stmts[1])
	}
	if len(whileStmt.Body) != 1 {
		t.Fatalf("unexpected while body: %#v", whileStmt.Body)
	}
}

func TestParseFunctionLoops(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("loop_fn.jbs", `
f = function(values) {
	for x in values {
		if x == 0 {
			continue
		}
		return x
	}
	while false {
		break
	}
	return 0
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	assign := prog.Stmts[0].(ast.GlobalAssign)
	fn := assign.Expr.(ast.FunctionExpr)
	if len(fn.Body) != 3 {
		t.Fatalf("expected three body statements, got %#v", fn.Body)
	}
	if _, ok := fn.Body[0].(ast.FuncForStmt); !ok {
		t.Fatalf("expected FuncForStmt, got %#v", fn.Body[0])
	}
	if _, ok := fn.Body[1].(ast.FuncWhileStmt); !ok {
		t.Fatalf("expected FuncWhileStmt, got %#v", fn.Body[1])
	}
}

func TestParseLoopMalformedSyntax(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "missing target", src: "for in xs {}\n", code: "E080"},
		{name: "missing in", src: "for x xs {}\n", code: "E080"},
		{name: "missing iterable", src: "for x in {}\n", code: "E080"},
		{name: "missing open brace", src: "for x in xs\n", code: "E080"},
		{name: "missing close brace", src: "while true { x = 1\n", code: "E025"},
		{name: "break outside loop", src: "break\n", code: "E080"},
		{name: "continue outside loop", src: "continue\n", code: "E080"},
		{name: "function break outside loop", src: "f = function() { break }\n", code: "E080"},
		{name: "function continue outside loop", src: "f = function() { continue }\n", code: "E080"},
		{name: "break trailing", src: "for x in xs { break x }\n", code: "E061"},
		{name: "continue trailing", src: "for x in xs { continue x }\n", code: "E061"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			_ = Parse("loop.jbs", tc.src, diags)
			if !hasDiag(diags, tc.code) {
				t.Fatalf("expected %s, got: %s", tc.code, diags.String())
			}
		})
	}
}

func TestParseLoopsRejectDeclarations(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "for do", src: "for x in xs { do run { echo hi } }\n", code: "E080"},
		{name: "for analyse", src: "for x in xs { analyse s { x = \"X: %d\" in \"out\" } }\n", code: "E080"},
		{name: "for use", src: "for x in xs { use lib }\n", code: "E430"},
		{name: "while do", src: "while true { do run { echo hi } }\n", code: "E080"},
		{name: "while analyse", src: "while true { analyse s { x = \"X: %d\" in \"out\" } }\n", code: "E080"},
		{name: "while use", src: "while true { use lib }\n", code: "E430"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			_ = Parse("loop.jbs", tc.src, diags)
			if !hasDiag(diags, tc.code) {
				t.Fatalf("expected %s, got: %s", tc.code, diags.String())
			}
		})
	}
}
