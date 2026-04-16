package sema

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/parser"
)

func builtinGlobalsForSemaTests() map[string]eval.Value {
	return map[string]eval.Value{
		"jbs_name":    eval.String("jbs_benchmark"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}
}

func analyzeSourceForW310Test(t *testing.T, file, src string) (*Result, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse(file, src, diags)
	res := Analyze(prog, builtinGlobalsForSemaTests(), diags)
	return res, diags
}

func hasW310For(diags *diag.Diagnostics, variable, source string) bool {
	want := "exposed variable '" + variable + "' from param '" + source + "'"
	for _, item := range diags.Items {
		if item.Code == string(diag.CodeW310) && strings.Contains(item.Message, want) {
			return true
		}
	}
	return false
}

func TestW310NoWarningForGlobalsUsedTransitivelyViaCombImport(t *testing.T) {
	// Baseline before fix: W310 was emitted for `x` and `a` here.
	src := `
x = (1, 2)
a = ("a", "b", "c")

params = comb(a * x)

do ex_step with params {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}

analyse ex_step {
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"
        (a as "name of a column", x, number, letter)
}
`
	_, diags := analyzeSourceForW310Test(t, "transitive.jbs", src)
	if hasW310For(diags, "x", "x") {
		t.Fatalf("did not expect W310 for x, got: %s", diags.String())
	}
	if hasW310For(diags, "a", "a") {
		t.Fatalf("did not expect W310 for a, got: %s", diags.String())
	}
}

func TestW310StillWarnsForTrulyUnusedGlobal(t *testing.T) {
	src := `
x = (1,2)
a = ("a","b")
params = comb(a * x)
unused = (10,20)

do s with params { echo ${a} ${x} }
`
	_, diags := analyzeSourceForW310Test(t, "unused.jbs", src)
	if !hasW310For(diags, "unused", "unused") {
		t.Fatalf("expected W310 for unused global, got: %s", diags.String())
	}
	if hasW310For(diags, "x", "x") || hasW310For(diags, "a", "a") {
		t.Fatalf("did not expect W310 for used globals x/a, got: %s", diags.String())
	}
}

func TestW310NoWarningForTransitiveGlobalChain(t *testing.T) {
	src := `
x = (1,2)
m = comb(x)
p = comb(m)

do s with p { echo ${x} }
`
	_, diags := analyzeSourceForW310Test(t, "chain.jbs", src)
	if hasW310For(diags, "x", "x") {
		t.Fatalf("did not expect W310 for x in transitive chain, got: %s", diags.String())
	}
	if hasW310For(diags, "x", "m") {
		t.Fatalf("did not expect W310 for m.x in transitive chain, got: %s", diags.String())
	}
}

func TestW310NoWarningForQualifiedDependencyGlobal(t *testing.T) {
	src := `
x = (1,2)
a = ("a","b")
params = comb(a * x)
only_a = params[a]

do s with only_a { echo ${a} }
`
	_, diags := analyzeSourceForW310Test(t, "qualified.jbs", src)
	if hasW310For(diags, "a", "a") || hasW310For(diags, "x", "x") {
		t.Fatalf("did not expect W310 for contributors in qualified dependency chain, got: %s", diags.String())
	}
}

func TestPropagateUsedByGlobalDepsCycleSafe(t *testing.T) {
	a := sourceKey{Kind: SourceKindParam, Name: "a"}
	b := sourceKey{Kind: SourceKindParam, Name: "b"}

	used := map[sourceKey]map[string]bool{
		a: {"va": true},
	}
	exposed := map[sourceKey]map[string]diag.Span{
		a: {"va": diag.Span{}},
		b: {"vb": diag.Span{}},
	}
	deps := map[sourceKey][]sourceKey{
		a: {b},
		b: {a},
	}

	propagateUsedByGlobalDeps(used, exposed, deps)
	if !used[b]["vb"] {
		t.Fatalf("expected cycle-safe propagation to mark b.vb used")
	}
	if !used[a]["va"] {
		t.Fatalf("expected original mark for a.va to remain")
	}
}

func TestBuildGlobalSourceDeps(t *testing.T) {
	a := sourceKey{Kind: SourceKindParam, Name: "a"}
	x := sourceKey{Kind: SourceKindParam, Name: "x"}
	params := sourceKey{Kind: SourceKindParam, Name: "params"}

	res := &Result{
		GlobalVarOrder: []string{"a", "x", "params"},
		GlobalVarByName: map[string]*GlobalVar{
			"a":      {Name: "a"},
			"x":      {Name: "x"},
			"params": {Name: "params", DependsOn: []string{"x", "a", "x"}},
		},
		ImportSourceByName: map[string]*ImportSource{
			"a":      {Name: "a", Kind: SourceKindParam},
			"x":      {Name: "x", Kind: SourceKindParam},
			"params": {Name: "params", Kind: SourceKindParam},
		},
	}
	exposed := map[sourceKey]map[string]diag.Span{
		a:      {"a": diag.Span{}},
		x:      {"x": diag.Span{}},
		params: {"a": diag.Span{}, "x": diag.Span{}},
	}

	deps := buildGlobalSourceDeps(res, exposed)
	got := deps[params]
	want := []sourceKey{a, x}
	if len(got) != len(want) {
		t.Fatalf("unexpected dependency count: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected dependency order/content at %d: got=%v want=%v", i, got, want)
		}
	}
}
