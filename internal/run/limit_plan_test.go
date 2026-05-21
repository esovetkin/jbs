package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

func TestBuildRuntimeSuitePlanLimitsAnalysedBranch(t *testing.T) {
	suite := buildSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
cases = table(x = [1, 2, 3])

do prep with cases {
        echo "$x" > prepared.txt
}

do run after prep {
        cat prep/prepared.txt > out.log
}

analyse run {
        value = "%d" in "out.log"
        (x, value)
}
`, Options{Limit: 1})
	plan := suite.Plans[0]
	if plan.Manifest.WorkLimit != 1 {
		t.Fatalf("work limit = %d, want 1", plan.Manifest.WorkLimit)
	}
	if got := len(manifestWorkForStep(plan.Manifest.Work, "prep")); got != 1 {
		t.Fatalf("prep work count = %d, want 1", got)
	}
	runWork := manifestWorkForStep(plan.Manifest.Work, "run")
	if len(runWork) != 1 {
		t.Fatalf("run work count = %d, want 1", len(runWork))
	}
	if runWork[0].Row != 0 || len(runWork[0].Deps) != 1 || runWork[0].Deps[0].Step != "prep" || runWork[0].Deps[0].Row != 0 {
		t.Fatalf("unexpected limited run work: %#v", runWork[0])
	}
}

func TestBuildRuntimeSuitePlanLimitsConfiguredComponentsIndependently(t *testing.T) {
	suite := buildSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run_small", "large": "run_large"}
cases = table(x = [1, 2, 3])

do prep with cases {
        echo "$x"
}

do run_small after prep {
        echo "$x" > out.log
}

do run_large after prep {
        echo "$x" > out.log
}

analyse run_small {
        value = "%d" in "out.log"
        (value)
}

analyse run_large {
        value = "%d" in "out.log"
        (value)
}
`, Options{Limit: 1})
	if len(suite.Plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(suite.Plans))
	}
	for _, plan := range suite.Plans {
		if got := len(manifestWorkForStep(plan.Manifest.Work, "prep")); got != 1 {
			t.Fatalf("%s prep work count = %d, want 1", plan.ComponentName, got)
		}
		target := "run_" + plan.ComponentName
		if got := len(manifestWorkForStep(plan.Manifest.Work, target)); got != 1 {
			t.Fatalf("%s target work count = %d, want 1", plan.ComponentName, got)
		}
	}
}

func TestBuildRuntimeSuitePlanLimitSelectedBenchmarkOnly(t *testing.T) {
	suite := buildSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run_small", "large": "run_large"}
cases = table(x = [1, 2])

do run_small with cases {
        echo "$x" > out.log
}

do run_large with cases {
        echo "$x" > out.log
}

analyse run_small {
        value = "%d" in "out.log"
        (value)
}

analyse run_large {
        value = "%d" in "out.log"
        (value)
}
`, Options{Benchmark: "small", Limit: 1})
	if len(suite.Plans) != 1 || suite.Plans[0].ComponentName != "small" {
		t.Fatalf("unexpected selected suite: %#v", suite)
	}
	plan := suite.Plans[0]
	if got := manifestStepNames(plan.Manifest); strings.Join(got, ",") != "run_small" {
		t.Fatalf("unexpected selected steps: %#v", got)
	}
	if got := len(manifestWorkForStep(plan.Manifest.Work, "run_small")); got != 1 {
		t.Fatalf("run_small work count = %d, want 1", got)
	}
}

func TestBuildRuntimeSuitePlanLimitWildcardBenchmarkUsesFullTargets(t *testing.T) {
	suite := buildSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
jbs_benchmarks = {"all": "*"}
cases = table(x = [1, 2, 3])

do prep with cases {
        echo "$x"
}

do run after prep {
        echo "$x" > out.log
}

analyse run {
        value = "%d" in "out.log"
        (value)
}
`, Options{Benchmark: "all", Limit: 1})
	plan := suite.Plans[0]
	if got := manifestStepNames(plan.Manifest); strings.Join(got, ",") != "prep,run" {
		t.Fatalf("unexpected wildcard limited steps: %#v", got)
	}
	if got := len(manifestWorkForStep(plan.Manifest.Work, "prep")); got != 1 {
		t.Fatalf("prep work count = %d, want 1", got)
	}
	if got := len(manifestWorkForStep(plan.Manifest.Work, "run")); got != 1 {
		t.Fatalf("run work count = %d, want 1", got)
	}
}

func TestBuildRuntimeSuitePlanLimitDoOnlyUsesTerminalStep(t *testing.T) {
	suite := buildSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
jbs_benchmarks = {"smoke": "run"}
cases = table(x = [1, 2])
extra = table(y = ["a", "b"])

do prep with cases {
        echo "$x"
}

do run after prep with extra {
        echo "$x $y"
}
`, Options{Benchmark: "smoke", Limit: 1})
	plan := suite.Plans[0]
	if len(plan.Analyses) != 0 {
		t.Fatalf("do-only benchmark should not select analyses: %#v", plan.Analyses)
	}
	if got := len(manifestWorkForStep(plan.Manifest.Work, "prep")); got != 1 {
		t.Fatalf("prep work count = %d, want 1", got)
	}
	if got := len(manifestWorkForStep(plan.Manifest.Work, "run")); got != 1 {
		t.Fatalf("run work count = %d, want 1", got)
	}
}

func TestBuildRuntimeSuitePlanLimitDropsFSubForPrunedSteps(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "unused.tpl"), []byte("TOKEN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := buildSuiteFromSourceWithOptionsInDir(t, cwd, `
jbs_name = "bench"
cases = table(x = [1, 2])

do run with cases {
        echo "$x" > out.log
}

do unused
        fsub "unused.tpl" { "TOKEN": "value" }
{
        echo unused
}

analyse run {
        value = "%d" in "out.log"
        (value)
}
`, Options{Limit: 1})
	plan := suite.Plans[0]
	if _, ok := plan.FileSubs["unused"]; ok {
		t.Fatalf("pruned fsub step retained: %#v", plan.FileSubs)
	}
	if len(plan.TemplateHashes) != 0 {
		t.Fatalf("pruned fsub template was snapshotted: %#v", plan.TemplateHashes)
	}
	if got := manifestStepNames(plan.Manifest); strings.Join(got, ",") != "run" {
		t.Fatalf("unexpected steps after limit: %#v", got)
	}
}

func TestBuildParameterPlanSuiteAppliesLimit(t *testing.T) {
	suite := buildParameterSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
cases = table(x = [1, 2, 3])

do prep with cases {
        echo "$x" > prepared.txt
}

do run after prep {
        cat prep/prepared.txt > out.log
}

analyse run {
        value = "%d" in "out.log"
        (x, value)
}
`, Options{Limit: 1})
	if len(suite.Plans) != 1 {
		t.Fatalf("plans = %d, want 1", len(suite.Plans))
	}
	plan := suite.Plans[0].WorkPlan
	if got := len(planWorkForStep(plan.Work, "prep")); got != 1 {
		t.Fatalf("prep work count = %d, want 1", got)
	}
	if got := len(planWorkForStep(plan.Work, "run")); got != 1 {
		t.Fatalf("run work count = %d, want 1", got)
	}
}

func TestBuildParameterPlanSuiteLimitsConfiguredComponentsIndependently(t *testing.T) {
	suite := buildParameterSuiteFromSourceWithOptions(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run_small", "large": "run_large"}
cases = table(x = [1, 2, 3])

do prep with cases {
        echo "$x"
}

do run_small after prep {
        echo "$x" > out.log
}

do run_large after prep {
        echo "$x" > out.log
}

analyse run_small {
        value = "%d" in "out.log"
        (value)
}

analyse run_large {
        value = "%d" in "out.log"
        (value)
}
`, Options{Limit: 1})
	if len(suite.Plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(suite.Plans))
	}
	for _, plan := range suite.Plans {
		if got := len(planWorkForStep(plan.WorkPlan.Work, "prep")); got != 1 {
			t.Fatalf("%s prep work count = %d, want 1", plan.ComponentName, got)
		}
		target := "run_" + plan.ComponentName
		if got := len(planWorkForStep(plan.WorkPlan.Work, target)); got != 1 {
			t.Fatalf("%s target work count = %d, want 1", plan.ComponentName, got)
		}
	}
}

func buildSuiteFromSourceWithOptions(t *testing.T, source string, opts Options) runtimeSuitePlan {
	t.Helper()
	return buildSuiteFromSourceWithOptionsInDir(t, t.TempDir(), source, opts)
}

func buildSuiteFromSourceWithOptionsInDir(t *testing.T, cwd string, source string, opts Options) runtimeSuitePlan {
	t.Helper()
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	opts.Result = res
	opts.Sources = loadRes.Sources
	opts.ProgramFile = filepath.Join(cwd, "test.jbs")
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		t.Fatal(err)
	}
	return suite
}

func buildParameterSuiteFromSourceWithOptions(t *testing.T, source string, opts Options) ParameterPlanSuite {
	t.Helper()
	cwd := t.TempDir()
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	opts.Result = res
	opts.Sources = loadRes.Sources
	opts.ProgramFile = filepath.Join(cwd, "test.jbs")
	suite, err := BuildParameterPlanSuite(opts, diags)
	if err != nil {
		t.Fatal(err)
	}
	return suite
}

func manifestWorkForStep(work []ManifestWork, step string) []ManifestWork {
	out := make([]ManifestWork, 0)
	for _, item := range work {
		if item.Step == step {
			out = append(out, item)
		}
	}
	return out
}

func planWorkForStep(work []workplan.WorkPackage, step string) []workplan.WorkPackage {
	out := make([]workplan.WorkPackage, 0)
	for _, item := range work {
		if item.StepName == step {
			out = append(out, item)
		}
	}
	return out
}
