package lower_test

import (
	"os"
	"path/filepath"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/emit"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestGoldenBasic(t *testing.T) {
	assertGolden(t, "basic")
}

func TestGoldenResultsBasic(t *testing.T) {
	assertGolden(t, "results_basic")
}

func TestGoldenAfterInheritZipPreserve(t *testing.T) {
	assertGolden(t, "after_inherit_zip_preserve")
}

func TestGoldenAfterInheritProductExpand(t *testing.T) {
	assertGolden(t, "after_inherit_product_expand")
}

func TestGoldenAfterInheritTransitiveChain(t *testing.T) {
	assertGolden(t, "after_inherit_transitive_chain")
}

func assertGolden(t *testing.T, name string) {
	t.Helper()
	inputPath := filepath.Join("..", "..", "tests", name+".jbs")
	expectedPath := filepath.Join("..", "..", "tests", name+".yaml")

	src, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read input: %v", err)
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}

	diags := &diag.Diagnostics{}
	prog := parser.Parse(inputPath, string(src), diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	doc := lower.ToJUBEYAML(res, lower.Options{InputPath: inputPath}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	got, err := emit.YAML(doc)
	if err != nil {
		t.Fatalf("emit yaml: %v", err)
	}
	if string(got) != string(expected) {
		t.Fatalf("golden mismatch\n--- got ---\n%s\n--- expected ---\n%s", string(got), string(expected))
	}
}
