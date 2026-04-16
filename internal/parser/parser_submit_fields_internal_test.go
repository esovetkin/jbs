package parser

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func countDiagCodeSubmit(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == code {
			count++
		}
	}
	return count
}

func newSubmitParser(src string, diags *diag.Diagnostics) *submitFieldParser {
	return &submitFieldParser{
		file:  "in.jbs",
		src:   []rune(src),
		base:  0,
		line:  1,
		col:   1,
		diags: diags,
	}
}

func TestParseSubmitExpr(t *testing.T) {
	start := diag.NewPos(0, 1, 1)

	{
		diags := &diag.Diagnostics{}
		expr := parseSubmitExpr("in.jbs", "   \n", start, diags)
		if expr != nil {
			t.Fatalf("expected nil expr for empty input, got %#v", expr)
		}
		if countDiagCodeSubmit(diags, "E077") == 0 {
			t.Fatalf("expected E077 for empty submit expression, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		expr := parseSubmitExpr("in.jbs", "1 2", start, diags)
		if expr == nil {
			t.Fatalf("expected non-nil expr for trailing-token case")
		}
		if countDiagCodeSubmit(diags, "E077") == 0 {
			t.Fatalf("expected E077 for trailing tokens, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		expr := parseSubmitExpr("in.jbs", `"abc"`, start, diags)
		if expr == nil {
			t.Fatalf("expected valid submit expression")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors for valid expr: %s", diags.String())
		}
	}
}

func TestSubmitFieldParserParseAssignOp(t *testing.T) {
	tests := []struct {
		src    string
		wantOp ast.AssignOp
		wantOK bool
	}{
		{src: "=", wantOp: ast.AssignEq, wantOK: true},
		{src: "+=", wantOp: ast.AssignPlusEq, wantOK: true},
		{src: "-=", wantOp: ast.AssignMinusEq, wantOK: true},
		{src: "*=", wantOp: ast.AssignStarEq, wantOK: true},
		{src: "/=", wantOp: ast.AssignSlashEq, wantOK: true},
		{src: "%=", wantOp: ast.AssignPctEq, wantOK: true},
		{src: "+x", wantOp: ast.AssignEq, wantOK: false},
		{src: ":", wantOp: ast.AssignEq, wantOK: false},
	}
	for _, tt := range tests {
		diags := &diag.Diagnostics{}
		p := newSubmitParser(tt.src, diags)
		op, _, ok := p.parseAssignOp()
		if op != tt.wantOp || ok != tt.wantOK {
			t.Fatalf("parseAssignOp(%q) -> (op=%q ok=%v), want (op=%q ok=%v)", tt.src, op, ok, tt.wantOp, tt.wantOK)
		}
	}
}

func TestSubmitFieldParserParseIdentAndPeek(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newSubmitParser("abc_1", diags)
	name, _, ok := p.parseIdent()
	if !ok || name != "abc_1" {
		t.Fatalf("expected valid identifier abc_1, got name=%q ok=%v", name, ok)
	}
	if p.peek() != 0 {
		t.Fatalf("expected EOF peek after consuming identifier")
	}
	if p.peekN(10) != 0 {
		t.Fatalf("expected 0 for out-of-range peekN")
	}

	p = newSubmitParser("1abc", diags)
	name, _, ok = p.parseIdent()
	if ok || name != "" {
		t.Fatalf("expected invalid leading-digit identifier to fail, got name=%q ok=%v", name, ok)
	}

	p = newSubmitParser("", diags)
	name, _, ok = p.parseIdent()
	if ok || name != "" {
		t.Fatalf("expected parseIdent to fail on EOF, got name=%q ok=%v", name, ok)
	}
}

func TestSubmitFieldParserTriviaAndRecover(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newSubmitParser(" \t;\n# c\nx=1", diags)
	p.skipTrivia()
	if p.peek() != 'x' {
		t.Fatalf("expected skipTrivia to land on 'x', got %q", p.peek())
	}

	p = newSubmitParser(" \t\r\nx", diags)
	p.skipInlineTrivia()
	if p.peek() != '\n' {
		t.Fatalf("skipInlineTrivia must stop at newline, got %q", p.peek())
	}

	p = newSubmitParser("abc #c\nz", diags)
	p.recoverLine()
	if p.peek() != 'z' {
		t.Fatalf("recoverLine should land on next line start, got %q", p.peek())
	}
}

func TestSubmitFieldParserScanExprUntilStmtEnd(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "semicolon in quotes does not terminate",
			src:  `"-lc 'echo a;b#c'" ; next`,
			want: `"-lc 'echo a;b#c'" `,
		},
		{
			name: "backslash newline continuation",
			src:  "\"a\" +\\\n\"b\"\nnext",
			want: "\"a\" +\\\n\"b\"",
		},
		{
			name: "comment starts termination in code mode",
			src:  `"x" # trailing`,
			want: `"x" `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newSubmitParser(tt.src, &diag.Diagnostics{})
			got := p.scanExprUntilStmtEnd()
			if got != tt.want {
				t.Fatalf("scanExprUntilStmtEnd(%q)=%q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestSubmitFieldParserScanExprUntilStmtEndEscapesAndQuoteModes(t *testing.T) {
	src := `'a\'b' + "c\"d" + x\y
next`
	p := newSubmitParser(src, &diag.Diagnostics{})
	got := p.scanExprUntilStmtEnd()
	want := `'a\'b' + "c\"d" + x\y`
	if got != want {
		t.Fatalf("scanExprUntilStmtEnd extra branch case got %q, want %q", got, want)
	}
}

func TestSubmitFieldParserHasUnexpectedTrailingTextAfterRawBlock(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: "", want: false},
		{src: "   ;rest", want: false},
		{src: "\nrest", want: false},
		{src: "# comment\nrest", want: false},
		{src: " trailing", want: true},
	}
	for _, tt := range tests {
		p := newSubmitParser(tt.src, &diag.Diagnostics{})
		if got := p.hasUnexpectedTrailingTextAfterRawBlock(); got != tt.want {
			t.Fatalf("hasUnexpectedTrailingTextAfterRawBlock(%q)=%v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestReadBalancedBlockSharedAndScanner(t *testing.T) {
	{
		src := []rune("x")
		off := 0
		line := 1
		col := 1
		peek := func() rune {
			if off >= len(src) {
				return 0
			}
			return src[off]
		}
		advance := func() rune {
			if off >= len(src) {
				return 0
			}
			r := src[off]
			off++
			if r == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			return r
		}
		eof := func() bool { return off >= len(src) }
		pos := func() diag.Position { return diag.NewPos(off, line, col) }
		getOff := func() int { return off }

		_, _, _, ok := readBalancedBlockShared(src, peek, advance, eof, pos, getOff)
		if ok {
			t.Fatalf("expected readBalancedBlockShared to fail when current rune is not '{'")
		}
	}

	{
		diags := &diag.Diagnostics{}
		p := newSubmitParser("{ echo ${x:-{a}} # c\n}", diags)
		raw, _, _, ok := p.readBalancedBlock()
		if !ok {
			t.Fatalf("expected balanced block parse to succeed, got: %s", diags.String())
		}
		if !strings.Contains(raw, "${x:-{a}}") {
			t.Fatalf("expected raw block content to include nested braces in expansion, got %q", raw)
		}
	}

	{
		diags := &diag.Diagnostics{}
		p := newSubmitParser("{ unterminated", diags)
		_, _, _, ok := p.readBalancedBlock()
		if ok {
			t.Fatalf("expected unterminated block parse to fail")
		}
		if countDiagCodeSubmit(diags, "E025") == 0 {
			t.Fatalf("expected E025 for unterminated block, got: %s", diags.String())
		}
	}
}

func TestScanBalancedBlockModesAndCommentBoundary(t *testing.T) {
	tests := []struct {
		name string
		src  string
		ok   bool
	}{
		{
			name: "single and double quote escapes",
			src:  "{ 'a\\'b' \"c\\\"d\" }",
			ok:   true,
		},
		{
			name: "line comment ignores braces until newline",
			src:  "{ x # comment with } { }\n }",
			ok:   true,
		},
		{
			name: "hash without boundary is code not comment",
			src:  "{ x#still_code }",
			ok:   true,
		},
		{
			name: "unterminated block fails",
			src:  "{ x { y ",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.src)
			off := 0
			peek := func() rune {
				if off >= len(runes) {
					return 0
				}
				return runes[off]
			}
			advance := func() rune {
				if off >= len(runes) {
					return 0
				}
				r := runes[off]
				off++
				return r
			}
			eof := func() bool { return off >= len(runes) }

			if peek() != '{' {
				t.Fatalf("test input must start with '{'")
			}
			advance()
			got := scanBalancedBlock(advance, eof)
			if got != tt.ok {
				t.Fatalf("scanBalancedBlock(%q)=%v, want %v", tt.src, got, tt.ok)
			}
		})
	}
}

func TestIsBalancedBlockCommentBoundary(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{r: ' ', want: true},
		{r: '\t', want: true},
		{r: ';', want: true},
		{r: '|', want: true},
		{r: '&', want: true},
		{r: '(', want: true},
		{r: ')', want: true},
		{r: 'a', want: false},
		{r: '.', want: false},
	}
	for _, tt := range tests {
		if got := isBalancedBlockCommentBoundary(tt.r); got != tt.want {
			t.Fatalf("isBalancedBlockCommentBoundary(%q)=%v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestParseSubmitFieldsTrailingTextAfterRawBlock(t *testing.T) {
	body := `
preprocess = {
  echo hi
} trailing
args_exec = "-lc hostname"
`
	diags := &diag.Diagnostics{}
	fields := parseSubmitFields("in.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if countDiagCodeSubmit(diags, "E077") == 0 {
		t.Fatalf("expected E077 for trailing text after raw block, got: %s", diags.String())
	}
}

func TestParseSubmitFieldsMalformedAndRawOperatorBranches(t *testing.T) {
	body := `
123
key 1
preprocess += {
  echo hi
}
args_exec = "-lc hostname"
`
	diags := &diag.Diagnostics{}
	fields := parseSubmitFields("in.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(fields) != 1 {
		t.Fatalf("expected only args_exec field to survive malformed statements, got %#v", fields)
	}
	if fields[0].Name != "args_exec" || fields[0].Expr == nil {
		t.Fatalf("unexpected surviving field parse result: %#v", fields[0])
	}
	if countDiagCodeSubmit(diags, "E077") < 3 {
		t.Fatalf("expected at least three E077 diagnostics for malformed statements, got: %s", diags.String())
	}
}

func TestSubmitFieldParserAdvanceAndAssignOpEOF(t *testing.T) {
	p := newSubmitParser("", &diag.Diagnostics{})
	if got := p.advance(); got != 0 {
		t.Fatalf("expected advance at EOF to return 0, got %q", got)
	}
	op, _, ok := p.parseAssignOp()
	if ok || op != ast.AssignEq {
		t.Fatalf("expected parseAssignOp on EOF to fail with default op, got op=%q ok=%v", op, ok)
	}
}
