package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestParseHeaderElementsKindsAndComments(t *testing.T) {
	raw := "\n" +
		"  # lead comment\n" +
		"with p[x], q # inline note\n" +
		"\n" +
		"max_async = 2\n" +
		"after prep\n" +
		"use defaults\n" +
		"unknown header line\n" +
		"\n"
	elems := parseHeaderElements("test.jbs", raw, diag.NewPos(0, 1, 1))
	if len(elems) != 7 {
		t.Fatalf("expected 7 header elements, got %d", len(elems))
	}

	if elems[0].Kind != ast.HeaderElemComment || elems[0].Comment == nil || elems[0].Comment.Text != "lead comment" {
		t.Fatalf("unexpected comment element: %#v", elems[0])
	}
	if elems[1].Kind != ast.HeaderElemWith || elems[1].Text != "with p[x], q" {
		t.Fatalf("unexpected with element: %#v", elems[1])
	}
	if elems[1].Inline == nil || elems[1].Inline.Text != "inline note" {
		t.Fatalf("expected inline comment on with element, got %#v", elems[1].Inline)
	}
	if elems[2].Kind != ast.HeaderElemBlank {
		t.Fatalf("expected internal blank line to be preserved, got %#v", elems[2])
	}
	if elems[3].Kind != ast.HeaderElemOption || elems[3].Text != "max_async = 2" {
		t.Fatalf("unexpected option element: %#v", elems[3])
	}
	if elems[4].Kind != ast.HeaderElemAfter {
		t.Fatalf("expected after element, got %#v", elems[4])
	}
	if elems[5].Kind != ast.HeaderElemUse {
		t.Fatalf("expected use element, got %#v", elems[5])
	}
	if elems[6].Kind != ast.HeaderElemUnknown || elems[6].Text != "unknown header line" {
		t.Fatalf("unexpected unknown element: %#v", elems[6])
	}
}

func TestParseHeaderElementsEmptyAndBlankOnly(t *testing.T) {
	start := diag.NewPos(3, 2, 4)
	if got := parseHeaderElements("test.jbs", "", start); got != nil {
		t.Fatalf("expected nil for empty header, got %#v", got)
	}

	if got := parseHeaderElements("test.jbs", " \n\t\n", start); got != nil {
		t.Fatalf("expected nil for blank-only header, got %#v", got)
	}
}

func TestParseHeaderElementsCRLFAndSpan(t *testing.T) {
	raw := "with p\r\nuse q\r\n"
	start := diag.NewPos(10, 4, 3)
	elems := parseHeaderElements("test.jbs", raw, start)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}

	if elems[0].Span.Start != start {
		t.Fatalf("unexpected first start span: got=%+v want=%+v", elems[0].Span.Start, start)
	}
	wantSecondStart := diag.NewPos(17, 5, 1)
	if elems[1].Span.Start != wantSecondStart {
		t.Fatalf("unexpected second start span: got=%+v want=%+v", elems[1].Span.Start, wantSecondStart)
	}
}

func TestAdvancePosition(t *testing.T) {
	start := diag.NewPos(5, 3, 7)
	got := advancePosition(start, "ab")
	want := diag.NewPos(7, 3, 9)
	if got != want {
		t.Fatalf("unexpected position for plain text: got=%+v want=%+v", got, want)
	}

	got = advancePosition(start, "a\nb")
	want = diag.NewPos(8, 4, 2)
	if got != want {
		t.Fatalf("unexpected position for text with newline: got=%+v want=%+v", got, want)
	}
}

func TestTrimHeaderBlankEdges(t *testing.T) {
	in := []ast.HeaderElem{
		{Kind: ast.HeaderElemBlank},
		{Kind: ast.HeaderElemWith, Text: "with p"},
		{Kind: ast.HeaderElemBlank},
		{Kind: ast.HeaderElemAfter, Text: "after s0"},
		{Kind: ast.HeaderElemBlank},
	}
	got := trimHeaderBlankEdges(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 elements after edge trim, got %d", len(got))
	}
	if got[0].Kind != ast.HeaderElemWith || got[1].Kind != ast.HeaderElemBlank || got[2].Kind != ast.HeaderElemAfter {
		t.Fatalf("unexpected trimmed sequence: %#v", got)
	}

	onlyBlank := []ast.HeaderElem{{Kind: ast.HeaderElemBlank}, {Kind: ast.HeaderElemBlank}}
	if got := trimHeaderBlankEdges(onlyBlank); got != nil {
		t.Fatalf("expected nil for all-blank input, got %#v", got)
	}
}

func TestSplitLineComment(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantCode   string
		wantCmt    string
		wantHasCmt bool
	}{
		{
			name:       "plain comment",
			line:       "with p # c1",
			wantCode:   "with p ",
			wantCmt:    " c1",
			wantHasCmt: true,
		},
		{
			name:       "hash in double quotes",
			line:       `args = "a#b" # c2`,
			wantCode:   `args = "a#b" `,
			wantCmt:    " c2",
			wantHasCmt: true,
		},
		{
			name:       "hash in single quotes",
			line:       "args = 'a#b'",
			wantCode:   "args = 'a#b'",
			wantCmt:    "",
			wantHasCmt: false,
		},
		{
			name:       "escaped quote in double quotes",
			line:       `args = "a\"#b" # c3`,
			wantCode:   `args = "a\"#b" `,
			wantCmt:    " c3",
			wantHasCmt: true,
		},
		{
			name:       "escaped quote in single quotes",
			line:       "args = 'a\\'b#c'",
			wantCode:   "args = 'a\\'b#c'",
			wantCmt:    "",
			wantHasCmt: false,
		},
		{
			name:       "unterminated double quote keeps hash in code",
			line:       `args = "abc # no comment`,
			wantCode:   `args = "abc # no comment`,
			wantCmt:    "",
			wantHasCmt: false,
		},
		{
			name:       "unterminated single quote keeps hash in code",
			line:       "args = 'abc # no comment",
			wantCode:   "args = 'abc # no comment",
			wantCmt:    "",
			wantHasCmt: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCode, gotCmt, gotHasCmt := splitLineComment(tt.line)
			if gotCode != tt.wantCode || gotCmt != tt.wantCmt || gotHasCmt != tt.wantHasCmt {
				t.Fatalf("splitLineComment(%q)=(%q,%q,%v), want (%q,%q,%v)",
					tt.line, gotCode, gotCmt, gotHasCmt, tt.wantCode, tt.wantCmt, tt.wantHasCmt)
			}
		})
	}
}

func TestClassifyHeaderElemKind(t *testing.T) {
	tests := []struct {
		code string
		want ast.HeaderElemKind
	}{
		{code: "after a", want: ast.HeaderElemAfter},
		{code: "use p", want: ast.HeaderElemUse},
		{code: "with p", want: ast.HeaderElemWith},
		{code: "afterx p", want: ast.HeaderElemUnknown},
		{code: "usex p", want: ast.HeaderElemUnknown},
		{code: "procs = 4", want: ast.HeaderElemOption},
		{code: "max_async=1", want: ast.HeaderElemOption},
		{code: "iterations = 3", want: ast.HeaderElemOption},
		{code: "withx p", want: ast.HeaderElemUnknown},
		{code: "iterattions = 1", want: ast.HeaderElemUnknown},
		{code: "max_async 1", want: ast.HeaderElemUnknown},
	}
	for _, tt := range tests {
		if got := classifyHeaderElemKind(tt.code); got != tt.want {
			t.Fatalf("classifyHeaderElemKind(%q)=%q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestHasKeywordPrefix(t *testing.T) {
	tests := []struct {
		text    string
		keyword string
		want    bool
	}{
		{text: "with", keyword: "with", want: true},
		{text: "with p", keyword: "with", want: true},
		{text: "with\tp", keyword: "with", want: true},
		{text: "with\u00a0p", keyword: "with", want: false},
		{text: "withx p", keyword: "with", want: false},
		{text: "w", keyword: "with", want: false},
	}
	for _, tt := range tests {
		if got := hasKeywordPrefix(tt.text, tt.keyword); got != tt.want {
			t.Fatalf("hasKeywordPrefix(%q,%q)=%v, want %v", tt.text, tt.keyword, got, tt.want)
		}
	}
}

func TestIsStepOptionLine(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "max_async=1", want: true},
		{text: "max_async = 1", want: true},
		{text: "max_async\t=\t1", want: true},
		{text: "procs = 2", want: true},
		{text: "iterations=3", want: true},
		{text: "iterattions=3", want: false},
		{text: "max_async 1", want: false},
		{text: "_max_async=1", want: false},
		{text: "=1", want: false},
	}
	for _, tt := range tests {
		if got := isStepOptionLine(tt.text); got != tt.want {
			t.Fatalf("isStepOptionLine(%q)=%v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestLeadingIdent(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "max_async = 1", want: "max_async"},
		{text: "name42=7", want: "name42"},
		{text: "_x=1", want: "_x"},
		{text: "a.b=1", want: "a"},
		{text: "=1", want: ""},
		{text: " x=1", want: ""},
	}
	for _, tt := range tests {
		if got := leadingIdent(tt.text); got != tt.want {
			t.Fatalf("leadingIdent(%q)=%q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestParseHeaderElementsCommentOnlySpanAndTrim(t *testing.T) {
	raw := "\n  # only\n"
	start := diag.NewPos(40, 7, 5)
	elems := parseHeaderElements("head.jbs", raw, start)
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if elems[0].Kind != ast.HeaderElemComment || elems[0].Comment == nil || elems[0].Comment.Text != "only" {
		t.Fatalf("unexpected comment element: %#v", elems[0])
	}

	wantStart := diag.NewPos(41, 8, 1)
	if elems[0].Span.Start != wantStart {
		t.Fatalf("unexpected comment span start: got=%+v want=%+v", elems[0].Span.Start, wantStart)
	}
	wantEnd := diag.NewPos(49, 8, 9)
	if elems[0].Span.End != wantEnd {
		t.Fatalf("unexpected comment span end: got=%+v want=%+v", elems[0].Span.End, wantEnd)
	}
}
