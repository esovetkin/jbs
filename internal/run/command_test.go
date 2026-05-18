package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestRunDryRunAndContinueCommands(t *testing.T) {
	cwd := t.TempDir()
	withWorkingDir(t, cwd)
	opts := commandOptionsFromSource(t, cwd, `
jbs_name = "cmd"

do s {
        echo run
}

analyse s {
        value = "value: %w" in "out.log"
        (value)
}
`)

	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr

	if err := DryRun(context.Background(), opts); err != nil {
		t.Fatalf("DryRun error = %v", err)
	}
	status, err := LoadRootStatus(filepath.Join(cwd, "cmd", "000000", "status"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusNotStarted {
		t.Fatalf("dry-run status = %s, want %s", status.Status, StatusNotStarted)
	}

	withRunWorkProcess(t, func(_ context.Context, dir string) processResult {
		if err := os.WriteFile(filepath.Join(dir, "out.log"), []byte("value: ok\n"), 0o644); err != nil {
			return processResult{Status: StatusError, Err: err}
		}
		code := 0
		return processResult{Status: StatusFinished, ExitCode: &code}
	})

	stdout.Reset()
	if err := Continue(context.Background(), opts); err != nil {
		t.Fatalf("Continue error = %v", err)
	}
	status, err = LoadRootStatus(filepath.Join(cwd, "cmd", "000000", "status"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusFinished {
		t.Fatalf("continued status = %s, want %s", status.Status, StatusFinished)
	}
	if data, err := os.ReadFile(filepath.Join(cwd, "cmd", "000000", "s", "analyse.csv")); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(string(data), "000000,ok\n") {
		t.Fatalf("continued analyse output = %q, want captured row", string(data))
	}
	if got := stdout.String(); !strings.Contains(got, "analysis") || !strings.Contains(got, "analyse.csv") {
		t.Fatalf("Continue stdout = %q, want post-run analyse summary", got)
	}

	stdout.Reset()
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run error = %v", err)
	}
	status, err = LoadRootStatus(filepath.Join(cwd, "cmd", "000001", "status"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusFinished {
		t.Fatalf("run status = %s, want %s", status.Status, StatusFinished)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no warnings", stderr.String())
	}
}

func TestCommandsReturnBuildErrors(t *testing.T) {
	for name, fn := range map[string]func(context.Context, Options) error{
		"Run":      Run,
		"DryRun":   DryRun,
		"Continue": Continue,
	} {
		t.Run(name, func(t *testing.T) {
			err := fn(context.Background(), Options{})
			if err == nil || !strings.Contains(err.Error(), "missing analysis result") {
				t.Fatalf("error = %v, want missing analysis result", err)
			}
		})
	}
}

func TestCommandsReturnWorkplanDiagnosticErrors(t *testing.T) {
	opts := workplanConflictOptions()
	for name, fn := range map[string]func(context.Context, Options) error{
		"Run":      Run,
		"DryRun":   DryRun,
		"Continue": Continue,
	} {
		t.Run(name, func(t *testing.T) {
			err := fn(context.Background(), opts)
			if err == nil || err.Error() != "failed to build runtime workplan" {
				t.Fatalf("error = %v, want runtime workplan diagnostic error", err)
			}
		})
	}
}

func TestCommandsReturnPreparationAndContinueErrors(t *testing.T) {
	cwd := t.TempDir()
	withWorkingDir(t, cwd)
	if err := os.WriteFile(filepath.Join(cwd, "occupied"), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := commandOptionsFromSource(t, cwd, `
jbs_name = "occupied"
do s {
        echo run
}
`)

	if err := DryRun(context.Background(), opts); err == nil {
		t.Fatal("expected DryRun preparation error")
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected Run preparation error")
	}

	missingOpts := commandOptionsFromSource(t, cwd, `
jbs_name = "missing"
do s {
        echo run
}
`)
	err := Continue(context.Background(), missingOpts)
	if err == nil || !strings.Contains(err.Error(), "cannot lock benchmark root missing") {
		t.Fatalf("Continue error = %v, want missing root lock error", err)
	}
}

func TestPrintEventsOrdersBySequenceAndQuotesStrings(t *testing.T) {
	printEvents(nil, []sema.PrintEvent{{Seq: 1, Values: []eval.Value{eval.String("ignored")}}})

	var empty bytes.Buffer
	printEvents(&empty, nil)
	if empty.Len() != 0 {
		t.Fatalf("empty events output = %q, want none", empty.String())
	}

	events := []sema.PrintEvent{
		{Seq: 20, Values: []eval.Value{eval.String("second")}},
		{Seq: 10, Values: []eval.Value{eval.String("first")}},
	}
	var out bytes.Buffer
	printEvents(&out, events)
	if got, want := out.String(), "\"first\"\n\"second\"\n"; got != want {
		t.Fatalf("printEvents output = %q, want %q", got, want)
	}
}

func TestPrintFileSubstitutionWarnings(t *testing.T) {
	printFileSubstitutionWarnings(nil, []preparedStore{{Warnings: []FileSubstitutionWarning{{Step: "ignored"}}}})

	var out bytes.Buffer
	printFileSubstitutionWarnings(&out, []preparedStore{{
		Warnings: []FileSubstitutionWarning{{
			Step:     "s",
			Row:      3,
			DestName: "config.sh",
			Pattern:  "%x",
			Matches:  2,
		}},
	}})
	want := "warning: fsub step s row 000003 file config.sh pattern \"%x\" matched 2 times; replaced all matches\n"
	if got := out.String(); got != want {
		t.Fatalf("warning output = %q, want %q", got, want)
	}
}

func TestCommandFormattingHelpers(t *testing.T) {
	if err := aggregateComponentResults([]componentResult{{Label: "done", Final: StatusFinished}}); err != nil {
		t.Fatalf("finished aggregate error = %v, want nil", err)
	}

	err := aggregateComponentResults([]componentResult{
		{Label: "a", Final: StatusError, Err: errors.New("failed")},
		{Label: "b", Final: StatusInterrupted},
	})
	if err == nil || err.Error() != "a: benchmark ERROR: failed; b: benchmark INTERRUPTED" {
		t.Fatalf("aggregate error = %v, want joined component failures", err)
	}

	if got := finalMessage(StatusInterrupted); got != "run interrupted" {
		t.Fatalf("interrupted message = %q", got)
	}
	if got := finalMessage(StatusFinished); got != "" {
		t.Fatalf("finished message = %q, want empty", got)
	}

	hashErr := sourceHashMismatchError("run", "old", "new", "manifest")
	for _, want := range []string{"cannot continue run", "manifest source hash", "stored old", "current new"} {
		if !strings.Contains(hashErr.Error(), want) {
			t.Fatalf("source hash mismatch error = %q, want %q", hashErr.Error(), want)
		}
	}

	var warnings bytes.Buffer
	printSummaryWarning(nil, "label", "status", errors.New("ignored"))
	printSummaryWarning(&warnings, "", "status", errors.New("broken"))
	printSummaryWarning(&warnings, "small", "analyse", errors.New("missing"))
	got := warnings.String()
	for _, want := range []string{
		"warning: failed to print status summary: broken\n",
		"warning: failed to print analyse summary for small: missing\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary warnings = %q, want %q", got, want)
		}
	}
}

func TestCreatePreparedStoresAndOpenContinuableStores(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	plan := testRuntimePlan(t.TempDir())
	plan.RootDir = root
	prepared, err := createPreparedStores([]runtimePlan{plan})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 1 || prepared[0].Store == nil || prepared[0].Store.RunDir != filepath.Join(root, "000000") {
		t.Fatalf("prepared stores = %#v, want one store at first run", prepared)
	}

	continuable, unlock, err := openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{plan}})
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if len(continuable) != 1 || continuable[0].Store == nil || continuable[0].Store.RunDir != filepath.Join(root, "000000") {
		t.Fatalf("continuable stores = %#v, want existing run", continuable)
	}

	fileRoot := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileRoot, []byte("occupied\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badPlan := plan
	badPlan.RootDir = fileRoot
	if _, err := createPreparedStores([]runtimePlan{badPlan}); err == nil {
		t.Fatal("expected createPreparedStores error for file root")
	}

	if _, _, err := openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{{RootDir: filepath.Join(t.TempDir(), "missing")}}}); err == nil {
		t.Fatal("expected missing benchmark root error")
	}

	emptyRoot := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(emptyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	emptyPlan := plan
	emptyPlan.RootDir = emptyRoot
	emptyPlan.ComponentName = "small"
	_, _, err = openContinuableStores(runtimeSuitePlan{Configured: true, Plans: []runtimePlan{emptyPlan}})
	if err == nil || !strings.Contains(err.Error(), `cannot continue benchmark "small"`) {
		t.Fatalf("empty configured continue error = %v, want benchmark-specific hint", err)
	}
	_, _, err = openContinuableStores(runtimeSuitePlan{Configured: true, SelectedName: "small", Plans: []runtimePlan{emptyPlan}})
	if err == nil || strings.Contains(err.Error(), "use --benchmark") {
		t.Fatalf("selected empty continue error = %v, want direct latest-run error", err)
	}

	runningRoot := filepath.Join(t.TempDir(), "running")
	runningPlan := plan
	runningPlan.RootDir = runningRoot
	if _, _, err := CreateRunDirectoryWithInitial(runningRoot, runningPlan, StatusRunning); err != nil {
		t.Fatal(err)
	}
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{runningPlan}})
	if err == nil || !strings.Contains(err.Error(), "benchmark status is RUNNING") {
		t.Fatalf("running continue error = %v, want RUNNING rejection", err)
	}

	mismatchRoot := filepath.Join(t.TempDir(), "mismatch")
	mismatchPlan := plan
	mismatchPlan.RootDir = mismatchRoot
	if _, _, err := CreateRunDirectoryWithInitial(mismatchRoot, mismatchPlan, StatusNotStarted); err != nil {
		t.Fatal(err)
	}
	mismatchPlan.Manifest.SourceHash = "changed"
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{mismatchPlan}})
	if err == nil || !strings.Contains(err.Error(), "root status source hash does not match") {
		t.Fatalf("mismatch continue error = %v, want source hash mismatch", err)
	}

	missingStatusRoot := filepath.Join(t.TempDir(), "missing-status")
	missingStatusPlan := plan
	missingStatusPlan.RootDir = missingStatusRoot
	if _, _, err := CreateRunDirectoryWithInitial(missingStatusRoot, missingStatusPlan, StatusNotStarted); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(missingStatusRoot, "000000", "status")); err != nil {
		t.Fatal(err)
	}
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{missingStatusPlan}})
	if err == nil || !strings.Contains(err.Error(), "cannot continue incomplete run") {
		t.Fatalf("missing status continue error = %v, want incomplete run error", err)
	}

	missingManifestRoot := filepath.Join(t.TempDir(), "missing-manifest")
	missingManifestPlan := plan
	missingManifestPlan.RootDir = missingManifestRoot
	if _, _, err := CreateRunDirectoryWithInitial(missingManifestRoot, missingManifestPlan, StatusNotStarted); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(missingManifestRoot, "000000", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{missingManifestPlan}})
	if err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("missing manifest continue error = %v, want manifest load error", err)
	}

	invalidManifestRoot := filepath.Join(t.TempDir(), "invalid-manifest")
	invalidManifestPlan := plan
	invalidManifestPlan.RootDir = invalidManifestRoot
	if _, _, err := CreateRunDirectoryWithInitial(invalidManifestRoot, invalidManifestPlan, StatusNotStarted); err != nil {
		t.Fatal(err)
	}
	invalidManifestPath := filepath.Join(invalidManifestRoot, "000000", "manifest.json")
	invalidManifest, err := LoadManifest(invalidManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	invalidManifest.AnalyseDatabasePath = filepath.Join(invalidManifestRoot, "analysis.sqlite")
	invalidManifest.Steps[0].AnalyseTable = "wrong"
	writeJSONForStoreTest(t, invalidManifestPath, invalidManifest)
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{invalidManifestPlan}})
	if err == nil || !strings.Contains(err.Error(), "manifest analyse table") {
		t.Fatalf("invalid manifest continue error = %v, want validation error", err)
	}

	templateRoot := filepath.Join(t.TempDir(), "template")
	templatePlan := plan
	templatePlan.RootDir = templateRoot
	if _, _, err := CreateRunDirectoryWithInitial(templateRoot, templatePlan, StatusNotStarted); err != nil {
		t.Fatal(err)
	}
	templatePlan.Manifest.TemplateHashes = []TemplateHash{{Step: "s", SourcePath: "template.in", DestName: "out", SHA256: "new"}}
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{templatePlan}})
	if err == nil || !strings.Contains(err.Error(), "was not part of the prepared run") {
		t.Fatalf("template mismatch continue error = %v, want template mismatch", err)
	}

	manifestMismatchRoot := filepath.Join(t.TempDir(), "manifest-mismatch")
	manifestMismatchPlan := plan
	manifestMismatchPlan.RootDir = manifestMismatchRoot
	if _, _, err := CreateRunDirectoryWithInitial(manifestMismatchRoot, manifestMismatchPlan, StatusNotStarted); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(manifestMismatchRoot, "000000", "manifest.json")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest.SourceHash = "manifest-only"
	writeJSONForStoreTest(t, manifestPath, manifest)
	_, _, err = openContinuableStores(runtimeSuitePlan{Plans: []runtimePlan{manifestMismatchPlan}})
	if err == nil || !strings.Contains(err.Error(), "manifest source hash does not match") {
		t.Fatalf("manifest mismatch continue error = %v, want manifest source hash mismatch", err)
	}
}

func TestRunPreparedStoresCoversMultiComponentAndCancelledRun(t *testing.T) {
	first := testRuntimePlan(t.TempDir())
	first.RootDir = filepath.Join(t.TempDir(), "first")
	first.ComponentName = "first"
	second := testRuntimePlan(t.TempDir())
	second.RootDir = filepath.Join(t.TempDir(), "second")
	second.ComponentName = "second"
	prepared, err := createPreparedStores([]runtimePlan{first, second})
	if err != nil {
		t.Fatal(err)
	}

	withRunWorkProcess(t, func(context.Context, string) processResult {
		code := 0
		return processResult{Status: StatusFinished, ExitCode: &code}
	})

	var out bytes.Buffer
	if err := runPreparedStores(context.Background(), Options{Stdout: &out}, prepared, false); err != nil {
		t.Fatalf("runPreparedStores error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "[first]") || !strings.Contains(got, "[second]") {
		t.Fatalf("multi-component stdout = %q, want component labels", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = runPreparedStores(ctx, Options{Stdout: &bytes.Buffer{}}, []preparedStore{{Plan: runtimePlan{ComponentName: "late"}}}, false)
	if err == nil || !strings.Contains(err.Error(), "late: benchmark INTERRUPTED") {
		t.Fatalf("cancelled aggregate error = %v, want interrupted component", err)
	}

	interruptedPlan := testRuntimePlan(t.TempDir())
	interruptedPlan.RootDir = filepath.Join(t.TempDir(), "interrupted")
	interruptedPlan.ComponentName = "interrupted"
	interruptedStore, _, err := CreateRunDirectoryWithInitial(interruptedPlan.RootDir, interruptedPlan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	if err := interruptedStore.WriteWorkStatus(interruptedStore.Manifest.Work[0], WorkStatus{
		Schema: 1,
		Status: StatusRunning,
		Step:   interruptedStore.Manifest.Work[0].Step,
		Row:    interruptedStore.Manifest.Work[0].Row,
	}); err != nil {
		t.Fatal(err)
	}
	secondPlan := testRuntimePlan(t.TempDir())
	secondPlan.RootDir = filepath.Join(t.TempDir(), "after-interrupted")
	secondPlan.ComponentName = "after"
	secondStore, _, err := CreateRunDirectoryWithInitial(secondPlan.RootDir, secondPlan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	err = runPreparedStores(context.Background(), Options{Stdout: &bytes.Buffer{}}, []preparedStore{
		{Plan: interruptedPlan, Store: interruptedStore},
		{Plan: secondPlan, Store: secondStore},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "interrupted: benchmark INTERRUPTED") {
		t.Fatalf("interrupted aggregate error = %v, want interrupted first component", err)
	}
	secondStatus, err := secondStore.LoadRootStatus()
	if err != nil {
		t.Fatal(err)
	}
	if secondStatus.Status != StatusNotStarted {
		t.Fatalf("second store status = %s, want not started after interrupted break", secondStatus.Status)
	}
}

func TestRunOneStoreErrorBranches(t *testing.T) {
	manifest := Manifest{
		Schema:        1,
		SourceHash:    "hash",
		BenchmarkName: "bench",
		GlobalNProc:   1,
		Steps:         []ManifestStep{{Name: "s", Dir: "s", NProc: 1}},
		Work:          []ManifestWork{{Step: "s", Row: 0, Dir: "000000"}},
	}

	missingStatusStore := NewStore(t.TempDir(), manifest, nil)
	result := runOneStore(context.Background(), preparedStore{
		Plan:  runtimePlan{ComponentName: "mark"},
		Store: missingStatusStore,
	}, false, false, nil)
	if result.Final != StatusError || result.Err == nil {
		t.Fatalf("missing root status result = %#v, want error", result)
	}

	result = runOneStore(context.Background(), preparedStore{
		Plan:  runtimePlan{ComponentName: "continue"},
		Store: NewStore(t.TempDir(), manifest, nil),
	}, true, false, nil)
	if result.Final != StatusError || result.Err == nil {
		t.Fatalf("stale normalization result = %#v, want error", result)
	}

	runDir := t.TempDir()
	writeJSONForStoreTest(t, filepath.Join(runDir, "status"), RootStatus{
		Schema:     1,
		Status:     StatusNotStarted,
		SourceHash: "hash",
	})
	loadFailureStore := NewStore(runDir, manifest, nil)
	result = runOneStore(context.Background(), preparedStore{
		Plan:  runtimePlan{ComponentName: "load"},
		Store: loadFailureStore,
	}, false, false, nil)
	if result.Final != StatusError || result.Err == nil || !strings.Contains(result.Err.Error(), "load work statuses") {
		t.Fatalf("load failure result = %#v, want scheduler load error", result)
	}

	finalFailurePlan := testRuntimePlan(t.TempDir())
	finalFailurePlan.RootDir = filepath.Join(t.TempDir(), "final-failure")
	finalFailureStore, _, err := CreateRunDirectoryWithInitial(finalFailurePlan.RootDir, finalFailurePlan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	withRunWorkProcess(t, func(_ context.Context, dir string) processResult {
		runDir := filepath.Dir(filepath.Dir(dir))
		statusPath := filepath.Join(runDir, "status")
		if err := os.Remove(statusPath); err != nil {
			return processResult{Status: StatusError, Err: err}
		}
		if err := os.Mkdir(statusPath, 0o755); err != nil {
			return processResult{Status: StatusError, Err: err}
		}
		code := 0
		return processResult{Status: StatusFinished, ExitCode: &code}
	})
	result = runOneStore(context.Background(), preparedStore{
		Plan:  finalFailurePlan,
		Store: finalFailureStore,
	}, false, false, nil)
	if result.Final != StatusError || result.Err == nil {
		t.Fatalf("final status failure result = %#v, want MarkRootFinal error", result)
	}
}

func TestRunOneStoreAnalysisErrorAndWeakAnalysis(t *testing.T) {
	cwd := t.TempDir()
	withWorkingDir(t, cwd)
	suite := buildSuiteFromSource(t, `
jbs_name = "analysis"
x = (1, 2)

do s with x {
        echo run
}

analyse s {
        value = "value: %w" in "out.log"
        (x, value)
}
`, "")
	plan := suite.Plans[0]
	plan.RootDir = filepath.Join(cwd, "analysis")
	store, _, err := CreateRunDirectoryWithInitial(plan.RootDir, plan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}

	withRunWorkProcess(t, func(context.Context, string) processResult {
		code := 0
		return processResult{Status: StatusFinished, ExitCode: &code}
	})
	result := runOneStore(context.Background(), preparedStore{Plan: plan, Store: store}, false, false, nil)
	if result.Final != StatusError || result.Err == nil || !strings.Contains(result.Err.Error(), "out.log") {
		t.Fatalf("analysis failure result = %#v, want missing analyse file error", result)
	}

	weakPlan := suite.Plans[0]
	weakPlan.RootDir = filepath.Join(cwd, "weak")
	weakStore, _, err := CreateRunDirectoryWithInitial(weakPlan.RootDir, weakPlan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	withRunWorkProcess(t, func(context.Context, string) processResult {
		code := 2
		return processResult{Status: StatusError, ExitCode: &code}
	})
	result = runOneStore(context.Background(), preparedStore{Plan: weakPlan, Store: weakStore}, false, true, nil)
	if result.Final != StatusError || !result.Analysed || result.Err != nil {
		t.Fatalf("weak analysis result = %#v, want analysed error status without scheduler error", result)
	}
}

func TestPrintPostRunSummariesWarnings(t *testing.T) {
	printPostRunSummaries(nil, nil, []componentResult{{Store: NewStore(t.TempDir(), Manifest{}, nil)}})

	manifest := Manifest{
		Schema:        1,
		SourceHash:    "hash",
		BenchmarkName: "bench",
		GlobalNProc:   1,
		Steps:         []ManifestStep{{Name: "s", Dir: "s", NProc: 1, AnalyseCSV: "analyse.csv"}},
		Work:          []ManifestWork{{Step: "s", Row: 0, Dir: "000000"}},
	}
	var stdout, stderr bytes.Buffer
	missingStatusStore := NewStore(t.TempDir(), manifest, nil)
	printPostRunSummaries(&stdout, &stderr, []componentResult{{Label: "bad-status", Store: missingStatusStore}})
	if got := stderr.String(); !strings.Contains(got, "warning: failed to print status summary for bad-status") {
		t.Fatalf("status warning = %q, want labelled status warning", got)
	}

	stdout.Reset()
	stderr.Reset()
	runDir := t.TempDir()
	store := NewStore(runDir, manifest, nil)
	writeJSONForStoreTest(t, store.WorkStatusPath(manifest.Work[0]), WorkStatus{
		Schema: 1,
		Status: StatusFinished,
		Step:   "s",
		Row:    0,
	})
	printPostRunSummaries(&stdout, &stderr, []componentResult{{Label: "bad-analyse", Store: store, Analysed: true}})
	if got := stderr.String(); !strings.Contains(got, "warning: failed to print analyse summary for bad-analyse") {
		t.Fatalf("analyse warning = %q, want labelled analyse warning", got)
	}

	stdout.Reset()
	stderr.Reset()
	failedRunDir := t.TempDir()
	failedStore := NewStore(failedRunDir, manifest, nil)
	writeJSONForStoreTest(t, failedStore.WorkStatusPath(manifest.Work[0]), WorkStatus{
		Schema: 1,
		Status: StatusError,
		Step:   "s",
		Row:    0,
	})
	printPostRunSummaries(&stdout, &stderr, []componentResult{{Label: "failed", Store: failedStore}})
	if got := stdout.String(); !strings.Contains(got, "failed workpackage directories:") || !strings.Contains(got, filepath.Join(failedRunDir, "s", "000000")) {
		t.Fatalf("failed summary stdout = %q, want failed work directories", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("failed summary stderr = %q, want none", got)
	}
}

func commandOptionsFromSource(t *testing.T, cwd, source string) Options {
	t.Helper()
	diags := &diag.Diagnostics{}
	program := filepath.Join(cwd, "case.jbs")
	loadRes, err := imports.LoadAndExpandSource(program, strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return Options{
		Input:       program,
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: program,
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func workplanConflictOptions() Options {
	p := sema.BindingVersionKey{Public: "p", Version: "p:v1"}
	q := sema.BindingVersionKey{Public: "q", Version: "q:v1"}
	res := &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{"jbs_name": eval.String("diag")}},
		StepOrder: []string{
			"s0",
			"s1",
			"s2",
		},
		DoBlocks: []ast.DoBlock{
			{Name: "s0", Body: "echo s0"},
			{Name: "s1", Body: "echo s1"},
			{Name: "s2", After: []string{"s0", "s1"}, Body: "echo s2"},
		},
		BindingsByName: map[string]*sema.GlobalBinding{},
		BindingsByKey:  map[sema.BindingVersionKey]*sema.GlobalBinding{},
		StepScopeByName: map[string]*sema.StepScopePlan{
			"s0": {
				Expansions: []sema.WithExpansion{{
					ItemID:        0,
					Source:        "p",
					SourceKey:     p,
					DisplaySource: "p",
					Vars:          []sema.ExpandedWithVar{{Visible: "x", SourceVar: "x"}},
					VarsByName:    map[string][]eval.Value{"x": {eval.Int(1)}},
					RowCount:      1,
				}},
			},
			"s1": {
				Expansions: []sema.WithExpansion{{
					ItemID:        0,
					Source:        "q",
					SourceKey:     q,
					DisplaySource: "q",
					Vars:          []sema.ExpandedWithVar{{Visible: "x", SourceVar: "x"}},
					VarsByName:    map[string][]eval.Value{"x": {eval.Int(2)}},
					RowCount:      1,
				}},
			},
			"s2": {},
		},
	}
	return Options{Result: res}
}
