package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestReadHeaderIntegerValue(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantValue string
		wantOK    bool
		wantPeek  rune
	}{
		{name: "whitespace eof", src: "   ", wantValue: "", wantOK: false, wantPeek: 0},
		{name: "sign without digits", src: "+", wantValue: "", wantOK: false, wantPeek: 0},
		{name: "valid integer eof", src: "42", wantValue: "42", wantOK: true, wantPeek: 0},
		{name: "valid signed with comma delimiter", src: "-12,rest", wantValue: "-12", wantOK: true, wantPeek: ','},
		{name: "valid with brace delimiter", src: "3{", wantValue: "3", wantOK: true, wantPeek: '{'},
		{name: "invalid suffix consumed until delimiter", src: "12abc,rest", wantValue: "12abc", wantOK: false, wantPeek: ','},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTopLevelParser(tc.src, &diag.Diagnostics{})
			got, _, ok := p.readHeaderIntegerValue()
			if got != tc.wantValue || ok != tc.wantOK {
				t.Fatalf("readHeaderIntegerValue(%q)=(%q,%v), want (%q,%v)", tc.src, got, ok, tc.wantValue, tc.wantOK)
			}
			if p.peek() != tc.wantPeek {
				t.Fatalf("readHeaderIntegerValue(%q) left peek=%q, want %q", tc.src, p.peek(), tc.wantPeek)
			}
		})
	}
}

func TestParseOptionalDoHeaderClauses(t *testing.T) {
	t.Run("do header parses clauses and nproc then stops at unknown", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser(`after prep with p["x"] fsub "a.tpl" { "A": x } nproc 3 unknown`, diags)
		after, withItems, opts := p.parseOptionalDoHeaderClauses()
		if len(after) != 1 || after[0] != "prep" {
			t.Fatalf("unexpected after: %#v", after)
		}
		if len(withItems) != 1 {
			t.Fatalf("unexpected with items: %#v", withItems)
		}
		assertWithIndexStringColumns(t, withItems[0], "p", []string{"x"})
		if opts.NProc == nil || *opts.NProc != 3 {
			t.Fatalf("unexpected parsed nproc: %#v", opts)
		}
		if len(opts.FSubs) != 1 || opts.FSubs[0].Path != "a.tpl" || len(opts.FSubs[0].Rules) != 1 {
			t.Fatalf("unexpected fsub clauses: %#v", opts.FSubs)
		}
		word, ok := p.peekWord()
		if !ok || word != "unknown" {
			t.Fatalf("expected parser to stop before unknown token, got word=%q ok=%v", word, ok)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

}

func TestParseFileSubstitutionErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "missing path", src: `fsub { "X": x }`},
		{name: "non string path", src: `fsub 1 { "X": x }`},
		{name: "non dict body", src: `fsub "x" [1]`},
		{name: "non string pattern", src: `fsub "x" { 1: x }`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			p := newTopLevelParser(tc.src, diags)
			start := p.pos()
			p.consumeWord()
			_ = p.parseFileSubstitution(start)
			if !hasDiag(diags, "E035") {
				t.Fatalf("expected E035 for %s, got: %s", tc.name, diags.String())
			}
		})
	}
}

func TestParseFileSubstitutionUnterminatedMap(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`fsub "x.tpl" { "A": x`, diags)
	start := p.pos()
	p.consumeWord()
	fsub := p.parseFileSubstitution(start)
	if fsub.Path != "x.tpl" {
		t.Fatalf("unexpected fsub path: %#v", fsub)
	}
	if len(fsub.Rules) != 0 {
		t.Fatalf("unterminated fsub should not produce rules, got %#v", fsub.Rules)
	}
	if !hasDiag(diags, "E025") {
		t.Fatalf("expected E025 for unterminated fsub map, got: %s", diags.String())
	}
}

func TestParseFSubPathEdges(t *testing.T) {
	t.Run("missing path at eof", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser(`fsub `, diags)
		p.consumeWord()
		path, _ := p.parseFSubPath()
		if path != "" {
			t.Fatalf("expected empty path, got %q", path)
		}
		if !hasDiag(diags, "E035") {
			t.Fatalf("expected E035 for missing path, got: %s", diags.String())
		}
	})

	t.Run("extra path expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser(`fsub "a.tpl" "b.tpl" { "A": x }`, diags)
		p.consumeWord()
		path, _ := p.parseFSubPath()
		if path != "a.tpl" {
			t.Fatalf("unexpected path: %q", path)
		}
		if !hasDiag(diags, "E035") {
			t.Fatalf("expected E035 for extra fsub path tokens, got: %s", diags.String())
		}
	})
}

func TestAdvanceUntilFSubMapOpenQuotesAndEscapes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		ok   bool
	}{
		{name: "double quoted brace escaped quote", src: `"a\"{b" { "A": x }`, ok: true},
		{name: "single quoted brace escaped quote", src: `'a\'{b' { "A": x }`, ok: true},
		{name: "no map open", src: `"unterminated {`, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTopLevelParser(tc.src, &diag.Diagnostics{})
			if got := p.advanceUntilFSubMapOpen(); got != tc.ok {
				t.Fatalf("advanceUntilFSubMapOpen(%q)=%v, want %v", tc.src, got, tc.ok)
			}
		})
	}
}

func TestParseFSubRulesErrorEdges(t *testing.T) {
	span := diag.NewPos(0, 1, 1)

	t.Run("extra syntax after map", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rules := parseFSubRules("fsub.jbs", `{ "A": x } trailing`, span, diags)
		if len(rules) != 1 || rules[0].Pattern != "A" {
			t.Fatalf("unexpected parsed rules: %#v", rules)
		}
		if !hasDiag(diags, "E035") {
			t.Fatalf("expected E035 for trailing fsub syntax, got: %s", diags.String())
		}
	})

	t.Run("empty expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		rules := parseFSubRules("fsub.jbs", ``, span, diags)
		if len(rules) != 0 {
			t.Fatalf("expected no rules for empty expression, got %#v", rules)
		}
		if !hasDiag(diags, "E035") {
			t.Fatalf("expected E035 for empty fsub rules, got: %s", diags.String())
		}
	})

}

func TestParseOptionalAfterAndWith(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`after a,b with p["x"] with q["y"] tail`, diags)
	after, withItems := p.parseOptionalAfterAndWith()
	if len(after) != 2 || after[0] != "a" || after[1] != "b" {
		t.Fatalf("unexpected after list: %#v", after)
	}
	if len(withItems) != 2 {
		t.Fatalf("unexpected with items length: %#v", withItems)
	}
	assertWithIndexStringColumns(t, withItems[0], "p", []string{"x"})
	assertWithIndexStringColumns(t, withItems[1], "q", []string{"y"})
	word, ok := p.peekWord()
	if !ok || word != "tail" {
		t.Fatalf("expected parser to stop before non-clause token, got word=%q ok=%v", word, ok)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestParseOptionalHeaderClausesStopAtInheritWord(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`after prep inherit base with cases["id"] tail`, diags)
	after, withItems, _ := p.parseOptionalDoHeaderClauses()
	if len(after) != 1 || after[0] != "prep" {
		t.Fatalf("unexpected after clauses: %#v", after)
	}
	if len(withItems) != 0 {
		t.Fatalf("unexpected with items: %#v", withItems)
	}
	word, ok := p.peekWord()
	if !ok || word != "inherit" {
		t.Fatalf("expected parser to stop before inherit word, got word=%q ok=%v", word, ok)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}
