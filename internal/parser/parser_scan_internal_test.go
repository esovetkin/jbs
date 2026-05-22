package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
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

	t.Run("not at block start", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("x", diags)
		_, innerStart, blockEnd, ok := p.readBalancedBlock()
		if ok {
			t.Fatalf("expected readBalancedBlock to fail without opening brace")
		}
		if innerStart.Offset != 0 || blockEnd.Offset != 0 {
			t.Fatalf("expected failure positions at start, got inner=%+v end=%+v", innerStart, blockEnd)
		}
		if !hasDiag(diags, "E025") {
			t.Fatalf("expected E025, got: %s", diags.String())
		}
	})

	t.Run("parameter expansion braces do not close block", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("{\n  echo \"${name:-'}'}\"\n  echo ${file#*.}\n}\n", diags)
		body, _, _, ok := p.readBalancedBlock()
		if !ok {
			t.Fatalf("expected balanced block with shell parameter expansion, got diagnostics: %s", diags.String())
		}
		if body != "\n  echo \"${name:-'}'}\"\n  echo ${file#*.}" {
			t.Fatalf("unexpected block body: %q", body)
		}
	})
}

func TestHereDocParsingHelpers(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		start     int
		wantOK    bool
		wantDelim string
		wantStrip bool
	}{
		{name: "plain delimiter", src: "<<EOF", wantOK: true, wantDelim: "EOF"},
		{name: "strip tabs", src: "<<-\tEOF", wantOK: true, wantDelim: "EOF", wantStrip: true},
		{name: "single quoted delimiter", src: "<<'END MARK'", wantOK: true, wantDelim: "END MARK"},
		{name: "double quoted delimiter with escape", src: `<<"E\"OF"`, wantOK: true, wantDelim: `E"OF`},
		{name: "backslash escaped delimiter char", src: `<<E\ OF`, wantOK: true, wantDelim: "E OF"},
		{name: "not redirect after less-than", src: "<<<EOF", start: 1},
		{name: "triple less-than rejected", src: "<<<EOF"},
		{name: "missing delimiter rejected", src: "<< "},
		{name: "unterminated single quote rejected", src: "<<'EOF"},
		{name: "unterminated double quote rejected", src: `<<"EOF`},
		{name: "dangling escape rejected", src: `<<EOF\`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := parseHereDocRedirect([]rune(tc.src), tc.start)
			if ok != tc.wantOK {
				t.Fatalf("parseHereDocRedirect() ok=%v want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if spec.Delimiter != tc.wantDelim || spec.StripTabs != tc.wantStrip {
				t.Fatalf("unexpected spec: got=%+v want delimiter=%q strip=%v", spec, tc.wantDelim, tc.wantStrip)
			}
		})
	}
}

func TestShellParameterExpansionScanner(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		start   int
		wantEnd int
		wantOK  bool
	}{
		{name: "not parameter expansion", src: "$name", wantOK: false},
		{name: "simple", src: "${name} tail", wantEnd: len("${name}"), wantOK: true},
		{name: "nested", src: "${name:-${fallback}} tail", wantEnd: len("${name:-${fallback}}"), wantOK: true},
		{name: "single quoted close brace", src: "${name:-'}'} tail", wantEnd: len("${name:-'}'}"), wantOK: true},
		{name: "double quoted close brace", src: `${name:-"}"} tail`, wantEnd: len(`${name:-"}"}`), wantOK: true},
		{name: "escaped close brace", src: `${name:-\}} tail`, wantEnd: len(`${name:-\}}`), wantOK: true},
		{name: "offset start", src: `echo ${name}`, start: len("echo "), wantEnd: len(`echo ${name}`), wantOK: true},
		{name: "unterminated", src: "${name", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			end, ok := scanShellParameterExpansion([]rune(tc.src), tc.start)
			if ok != tc.wantOK {
				t.Fatalf("scanShellParameterExpansion() ok=%v want %v", ok, tc.wantOK)
			}
			if end != tc.wantEnd {
				t.Fatalf("scanShellParameterExpansion() end=%d want %d", end, tc.wantEnd)
			}
		})
	}
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

func TestParserSeekTo(t *testing.T) {
	p := newTopLevelParser("a\nbc", &diag.Diagnostics{})
	for !p.eof() {
		p.advance()
	}
	p.seekTo(diag.NewPos(2, 2, 1))
	if p.off != 2 || p.line != 2 || p.col != 1 {
		t.Fatalf("seekTo earlier offset failed: off=%d line=%d col=%d", p.off, p.line, p.col)
	}
	p.seekTo(diag.NewPos(-1, 1, 1))
	if p.off != 2 || p.line != 2 || p.col != 1 {
		t.Fatalf("seekTo negative offset should not move parser: off=%d line=%d col=%d", p.off, p.line, p.col)
	}
	p.seekTo(diag.NewPos(4, 2, 3))
	if p.off != 4 || p.line != 2 || p.col != 3 {
		t.Fatalf("seekTo later offset failed: off=%d line=%d col=%d", p.off, p.line, p.col)
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

	t.Run("peekN beyond non-empty stream returns final token", func(t *testing.T) {
		eof := lexer.Token{Type: lexer.TokenEOF}
		tp := &tokenParser{
			tokens: []lexer.Token{
				{Type: lexer.TokenIdent, Value: "x"},
				eof,
			},
			diags: &diag.Diagnostics{},
		}
		if got := tp.peekN(10); got.Type != lexer.TokenEOF {
			t.Fatalf("expected final EOF token from peekN beyond stream, got %#v", got)
		}
	})
}
