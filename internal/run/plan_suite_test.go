package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestBuildRuntimeSuitePlanConfiguredBenchmarks(t *testing.T) {
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run_small", "large": "run_large"}

do prep {
        echo prep
}
do run_small after prep {
        echo small
}
do run_large after prep {
        echo large
}
do unused {
        echo unused
}
analyse run_small {
        value = "small: %d" in "out.log"
        (value)
}
analyse run_large {
        value = "large: %d" in "out.log"
        (value)
}
`, "")
	if !suite.Configured || len(suite.Plans) != 2 {
		t.Fatalf("unexpected suite: %#v", suite)
	}
	small := suite.Plans[0]
	if small.RootDir != "bench/small" || small.ComponentName != "small" || small.TablePrefix != "bench_small" {
		t.Fatalf("unexpected small plan identity: %#v", small)
	}
	if len(small.Analyses) != 1 {
		t.Fatalf("small analyses = %#v", small.Analyses)
	}
	if _, ok := small.Analyses["run_small"]; !ok {
		t.Fatalf("small plan missing run_small analysis: %#v", small.Analyses)
	}
	stepNames := make([]string, 0, len(small.Manifest.Steps))
	for _, step := range small.Manifest.Steps {
		stepNames = append(stepNames, step.Name)
	}
	if strings.Join(stepNames, ",") != "prep,run_small" {
		t.Fatalf("small steps = %#v", stepNames)
	}
}

func TestBuildRuntimeSuitePlanSelectedBenchmark(t *testing.T) {
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run_small", "large": "run_large"}
do run_small {
        echo small
}
do run_large {
        echo large
}
analyse run_small {
        value = "small: %d" in "out.log"
        (value)
}
analyse run_large {
        value = "large: %d" in "out.log"
        (value)
}
`, "large")
	if len(suite.Plans) != 1 || suite.Plans[0].ComponentName != "large" || suite.SelectedName != "large" {
		t.Fatalf("unexpected selected suite: %#v", suite)
	}
}

func TestBuildRuntimeSuitePlanConfiguredBenchmarkDoOnlyTarget(t *testing.T) {
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
jbs_benchmarks = {"smoke": "prepare"}

do prepare {
        echo prepare
}
do run after prepare {
        echo run
}
analyse run {
        value = "value: %d" in "out.log"
        (value)
}
`, "smoke")
	if len(suite.Plans) != 1 || suite.Plans[0].ComponentName != "smoke" {
		t.Fatalf("unexpected suite: %#v", suite)
	}
	plan := suite.Plans[0]
	if len(plan.Analyses) != 0 {
		t.Fatalf("do-only target should not select analyses: %#v", plan.Analyses)
	}
	if got := manifestStepNames(plan.Manifest); strings.Join(got, ",") != "prepare" {
		t.Fatalf("unexpected steps: %#v", got)
	}
	if plan.Manifest.Steps[0].HasAnalyse() {
		t.Fatalf("do-only target should not have analyse metadata: %#v", plan.Manifest.Steps[0])
	}
}

func TestBuildRuntimeSuitePlanConfiguredBenchmarkMixedTargets(t *testing.T) {
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": ["prepare", "run"]}

do prepare {
        echo prepare
}
do run after prepare {
        echo run
}
do unused {
        echo unused
}
analyse prepare {
        value = "prepare: %d" in "out.log"
        (value)
}
analyse run {
        value = "run: %d" in "out.log"
        (value)
}
`, "small")
	plan := suite.Plans[0]
	if got := manifestStepNames(plan.Manifest); strings.Join(got, ",") != "prepare,run" {
		t.Fatalf("unexpected steps: %#v", got)
	}
	if len(plan.Analyses) != 2 {
		t.Fatalf("mixed targets should select both targeted analyses: %#v", plan.Analyses)
	}
	if !plan.Manifest.Steps[0].HasAnalyse() || !plan.Manifest.Steps[1].HasAnalyse() {
		t.Fatalf("targeted analyse metadata missing: %#v", plan.Manifest.Steps)
	}
}

func TestBuildRuntimeSuitePlanConfiguredBenchmarkDoesNotAnalyseDependenciesImplicitly(t *testing.T) {
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run"}

do prepare {
        echo prepare
}
do run after prepare {
        echo run
}
analyse prepare {
        value = "prepare: %d" in "out.log"
        (value)
}
analyse run {
        value = "run: %d" in "out.log"
        (value)
}
`, "small")
	plan := suite.Plans[0]
	if got := manifestStepNames(plan.Manifest); strings.Join(got, ",") != "prepare,run" {
		t.Fatalf("unexpected steps: %#v", got)
	}
	if len(plan.Analyses) != 1 {
		t.Fatalf("dependency analysis should not be selected implicitly: %#v", plan.Analyses)
	}
	if _, ok := plan.Analyses["run"]; !ok {
		t.Fatalf("missing targeted run analysis: %#v", plan.Analyses)
	}
	if plan.Manifest.Steps[0].HasAnalyse() {
		t.Fatalf("dependency analyse metadata should not be selected: %#v", plan.Manifest.Steps[0])
	}
	if !plan.Manifest.Steps[1].HasAnalyse() {
		t.Fatalf("target analyse metadata should be selected: %#v", plan.Manifest.Steps[1])
	}
}

func TestBuildRuntimePlanEmitsWithValuesForManifestAndRunScript(t *testing.T) {
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
series = (dict(k=1), dict(k=2))
rows = dict(x = [1,2], y = ["a","b"])

do series_step with series {
        echo "${series}"
}

do dict_step with rows {
        echo "${x} ${y}"
}
`, "")
	plan := suite.Plans[0]
	seenSeries := map[string]bool{}
	seenRows := map[string]bool{}
	for _, work := range plan.Manifest.Work {
		switch work.Step {
		case "series_step":
			seenSeries[work.Values["series"]] = true
		case "dict_step":
			seenRows[work.Values["x"]+"|"+work.Values["y"]] = true
		}
	}
	for _, want := range []string{"{k:1}", "{k:2}"} {
		if !seenSeries[want] {
			t.Fatalf("missing stringified series value %q in manifest: %#v", want, plan.Manifest.Work)
		}
	}
	for _, want := range []string{"1|a", "2|b"} {
		if !seenRows[want] {
			t.Fatalf("missing dict row value %q in manifest: %#v", want, plan.Manifest.Work)
		}
	}

	store, warnings, err := CreateRunDirectoryWithInitial(filepath.Join(t.TempDir(), "bench"), plan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected file-substitution warnings: %#v", warnings)
	}
	for _, work := range store.Manifest.Work {
		if work.Step != "dict_step" || work.Values["x"] != "2" {
			continue
		}
		script, err := os.ReadFile(filepath.Join(store.WorkDir(work), "run.sh"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(script)
		if !strings.Contains(text, "x='2'") || !strings.Contains(text, "y='b'") {
			t.Fatalf("run.sh missing dict-derived shell assignments:\n%s", text)
		}
		return
	}
	t.Fatalf("did not find dict_step workpackage for x=2: %#v", store.Manifest.Work)
}

func TestBuildRuntimeSuitePlanRejectsBenchmarkWithoutConfig(t *testing.T) {
	diags := &diag.Diagnostics{}
	cwd := t.TempDir()
	loadRes, err := imports.LoadAndExpandSource("test.jbs", `
do run {
        echo run
}
analyse run {
        value = "value: %d" in "out.log"
        (value)
}
`, cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	_, err = buildRuntimeSuitePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: "test.jbs",
		Benchmark:   "small",
	}, diags)
	if err == nil || !strings.Contains(err.Error(), "--benchmark requires non-empty jbs_benchmarks") {
		t.Fatalf("expected benchmark config error, got %v", err)
	}
}

func buildSuiteFromSource(t *testing.T, source, benchmark string) runtimeSuitePlan {
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
	suite, err := buildRuntimeSuitePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: "test.jbs",
		Benchmark:   benchmark,
	}, diags)
	if err != nil {
		t.Fatal(err)
	}
	return suite
}

func manifestStepNames(manifest Manifest) []string {
	names := make([]string, 0, len(manifest.Steps))
	for _, step := range manifest.Steps {
		names = append(names, step.Name)
	}
	return names
}
