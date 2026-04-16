package parser

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lexer"
)

func TestReadBalancedBlock(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("{\n  echo \"{ok}\"\n}\n", diags)
		body, innerStart, blockEnd, ok := p.readBalancedBlock()
		if !ok {
			t.Fatalf("expected balanced block parse success, got diagnostics: %s", diags.String())
		}
		if body == "" {
			t.Fatalf("expected non-empty block body")
		}
		if innerStart.Offset <= 0 || blockEnd.Offset <= innerStart.Offset {
			t.Fatalf("unexpected positions: inner=%+v end=%+v", innerStart, blockEnd)
		}
	})

	t.Run("unterminated", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("{\n  echo x\n", diags)
		_, _, _, ok := p.readBalancedBlock()
		if ok {
			t.Fatalf("expected readBalancedBlock to fail for unterminated block")
		}
		if !hasDiag(diags, "E025") {
			t.Fatalf("expected E025, got: %s", diags.String())
		}
	})
}

func TestSkipTriviaAndAdvanceEOF(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(" ;\n\t# c0\n# c1\nx", diags)
	p.skipTrivia()
	if got := p.peek(); got != 'x' {
		t.Fatalf("expected skipTrivia to stop at x, got %q", got)
	}

	p2 := newTopLevelParser("", &diag.Diagnostics{})
	if got := p2.advance(); got != 0 {
		t.Fatalf("expected advance() at EOF to return 0, got %q", got)
	}
}

func TestTokenParserScanHelpers(t *testing.T) {
	t.Run("peek and peekN on empty token slice", func(t *testing.T) {
		tp := &tokenParser{tokens: nil, diags: &diag.Diagnostics{}}
		if got := tp.peek(); got.Type != lexer.TokenEOF {
			t.Fatalf("expected EOF token from peek on empty stream, got %#v", got)
		}
		if got := tp.peekN(3); got.Type != lexer.TokenEOF {
			t.Fatalf("expected EOF token from peekN on empty stream, got %#v", got)
		}
	})

	t.Run("expect mismatch emits diagnostic and preserve current token", func(t *testing.T) {
		span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
		diags := &diag.Diagnostics{}
		tp := &tokenParser{
			tokens: []lexer.Token{
				{Type: lexer.TokenIdent, Value: "x", Span: span},
				{Type: lexer.TokenEOF, Span: span},
			},
			diags: diags,
		}
		got := tp.expect(lexer.TokenLBrace, diag.CodeE050, "expected lbrace")
		if got.Type != lexer.TokenIdent {
			t.Fatalf("expected mismatched token to be returned unchanged, got %#v", got)
		}
		if !hasDiag(diags, "E050") {
			t.Fatalf("expected E050 from expect mismatch, got: %s", diags.String())
		}
	})
}
