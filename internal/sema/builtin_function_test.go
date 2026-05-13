package sema

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func analyzeBuiltinFunctionSource(t *testing.T, src string) (*Result, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("builtin_functions.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("parse failed: %s", diags.String())
	}
	res := Analyze(prog, BuiltinGlobalValues(), diags)
	return res, diags
}

func TestAnalyzeBuiltinFunctionAsMapCallback(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
values = map(int, ["1", "2"])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	if slices.Contains(values.DependsOn, "int") || slices.Contains(values.DependsOn, "map") {
		t.Fatalf("unshadowed builtins should not be recorded as dependencies: %#v", values.DependsOn)
	}
	for _, key := range values.DependsOnKeys {
		if key.Public == "int" || key.Public == "map" {
			t.Fatalf("unshadowed builtin dependency key recorded: %#v", values.DependsOnKeys)
		}
	}
}

func TestAnalyzeShadowedBuiltinFunctionDependency(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
int = function(x) { 42 }
values = map(int, ["1"])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(42)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	if !slices.Contains(values.DependsOn, "int") {
		t.Fatalf("expected shadowed builtin dependency on user global int, got %#v", values.DependsOn)
	}
}

func TestAnalyzeSumProdBuiltins(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
values = sum([1, 2, 3])
product_value = prod((2, 3, 4))
mapped = map(sum, [[1, 2], [3, 4]])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.Int(6)) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	productValue := res.GlobalVarByName["product_value"]
	if productValue == nil || !eval.Equal(productValue.Value, eval.Int(24)) {
		t.Fatalf("unexpected product_value global: %#v", productValue)
	}
	mapped := res.GlobalVarByName["mapped"]
	if mapped == nil || !eval.Equal(mapped.Value, eval.List([]eval.Value{eval.Int(3), eval.Int(7)})) {
		t.Fatalf("unexpected mapped global: %#v", mapped)
	}
	for _, gv := range []*GlobalVar{values, productValue, mapped} {
		for _, dep := range []string{"sum", "prod"} {
			if slices.Contains(gv.DependsOn, dep) {
				t.Fatalf("unshadowed builtin %q should not be recorded as dependency for %s: %#v", dep, gv.Name, gv.DependsOn)
			}
		}
		for _, key := range gv.DependsOnKeys {
			if key.Public == "sum" || key.Public == "prod" {
				t.Fatalf("unshadowed builtin dependency key recorded for %s: %#v", gv.Name, gv.DependsOnKeys)
			}
		}
	}
}

func TestAnalyzeShadowedSumProdDependencies(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
sum = function(values) { 42 }
prod = function(values) { 99 }
values = sum([1, 2, 3])
product_value = prod((2, 3, 4))
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.Int(42)) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	if !slices.Contains(values.DependsOn, "sum") {
		t.Fatalf("expected shadowed sum dependency, got %#v", values.DependsOn)
	}
	productValue := res.GlobalVarByName["product_value"]
	if productValue == nil || !eval.Equal(productValue.Value, eval.Int(99)) {
		t.Fatalf("unexpected product_value global: %#v", productValue)
	}
	if !slices.Contains(productValue.DependsOn, "prod") {
		t.Fatalf("expected shadowed prod dependency, got %#v", productValue.DependsOn)
	}
}

func TestBuiltinFunctionWithSourcesAreRejectedAsData(t *testing.T) {
	_, diags := analyzeBuiltinFunctionSource(t, `
to_int = int

do s with to_int {
  echo "$to_int"
}
`)
	if !diags.HasErrors() {
		t.Fatalf("expected function-valued with source to fail")
	}
	if !strings.Contains(diags.String(), "with-clause can only import data bindings; 'to_int' is not a data binding") {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestDeleteShadowedBuiltinRestoresFallback(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
int = function(x) { 42 }
delete(int)
values = map(int, ["1"])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if _, ok := res.GlobalVarByName["int"]; ok {
		t.Fatalf("expected user-defined int to be deleted")
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(1)})) {
		t.Fatalf("expected fallback builtin int after delete, got %#v", values)
	}
}

func TestDeleteUnshadowedBuiltinFunctionStillRejected(t *testing.T) {
	_, diags := analyzeBuiltinFunctionSource(t, `delete(int)`)
	if !diags.HasErrors() {
		t.Fatalf("expected deleting builtin int to fail")
	}
	if !strings.Contains(diags.String(), "cannot delete built-in function 'int'") {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestBuiltinFunctionModuleExports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.jbs"), []byte("to_int = int\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := strings.Join([]string{
		`use to_int from "./lib.jbs"`,
		`use "./lib.jbs" as lib`,
		`values = map(to_int, ["1", "2"])`,
		`ns_values = map(lib.to_int, ["3", "4"])`,
		``,
	}, "\n")

	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("main.jbs", entry, dir, dir, diags)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("unexpected selective import values: %#v", values)
	}
	nsValues := res.GlobalVarByName["ns_values"]
	if nsValues == nil || !eval.Equal(nsValues.Value, eval.List([]eval.Value{eval.Int(3), eval.Int(4)})) {
		t.Fatalf("unexpected namespace import values: %#v", nsValues)
	}
}
