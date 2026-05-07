package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestParseStandaloneExpr(t *testing.T) {
	t.Run("parses standalone expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		expr, ok := ParseStandaloneExpr("expr.jbs", "range(10)", diag.NewPos(0, 1, 1), diags)
		if !ok {
			t.Fatalf("expected standalone expression to be handled")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		call, ok := expr.(ast.CallExpr)
		if !ok {
			t.Fatalf("expected call expression, got %#v", expr)
		}
		callee, ok := call.Callee.(ast.IdentExpr)
		if !ok || callee.Name != "range" {
			t.Fatalf("expected range call, got %#v", call.Callee)
		}
	})

	t.Run("accepts trailing comment", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		expr, ok := ParseStandaloneExpr("expr.jbs", "range(3) # comment", diag.NewPos(0, 1, 1), diags)
		if !ok || expr == nil {
			t.Fatalf("expected expression with trailing comment to be handled")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("falls back for assignment statement", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		expr, ok := ParseStandaloneExpr("expr.jbs", "x = 1", diag.NewPos(0, 1, 1), diags)
		if ok {
			t.Fatalf("expected assignment to fall back to statement path, expr=%#v", expr)
		}
		if expr != nil {
			t.Fatalf("expected nil expression for fallback, got %#v", expr)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect parse errors on fallback, got: %s", diags.String())
		}
	})

	t.Run("falls back for block keyword statement", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		expr, ok := ParseStandaloneExpr("expr.jbs", "do run {}", diag.NewPos(0, 1, 1), diags)
		if ok {
			t.Fatalf("expected do-block start to fall back, expr=%#v", expr)
		}
		if expr != nil {
			t.Fatalf("expected nil expression for fallback, got %#v", expr)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect parse errors on fallback, got: %s", diags.String())
		}
	})

	t.Run("reports trailing tokens for expression-shaped input", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		expr, ok := ParseStandaloneExpr("expr.jbs", "1 2", diag.NewPos(0, 1, 1), diags)
		if !ok {
			t.Fatalf("expected expression-shaped input to be handled")
		}
		if expr == nil {
			t.Fatalf("expected parsed expression for handled input")
		}
		if !hasCode(diags, "E061") {
			t.Fatalf("expected E061, got: %s", diags.String())
		}
	})

	t.Run("reports malformed expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		expr, ok := ParseStandaloneExpr("expr.jbs", "range(,)", diag.NewPos(0, 1, 1), diags)
		if !ok {
			t.Fatalf("expected malformed expression to stay in expression path")
		}
		if expr == nil {
			t.Fatalf("expected non-nil expression node even with diagnostics")
		}
		if !hasCode(diags, "E058") {
			t.Fatalf("expected E058, got: %s", diags.String())
		}
	})
}
