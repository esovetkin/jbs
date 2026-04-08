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

func TestGolden(t *testing.T) {
	cases := []struct {
		name string
	}{
		{name: "basic"},
		{name: "results_basic"},
		{name: "after_inherit_zip_preserve"},
		{name: "after_inherit_product_expand"},
		{name: "after_inherit_transitive_chain"},
		{name: "let_step_imports"},
		{name: "use_embed_defaults"},
		{name: "use_import_step_chain"},
		{name: "use_submit_defaults_override"},
		{name: "submit_param_collision_escape"},
		{name: "submit_use_helper_alias"},
		{name: "step_options"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertGolden(t, tc.name)
		})
	}
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
	doc := lower.ToJUBEYAML(res, diags)
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
