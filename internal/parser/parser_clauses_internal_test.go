package parser

import (
	"testing"

	"jbs/internal/diag"
)

func TestLooksLikeStepHeaderAssignment(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "plain assignment", src: "max_async=1", want: true},
		{name: "assignment with spaces", src: "procs   =2", want: true},
		{name: "assignment with inline comment before equals", src: "iterations # keep\n=3", want: true},
		{name: "unknown key but assignment shape", src: "foo=1", want: true},
		{name: "identifier without assignment", src: "with p", want: false},
		{name: "identifier then eof", src: "max_async", want: false},
		{name: "comment to eof after identifier", src: "max_async # trailing", want: false},
		{name: "starts with non-identifier", src: "1x=1", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTopLevelParser(tc.src, &diag.Diagnostics{})
			if got := p.looksLikeStepHeaderAssignment(); got != tc.want {
				t.Fatalf("looksLikeStepHeaderAssignment(%q)=%v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestReadStepHeaderOptionValue(t *testing.T) {
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
			got, _, ok := p.readStepHeaderOptionValue()
			if got != tc.wantValue || ok != tc.wantOK {
				t.Fatalf("readStepHeaderOptionValue(%q)=(%q,%v), want (%q,%v)", tc.src, got, ok, tc.wantValue, tc.wantOK)
			}
			if p.peek() != tc.wantPeek {
				t.Fatalf("readStepHeaderOptionValue(%q) left peek=%q, want %q", tc.src, p.peek(), tc.wantPeek)
			}
		})
	}
}

func TestParseStepHeaderOption(t *testing.T) {
	t.Run("returns false when token is not an option", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("with p", diags)
		opts := stepHeaderOptions{}
		if ok := p.parseStepHeaderOption("do", &opts); ok {
			t.Fatalf("expected parseStepHeaderOption to return false")
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect diagnostics, got: %s", diags.String())
		}
	})

	t.Run("missing equals emits E035", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("procs 1", diags)
		opts := stepHeaderOptions{}
		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected parseStepHeaderOption to consume recognized key")
		}
		if !hasDiag(diags, "E035") {
			t.Fatalf("expected E035, got: %s", diags.String())
		}
	})

	t.Run("unknown assignment key emits E032", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("unknown_key=1", diags)
		opts := stepHeaderOptions{}
		if ok := p.parseStepHeaderOption("submit", &opts); !ok {
			t.Fatalf("expected parseStepHeaderOption to parse assignment-shaped token")
		}
		if !hasDiag(diags, "E032") {
			t.Fatalf("expected E032, got: %s", diags.String())
		}
	})

	t.Run("invalid value emits E034", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("max_async=+", diags)
		opts := stepHeaderOptions{}
		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected parseStepHeaderOption to consume option")
		}
		if !hasDiag(diags, "E034") {
			t.Fatalf("expected E034, got: %s", diags.String())
		}
	})

	t.Run("overflow value emits E034", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("iterations=999999999999999999999999", diags)
		opts := stepHeaderOptions{}
		if ok := p.parseStepHeaderOption("submit", &opts); !ok {
			t.Fatalf("expected parseStepHeaderOption to consume option")
		}
		if !hasDiag(diags, "E034") {
			t.Fatalf("expected E034 from atoi overflow, got: %s", diags.String())
		}
	})

	t.Run("duplicate key emits E033", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("max_async=1 max_async=2", diags)
		opts := stepHeaderOptions{}
		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected first option parse to succeed")
		}
		p.skipTriviaInline()
		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected second option parse to be consumed")
		}
		if !hasDiag(diags, "E033") {
			t.Fatalf("expected E033 for duplicate option, got: %s", diags.String())
		}
	})

	t.Run("valid keys set options", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("max_async=1 procs=2 iterations=3", diags)
		opts := stepHeaderOptions{}

		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected max_async to parse")
		}
		p.skipTriviaInline()
		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected procs to parse")
		}
		p.skipTriviaInline()
		if ok := p.parseStepHeaderOption("do", &opts); !ok {
			t.Fatalf("expected iterations to parse")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics for valid options: %s", diags.String())
		}
		if opts.MaxAsync == nil || *opts.MaxAsync != 1 {
			t.Fatalf("unexpected MaxAsync: %#v", opts.MaxAsync)
		}
		if opts.Procs == nil || *opts.Procs != 2 {
			t.Fatalf("unexpected Procs: %#v", opts.Procs)
		}
		if opts.Iterations == nil || *opts.Iterations != 3 {
			t.Fatalf("unexpected Iterations: %#v", opts.Iterations)
		}
	})
}

func TestParseOptionalDoAndSubmitHeaderClauses(t *testing.T) {
	t.Run("do header parses clauses and options then stops at unknown", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("after prep with p[x] max_async=3 procs=4 iterations=5 unknown", diags)
		after, withItems, opts := p.parseOptionalDoHeaderClauses()
		if len(after) != 1 || after[0] != "prep" {
			t.Fatalf("unexpected after: %#v", after)
		}
		if len(withItems) != 1 || withItems[0].Source != "p" || len(withItems[0].Selectors) != 1 || withItems[0].Selectors[0] != "x" {
			t.Fatalf("unexpected with items: %#v", withItems)
		}
		if opts.MaxAsync == nil || *opts.MaxAsync != 3 || opts.Procs == nil || *opts.Procs != 4 || opts.Iterations == nil || *opts.Iterations != 5 {
			t.Fatalf("unexpected parsed options: %#v", opts)
		}
		word, ok := p.peekWord()
		if !ok || word != "unknown" {
			t.Fatalf("expected parser to stop before unknown token, got word=%q ok=%v", word, ok)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("submit header parses after/use/with/options", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("after prep0,prep1 use defaults,gpu with p[x] max_async=1 procs=2 iterations=3 tail", diags)
		after, useNames, withItems, opts := p.parseOptionalSubmitHeaderClauses()
		if len(after) != 2 || after[0] != "prep0" || after[1] != "prep1" {
			t.Fatalf("unexpected after: %#v", after)
		}
		if len(useNames) != 2 || useNames[0] != "defaults" || useNames[1] != "gpu" {
			t.Fatalf("unexpected use names: %#v", useNames)
		}
		if len(withItems) != 1 || withItems[0].Source != "p" || len(withItems[0].Selectors) != 1 || withItems[0].Selectors[0] != "x" {
			t.Fatalf("unexpected with items: %#v", withItems)
		}
		if opts.MaxAsync == nil || *opts.MaxAsync != 1 || opts.Procs == nil || *opts.Procs != 2 || opts.Iterations == nil || *opts.Iterations != 3 {
			t.Fatalf("unexpected parsed options: %#v", opts)
		}
		word, ok := p.peekWord()
		if !ok || word != "tail" {
			t.Fatalf("expected parser to stop before non-clause token, got word=%q ok=%v", word, ok)
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

func TestParseOptionalHeaderClausesRejectInherit(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("after prep inherit base with cases[id] tail", diags)
	after, withItems, _ := p.parseOptionalDoHeaderClauses()
	if len(after) != 1 || after[0] != "prep" {
		t.Fatalf("unexpected after clauses: %#v", after)
	}
	if len(withItems) != 1 || withItems[0].Source != "cases" || withItems[0].Selectors[0] != "id" {
		t.Fatalf("unexpected with items: %#v", withItems)
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected inherit rejection diagnostic, got: %s", diags.String())
	}
}
