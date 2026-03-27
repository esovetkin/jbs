package lexer

import (
	"testing"

	"jbs/internal/diag"
)

func TestLexBasicTokens(t *testing.T) {
	src := "a = (1, 2) # comment\nb = \"x\"\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	if len(tokens) == 0 {
		t.Fatalf("expected tokens")
	}
	if tokens[0].Type != TokenIdent || tokens[0].Value != "a" {
		t.Fatalf("unexpected first token: %#v", tokens[0])
	}
}

func TestLexUnexpectedCharacter(t *testing.T) {
	src := "a = @\n"
	diags := &diag.Diagnostics{}
	_ = Lex("in.jbs", src, diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E003" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E003 for unexpected character, got: %s", diags.String())
	}
}

func TestLexStringKeepsBackslashNLiteral(t *testing.T) {
	src := `x = "a\nb \"q\""`
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	var got string
	for _, tok := range tokens {
		if tok.Type == TokenString {
			got = tok.Value
			break
		}
	}
	if got == "" {
		t.Fatalf("expected string token")
	}
	if got != `a\nb "q"` {
		t.Fatalf("expected literal backslash-n preserved, got %q", got)
	}
}
