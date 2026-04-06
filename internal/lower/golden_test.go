package lower_test

import (
	"os"
	"path/filepath"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/emit"
	"jbs/internal/imports"
	"jbs/internal/lower"
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

func TestGoldenLetStepImports(t *testing.T) {
	assertGolden(t, "let_step_imports")
}

func TestGoldenUseEmbedDefaults(t *testing.T) {
	assertGolden(t, "use_embed_defaults")
}

func TestGoldenUseImportStepChain(t *testing.T) {
	assertGolden(t, "use_import_step_chain")
}

func TestGoldenUseSubmitDefaultsOverride(t *testing.T) {
	assertGolden(t, "use_submit_defaults_override")
}

func TestGoldenSubmitParamCollisionEscape(t *testing.T) {
	assertGolden(t, "submit_param_collision_escape")
}

func assertGolden(t *testing.T, name string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	inputPath := filepath.Join(repoRoot, "tests", name+".jbs")
	expectedPath := filepath.Join(repoRoot, "tests", name+".yaml")

	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}

	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpand(inputPath, repoRoot, diags)
	if err != nil {
		t.Fatalf("load+expand: %v", err)
	}
	res := sema.Analyze(loadRes.Program, lower.BuiltinGlobalValues(), diags)
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
