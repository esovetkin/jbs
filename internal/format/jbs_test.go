package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/parser"
)

func TestFormatGoldenFixtures(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "tests")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "fmt_") || !strings.HasSuffix(entry.Name(), ".jbs") {
			continue
		}
		count++
		t.Run(entry.Name(), func(t *testing.T) {
			inPath := filepath.Join(fixtureDir, entry.Name())
			expectedPath := filepath.Join(fixtureDir, strings.TrimSuffix(entry.Name(), ".jbs")+".yaml")
			in, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			diags := &diag.Diagnostics{}
			got, err := JBS(inPath, string(in), diags)
			if err != nil {
				t.Fatalf("format failed: %v", err)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
			if got != string(expected) {
				t.Fatalf("golden mismatch\n--- got ---\n%s\n--- expected ---\n%s", got, string(expected))
			}
		})
	}
	if count == 0 {
		t.Fatalf("no formatter fixtures found")
	}
}

func TestFormatIdempotent(t *testing.T) {
	src := `jbs_name="test"
param p{a=(1,2)
a
}
`
	diags := &diag.Diagnostics{}
	first, err := JBS("idempotent.jbs", src, diags)
	if err != nil {
		t.Fatalf("first format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	diags = &diag.Diagnostics{}
	second, err := JBS("idempotent.jbs", first, diags)
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors on second format: %s", diags.String())
	}
	if first != second {
		t.Fatalf("formatter is not idempotent\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestFormatPreservesTopLevelCommentsAroundBlocks(t *testing.T) {
	src := `# another comment

param testcases
{
    id = tuple([1,2,3] * 100)
    label = ("a",) * 2 + ("b",)

    id + label
}

# some comment
do run
   with testcases
{
   echo $id $label
}

# comment
`
	diags := &diag.Diagnostics{}
	got, err := JBS("comments_blocks.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `# another comment

param testcases
{
        id = tuple([1,2,3] * 100)
        label = ("a",) * 2 + ("b",)

        id + label
}

# some comment
do run
        with testcases
{
        echo $id $label
}

# comment
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesInlineTopLevelTrailingComment(t *testing.T) {
	src := `jbs_name="x"   # benchmark
param p
{
    a = 1

    a
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("inline_top_comment.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `jbs_name = "x"   # benchmark

param p
{
        a = 1

        a
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesTrailingEOFComment(t *testing.T) {
	src := `param p
{
    a = 1

    a
}
# eof comment`
	diags := &diag.Diagnostics{}
	got, err := JBS("eof_comment.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `param p
{
        a = 1

        a
}
# eof comment
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatCommentOnlyFile(t *testing.T) {
	src := `# first

# second
`
	diags := &diag.Diagnostics{}
	got, err := JBS("comment_only.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `# first

# second
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatTopLevelCommentsWithSemicolonStatements(t *testing.T) {
	src := `jbs_name="x"; # name
jbs_outpath="y"
param p
{
    a = 1

    a
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("semicolon_comment.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `jbs_name = "x"
# name
jbs_outpath = "y"

param p
{
        a = 1

        a
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatCommentsIdempotent(t *testing.T) {
	src := `# lead

jbs_name="x" # inline

# mid
param p
{
    a = 1

    a
}
# tail`
	firstDiags := &diag.Diagnostics{}
	first, err := JBS("comments_idempotent.jbs", src, firstDiags)
	if err != nil {
		t.Fatalf("first format failed: %v", err)
	}
	if firstDiags.HasErrors() {
		t.Fatalf("unexpected first-pass errors: %s", firstDiags.String())
	}
	secondDiags := &diag.Diagnostics{}
	second, err := JBS("comments_idempotent.jbs", first, secondDiags)
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}
	if secondDiags.HasErrors() {
		t.Fatalf("unexpected second-pass errors: %s", secondDiags.String())
	}
	if first != second {
		t.Fatalf("formatter is not idempotent for comments\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestFormatParseErrorReturnsNoOutput(t *testing.T) {
	src := "param p {\n  a = @\n  a\n}\n"
	diags := &diag.Diagnostics{}
	got, err := JBS("bad.jbs", src, diags)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}
	if !diags.HasErrors() {
		t.Fatalf("expected parse errors")
	}
	if got != "" {
		t.Fatalf("expected empty formatted output on errors, got %q", got)
	}
}

func TestNormalizeBodyDedentAndIndent(t *testing.T) {
	raw := "\n\tline1\n\t\tline2\n\n\tline3\n"
	got := normalizeBody(raw, "        ")
	want := []string{
		"        line1",
		"        line2",
		"",
		"        line3",
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected line count: got=%d want=%d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("line %d mismatch: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestLeadingIndentAndDropIndent(t *testing.T) {
	if got := leadingIndent("\t  x"); got != 3 {
		t.Fatalf("unexpected leading indent: %d", got)
	}
	if got := dropIndent("\t  x", 2); got != " x" {
		t.Fatalf("unexpected dropIndent result: %q", got)
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	in := "a\r\nb\rc\n"
	got := normalizeLineEndings(in)
	if got != "a\nb\nc\n" {
		t.Fatalf("unexpected normalized line endings: %q", got)
	}
}

func TestRenderSubmitFields(t *testing.T) {
	src := `submit run
{
args_exec = "-lc hostname"
preprocess = {
echo pre
}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("render_submit_fields.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("unexpected stmt count: got=%d want=1", len(prog.Stmts))
	}
	submit, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("unexpected statement type: %T", prog.Stmts[0])
	}
	fields := append([]ast.SubmitField{}, submit.Fields...)
	fields = append(fields, ast.SubmitField{Name: "measurement", Op: ast.AssignPlusEq})
	got := renderSubmitFields(fields, []rune(src))
	want := []string{
		`        args_exec = "-lc hostname"`,
		`        preprocess = {`,
		`                echo pre`,
		`        }`,
		`        measurement += ""`,
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected line count: got=%d want=%d\nlines:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("line %d mismatch: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestFormatGlobalCompoundAssignment(t *testing.T) {
	src := "jbs_comment += \"hello\"\n"
	diags := &diag.Diagnostics{}
	got, err := JBS("global_compound_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := "jbs_comment += \"hello\"\n"
	if got != want {
		t.Fatalf("unexpected format output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatGlobalCompoundAssignmentContinuationIndent(t *testing.T) {
	src := "let l{\nx = \"a\" +\\\n\"b\"\n}\n"
	diags := &diag.Diagnostics{}
	got, err := JBS("global_compound_continuation_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !strings.Contains(got, "x = \"a\" +\\\n            \"b\"") {
		t.Fatalf("expected continuation indentation in formatted body, got:\n%s", got)
	}
}

func TestFormatCompoundAssignmentsWithSemicolons(t *testing.T) {
	src := "let l{a=1;a+=2}\n"
	diags := &diag.Diagnostics{}
	got, err := JBS("compound_semicolon_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !strings.Contains(got, "a+=2") {
		t.Fatalf("expected formatted output to preserve '+=' operator, got:\n%s", got)
	}
}

func TestGlobalsRemainContiguous(t *testing.T) {
	src := "jbs_name=\"x\"\njbs_outpath=\"y\"\nparam p{a=1\na\n}\n"
	diags := &diag.Diagnostics{}
	got, err := JBS("globals.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	wantPrefix := "jbs_name = \"x\"\njbs_outpath = \"y\"\n\nparam p\n"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("unexpected global grouping:\n%s", got)
	}
}

func TestFormatTopLevelUseStatements(t *testing.T) {
	src := `use jsc
use "./path/mod.jbs" as mod
use a,b from jsc
`
	diags := &diag.Diagnostics{}
	got, err := JBS("use_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `use jsc

use "./path/mod.jbs" as mod

use a, b from jsc
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatSubmitHeaderWithUseClause(t *testing.T) {
	src := `let defaults{queue="batch"}
param p{a=1
a
}
do prep{echo prep}
submit run
with p
use defaults
after prep
{
args_exec="-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("submit_use_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !strings.Contains(got, "submit run\n        after prep\n        use defaults\n        with p\n{") {
		t.Fatalf("expected canonical submit header with use clause, got:\n%s", got)
	}
}

func TestFormatSubmitHeaderWithRepeatedUseClauses(t *testing.T) {
	src := `let defaults{queue="batch"}
let gpu_defaults{gres="gpu:4"}
submit run
use defaults
use gpu_defaults
{
args_exec="-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("submit_use_repeated_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !strings.Contains(got, "submit run\n        use defaults, gpu_defaults\n{") {
		t.Fatalf("expected canonical merged submit use clause, got:\n%s", got)
	}
}

func TestFormatStepHeaderOptionsCanonical(t *testing.T) {
	src := `do run
iterations=2
with p
max_async=5
{
echo hi
}
submit bench
max_async=0
with p
iterations=3
use defaults
after run
{
args_exec="-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("step_options_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if !strings.Contains(got, "do run\n        with p\n        max_async=5 iterations=2\n{") {
		t.Fatalf("expected canonical do option line, got:\n%s", got)
	}
	if !strings.Contains(got, "submit bench\n        after run\n        use defaults\n        with p\n        max_async=0 iterations=3\n{") {
		t.Fatalf("expected canonical submit option line, got:\n%s", got)
	}
}

func TestFormatParamInlineBodyIndentation(t *testing.T) {
	src := `param p{a=(1,2)
        b=(3,4)
                 a+b
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("param_inline.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `param p
{
        a=(1,2)
        b=(3,4)
        a+b
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatDoInlineBodyIndentation(t *testing.T) {
	src := `do run{echo one
        echo two
                 echo three
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("do_inline.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `do run
{
        echo one
        echo two
        echo three
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatLetInlineBodyIndentation(t *testing.T) {
	src := `let p{number = "Number: %d"
        zahl = "Zahl: %d"
                 letter = "Letter: %w"
        buchstabe = "Buchstabe: %w"
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("let_inline.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `let p
{
        number = "Number: %d"
        zahl = "Zahl: %d"
        letter = "Letter: %w"
        buchstabe = "Buchstabe: %w"
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatAnalyseInlineBodyIndentation(t *testing.T) {
	src := `do write{echo "Number: 1" > out
        echo "Word: hello" >> out
}
let p{number = "Number: %d"
        word = "Word: %w"
}
analyse write with p{x = number
        n = x in "out"
        w = word in "out"
                 (n, w)
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("analyse_inline.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `do write
{
        echo "Number: 1" > out
        echo "Word: hello" >> out
}

let p
{
        number = "Number: %d"
        word = "Word: %w"
}

analyse write
        with p
{
        x = number
        n = x in "out"
        w = word in "out"
        (n, w)
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatSubmitInlineBodyIndentation(t *testing.T) {
	src := `submit run{args_exec="-lc hostname"
        queue="devel"
                 timelimit="00:10:00"
        preprocess = {
                export X=1
        }
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("submit_inline.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `submit run
{
        args_exec = "-lc hostname"
        queue = "devel"
        timelimit = "00:10:00"
        preprocess = {
                export X=1
        }
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatPreservesSemicolonSeparatedStatements(t *testing.T) {
	src := `let p{a=1; b=2; c=3}
param q{x=(1,2); y=("a","b"); x+y}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("semicolon_fmt.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `let p
{
        a=1; b=2; c=3
}

param q
{
        x=(1,2); y=("a","b"); x+y
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationIndentInParamBody(t *testing.T) {
	src := `param p{a = "x" + \
"y"
a
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("param_continuation.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `param p
{
        a = "x" + \
            "y"
        a
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatTupleMultilineExtraIndent(t *testing.T) {
	src := `param p{
hydra_args = ("",
"dataset=fineweb",
"model=llama7b trainer=fsdp",
)
hydra_args
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("tuple_multiline_indent.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `param p
{
        hydra_args = ("",
            "dataset=fineweb",
            "model=llama7b trainer=fsdp",
        )
        hydra_args
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatListMultilineExtraIndent(t *testing.T) {
	src := `let l{
vals = [
1,
2,
3,
]
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("list_multiline_indent.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `let l
{
        vals = [
            1,
            2,
            3,
        ]
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatTupleClosingDelimiterAlignment(t *testing.T) {
	src := `param p{
x = (
1,
2,
)
x
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("tuple_closer_align.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if strings.Contains(got, "\n            )") {
		t.Fatalf("closing delimiter is over-indented:\n%s", got)
	}
	if !strings.Contains(got, "\n        )") {
		t.Fatalf("expected closing delimiter at assignment indent:\n%s", got)
	}
}

func TestFormatBackslashAndTupleIndentCompose(t *testing.T) {
	src := `param p{
x = ("a" + \
"b",
"c",
)
x
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("tuple_continuation_compose.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `param p
{
        x = ("a" + \
                "b",
            "c",
        )
        x
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatIdempotentTupleIndent(t *testing.T) {
	src := `param p{
x = ("",
"a",
"b",
)
x
}
`
	diags := &diag.Diagnostics{}
	first, err := JBS("tuple_indent_idempotent.jbs", src, diags)
	if err != nil {
		t.Fatalf("first format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected first-pass errors: %s", diags.String())
	}
	secondDiags := &diag.Diagnostics{}
	second, err := JBS("tuple_indent_idempotent.jbs", first, secondDiags)
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}
	if secondDiags.HasErrors() {
		t.Fatalf("unexpected second-pass errors: %s", secondDiags.String())
	}
	if first != second {
		t.Fatalf("formatter is not idempotent for tuple indentation\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestFormatContinuationIndentInDoBody(t *testing.T) {
	src := `do run{echo one \
two
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("do_continuation.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `do run
{
        echo one \
            two
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationIndentInLetBody(t *testing.T) {
	src := `let l{msg = "x" + \
"y"
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("let_continuation.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `let l
{
        msg = "x" + \
            "y"
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationIndentInAnalyseBody(t *testing.T) {
	src := `analyse write{x = "n" + \
"m"
(x)
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("analyse_continuation.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `analyse write
{
        x = "n" + \
            "m"
        (x)
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationIndentInSubmitBody(t *testing.T) {
	src := `submit run{args_exec = "-lc " + \
"'hostname'"
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("submit_continuation.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `submit run
{
        args_exec = "-lc " + \
            "'hostname'"
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationIndentInSubmitRawBlock(t *testing.T) {
	src := `submit run{preprocess = {
echo pre \
work
}
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("submit_raw_continuation.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `submit run
{
        preprocess = {
                echo pre \
                    work
        }
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationBackslashInCommentNoIndent(t *testing.T) {
	src := `do s{echo one # \
echo two
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("comment_backslash.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `do s
{
        echo one # \
        echo two
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationBackslashInStringNoIndent(t *testing.T) {
	src := `do s{echo "path\\"
echo two
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("string_backslash.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `do s
{
        echo "path\\"
        echo two
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatContinuationResetsAfterBlankLine(t *testing.T) {
	src := `do s{echo one \
two

echo three
}
`
	diags := &diag.Diagnostics{}
	got, err := JBS("continuation_blank_reset.jbs", src, diags)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	want := `do s
{
        echo one \
            two

        echo three
}
`
	if got != want {
		t.Fatalf("unexpected formatted output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
