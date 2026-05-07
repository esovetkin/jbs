package run

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestAnalysePlansByStepBuildsHeaders(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
cases = table(x=[1])
do run with cases {
    echo "pair=AA-17" > out.log
}
analyse run {
    pair = "pair=([A-Z]+)-([0-9]+)" in "out.log"
    n = "n=%d" in "out.log"
    (x as "X", pair as "Pair", n)
}
`)
	analysis := plan.Analyses["run"]
	if analysis.Step != "run" || analysis.CSV != "analyse.csv" {
		t.Fatalf("unexpected analyse plan identity: %#v", analysis)
	}
	want := []string{"run_id", "X", "Pair.0", "Pair.1", "n"}
	if strings.Join(analysis.Header, "|") != strings.Join(want, "|") {
		t.Fatalf("header = %#v, want %#v", analysis.Header, want)
	}
	if len(analysis.Columns) != 3 {
		t.Fatalf("columns = %#v", analysis.Columns)
	}
	if analysis.Patterns["pair"].GroupCount != 2 {
		t.Fatalf("pair group count = %d, want 2", analysis.Patterns["pair"].GroupCount)
	}
	if analysis.Patterns["n"].GroupCount != 1 {
		t.Fatalf("n group count = %d, want 1", analysis.Patterns["n"].GroupCount)
	}
	if plan.Manifest.Steps[0].AnalyseCSV != "analyse.csv" {
		t.Fatalf("manifest analyse csv = %q", plan.Manifest.Steps[0].AnalyseCSV)
	}
}

func TestAnalysePlansByStepRejectsDuplicateTarget(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    ()
}
analyse run {
    ()
}
`)
	if err == nil || !strings.Contains(err.Error(), "multiple analyse blocks target step") {
		t.Fatalf("expected duplicate analyse error, got %v", err)
	}
}

func TestAnalysePlansByStepRejectsInvalidRegex(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    bad = "(" in "out.log"
    (bad)
}
`)
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestAnalysePlansByStepRejectsZeroCapturePattern(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    bad = "literal" in "out.log"
    (bad)
}
`)
	if err == nil || !strings.Contains(err.Error(), "at least one capture group") {
		t.Fatalf("expected zero-capture error, got %v", err)
	}
}

func TestAnalysePlansByStepIgnoresUnusedExtractionAssignments(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
cases = table(x=[1])
do run with cases {
    echo ok
}
analyse run {
    unused = "literal" in "missing.log"
    (x)
}
`)
	analysis := plan.Analyses["run"]
	if len(analysis.Patterns) != 0 {
		t.Fatalf("unused extraction should not be compiled, got %#v", analysis.Patterns)
	}
	want := []string{"run_id", "x"}
	if strings.Join(analysis.Header, "|") != strings.Join(want, "|") {
		t.Fatalf("header = %#v, want %#v", analysis.Header, want)
	}
}

func buildPlanFromSourceErr(t *testing.T, source string) (runtimePlan, error) {
	t.Helper()
	diags := &diag.Diagnostics{}
	cwd := t.TempDir()
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return buildRuntimePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: "test.jbs",
	}, diags)
}
