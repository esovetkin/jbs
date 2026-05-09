package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestAnalyzeEnvTopLevelAndFunctions(t *testing.T) {
	src := strings.Join([]string{
		`value = env("JBS_ENV_VALUE", "missing")`,
		`f = function(x = env("JBS_ENV_DEFAULT")) { env("JBS_ENV_BODY") + ":" + x }`,
		`out = f()`,
		`fallback = env("JBS_ENV_MISSING", 9)`,
		``,
	}, "\n")
	diags := &diag.Diagnostics{}
	prog := parser.Parse("env.jbs", src, diags)
	res := AnalyzeWithOptions(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, AnalyzeOptions{
		Environ: semaFixedEnviron("JBS_ENV_VALUE=from-env", "JBS_ENV_DEFAULT=default", "JBS_ENV_BODY=body"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got := res.GlobalVarByName["value"].Value; got.Kind != eval.KindString || got.S != "from-env" {
		t.Fatalf("unexpected env top-level value: %#v", got)
	}
	if got := res.GlobalVarByName["out"].Value; got.Kind != eval.KindString || got.S != "body:default" {
		t.Fatalf("unexpected env function value: %#v", got)
	}
	if got := res.GlobalVarByName["fallback"].Value; got.Kind != eval.KindInt || got.I != 9 {
		t.Fatalf("unexpected env fallback value: %#v", got)
	}
}

func TestAnalyzeWithImportsPropagatesEnvProvider(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte(`from_env = env("JBS_ENV_IMPORT")`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", `use from_env from "./lib.jbs"`+"\nvalue = from_env\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImportsOptions(loadRes, map[string]eval.Value{"jbs_name": eval.String("bench")}, AnalyzeOptions{
		Environ: semaFixedEnviron("JBS_ENV_IMPORT=imported"),
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got := res.GlobalVarByName["value"].Value; got.Kind != eval.KindString || got.S != "imported" {
		t.Fatalf("unexpected imported env value: %#v", got)
	}
}

func semaFixedEnviron(items ...string) func() []string {
	return func() []string {
		return append([]string(nil), items...)
	}
}
