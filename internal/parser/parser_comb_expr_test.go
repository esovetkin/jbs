package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

func parseCombPrimaryFrom(src string, diags *diag.Diagnostics) (ast.CombExpr, *tokenParser) {
	tokens := lexer.LexFrom("comb_expr.jbs", src, diag.NewPos(0, 1, 1), diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	return tp.parseCombPrimary(), tp
}

func TestParseCombPrimaryIdent(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, tp := parseCombPrimaryFrom("alpha", diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	id, ok := expr.(ast.CombIdent)
	if !ok {
		t.Fatalf("expected CombIdent, got %T", expr)
	}
	if id.Name != "alpha" {
		t.Fatalf("expected identifier name alpha, got %q", id.Name)
	}
	if tp.peek().Type != lexer.TokenEOF {
		t.Fatalf("expected parser to stop at EOF, got %s", tp.peek().Type)
	}
}

func TestParseCombPrimaryQualifiedIdent(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, tp := parseCombPrimaryFrom("ns.value", diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	id, ok := expr.(ast.CombIdent)
	if !ok {
		t.Fatalf("expected CombIdent, got %T", expr)
	}
	if id.Name != "ns.value" {
		t.Fatalf("expected qualified identifier ns.value, got %q", id.Name)
	}
	if tp.peek().Type != lexer.TokenEOF {
		t.Fatalf("expected parser to stop at EOF, got %s", tp.peek().Type)
	}
}

func TestParseCombPrimaryQualifiedIdentMissingMember(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, _ := parseCombPrimaryFrom("ns.)", diags)
	id, ok := expr.(ast.CombIdent)
	if !ok {
		t.Fatalf("expected CombIdent, got %T", expr)
	}
	if len(id.Name) < 3 || id.Name[:3] != "ns." {
		t.Fatalf("expected partial qualified identifier starting with ns., got %q", id.Name)
	}
	if !hasCode(diags, "E064") {
		t.Fatalf("expected E064 for missing identifier after dot, got: %s", diags.String())
	}
}

func TestParseCombPrimaryParenthesizedExpression(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, tp := parseCombPrimaryFrom("(a*b+c)", diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}

	top, ok := expr.(ast.CombBinary)
	if !ok {
		t.Fatalf("expected top-level CombBinary, got %T", expr)
	}
	if top.Op != "+" {
		t.Fatalf("expected top-level '+' operator, got %q", top.Op)
	}
	left, ok := top.Left.(ast.CombBinary)
	if !ok || left.Op != "*" {
		t.Fatalf("expected left side to be '*' CombBinary, got %#v", top.Left)
	}
	if tp.peek().Type != lexer.TokenEOF {
		t.Fatalf("expected parser to stop at EOF, got %s", tp.peek().Type)
	}
}

func TestParseCombPrimaryParenthesizedMissingClosingParen(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, tp := parseCombPrimaryFrom("(a+b", diags)
	top, ok := expr.(ast.CombBinary)
	if !ok || top.Op != "+" {
		t.Fatalf("expected parsed inner expression a+b, got %#v", expr)
	}
	if !hasCode(diags, "E059") {
		t.Fatalf("expected E059 for missing closing ')', got: %s", diags.String())
	}
	if tp.peek().Type != lexer.TokenEOF {
		t.Fatalf("expected parser position at EOF after missing ')', got %s", tp.peek().Type)
	}
}

func TestParseCombPrimaryUnexpectedTokenReportsE060(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, tp := parseCombPrimaryFrom("1", diags)
	id, ok := expr.(ast.CombIdent)
	if !ok {
		t.Fatalf("expected fallback CombIdent, got %T", expr)
	}
	if id.Name != "" {
		t.Fatalf("expected empty fallback identifier name, got %q", id.Name)
	}
	if !hasCode(diags, "E060") {
		t.Fatalf("expected E060 for invalid token in combination expression, got: %s", diags.String())
	}
	if tp.peek().Type != lexer.TokenEOF {
		t.Fatalf("expected parser to advance past invalid token, got %s", tp.peek().Type)
	}
}
