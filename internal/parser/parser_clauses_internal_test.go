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
		p := newTopLevelParser("after prep with p[x] nproc 3 unknown", diags)
		after, withItems, opts := p.parseOptionalDoHeaderClauses()
		if len(after) != 1 || after[0] != "prep" {
			t.Fatalf("unexpected after: %#v", after)
		}
		if len(withItems) != 1 || withItems[0].Source != "p" || len(withItems[0].Selectors) != 1 || withItems[0].Selectors[0] != "x" {
			t.Fatalf("unexpected with items: %#v", withItems)
		}
		if opts.NProc == nil || *opts.NProc != 3 {
			t.Fatalf("unexpected parsed nproc: %#v", opts)
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

func TestParseOptionalAfterAndWith(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("after a,b with p[x] with q[y] tail", diags)
	after, withItems := p.parseOptionalAfterAndWith()
	if len(after) != 2 || after[0] != "a" || after[1] != "b" {
		t.Fatalf("unexpected after list: %#v", after)
	}
	if len(withItems) != 2 {
		t.Fatalf("unexpected with items length: %#v", withItems)
	}
	if withItems[0].Source != "p" || withItems[0].Selectors[0] != "x" || withItems[1].Source != "q" || withItems[1].Selectors[0] != "y" {
		t.Fatalf("unexpected with items: %#v", withItems)
	}
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
	p := newTopLevelParser("after prep inherit base with cases[id] tail", diags)
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
