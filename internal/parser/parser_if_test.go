package parser

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestParseTopLevelIf(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("if.jbs", `
if enabled {
	x = 1
	x
} else {
	if other {
		x = 2
	} else {
		x = 3
	}
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %#v", prog.Stmts)
	}
	stmt, ok := prog.Stmts[0].(ast.IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %#v", prog.Stmts[0])
	}
	if len(stmt.Then) != 2 || len(stmt.Else) != 1 {
		t.Fatalf("unexpected branch sizes: then=%d else=%d", len(stmt.Then), len(stmt.Else))
	}
	if _, ok := stmt.Then[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected branch assignment, got %#v", stmt.Then[0])
	}
	if _, ok := stmt.Then[1].(ast.ExprStmt); !ok {
		t.Fatalf("expected branch expression, got %#v", stmt.Then[1])
	}
	if _, ok := stmt.Else[0].(ast.IfStmt); !ok {
		t.Fatalf("expected nested if, got %#v", stmt.Else[0])
	}
}

func TestParseTopLevelElif(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("if.jbs", `
if a {
	x = 1
} elif b {
	x = 2
} elif c {
	x = 3
} else {
	x = 4
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %#v", prog.Stmts)
	}
	stmt, ok := prog.Stmts[0].(ast.IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %#v", prog.Stmts[0])
	}
	if len(stmt.Elifs) != 2 {
		t.Fatalf("expected two elif branches, got %#v", stmt.Elifs)
	}
	if len(stmt.Elifs[0].Body) != 1 || len(stmt.Elifs[1].Body) != 1 || len(stmt.Else) != 1 {
		t.Fatalf("unexpected branch sizes: %#v", stmt)
	}
}

func TestParseTopLevelIfRejectsDeclarations(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "do", src: "if true { do run { echo hi } }\n", code: "E080"},
		{name: "analyse", src: "if true { analyse run { x = \"X: %d\" in \"out\" } }\n", code: "E080"},
		{name: "use", src: "if true { use lib }\n", code: "E430"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			_ = Parse("if.jbs", tc.src, diags)
			if !hasDiag(diags, tc.code) {
				t.Fatalf("expected %s, got: %s", tc.code, diags.String())
			}
		})
	}
}

func TestParseTopLevelIfMalformedSyntax(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "missing open brace", src: "if true\nx = 1\n", code: "E080"},
		{name: "missing close brace", src: "if true { x = 1\n", code: "E025"},
		{name: "else if rejected", src: "if true { x = 1 } else if false { x = 2 }\n", code: "E080"},
		{name: "stray elif", src: "elif true { x = 1 }\n", code: "E080"},
		{name: "missing elif open brace", src: "if true { x = 1 } elif false\nx = 2\n", code: "E080"},
		{name: "missing elif close brace", src: "if true { x = 1 } elif false { x = 2\n", code: "E025"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			_ = Parse("if.jbs", tc.src, diags)
			if !hasDiag(diags, tc.code) {
				t.Fatalf("expected %s, got: %s", tc.code, diags.String())
			}
		})
	}
}

func TestParseTopLevelElseIfDiagnosticMentionsElif(t *testing.T) {
	diags := &diag.Diagnostics{}
	_ = Parse("if.jbs", "if true { x = 1 } else if false { x = 2 }\n", diags)
	if !hasDiag(diags, "E080") {
		t.Fatalf("expected E080, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "elif") {
		t.Fatalf("expected diagnostic to mention elif, got: %s", diags.String())
	}
}

func TestParseTopLevelIfConditionScanner(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("if.jbs", `
if ("{" == "{") & values[0] == 1 # comment with {
{
	x = 1
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	stmt, ok := prog.Stmts[0].(ast.IfStmt)
	if !ok || stmt.Cond == nil || len(stmt.Then) != 1 {
		t.Fatalf("unexpected if parse result: %#v", prog.Stmts)
	}
}

func TestParseFunctionIf(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("fn_if.jbs", `
f = function(x) {
	if x < 0 {
		return -x
	} else {
		y = x
		y
	}
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	assign := prog.Stmts[0].(ast.GlobalAssign)
	fn := assign.Expr.(ast.FunctionExpr)
	if len(fn.Body) != 1 {
		t.Fatalf("expected one function body statement, got %#v", fn.Body)
	}
	stmt, ok := fn.Body[0].(ast.FuncIfStmt)
	if !ok {
		t.Fatalf("expected FuncIfStmt, got %#v", fn.Body[0])
	}
	if len(stmt.Then) != 1 || len(stmt.Else) != 2 {
		t.Fatalf("unexpected function branch sizes: %#v", stmt)
	}
}

func TestParseFunctionElif(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("fn_if.jbs", `
f = function(x) {
	if x < 0 {
		return -1
	} elif x == 0 {
		return 0
	} else {
		return 1
	}
}
`, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	assign := prog.Stmts[0].(ast.GlobalAssign)
	fn := assign.Expr.(ast.FunctionExpr)
	stmt, ok := fn.Body[0].(ast.FuncIfStmt)
	if !ok {
		t.Fatalf("expected FuncIfStmt, got %#v", fn.Body[0])
	}
	if len(stmt.Elifs) != 1 || len(stmt.Elifs[0].Body) != 1 || len(stmt.Else) != 1 {
		t.Fatalf("unexpected function elif shape: %#v", stmt)
	}
}

func TestParseFunctionIfMalformedElifSyntax(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "stray elif", src: "f = function() { elif true { x = 1 } }\n", code: "E080"},
		{name: "missing elif open brace", src: "f = function() { if true { x = 1 } elif false x = 2 }\n", code: "E080"},
		{name: "missing elif close brace", src: "f = function() { if true { x = 1 } elif false { x = 2 }\n", code: "E025"},
		{name: "else if rejected", src: "f = function() { if true { x = 1 } else if false { x = 2 } }\n", code: "E080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			_ = Parse("fn_if.jbs", tc.src, diags)
			if !hasDiag(diags, tc.code) {
				t.Fatalf("expected %s, got: %s", tc.code, diags.String())
			}
			if tc.name == "else if rejected" && !strings.Contains(diags.String(), "elif") {
				t.Fatalf("expected diagnostic to mention elif, got: %s", diags.String())
			}
		})
	}
}
