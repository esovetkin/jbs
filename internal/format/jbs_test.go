package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/diag"
)

func TestFormatGoldenFixtures(t *testing.T) {
	inputDir := filepath.Join("..", "..", "testdata", "fmt", "input")
	expectedDir := filepath.Join("..", "..", "testdata", "fmt", "expected")
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		t.Fatalf("read input dir: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jbs") {
			continue
		}
		count++
		t.Run(entry.Name(), func(t *testing.T) {
			inPath := filepath.Join(inputDir, entry.Name())
			expectedPath := filepath.Join(expectedDir, entry.Name())
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
		"        \tline2",
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
