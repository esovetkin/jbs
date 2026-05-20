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
direct = sum(1, 2, 3, 4)
zero = sum()
from_range = sum(range(5))
spread_range = sum(*range(5))
product_value = prod((2, 3, 4))
product_direct = prod(2, 3, 4)
reduced = reduce(sum, [0, 1, 2, 3, 4])
mapped = map(sum, [[1, 2], [3, 4]])
	`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.Int(6)) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	direct := res.GlobalVarByName["direct"]
	if direct == nil || !eval.Equal(direct.Value, eval.Int(10)) {
		t.Fatalf("unexpected direct global: %#v", direct)
	}
	zero := res.GlobalVarByName["zero"]
	if zero == nil || !eval.Equal(zero.Value, eval.Int(0)) {
		t.Fatalf("unexpected zero global: %#v", zero)
	}
	fromRange := res.GlobalVarByName["from_range"]
	if fromRange == nil || !eval.Equal(fromRange.Value, eval.Int(10)) {
		t.Fatalf("unexpected from_range global: %#v", fromRange)
	}
	spreadRange := res.GlobalVarByName["spread_range"]
	if spreadRange == nil || !eval.Equal(spreadRange.Value, eval.Int(10)) {
		t.Fatalf("unexpected spread_range global: %#v", spreadRange)
	}
	productValue := res.GlobalVarByName["product_value"]
	if productValue == nil || !eval.Equal(productValue.Value, eval.Int(24)) {
		t.Fatalf("unexpected product_value global: %#v", productValue)
	}
	productDirect := res.GlobalVarByName["product_direct"]
	if productDirect == nil || !eval.Equal(productDirect.Value, eval.Int(24)) {
		t.Fatalf("unexpected product_direct global: %#v", productDirect)
	}
	reduced := res.GlobalVarByName["reduced"]
	if reduced == nil || !eval.Equal(reduced.Value, eval.Int(10)) {
		t.Fatalf("unexpected reduced global: %#v", reduced)
	}
	mapped := res.GlobalVarByName["mapped"]
	if mapped == nil || !eval.Equal(mapped.Value, eval.List([]eval.Value{eval.Int(3), eval.Int(7)})) {
		t.Fatalf("unexpected mapped global: %#v", mapped)
	}
	for _, gv := range []*GlobalVar{values, direct, zero, fromRange, spreadRange, productValue, productDirect, reduced, mapped} {
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

func TestAnalyzeHeadTailBuiltins(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
values = [1, 2, 3, 4]
first = head(values, n = 2)
last = tail(values, 2)
mapped = map(len, map(head, [[1, 2, 3, 4, 5, 6], [7, 8]]))
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	first := res.GlobalVarByName["first"]
	if first == nil || !eval.Equal(first.Value, eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("unexpected first global: %#v", first)
	}
	last := res.GlobalVarByName["last"]
	if last == nil || !eval.Equal(last.Value, eval.List([]eval.Value{eval.Int(3), eval.Int(4)})) {
		t.Fatalf("unexpected last global: %#v", last)
	}
	mapped := res.GlobalVarByName["mapped"]
	wantMapped := eval.List([]eval.Value{eval.Int(5), eval.Int(2)})
	if mapped == nil || !eval.Equal(mapped.Value, wantMapped) {
		t.Fatalf("unexpected mapped global: %#v", mapped)
	}
	for _, gv := range []*GlobalVar{first, last, mapped} {
		for _, dep := range []string{"head", "tail"} {
			if slices.Contains(gv.DependsOn, dep) {
				t.Fatalf("unshadowed builtin %q should not be recorded as dependency for %s: %#v", dep, gv.Name, gv.DependsOn)
			}
		}
		for _, key := range gv.DependsOnKeys {
			if key.Public == "head" || key.Public == "tail" {
				t.Fatalf("unshadowed builtin dependency key recorded for %s: %#v", gv.Name, gv.DependsOnKeys)
			}
		}
	}
}

func TestAnalyzeSortOrderBuiltins(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
values = sort([3, 1, 2])
perm = order(values)
mapped_count = len(map(sort, [[2, 1], [4, 3]]))
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(1), eval.Int(2), eval.Int(3)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	perm := res.GlobalVarByName["perm"]
	if perm == nil || !eval.Equal(perm.Value, eval.List([]eval.Value{eval.Int(0), eval.Int(1), eval.Int(2)})) {
		t.Fatalf("unexpected perm global: %#v", perm)
	}
	mappedCount := res.GlobalVarByName["mapped_count"]
	if mappedCount == nil || !eval.Equal(mappedCount.Value, eval.Int(2)) {
		t.Fatalf("unexpected mapped_count global: %#v", mappedCount)
	}
	for _, gv := range []*GlobalVar{values, perm, mappedCount} {
		for _, dep := range []string{"sort", "order"} {
			if slices.Contains(gv.DependsOn, dep) {
				t.Fatalf("unshadowed builtin %q should not be recorded as dependency for %s: %#v", dep, gv.Name, gv.DependsOn)
			}
		}
		for _, key := range gv.DependsOnKeys {
			if key.Public == "sort" || key.Public == "order" {
				t.Fatalf("unshadowed sort/order dependency key recorded for %s: %#v", gv.Name, gv.DependsOnKeys)
			}
		}
	}
}

func TestAnalyzeShadowedSortOrderDependency(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
sort = function(x) { 42 }
order = function(x) { 99 }
values = sort([3, 1, 2])
perm = order([3, 1, 2])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.Int(42)) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	perm := res.GlobalVarByName["perm"]
	if perm == nil || !eval.Equal(perm.Value, eval.Int(99)) {
		t.Fatalf("unexpected perm global: %#v", perm)
	}
	if !slices.Contains(values.DependsOn, "sort") {
		t.Fatalf("expected dependency on shadowed sort, got %#v", values.DependsOn)
	}
	if !slices.Contains(perm.DependsOn, "order") {
		t.Fatalf("expected dependency on shadowed order, got %#v", perm.DependsOn)
	}
}

func TestAnalyzeRbindBuiltin(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
a = table(id = [1], label = ["a"])
b = table(label = ["b"], id = [2])
out = rbind(a, b)
mapped = map(rbind, [a, b])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	out := res.GlobalVarByName["out"]
	if out == nil || !eval.IsComb(out.Value) || !slices.Equal(out.Value.C.Order, []string{"id", "label"}) || len(out.Value.C.Rows) != 2 {
		t.Fatalf("unexpected out global: %#v", out)
	}
	if !eval.Equal(out.Value.C.Rows[0].Values["id"].Value, eval.Int(1)) ||
		!eval.Equal(out.Value.C.Rows[1].Values["id"].Value, eval.Int(2)) ||
		!eval.Equal(out.Value.C.Rows[1].Values["label"].Value, eval.String("b")) {
		t.Fatalf("unexpected out rows: %#v", out.Value.C.Rows)
	}
	if !slices.Contains(out.DependsOn, "a") || !slices.Contains(out.DependsOn, "b") {
		t.Fatalf("expected dependencies on a and b, got %#v", out.DependsOn)
	}
	if slices.Contains(out.DependsOn, "rbind") {
		t.Fatalf("unshadowed rbind should not be recorded as dependency: %#v", out.DependsOn)
	}
	mapped := res.GlobalVarByName["mapped"]
	if mapped == nil || mapped.Value.Kind != eval.KindList || len(mapped.Value.L) != 2 || !eval.IsComb(mapped.Value.L[0]) || !eval.IsComb(mapped.Value.L[1]) {
		t.Fatalf("unexpected mapped global: %#v", mapped)
	}
	for _, gv := range []*GlobalVar{out, mapped} {
		for _, key := range gv.DependsOnKeys {
			if key.Public == "rbind" {
				t.Fatalf("unshadowed rbind dependency key recorded for %s: %#v", gv.Name, gv.DependsOnKeys)
			}
		}
	}
}

func TestAnalyzeFilterBuiltinFunction(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
values = filter([0, 1, 2], bool)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	for _, dep := range []string{"filter", "bool"} {
		if slices.Contains(values.DependsOn, dep) {
			t.Fatalf("unshadowed builtin %q should not be recorded as dependency: %#v", dep, values.DependsOn)
		}
	}
	for _, key := range values.DependsOnKeys {
		if key.Public == "filter" || key.Public == "bool" {
			t.Fatalf("unshadowed builtin dependency key recorded: %#v", values.DependsOnKeys)
		}
	}
}

func TestAnalyzeNamedBuiltinArgumentsAndNone(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
raw = ["1", "2"]
values = map(fn = int, values = raw)
missing = None
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(1), eval.Int(2)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	if !slices.Contains(values.DependsOn, "raw") {
		t.Fatalf("expected dependency on raw, got %#v", values.DependsOn)
	}
	for _, dep := range []string{"map", "int", "None"} {
		if slices.Contains(values.DependsOn, dep) {
			t.Fatalf("unshadowed builtin %q should not be a dependency: %#v", dep, values.DependsOn)
		}
	}
	missing := res.GlobalVarByName["missing"]
	if missing == nil || !eval.Equal(missing.Value, eval.Null()) {
		t.Fatalf("unexpected missing global: %#v", missing)
	}
	if slices.Contains(missing.DependsOn, "None") {
		t.Fatalf("None should not be recorded as a dependency: %#v", missing.DependsOn)
	}
}

func TestAnalyzeCallSpreadsAndFunctionRestDependencies(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
x = 1
vals = [2, 3]
kw = dict(extra = 4)
f = function(*x, **kwargs) { x }
out = f(*vals, **kw)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	out := res.GlobalVarByName["out"]
	if out == nil || !eval.Equal(out.Value, eval.List([]eval.Value{eval.Int(2), eval.Int(3)})) {
		t.Fatalf("unexpected out global: %#v", out)
	}
	for _, dep := range []string{"f", "vals", "kw"} {
		if !slices.Contains(out.DependsOn, dep) {
			t.Fatalf("expected dependency on %s, got %#v", dep, out.DependsOn)
		}
	}
	if slices.Contains(res.GlobalVarByName["f"].DependsOn, "x") {
		t.Fatalf("rest parameter x should shadow global x inside function body: %#v", res.GlobalVarByName["f"].DependsOn)
	}
}

func TestAnalyzeFilterUserPredicateDependency(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
keep = function(x) { x > 1 }
values = filter([0, 1, 2], keep)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(2)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	if !slices.Contains(values.DependsOn, "keep") {
		t.Fatalf("expected dependency on keep, got %#v", values.DependsOn)
	}
	if slices.Contains(values.DependsOn, "filter") {
		t.Fatalf("unshadowed filter should not be recorded as dependency: %#v", values.DependsOn)
	}
}

func TestAnalyzeShadowedFilterDependency(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
filter = function(values, fn) { [42] }
values = filter([0, 1, 2], bool)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	values := res.GlobalVarByName["values"]
	if values == nil || !eval.Equal(values.Value, eval.List([]eval.Value{eval.Int(42)})) {
		t.Fatalf("unexpected values global: %#v", values)
	}
	if !slices.Contains(values.DependsOn, "filter") {
		t.Fatalf("expected dependency on shadowed filter, got %#v", values.DependsOn)
	}
	if slices.Contains(values.DependsOn, "bool") {
		t.Fatalf("unshadowed bool should not be recorded as dependency: %#v", values.DependsOn)
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

func TestAnalyzeShadowedHeadTailDependencies(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
head = function(values, n = 5) { [42] }
tail = function(values, n = 5) { [99] }
first = head([1, 2, 3])
last = tail([1, 2, 3])
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	first := res.GlobalVarByName["first"]
	if first == nil || !eval.Equal(first.Value, eval.List([]eval.Value{eval.Int(42)})) {
		t.Fatalf("unexpected first global: %#v", first)
	}
	if !slices.Contains(first.DependsOn, "head") {
		t.Fatalf("expected shadowed head dependency, got %#v", first.DependsOn)
	}
	last := res.GlobalVarByName["last"]
	if last == nil || !eval.Equal(last.Value, eval.List([]eval.Value{eval.Int(99)})) {
		t.Fatalf("unexpected last global: %#v", last)
	}
	if !slices.Contains(last.DependsOn, "tail") {
		t.Fatalf("expected shadowed tail dependency, got %#v", last.DependsOn)
	}
}

func TestAnalyzeShadowedRbindDependency(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
rbind = function(left, right) { 42 }
a = table(id = [1])
b = table(id = [2])
out = rbind(a, b)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	out := res.GlobalVarByName["out"]
	if out == nil || !eval.Equal(out.Value, eval.Int(42)) {
		t.Fatalf("unexpected out global: %#v", out)
	}
	if !slices.Contains(out.DependsOn, "rbind") {
		t.Fatalf("expected shadowed rbind dependency, got %#v", out.DependsOn)
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
