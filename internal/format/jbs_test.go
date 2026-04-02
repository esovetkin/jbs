package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/diag"
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

func TestFormatParseErrorReturnsNoOutput(t *testing.T) {
	src := "param p {\n  a = @\n  a\n}\n"
	diags := &diag.Diagnostics{}
	got, err := JBS("bad.jbs", src, diags)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}
	if !diags.HasErrors() {
		t.Fatalf("expected parse/sema errors")
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
