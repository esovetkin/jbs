package sema

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestAnalyzeSetSeedSampleReproducible(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
setseed(7)
a = sample(range(10), size = 4)
setseed(7)
b = sample(range(10), size = 4)
same = all(a == b)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	a := res.GlobalVarByName["a"]
	b := res.GlobalVarByName["b"]
	same := res.GlobalVarByName["same"]
	if a == nil || b == nil || !eval.Equal(a.Value, b.Value) {
		t.Fatalf("seeded samples differ: a=%#v b=%#v", a, b)
	}
	if same == nil || !eval.Equal(same.Value, eval.Bool(true)) {
		t.Fatalf("unexpected equality global: %#v", same)
	}
	if len(res.TopLevelExprs) != 2 || res.TopLevelExprs[0].Echo || res.TopLevelExprs[1].Echo {
		t.Fatalf("setseed() top-level expressions should be quiet: %#v", res.TopLevelExprs)
	}
	if slices.Contains(a.DependsOn, "sample") || slices.Contains(a.DependsOn, "setseed") {
		t.Fatalf("unshadowed random builtins should not be dependencies: %#v", a.DependsOn)
	}
}

func TestAnalyzeSampledTableCanBeUsedByDoWith(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
setseed(1)
cases = sample(table(id = range(5), label = ["a", "b", "c", "d", "e"]), size = 2)

do s with cases {
    echo "$id $label"
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	cases := res.GlobalVarByName["cases"]
	if cases == nil || !eval.IsComb(cases.Value) || len(cases.Value.C.Rows) != 2 {
		t.Fatalf("unexpected sampled cases global: %#v", cases)
	}
	plan := res.StepScopeByName["s"]
	if plan == nil {
		t.Fatalf("missing step plan for sampled table")
	}
	if len(plan.EffectiveValues["id"]) != 2 || len(plan.EffectiveValues["label"]) != 2 {
		t.Fatalf("expected sampled table values in step plan, got %#v", plan.EffectiveValues)
	}
}

func TestAnalyzeRandomBuiltinsCanBeShadowed(t *testing.T) {
	res, diags := analyzeBuiltinFunctionSource(t, `
sample = function(x) { x }
setseed = function(x) { 42 }
out = sample(1)
seed = setseed(1)
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	out := res.GlobalVarByName["out"]
	seed := res.GlobalVarByName["seed"]
	if out == nil || !eval.Equal(out.Value, eval.Int(1)) {
		t.Fatalf("unexpected shadowed sample result: %#v", out)
	}
	if seed == nil || !eval.Equal(seed.Value, eval.Int(42)) {
		t.Fatalf("unexpected shadowed setseed result: %#v", seed)
	}
	if !slices.Contains(out.DependsOn, "sample") {
		t.Fatalf("expected dependency on shadowed sample, got %#v", out.DependsOn)
	}
	if !slices.Contains(seed.DependsOn, "setseed") {
		t.Fatalf("expected dependency on shadowed setseed, got %#v", seed.DependsOn)
	}
}
