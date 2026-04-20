package format

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestClampRangeEdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		start     int
		end       int
		size      int
		wantStart int
		wantEnd   int
	}{
		{name: "negative size", start: -5, end: 2, size: -1, wantStart: 0, wantEnd: 0},
		{name: "negative start", start: -3, end: 2, size: 10, wantStart: 0, wantEnd: 2},
		{name: "end before start", start: 4, end: 1, size: 10, wantStart: 4, wantEnd: 4},
		{name: "start beyond size", start: 20, end: 30, size: 5, wantStart: 5, wantEnd: 5},
		{name: "end beyond size", start: 2, end: 20, size: 5, wantStart: 2, wantEnd: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStart, gotEnd := clampRange(tc.start, tc.end, tc.size)
			if gotStart != tc.wantStart || gotEnd != tc.wantEnd {
				t.Fatalf("clampRange(%d,%d,%d)=(%d,%d), want (%d,%d)", tc.start, tc.end, tc.size, gotStart, gotEnd, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestWhitespaceAndLineStartHelpers(t *testing.T) {
	if !isWhitespaceOrSemicolon(" \t;;\t") {
		t.Fatalf("expected whitespace+semicolon string to pass")
	}
	if isWhitespaceOrSemicolon(" ;x") {
		t.Fatalf("expected non-semicolon token to fail")
	}
	src := []rune("a\nb")
	if !isLineStartOffset(src, 0) {
		t.Fatalf("offset 0 should be line start")
	}
	if isLineStartOffset(src, 1) {
		t.Fatalf("offset 1 should not be line start")
	}
	if !isLineStartOffset(src, 2) {
		t.Fatalf("offset after newline should be line start")
	}
	if isLineStartOffset(src, 100) {
		t.Fatalf("offset clamped to eof where previous rune is not newline should not be line start")
	}
}

func TestIsGlobalAndFormatStmtFallback(t *testing.T) {
	if isGlobal(nil) {
		t.Fatalf("nil statement should not be global")
	}
	if !isGlobal(ast.GlobalAssign{Name: "jbs_name"}) {
		t.Fatalf("global assign should be recognized as global")
	}
	if got := formatStmt(nil, nil); got != nil {
		t.Fatalf("unknown stmt should format to nil, got %#v", got)
	}
}

func TestFormatGlobalAssignDefaults(t *testing.T) {
	got := formatGlobalAssign(ast.GlobalAssign{Name: "jbs_comment"}, nil)
	want := []string{`jbs_comment = ""`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatGlobalAssign mismatch: got=%v want=%v", got, want)
	}
}

func TestFormatExprStmt(t *testing.T) {
	src := []rune("  x + 1  \n")
	stmt := ast.ExprStmt{
		Span: diag.Span{
			Start: diag.Position{Offset: 0},
			End:   diag.Position{Offset: 9},
		},
	}
	got := formatExprStmt(stmt, src)
	want := []string{"x + 1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatExprStmt mismatch: got=%v want=%v", got, want)
	}
}

func TestFormatFunctionExprHelpers(t *testing.T) {
	src := []rune("function(x){ return x }\n")
	fn := ast.FunctionExpr{
		Params: []ast.FuncParam{{Name: "x"}},
		Body: []ast.FuncBodyStmt{
			ast.ReturnStmt{
				Expr: ast.IdentExpr{Name: "x"},
			},
		},
	}
	got := formatExprLines(fn, src)
	want := []string{
		"function(x) {",
		"    return x",
		"}",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatFunctionExprLines mismatch: got=%v want=%v", got, want)
	}

	call := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "f"},
		Args: []ast.CallArg{
			ast.PosCallArg(ast.NumberExpr{Raw: "1", Int: true, IntValue: 1}),
			{Name: "b", Expr: ast.NumberExpr{Raw: "2", Int: true, IntValue: 2}},
		},
	}
	if got := flattenFormattedLines(formatExprLines(call, src)); got != "f(1, b = 2)" {
		t.Fatalf("unexpected call formatting: %q", got)
	}
}

func TestFormatSubmitBlockRendersFieldFallback(t *testing.T) {
	submit := ast.SubmitBlock{
		Name: "run",
		Fields: []ast.SubmitField{
			{Name: "queue"},
			{Name: "preprocess", IsRaw: true, Raw: "echo pre"},
		},
	}
	got := formatSubmitBlock(submit, nil)
	want := []string{
		"submit run",
		"{",
		`        queue = ""`,
		`        preprocess = {`,
		`                echo pre`,
		`        }`,
		"}",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatSubmitBlock mismatch\n--- got ---\n%v\n--- want ---\n%v", got, want)
	}
}

func TestFormatUseStmtVariants(t *testing.T) {
	cases := []struct {
		name string
		in   ast.UseStmt
		want []string
	}{
		{
			name: "bare module",
			in: ast.UseStmt{
				Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "lib"},
			},
			want: []string{"use lib"},
		},
		{
			name: "path module default alias",
			in: ast.UseStmt{
				Source: ast.UseSource{Kind: ast.UseSourcePath, Value: "./lib.jbs"},
			},
			want: []string{`use "./lib.jbs" as module`},
		},
		{
			name: "path module explicit alias",
			in: ast.UseStmt{
				Source: ast.UseSource{Kind: ast.UseSourcePath, Value: "./lib.jbs"},
				Alias:  "m",
			},
			want: []string{`use "./lib.jbs" as m`},
		},
		{
			name: "selected names from bare",
			in: ast.UseStmt{
				Names:  []string{"a", "b"},
				Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "lib"},
			},
			want: []string{"use a, b from lib"},
		},
		{
			name: "selected names from path",
			in: ast.UseStmt{
				Names:  []string{"a"},
				Source: ast.UseSource{Kind: ast.UseSourcePath, Value: "./lib.jbs"},
			},
			want: []string{`use a from "./lib.jbs"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatUseStmt(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("formatUseStmt mismatch: got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestCollectHeaderCommentBucketsEdgeCases(t *testing.T) {
	inline0 := ast.Comment{Text: "inline0"}
	inline1 := ast.Comment{Text: "inline1"}
	hdrComment := ast.Comment{Text: "head"}
	unknownInline := ast.Comment{Text: "unknown-inline"}
	header := []ast.HeaderElem{
		{Kind: ast.HeaderElemBlank},
		{Kind: ast.HeaderElemComment, Comment: &hdrComment},
		{Kind: ast.HeaderElemAfter, Inline: &inline0},
		{Kind: ast.HeaderElemAfter, Inline: &inline1},
		{Kind: ast.HeaderElemUnknown, Text: "unknown text", Inline: &unknownInline},
		{Kind: ast.HeaderElemComment},
	}
	buckets, trailing := collectHeaderCommentBuckets(header)
	afterBucket := buckets[headerClauseAfter]
	if afterBucket == nil {
		t.Fatalf("expected after bucket")
	}
	if len(afterBucket.Before) < 3 {
		t.Fatalf("expected pending+rolled-inline comments in after bucket, got %v", afterBucket.Before)
	}
	if afterBucket.Inline != "# inline1" {
		t.Fatalf("unexpected after inline comment: %q", afterBucket.Inline)
	}
	if len(trailing) != 3 {
		t.Fatalf("unexpected trailing length: got=%d want=3 values=%v", len(trailing), trailing)
	}
}

func TestRenderHeaderCommentLineAndWithClause(t *testing.T) {
	if got := renderHeaderCommentLine(""); got != "" {
		t.Fatalf("empty comment line should remain empty, got %q", got)
	}
	if got := renderHeaderCommentLine("# x"); got != clauseIndent+"# x" {
		t.Fatalf("comment line indentation mismatch, got %q", got)
	}
	with := []ast.WithItem{
		{Source: "p"},
		{Source: "cases", Selectors: []string{"x", "y"}},
		{Source: "env", Selectors: []string{"host"}},
	}
	got := renderWithClause(with)
	want := "p, cases[x,y], env[host]"
	if got != want {
		t.Fatalf("renderWithClause mismatch: got=%q want=%q", got, want)
	}
}

func TestRebaseInlineBodyIndentBranches(t *testing.T) {
	blank := []string{"", "  "}
	if got := rebaseInlineBodyIndent(blank); !reflect.DeepEqual(got, blank) {
		t.Fatalf("blank-only lines should be unchanged: got=%v want=%v", got, blank)
	}
	alreadyIndented := []string{"  first", "    second"}
	if got := rebaseInlineBodyIndent(alreadyIndented); !reflect.DeepEqual(got, alreadyIndented) {
		t.Fatalf("already-indented first line should be unchanged: got=%v want=%v", got, alreadyIndented)
	}
	flatRest := []string{"first", "second"}
	if got := rebaseInlineBodyIndent(flatRest); !reflect.DeepEqual(got, flatRest) {
		t.Fatalf("non-indented following line should be unchanged: got=%v want=%v", got, flatRest)
	}
	rebased := []string{"first", "    second", "      third"}
	got := rebaseInlineBodyIndent(rebased)
	want := []string{"first", "second", "  third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rebaseInlineBodyIndent mismatch: got=%v want=%v", got, want)
	}
}

func TestRenderSubmitTopLevelBodyAndCanonHelpers(t *testing.T) {
	lines := []string{
		"queue=\"batch\"",
		"preprocess = {",
		"echo one \\",
		"two",
		"}",
		"",
		"# trailing",
	}
	got := renderSubmitTopLevelBody(lines, bodyIndent)
	want := []string{
		`        queue = "batch"`,
		`        preprocess = {`,
		`                echo one \`,
		`                    two`,
		`        }`,
		"",
		`        # trailing`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("renderSubmitTopLevelBody mismatch\n--- got ---\n%v\n--- want ---\n%v", got, want)
	}

	if got := canonicalizeTopLevelSubmitLine("  # c"); got != "# c" {
		t.Fatalf("comment canonicalization mismatch: %q", got)
	}
	if got := canonicalizeTopLevelSubmitLine("  not-an-ident+=x"); got != "not-an-ident+=x" {
		t.Fatalf("non-ident left side should stay unchanged, got %q", got)
	}
}

func TestIsIdentStartsWithCloserDropIndentAndSpanText(t *testing.T) {
	if !isIdent("_x1") {
		t.Fatalf("expected identifier to be valid")
	}
	if isIdent("1x") || isIdent("x-y") {
		t.Fatalf("expected invalid identifiers to fail")
	}
	if !startsWithGroupingCloser("   ]x") {
		t.Fatalf("expected grouping closer detection to succeed")
	}
	if startsWithGroupingCloser("   x]") {
		t.Fatalf("unexpected grouping closer detection")
	}
	if got := dropIndent("abc", 0); got != "abc" {
		t.Fatalf("dropIndent with n<=0 mismatch: %q", got)
	}
	src := []rune("abcdef")
	if got := spanText(src, diag.Span{Start: diag.Position{Offset: -2}, End: diag.Position{Offset: 2}}); got != "ab" {
		t.Fatalf("spanText negative start mismatch: %q", got)
	}
	if got := spanText(src, diag.Span{Start: diag.Position{Offset: 8}, End: diag.Position{Offset: 9}}); got != "" {
		t.Fatalf("spanText beyond end should be empty, got %q", got)
	}
	if got := spanText(src, diag.Span{Start: diag.Position{Offset: 4}, End: diag.Position{Offset: 2}}); got != "" {
		t.Fatalf("spanText with inverted range should be empty, got %q", got)
	}
}
