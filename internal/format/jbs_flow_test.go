package format

import (
	"reflect"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func intPtr(v int) *int { return &v }

func TestJBSCommentsOnly(t *testing.T) {
	src := "  # first  \n\n# second\n"
	var diags diag.Diagnostics
	got, err := JBS("comments.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := "  # first\n\n# second\n"
	if got != want {
		t.Fatalf("unexpected output\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestJBSEmptySource(t *testing.T) {
	var diags diag.Diagnostics
	got, err := JBS("empty.jbs", "", &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got != "\n" {
		t.Fatalf("unexpected output for empty source: %q", got)
	}
}

func TestJBSInvalidSource(t *testing.T) {
	src := "do run {\n"
	var diags diag.Diagnostics
	got, err := JBS("invalid.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty output on parse error, got %q", got)
	}
	if !diags.HasErrors() {
		t.Fatalf("expected parse diagnostics")
	}
}

func TestJBSMixedActiveStatements(t *testing.T) {
	src := `
jbs_name="bench" # inline global
jbs_outpath="out"
# use comment
use "./lib.jbs" as m
do prep
   with p[x,y]
   max_async=2 procs=4 iterations=1
{
echo one \
two
}
submit run
        after prep
        use defaults
        with p0, p1[x]
{
queue="batch"
preprocess = {
echo pre
}
args_exec="-lc hostname"
}
analyse run
   with p[x]
{
n = "N: %d" in "out.log"
(x, n as "num")
}
`
	var diags diag.Diagnostics
	got, err := JBS("mixed.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	checks := []string{
		`jbs_name = "bench" # inline global`,
		`jbs_outpath = "out"`,
		`# use comment`,
		`use "./lib.jbs" as m`,
		`do prep`,
		`        with p[x,y]`,
		`        max_async=2 procs=4 iterations=1`,
		`submit run`,
		`        after prep`,
		`        use defaults`,
		`        queue = "batch"`,
		`analyse run`,
		`        with p[x]`,
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted output missing %q\n--- output ---\n%s", needle, got)
		}
	}
}

func TestJBSFormatsTopLevelExprLines(t *testing.T) {
	src := `
use jsc
  jsc.systemname
x=(1, 2)
 x
`
	var diags diag.Diagnostics
	got, err := JBS("exprs.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	checks := []string{
		"use jsc",
		"jsc.systemname",
		"x = (1, 2)",
		"x",
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted output missing %q\n--- output ---\n%s", needle, got)
		}
	}
}

func TestJBSFormatsFunctionAssignment(t *testing.T) {
	src := `
f=function(x,y=1){
x + y
}
`
	var diags diag.Diagnostics
	got, err := JBS("functions.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := "f = function(x, y = 1) {\n    x + y\n}\n"
	if got != want {
		t.Fatalf("unexpected formatted function assignment\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestJBSFormatsNamedCallArguments(t *testing.T) {
	src := `
f(1,b=2)
`
	var diags diag.Diagnostics
	got, err := JBS("named_args.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got != "f(1, b = 2)\n" {
		t.Fatalf("unexpected named-arg formatting: %q", got)
	}
}

func TestJBSFormatsInlineAnonymousFunctionCall(t *testing.T) {
	src := `
function(x){x}(1,b=2)
`
	var diags diag.Diagnostics
	got, err := JBS("inline_function.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	checks := []string{
		"function(x) {",
		"    x",
		"}(1, b = 2)",
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted anonymous call missing %q\n--- output ---\n%s", needle, got)
		}
	}
}

func TestSplitSegmentLinesAndComments(t *testing.T) {
	if got := splitSegmentLines("a\nb\n"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected split with trailing newline: %v", got)
	}
	if got := splitSegmentLines("a\nb"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected split without trailing newline: %v", got)
	}

	if line, ok := parseCommentFragment("  # c  ", false); !ok || line != "  # c" {
		t.Fatalf("unexpected inline comment parse: ok=%v line=%q", ok, line)
	}
	if _, ok := parseCommentFragment("x # c", false); ok {
		t.Fatalf("expected inline parse to reject non-whitespace prefix")
	}
	if line, ok := parseCommentFragment("; # c  ", true); !ok || line != "# c" {
		t.Fatalf("unexpected semicolon comment parse: ok=%v line=%q", ok, line)
	}
	if !isWhitespace(" \t") || isWhitespace(" \tx") {
		t.Fatalf("isWhitespace unexpected behavior")
	}
}

func TestExtractTopLevelTrivia(t *testing.T) {
	trivia := extractTopLevelTrivia("   # inline\n# line\n\n", true)
	if trivia.InlineSuffix != "   # inline" {
		t.Fatalf("unexpected inline suffix: %q", trivia.InlineSuffix)
	}
	if !reflect.DeepEqual(trivia.Lines, []string{"# line", ""}) {
		t.Fatalf("unexpected lines: %v", trivia.Lines)
	}

	trivia = extractTopLevelTrivia("\n# only\n", true)
	if trivia.InlineSuffix != "" {
		t.Fatalf("unexpected inline suffix after leading newline: %q", trivia.InlineSuffix)
	}
	if !reflect.DeepEqual(trivia.Lines, []string{"# only"}) {
		t.Fatalf("unexpected lines for leading-newline trivia: %v", trivia.Lines)
	}

	if got := extractTopLevelTrivia("value", true); got.InlineSuffix != "" || len(got.Lines) != 0 {
		t.Fatalf("expected empty trivia for non-comment segment, got %+v", got)
	}
}

func TestCollectStmtRangesAndSlice(t *testing.T) {
	stmts := []ast.Stmt{
		ast.GlobalAssign{
			Name: "a",
			Span: diag.Span{
				Start: diag.Position{Offset: -3},
				End:   diag.Position{Offset: 4},
			},
		},
		ast.DoBlock{
			Name: "s",
			Span: diag.Span{
				Start: diag.Position{Offset: 6},
				End:   diag.Position{Offset: 40},
			},
		},
	}
	ranges := collectStmtRanges(stmts, 10)
	if len(ranges) != 2 {
		t.Fatalf("unexpected ranges length: %d", len(ranges))
	}
	if ranges[0].Start != 0 || ranges[0].End != 4 {
		t.Fatalf("unexpected first range: %+v", ranges[0])
	}
	if ranges[1].Start != 6 || ranges[1].End != 10 {
		t.Fatalf("unexpected second range: %+v", ranges[1])
	}

	src := []rune("0123456789")
	if got := sliceSourceRange(src, -4, 3); got != "012" {
		t.Fatalf("unexpected clamped slice: %q", got)
	}
	if got := sliceSourceRange(src, 8, 20); got != "89" {
		t.Fatalf("unexpected end-clamped slice: %q", got)
	}
	if got := sliceSourceRange(src, 9, 1); got != "" {
		t.Fatalf("expected empty slice for inverted range, got %q", got)
	}
}

func TestHeaderClauseRenderingCoverage(t *testing.T) {
	with := []ast.WithItem{
		{
			Source:    "p",
			Selectors: []string{"x", "y"},
		},
		{
			Source:    "p0",
			Selectors: []string{"x"},
		},
	}
	clauses := buildRenderedHeaderClauses(
		[]string{"s0", "s1"},
		[]string{"defaults"},
		with,
		intPtr(2),
		intPtr(4),
		intPtr(1),
	)
	if len(clauses) != 4 {
		t.Fatalf("unexpected clause count: %d", len(clauses))
	}
	if got := clauses[2].Text; got != "with p[x,y], p0[x]" {
		t.Fatalf("unexpected with clause: %q", got)
	}
	if got := clauses[3].Text; got != "max_async=2 procs=4 iterations=1" {
		t.Fatalf("unexpected option clause: %q", got)
	}

	if got := renderStepOptionClause(nil, nil, nil); got != "" {
		t.Fatalf("expected empty step options, got %q", got)
	}
	if got := toHeaderClauseKind(ast.HeaderElemWith); got != headerClauseWith {
		t.Fatalf("unexpected with clause kind: %v", got)
	}
	if got := toHeaderClauseKind(ast.HeaderElemOption); got != headerClauseOptions {
		t.Fatalf("unexpected option clause kind: %v", got)
	}
}

func TestActiveBlockFormatters(t *testing.T) {
	doBlock := ast.DoBlock{
		Name:       "run",
		After:      []string{"setup"},
		WithItems:  []ast.WithItem{{Source: "p"}},
		MaxAsync:   intPtr(2),
		Procs:      intPtr(3),
		Iterations: intPtr(1),
		Body:       "echo one \\\ntwo",
	}
	doLines := formattedLineTexts(formatDoBlock(doBlock))
	if len(doLines) == 0 || doLines[0] != "do run" {
		t.Fatalf("unexpected do block header: %v", doLines)
	}
	if !containsLine(doLines, "        with p") {
		t.Fatalf("missing with clause in do block: %v", doLines)
	}
	if !containsLine(doLines, "two") {
		t.Fatalf("missing preserved raw line in do block: %v", doLines)
	}

	analyseBlock := ast.AnalyseBlock{
		StepName:  "run",
		WithItems: []ast.WithItem{{Source: "p"}},
		BodyRaw:   "n = \"N: %d\" in \"out.log\"\n(n)",
	}
	analyseLines := formatAnalyseBlock(analyseBlock)
	if len(analyseLines) == 0 || analyseLines[0] != "analyse run" {
		t.Fatalf("unexpected analyse block header: %v", analyseLines)
	}
	if !containsLine(analyseLines, "        with p") {
		t.Fatalf("missing with clause in analyse block: %v", analyseLines)
	}

	submitRaw := ast.SubmitBlock{
		Name: "run",
		BodyRaw: `queue="batch"
preprocess = {
echo pre
}`,
	}
	submitLines := formattedLineTexts(formatSubmitBlock(submitRaw, nil))
	if len(submitLines) == 0 || submitLines[0] != "submit run" {
		t.Fatalf("unexpected submit block header: %v", submitLines)
	}
	if !containsLine(submitLines, `        queue = "batch"`) {
		t.Fatalf("missing queue assignment in submit raw-body formatting: %v", submitLines)
	}
	if !containsLine(submitLines, `                echo pre`) {
		t.Fatalf("missing nested raw-block indentation in submit formatting: %v", submitLines)
	}
}

func TestRenderSubmitFieldsOperators(t *testing.T) {
	src := []rune(`"batch" 4 2`)
	fields := []ast.SubmitField{
		{
			Name: "queue",
			Op:   ast.AssignPlusEq,
			Expr: ast.StringExpr{
				Span: diag.Span{
					Start: diag.Position{Offset: 0},
					End:   diag.Position{Offset: 7},
				},
			},
		},
		{
			Name: "tasks",
			Op:   ast.AssignStarEq,
			Expr: ast.NumberExpr{
				Span: diag.Span{
					Start: diag.Position{Offset: 8},
					End:   diag.Position{Offset: 9},
				},
			},
		},
		{
			Name: "nodes",
			Expr: nil,
		},
	}
	got := formattedLineTexts(renderSubmitFields(fields, src))
	want := []string{
		`        queue += "batch"`,
		`        tasks *= 4`,
		`        nodes = ""`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rendered submit fields\n--- got ---\n%v\n--- want ---\n%v", got, want)
	}
}

func TestGroupingDelimsOutsideQuotes(t *testing.T) {
	open, close := countGroupingDelimsOutsideQuotes(`[(1 + "{x}") # (comment)`)
	if open != 2 || close != 1 {
		t.Fatalf("unexpected grouping counts: open=%d close=%d", open, close)
	}
}

func containsLine(lines []string, target string) bool {
	for _, line := range lines {
		if line == target {
			return true
		}
	}
	return false
}
