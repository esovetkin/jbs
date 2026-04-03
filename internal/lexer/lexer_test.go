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

func TestLexSemicolonToken(t *testing.T) {
	src := "a = 1; b = 2\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	found := false
	for _, tok := range tokens {
		if tok.Type == TokenSemicolon {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected semicolon token in token stream")
	}
}

func TestLexStringKeepsSemicolonLiteral(t *testing.T) {
	src := `x = "a;b;c"`
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
	if got != "a;b;c" {
		t.Fatalf("expected semicolon literal preserved, got %q", got)
	}
}

func TestLexBackslashNewlineContinuation(t *testing.T) {
	src := "a = 1 + \\\n2\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	newlineCount := 0
	for _, tok := range tokens {
		if tok.Type == TokenNewline {
			newlineCount++
		}
	}
	if newlineCount != 1 {
		t.Fatalf("expected exactly one newline token, got %d", newlineCount)
	}

	hasPlusThenTwo := false
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Type == TokenPlus && tokens[i+1].Type == TokenNumber && tokens[i+1].Value == "2" {
			hasPlusThenTwo = true
			break
		}
	}
	if !hasPlusThenTwo {
		t.Fatalf("expected '+' followed directly by numeric token 2 after continuation")
	}
}

func TestLexCommentTrailingBackslashDoesNotContinue(t *testing.T) {
	src := "a = 1 # trailing \\\nb = 2\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	newlineCount := 0
	for _, tok := range tokens {
		if tok.Type == TokenNewline {
			newlineCount++
		}
	}
	if newlineCount != 2 {
		t.Fatalf("expected two newline tokens (comment line + second line), got %d", newlineCount)
	}
}

func TestLexUseKeyword(t *testing.T) {
	src := "use jsc\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	if len(tokens) < 2 {
		t.Fatalf("expected at least two tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TokenUse {
		t.Fatalf("expected first token to be TokenUse, got %s", tokens[0].Type)
	}
	if tokens[1].Type != TokenIdent || tokens[1].Value != "jsc" {
		t.Fatalf("unexpected second token: %#v", tokens[1])
	}
}
