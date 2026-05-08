package sema

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestAnalyzeValidatesBenchmarksGlobal(t *testing.T) {
	src := `
jbs_benchmarks = {"small": "missing"}

do run {
        echo ok
}

analyse run {
        (x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("benchmarks.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{}, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.String(), `jbs_benchmarks["small"] references unknown analyse block "missing"`) {
		t.Fatalf("missing benchmark diagnostic: %s", diags.String())
	}
}

func TestAnalyzeAcceptsConfiguredBenchmarks(t *testing.T) {
	src := `
jbs_benchmarks = {"small": "run"}
x = 1

do run with x {
        echo ok
}

analyse run {
        (x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("benchmarks.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}
