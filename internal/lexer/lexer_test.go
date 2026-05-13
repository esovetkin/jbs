package lexer

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
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

func TestLexDictionaryPunctuation(t *testing.T) {
	src := `d = {"a": 1, 2: "b"}`
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	found := map[TokenType]bool{}
	for _, tok := range tokens {
		found[tok.Type] = true
	}
	for _, tt := range []TokenType{TokenLBrace, TokenColon, TokenComma, TokenRBrace} {
		if !found[tt] {
			t.Fatalf("expected token %s, got %#v", tt, tokens)
		}
	}
}

func TestLexStarSymbols(t *testing.T) {
	src := "a * b\nx *= y\nf(**kwargs)\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	counts := map[TokenType]int{}
	for _, tok := range tokens {
		counts[tok.Type]++
	}
	if counts[TokenStar] != 1 {
		t.Fatalf("expected one TokenStar, got %#v", tokens)
	}
	if counts[TokenStarEqual] != 1 {
		t.Fatalf("expected one TokenStarEqual, got %#v", tokens)
	}
	if counts[TokenStarStar] != 1 {
		t.Fatalf("expected one TokenStarStar, got %#v", tokens)
	}
}

func TestLexFunctionAndReturnKeywords(t *testing.T) {
	src := "fn = function(x) {\n    return x\n}\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	var sawFunction, sawReturn bool
	for _, tok := range tokens {
		switch tok.Type {
		case TokenFunction:
			sawFunction = true
		case TokenReturn:
			sawReturn = true
		}
	}
	if !sawFunction || !sawReturn {
		t.Fatalf("expected function and return tokens, got %#v", tokens)
	}
}

func TestLexIfAndElseKeywords(t *testing.T) {
	src := "if enabled { x = 1 } elif other { x = 2 } else { x = 3 }\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	var sawIf, sawElif, sawElse bool
	for _, tok := range tokens {
		switch tok.Type {
		case TokenIf:
			sawIf = true
		case TokenElif:
			sawElif = true
		case TokenElse:
			sawElse = true
		}
	}
	if !sawIf || !sawElif || !sawElse {
		t.Fatalf("expected if, elif, and else tokens, got %#v", tokens)
	}
}

func TestLexLoopKeywords(t *testing.T) {
	src := "for x in xs { while ok { break; continue } }\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	found := map[TokenType]bool{}
	for _, tok := range tokens {
		found[tok.Type] = true
	}
	for _, tt := range []TokenType{TokenFor, TokenIn, TokenWhile, TokenBreak, TokenContinue} {
		if !found[tt] {
			t.Fatalf("expected token %s, got %#v", tt, tokens)
		}
	}
}

func TestLexKeywordLikeIdentifiersStayIdentifiers(t *testing.T) {
	src := "function_name = 1\nreturn_value = 2\nifdef = 3\nelseif = 4\nelif_value = 5\nelifx = 6\nforeach = 7\nwhiled = 8\nbreakfast = 9\ncontinued = 10\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	found := map[string]TokenType{}
	for _, tok := range tokens {
		if tok.Type == TokenIdent {
			found[tok.Value] = tok.Type
		}
	}
	if found["function_name"] != TokenIdent || found["return_value"] != TokenIdent {
		t.Fatalf("expected identifier tokens for keyword-like names, got %#v", tokens)
	}
	if found["ifdef"] != TokenIdent || found["elseif"] != TokenIdent || found["elif_value"] != TokenIdent || found["elifx"] != TokenIdent {
		t.Fatalf("expected identifier tokens for if-like names, got %#v", tokens)
	}
	if found["foreach"] != TokenIdent || found["whiled"] != TokenIdent || found["breakfast"] != TokenIdent || found["continued"] != TokenIdent {
		t.Fatalf("expected identifier tokens for loop-keyword-like names, got %#v", tokens)
	}
}

func TestLexEmitsCommentToken(t *testing.T) {
	src := "a = 1 # trailing comment\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	found := false
	for _, tok := range tokens {
		if tok.Type != TokenComment {
			continue
		}
		found = true
		if tok.Text != "# trailing comment" {
			t.Fatalf("unexpected comment token text: %q", tok.Text)
		}
		if tok.Value != " trailing comment" {
			t.Fatalf("unexpected comment token value: %q", tok.Value)
		}
	}
	if !found {
		t.Fatalf("expected TokenComment in token stream")
	}
}

func TestLexHashInsideStringIsNotCommentToken(t *testing.T) {
	src := `a = "value # not comment"` + "\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	for _, tok := range tokens {
		if tok.Type == TokenComment {
			t.Fatalf("unexpected TokenComment for hash inside string")
		}
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

func TestLexSingleQuotedStringWithEscapedQuote(t *testing.T) {
	src := "x = 'a\\'b'\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	var str Token
	found := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			str = tok
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected string token")
	}
	if str.Text != `'a\'b'` {
		t.Fatalf("unexpected string token text: got %q want %q", str.Text, `'a\'b'`)
	}
	if str.Value != "a'b" {
		t.Fatalf("unexpected string token value: got %q want %q", str.Value, "a'b")
	}
}

func TestLexUnterminatedStringReportsE001(t *testing.T) {
	src := `x = "abc`
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)

	hasE001 := false
	for _, d := range diags.Items {
		if d.Code == "E001" {
			hasE001 = true
			break
		}
	}
	if !hasE001 {
		t.Fatalf("expected E001 for unterminated string, got: %s", diags.String())
	}

	var str Token
	found := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			str = tok
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TokenString even on unterminated string")
	}
	if str.Text != "abc" || str.Value != "abc" {
		t.Fatalf("unexpected unterminated string token payload: %#v", str)
	}
}

func TestLexUnterminatedStringWithTrailingBackslashAtEOF(t *testing.T) {
	src := `x = "abc\`
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)

	hasE001 := false
	for _, d := range diags.Items {
		if d.Code == "E001" {
			hasE001 = true
			break
		}
	}
	if !hasE001 {
		t.Fatalf("expected E001 for unterminated trailing-backslash string, got: %s", diags.String())
	}

	var str Token
	found := false
	for _, tok := range tokens {
		if tok.Type == TokenString {
			str = tok
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TokenString even on unterminated trailing-backslash string")
	}
	if str.Text != "abc" || str.Value != "abc" {
		t.Fatalf("unexpected token payload for trailing-backslash unterminated string: %#v", str)
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
	src := "use lib\n"
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
	if tokens[1].Type != TokenIdent || tokens[1].Value != "lib" {
		t.Fatalf("unexpected second token: %#v", tokens[1])
	}
}

func TestLexCompoundAssignmentTokens(t *testing.T) {
	src := "a += 1; b -= 2; c *= 3; d /= 4; e %= 5\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}

	expected := []TokenType{
		TokenIdent, TokenPlusEqual, TokenNumber, TokenSemicolon,
		TokenIdent, TokenMinusEqual, TokenNumber, TokenSemicolon,
		TokenIdent, TokenStarEqual, TokenNumber, TokenSemicolon,
		TokenIdent, TokenSlashEqual, TokenNumber, TokenSemicolon,
		TokenIdent, TokenPercentEqual, TokenNumber, TokenNewline, TokenEOF,
	}
	got := make([]TokenType, len(tokens))
	for i, tok := range tokens {
		got[i] = tok.Type
	}
	if len(got) != len(expected) {
		t.Fatalf("unexpected token count: got=%d want=%d\ngot=%v", len(got), len(expected), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("token %d mismatch: got=%s want=%s", i, got[i], expected[i])
		}
	}
}

func TestLexCompoundAssignmentAdjacency(t *testing.T) {
	src := "x+ =1\n"
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	if len(tokens) < 4 {
		t.Fatalf("unexpected token count: %d", len(tokens))
	}
	if tokens[0].Type != TokenIdent || tokens[1].Type != TokenPlus || tokens[2].Type != TokenEqual {
		t.Fatalf("expected IDENT '+' '=' sequence, got: %#v %#v %#v", tokens[0], tokens[1], tokens[2])
	}
}

func TestLexSymbolTokens(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		tt    TokenType
		text  string
		value string
	}{
		{name: "comma", src: ",", tt: TokenComma, text: ",", value: ","},
		{name: "semicolon", src: ";", tt: TokenSemicolon, text: ";", value: ";"},
		{name: "dot", src: ".", tt: TokenDot, text: ".", value: "."},
		{name: "equal", src: "=", tt: TokenEqual, text: "=", value: "="},
		{name: "eqeq", src: "==", tt: TokenEqEq, text: "==", value: "=="},
		{name: "neq", src: "!=", tt: TokenNeq, text: "!=", value: "!="},
		{name: "bang", src: "!", tt: TokenBang, text: "!", value: "!"},
		{name: "amp", src: "&", tt: TokenAmp, text: "&", value: "&"},
		{name: "pipe", src: "|", tt: TokenPipe, text: "|", value: "|"},
		{name: "lt", src: "<", tt: TokenLT, text: "<", value: "<"},
		{name: "le", src: "<=", tt: TokenLE, text: "<=", value: "<="},
		{name: "gt", src: ">", tt: TokenGT, text: ">", value: ">"},
		{name: "ge", src: ">=", tt: TokenGE, text: ">=", value: ">="},
		{name: "plus", src: "+", tt: TokenPlus, text: "+", value: "+"},
		{name: "plus equal", src: "+=", tt: TokenPlusEqual, text: "+=", value: "+="},
		{name: "minus", src: "-", tt: TokenMinus, text: "-", value: "-"},
		{name: "minus equal", src: "-=", tt: TokenMinusEqual, text: "-=", value: "-="},
		{name: "star", src: "*", tt: TokenStar, text: "*", value: "*"},
		{name: "star equal", src: "*=", tt: TokenStarEqual, text: "*=", value: "*="},
		{name: "slash", src: "/", tt: TokenSlash, text: "/", value: "/"},
		{name: "slash equal", src: "/=", tt: TokenSlashEqual, text: "/=", value: "/="},
		{name: "percent", src: "%", tt: TokenPercent, text: "%", value: "%"},
		{name: "percent equal", src: "%=", tt: TokenPercentEqual, text: "%=", value: "%="},
		{name: "lparen", src: "(", tt: TokenLParen, text: "(", value: "("},
		{name: "rparen", src: ")", tt: TokenRParen, text: ")", value: ")"},
		{name: "lbracket", src: "[", tt: TokenLBracket, text: "[", value: "["},
		{name: "rbracket", src: "]", tt: TokenRBracket, text: "]", value: "]"},
		{name: "lbrace", src: "{", tt: TokenLBrace, text: "{", value: "{"},
		{name: "rbrace", src: "}", tt: TokenRBrace, text: "}", value: "}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tokens := Lex("in.jbs", tc.src+"\n", diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected lexer errors for %q: %s", tc.src, diags.String())
			}
			if len(tokens) < 3 {
				t.Fatalf("unexpected token count for %q: %d", tc.src, len(tokens))
			}
			if tokens[0].Type != tc.tt {
				t.Fatalf("expected token type %s for %q, got %s", tc.tt, tc.src, tokens[0].Type)
			}
			if tokens[0].Text != tc.text || tokens[0].Value != tc.value {
				t.Fatalf("unexpected token text/value for %q: %#v", tc.src, tokens[0])
			}
			if tokens[1].Type != TokenNewline || tokens[2].Type != TokenEOF {
				t.Fatalf("expected newline and eof after symbol %q, got %s %s", tc.src, tokens[1].Type, tokens[2].Type)
			}
		})
	}
}

func TestLexDoubleLogicalSymbols(t *testing.T) {
	tests := []struct {
		src  string
		tt   TokenType
		text string
	}{
		{src: "&&\n", tt: TokenAmp, text: "&&"},
		{src: "||\n", tt: TokenPipe, text: "||"},
	}
	for _, tc := range tests {
		diags := &diag.Diagnostics{}
		tokens := Lex("in.jbs", tc.src, diags)
		if diags.HasErrors() {
			t.Fatalf("unexpected lexer errors for %q: %s", tc.src, diags.String())
		}
		if len(tokens) < 3 {
			t.Fatalf("unexpected token count for %q: %d", tc.src, len(tokens))
		}
		if tokens[0].Type != tc.tt || tokens[0].Text != tc.text || tokens[0].Value != tc.text {
			t.Fatalf("unexpected token for %q: %#v", tc.src, tokens[0])
		}
		if tokens[0].Span.Start.Column != 1 || tokens[0].Span.End.Column != 3 {
			t.Fatalf("expected two-character span for %q, got %#v", tc.src, tokens[0].Span)
		}
		if tokens[1].Type != TokenNewline || tokens[2].Type != TokenEOF {
			t.Fatalf("expected newline and eof after %q, got %#v", tc.src, tokens)
		}
	}
}

func TestLexLogicalSymbolsInExpression(t *testing.T) {
	diags := &diag.Diagnostics{}
	tokens := Lex("in.jbs", "!a || b && c or d and e\n", diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lexer errors: %s", diags.String())
	}
	want := []TokenType{
		TokenBang, TokenIdent,
		TokenPipe, TokenIdent,
		TokenAmp, TokenIdent,
		TokenOr, TokenIdent,
		TokenAnd, TokenIdent,
	}
	for i := range want {
		if tokens[i].Type != want[i] {
			t.Fatalf("unexpected token sequence at %d: got=%v want=%v", i, tokens[i].Type, want[i])
		}
	}
}

func TestLexNumberTokenization(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantTypes []TokenType
		wantVals  []string
	}{
		{
			name:      "integer literal",
			src:       "123\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"123", "", ""},
		},
		{
			name:      "float literal",
			src:       "12.34\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"12.34", "", ""},
		},
		{
			name:      "scientific notation lower",
			src:       "1e3\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"1e3", "", ""},
		},
		{
			name:      "scientific notation upper",
			src:       "1E5\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"1E5", "", ""},
		},
		{
			name:      "scientific float with signed exponent",
			src:       "2.331e-5\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"2.331e-5", "", ""},
		},
		{
			name:      "scientific float with plus exponent",
			src:       "1.3151e+5\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"1.3151e+5", "", ""},
		},
		{
			name:      "scientific float with integer and decimal part",
			src:       "1411211.1e5\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"1411211.1e5", "", ""},
		},
		{
			name:      "leading dot scientific lower",
			src:       ".121e-1\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{".121e-1", "", ""},
		},
		{
			name:      "leading dot scientific upper",
			src:       ".1E-12\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{".1E-12", "", ""},
		},
		{
			name:      "valid exponent variants",
			src:       "0e0 0E0 10e+2 10E-2 .5e2 .5E+2\n",
			wantTypes: []TokenType{TokenNumber, TokenNumber, TokenNumber, TokenNumber, TokenNumber, TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"0e0", "0E0", "10e+2", "10E-2", ".5e2", ".5E+2", "", ""},
		},
		{
			name:      "dot without following digit splits token",
			src:       "12.\n",
			wantTypes: []TokenType{TokenNumber, TokenDot, TokenNewline, TokenEOF},
			wantVals:  []string{"12", ".", "", ""},
		},
		{
			name:      "multiple dots split into two numeric tokens",
			src:       "1.2.3\n",
			wantTypes: []TokenType{TokenNumber, TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"1.2", ".3", "", ""},
		},
		{
			name:      "number followed by identifier",
			src:       "12abc\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{"12", "abc", "", ""},
		},
		{
			name:      "number followed by underscore identifier",
			src:       "12_3\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{"12", "_3", "", ""},
		},
		{
			name:      "leading dot is part of number",
			src:       ".5\n",
			wantTypes: []TokenType{TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{".5", "", ""},
		},
		{
			name:      "malformed exponent no digits",
			src:       "1e\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{"1", "e", "", ""},
		},
		{
			name:      "malformed exponent with sign no digits",
			src:       "1e+\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenPlus, TokenNewline, TokenEOF},
			wantVals:  []string{"1", "e", "+", "", ""},
		},
		{
			name:      "malformed leading dot exponent with sign no digits",
			src:       ".1e-\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenMinus, TokenNewline, TokenEOF},
			wantVals:  []string{".1", "e", "-", "", ""},
		},
		{
			name:      "delimiter boundary with identifier",
			src:       "1e3abc\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{"1e3", "abc", "", ""},
		},
		{
			name:      "delimiter boundary with underscore identifier",
			src:       ".1E-12_x\n",
			wantTypes: []TokenType{TokenNumber, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{".1E-12", "_x", "", ""},
		},
		{
			name:      "unary negative leading dot tokenization",
			src:       "-.2\n",
			wantTypes: []TokenType{TokenMinus, TokenNumber, TokenNewline, TokenEOF},
			wantVals:  []string{"-", ".2", "", ""},
		},
		{
			name:      "qualified identifier remains dot-separated",
			src:       "ns.value\n",
			wantTypes: []TokenType{TokenIdent, TokenDot, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{"ns", ".", "value", "", ""},
		},
		{
			name:      "dot before identifier is not number",
			src:       ".abc\n",
			wantTypes: []TokenType{TokenDot, TokenIdent, TokenNewline, TokenEOF},
			wantVals:  []string{".", "abc", "", ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tokens := Lex("in.jbs", tc.src, diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected lexer errors for %q: %s", tc.src, diags.String())
			}
			if len(tokens) != len(tc.wantTypes) {
				t.Fatalf("unexpected token count for %q: got=%d want=%d", tc.src, len(tokens), len(tc.wantTypes))
			}
			for i := range tc.wantTypes {
				if tokens[i].Type != tc.wantTypes[i] {
					t.Fatalf("token[%d] type mismatch for %q: got=%s want=%s", i, tc.src, tokens[i].Type, tc.wantTypes[i])
				}
				if tokens[i].Value != tc.wantVals[i] {
					t.Fatalf("token[%d] value mismatch for %q: got=%q want=%q", i, tc.src, tokens[i].Value, tc.wantVals[i])
				}
			}
		})
	}
}
