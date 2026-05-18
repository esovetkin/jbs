package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

func analyzeImportedModuleSource(t *testing.T, entry string, files map[string]string) (*Result, *diag.Diagnostics) {
	t.Helper()
	cwd := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", entry, cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	return res, diags
}

func TestImportedFunctionDefaultCapturesUseDefinitionModule(t *testing.T) {
	res, diags := analyzeImportedModuleSource(t, strings.Join([]string{
		`use make from "./lib.jbs"`,
		`f = make()`,
		`f(10)`,
		``,
	}, "\n"), map[string]string{
		"lib.jbs": strings.Join([]string{
			`base = 1`,
			`make = function(delta = base + 1) {`,
			`  function(x) { x + delta }`,
			`}`,
			``,
		}, "\n"),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(12)) {
		t.Fatalf("unexpected imported captured function result: %#v", res.TopLevelExprs)
	}
}

func TestImportedContainerFunctionsUseDefinitionModuleCaptures(t *testing.T) {
	res, diags := analyzeImportedModuleSource(t, strings.Join([]string{
		`use functions from "./lib.jbs"`,
		`functions[0](4)`,
		``,
	}, "\n"), map[string]string{
		"lib.jbs": strings.Join([]string{
			`base = 3`,
			`functions = [function(x) { x + base }]`,
			``,
		}, "\n"),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(7)) {
		t.Fatalf("unexpected imported container function result: %#v", res.TopLevelExprs)
	}
}
