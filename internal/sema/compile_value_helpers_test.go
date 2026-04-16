package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestWarnModeExprInCollections(t *testing.T) {
	span := diag.NewSpan("x.jbs", diag.NewPos(1, 1, 1), diag.NewPos(1, 10, 10))
	modeExpr := ast.ModeExpr{
		Mode: "python",
		Expr: ast.StringExpr{Value: "x", Span: span},
		Span: span,
	}

	t.Run("top-level mode has no warning", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		warnModeExprInCollections(modeExpr, diags)
		if got := countDiagCode(diags, string(diag.CodeW301)); got != 0 {
			t.Fatalf("expected 0 W301 diagnostics, got %d", got)
		}
	})

	t.Run("nested mode emits warning", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		warnModeExprInCollections(ast.ListExpr{Items: []ast.Expr{modeExpr}, Span: span}, diags)
		if got := countDiagCode(diags, string(diag.CodeW301)); got != 1 {
			t.Fatalf("expected 1 W301 diagnostic, got %d", got)
		}
	})
}

func TestSeriesAsValue(t *testing.T) {
	if got := seriesAsValue(nil); got.Kind != eval.KindNull {
		t.Fatalf("expected null for empty series, got %#v", got)
	}

	if got := seriesAsValue([]eval.Value{eval.Int(7)}); got.Kind != eval.KindInt || got.I != 7 {
		t.Fatalf("expected scalar passthrough, got %#v", got)
	}

	got := seriesAsValue([]eval.Value{eval.Int(1), eval.Int(2)})
	if got.Kind != eval.KindList || len(got.L) != 2 {
		t.Fatalf("expected list for multi-value series, got %#v", got)
	}
}

func TestUnwrapModeExpr(t *testing.T) {
	span := diag.NewSpan("x.jbs", diag.NewPos(1, 1, 1), diag.NewPos(1, 10, 10))
	mode := ast.ModeExpr{
		Mode: "shell",
		Expr: ast.StringExpr{Value: "hostname", Span: span},
		Span: span,
	}

	gotMode, gotExpr, ok := unwrapModeExpr(mode)
	if !ok {
		t.Fatalf("expected mode expression to unwrap")
	}
	if gotMode != "shell" {
		t.Fatalf("expected mode shell, got %q", gotMode)
	}
	if _, ok := gotExpr.(ast.StringExpr); !ok {
		t.Fatalf("expected inner string expression, got %#v", gotExpr)
	}

	if _, _, ok := unwrapModeExpr(ast.StringExpr{Value: "x", Span: span}); ok {
		t.Fatalf("did not expect non-mode expression to unwrap")
	}
}

func TestCoerceModeValue(t *testing.T) {
	span := diag.NewSpan("x.jbs", diag.NewPos(1, 1, 1), diag.NewPos(1, 10, 10))

	t.Run("string passthrough", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := coerceModeValue("python", eval.String("x"), span, diags)
		if got.Kind != eval.KindString || got.S != "x" {
			t.Fatalf("unexpected coerced value: %#v", got)
		}
		if gotErr := countDiagCode(diags, string(diag.CodeE215)); gotErr != 0 {
			t.Fatalf("expected 0 E215 diagnostics, got %d", gotErr)
		}
	})

	t.Run("list stringifies non-string entries and reports E215", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := coerceModeValue("python", eval.List([]eval.Value{eval.String("x"), eval.Int(2)}), span, diags)
		if got.Kind != eval.KindList || len(got.L) != 2 || got.L[1].Kind != eval.KindString || got.L[1].S != "2" {
			t.Fatalf("unexpected coerced list: %#v", got)
		}
		if gotErr := countDiagCode(diags, string(diag.CodeE215)); gotErr != 1 {
			t.Fatalf("expected 1 E215 diagnostic, got %d", gotErr)
		}
	})

	t.Run("scalar non-string is stringified and reports E215", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := coerceModeValue("shell", eval.Int(5), span, diags)
		if got.Kind != eval.KindString || got.S != "5" {
			t.Fatalf("unexpected coerced scalar: %#v", got)
		}
		if gotErr := countDiagCode(diags, string(diag.CodeE215)); gotErr != 1 {
			t.Fatalf("expected 1 E215 diagnostic, got %d", gotErr)
		}
	})
}
