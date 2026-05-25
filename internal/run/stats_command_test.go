package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestShowStatusEntrypointForJBSInput(t *testing.T) {
	dir := t.TempDir()
	changeWorkingDirForStatsCommandTest(t, dir)

	var buf bytes.Buffer
	opts := statsCommandOptionsFromSource(t, `
jbs_name = "bench"
x = 1
do run with x {
  echo $x
}
`, &buf)
	stores := createStatsCommandRuns(t, opts)
	writeAllWorkStatusesForStatsCommandTest(t, stores[0], StatusError)

	if err := ShowStatus(context.Background(), opts); err != nil {
		t.Fatalf("ShowStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"| step", "ERROR", "duration_s", "failed workpackage directories:", filepath.Join(dir, "bench", "000000", "run", "000000")} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestLsAnalyseEntrypointForJBSInput(t *testing.T) {
	changeWorkingDirForStatsCommandTest(t, t.TempDir())

	var buf bytes.Buffer
	opts := statsCommandOptionsFromSource(t, `
jbs_name = "bench"
x = 1
do run with x {
  echo "Runtime 1.5" > job.out
}
analyse run {
  runtime = "Runtime %f" in "job.out"
  (runtime)
}
`, &buf)
	createStatsCommandRuns(t, opts)

	if err := LsAnalyse(context.Background(), opts); err != nil {
		t.Fatalf("LsAnalyse: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"analysis", "nrows", "ncols", filepath.Join("bench", "000000", "run", "analyse.csv"), "|     0 |", "|     2 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("analyse output missing %q:\n%s", want, out)
		}
	}
}

func TestTreeEntrypointForJBSInput(t *testing.T) {
	changeWorkingDirForStatsCommandTest(t, t.TempDir())

	var buf bytes.Buffer
	opts := statsCommandOptionsFromSource(t, `
jbs_name = "bench"
do prep {
  echo prep
}
do run after prep {
  echo run
}
`, &buf)

	if err := Tree(context.Background(), opts); err != nil {
		t.Fatalf("Tree: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"| step", "| #", "prep", "run", "total:", "| 2 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree output missing %q:\n%s", want, out)
		}
	}
}

func TestBenchmarkDirEntrypoints(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	manifest := statsCommandManifest("small", true)
	store := writeStatsCommandInspectionRun(t, root, "000000", manifest, "run_id,x\n000000,1\n")
	writeAllWorkStatusesForStatsCommandTest(t, store, StatusFinished)

	var status bytes.Buffer
	if err := ShowStatusForBenchmarkDir(context.Background(), BenchmarkDirOptions{Root: root, Benchmark: "small", Stdout: &status}); err != nil {
		t.Fatalf("ShowStatusForBenchmarkDir: %v", err)
	}
	if out := status.String(); !strings.Contains(out, "FINISHED") || strings.Contains(out, "[small]") {
		t.Fatalf("unexpected status output:\n%s", out)
	}

	var analyse bytes.Buffer
	if err := LsAnalyseForBenchmarkDir(context.Background(), BenchmarkDirOptions{Root: root, Benchmark: "small", Stdout: &analyse}); err != nil {
		t.Fatalf("LsAnalyseForBenchmarkDir: %v", err)
	}
	if out := analyse.String(); !strings.Contains(out, "analyse.csv") || !strings.Contains(out, "|     1 |") || !strings.Contains(out, "|     2 |") {
		t.Fatalf("unexpected analyse output:\n%s", out)
	}
}

func TestStatsCommandEntrypointErrors(t *testing.T) {
	ctx := context.Background()
	if err := ShowStatus(ctx, Options{}); err == nil || !strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("ShowStatus error = %v, want missing analysis result", err)
	}
	if err := LsAnalyse(ctx, Options{}); err == nil || !strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("LsAnalyse error = %v, want missing analysis result", err)
	}
	if err := Tree(ctx, Options{}); err == nil || !strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("Tree error = %v, want missing analysis result", err)
	}
	if err := ShowStatusForBenchmarkDir(ctx, BenchmarkDirOptions{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("expected benchmark-dir status error")
	}
	if err := LsAnalyseForBenchmarkDir(ctx, BenchmarkDirOptions{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("expected benchmark-dir analyse error")
	}

	changeWorkingDirForStatsCommandTest(t, t.TempDir())
	var buf bytes.Buffer
	opts := statsCommandOptionsFromSource(t, `
jbs_name = "bench"
do run {
  echo run
}
`, &buf)
	if err := ShowStatus(ctx, opts); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("ShowStatus error = %v, want missing benchmark root", err)
	}
	if err := LsAnalyse(ctx, opts); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("LsAnalyse error = %v, want missing benchmark root", err)
	}
}

func TestOpenLatestStoresForInspectionReportsConfiguredMissingRuns(t *testing.T) {
	changeWorkingDirForStatsCommandTest(t, t.TempDir())
	suite := buildSuiteFromSource(t, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run", "large": "run"}
do run {
  echo run
}
`, "")

	_, err := openLatestStoresForInspection(suite)
	if err == nil || !strings.Contains(err.Error(), `cannot inspect benchmark "small"`) || !strings.Contains(err.Error(), "use --benchmark") {
		t.Fatalf("error = %v, want configured benchmark guidance", err)
	}
}

func TestOpenLatestStoresForInspectionReportsMalformedManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	if err := os.MkdirAll(filepath.Join(root, "000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "000000", "manifest.json"), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	suite := runtimeSuitePlan{Plans: []runtimePlan{{RootDir: root, ComponentName: "bench"}}}

	_, err := openLatestStoresForInspection(suite)
	if err == nil {
		t.Fatal("expected malformed manifest error")
	}
}

func TestOpenLatestStoresForInspectionReportsInvalidManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	writeInspectionRun(t, root, "000000", Manifest{Schema: 1, BenchmarkName: "bench"})
	suite := runtimeSuitePlan{Plans: []runtimePlan{{RootDir: root, ComponentName: "bench"}}}

	_, err := openLatestStoresForInspection(suite)
	if err == nil || !strings.Contains(err.Error(), "missing run_id") {
		t.Fatalf("error = %v, want invalid manifest", err)
	}
}

func TestOpenLatestStoresForBenchmarkDirErrors(t *testing.T) {
	t.Run("malformed manifest", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		writeInspectionRun(t, root, "000000", Manifest{Schema: 1, BenchmarkName: "bench"})

		_, err := openLatestStoresForBenchmarkDir(root, "")
		if err == nil || !strings.Contains(err.Error(), "missing run_id") {
			t.Fatalf("error = %v, want malformed manifest", err)
		}
	})

	t.Run("selected direct root mismatch", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench", "small")
		writeInspectionRun(t, root, "000000", Manifest{
			Schema:             1,
			RunID:              "000000",
			BenchmarkName:      "bench",
			BenchmarkComponent: "small",
		})

		_, err := openLatestStoresForBenchmarkDir(root, "large")
		if err == nil || !strings.Contains(err.Error(), `does not match --benchmark "large"`) {
			t.Fatalf("error = %v, want selected benchmark mismatch", err)
		}
	})

	t.Run("missing latest run", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := openLatestStoresForBenchmarkDir(root, "")
		if err == nil || !strings.Contains(err.Error(), "no run directories found") {
			t.Fatalf("error = %v, want missing latest run", err)
		}
	})
}

func TestLatestManifestForRootReportsLoadError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	if err := os.MkdirAll(filepath.Join(root, "000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "000000", "manifest.json"), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := latestManifestForRoot(root)
	if err == nil {
		t.Fatal("expected manifest load error")
	}
}

func TestLatestManifestForRootSelectsSevenDigitRunID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	writeStatsCommandInspectionRun(t, root, "999999", statsCommandManifest("bench", false), "")
	writeStatsCommandInspectionRun(t, root, "1000000", statsCommandManifest("bench", false), "")

	manifest, runDir, err := latestManifestForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(runDir) != "1000000" || manifest.RunID != "1000000" {
		t.Fatalf("selected run %q with manifest %q, want 1000000", runDir, manifest.RunID)
	}
}

func TestManifestOrDirMatchesBenchmarkAllowsEmptySelection(t *testing.T) {
	if !manifestOrDirMatchesBenchmark(Manifest{}, filepath.Join(t.TempDir(), "bench"), "") {
		t.Fatal("empty selected benchmark should match")
	}
}

func TestDiscoverBenchmarkRoots(t *testing.T) {
	t.Run("direct root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		writeInspectionRun(t, root, "000001", Manifest{
			Schema:        1,
			RunID:         "000001",
			BenchmarkName: "direct",
		})

		roots, err := discoverBenchmarkRoots(root, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(roots) != 1 || roots[0].rootDir != root || roots[0].label != "direct" {
			t.Fatalf("roots = %#v, want direct root", roots)
		}
	})

	t.Run("direct root path fallback", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "small")
		writeInspectionRun(t, root, "000000", Manifest{
			Schema: 1,
			RunID:  "000000",
		})

		roots, err := discoverBenchmarkRoots(root, "small")
		if err != nil {
			t.Fatal(err)
		}
		if len(roots) != 1 || roots[0].label != "small" {
			t.Fatalf("roots = %#v, want path-derived small label", roots)
		}
	})

	t.Run("child roots skip hidden and sort", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		writeInspectionRun(t, filepath.Join(root, ".hidden"), "000000", Manifest{
			Schema:             1,
			RunID:              "000000",
			BenchmarkName:      "bench",
			BenchmarkComponent: "hidden",
		})
		writeInspectionRun(t, filepath.Join(root, "zeta"), "000000", Manifest{
			Schema:             1,
			RunID:              "000000",
			BenchmarkName:      "bench",
			BenchmarkComponent: "zeta",
		})
		writeInspectionRun(t, filepath.Join(root, "alpha"), "000000", Manifest{
			Schema:             1,
			RunID:              "000000",
			BenchmarkName:      "bench",
			BenchmarkComponent: "alpha",
		})
		if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
			t.Fatal(err)
		}

		roots, err := discoverBenchmarkRoots(root, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(roots) != 2 || roots[0].label != "alpha" || roots[1].label != "zeta" {
			t.Fatalf("roots = %#v, want sorted visible child roots", roots)
		}
	})

	t.Run("unknown selected child", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		writeInspectionRun(t, filepath.Join(root, "small"), "000000", Manifest{
			Schema:             1,
			RunID:              "000000",
			BenchmarkName:      "bench",
			BenchmarkComponent: "small",
		})

		_, err := discoverBenchmarkRoots(root, "large")
		if err == nil || !strings.Contains(err.Error(), `unknown benchmark "large"`) {
			t.Fatalf("error = %v, want unknown selected benchmark", err)
		}
	})

	t.Run("invalid child run", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		writeInspectionRun(t, filepath.Join(root, "bad"), "000000", Manifest{
			Schema:        1,
			BenchmarkName: "bench",
		})

		_, err := discoverBenchmarkRoots(root, "")
		if err == nil || !strings.Contains(err.Error(), "missing run_id") {
			t.Fatalf("error = %v, want invalid child run", err)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		_, err := discoverBenchmarkRoots(filepath.Join(t.TempDir(), "missing"), "")
		if err == nil || !strings.Contains(err.Error(), "no such file") {
			t.Fatalf("error = %v, want read-dir failure", err)
		}
	})
}

func TestPrintStatusForStores(t *testing.T) {
	if err := printStatusForStores(nil, nil); err != nil {
		t.Fatalf("nil writer returned error: %v", err)
	}

	small := statsCommandStatusStore(t, StatusError)
	large := statsCommandStatusStore(t, StatusFinished)
	var buf bytes.Buffer
	err := printStatusForStores(&buf, []preparedStore{
		{Plan: runtimePlan{ComponentName: "small"}, Store: small},
		{Plan: runtimePlan{ComponentName: "large"}, Store: large},
	})
	if err != nil {
		t.Fatalf("printStatusForStores: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"[small]", "[large]", "failed workpackage directories:", "duration_s"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}

	broken := NewStore(t.TempDir(), statusSummaryManifest(), nil)
	if err := printStatusForStores(&buf, []preparedStore{{Plan: runtimePlan{ComponentName: "broken"}, Store: broken}}); err == nil {
		t.Fatal("expected status read error")
	}
}

func TestPrintAnalyseOutputsForStores(t *testing.T) {
	if err := printAnalyseOutputsForStores(nil, nil); err != nil {
		t.Fatalf("nil writer returned error: %v", err)
	}

	noOutput := NewStore(t.TempDir(), Manifest{
		Schema:        1,
		RunID:         "000000",
		BenchmarkName: "bench",
		Steps:         []ManifestStep{{Name: "run", Dir: "run", NProc: 1}},
	}, nil)
	var empty bytes.Buffer
	if err := printAnalyseOutputsForStores(&empty, []preparedStore{{Plan: runtimePlan{ComponentName: "none"}, Store: noOutput}}); err != nil {
		t.Fatalf("printAnalyseOutputsForStores no output: %v", err)
	}
	if empty.Len() != 0 {
		t.Fatalf("no-output store printed %q", empty.String())
	}

	csvRoot := filepath.Join(t.TempDir(), "csv")
	csvStore := writeStatsCommandInspectionRun(t, csvRoot, "000000", statsCommandManifest("csv", true), "run_id,x\n000000,1\n")
	sqliteStore := statsCommandSQLiteStore(t)
	var buf bytes.Buffer
	err := printAnalyseOutputsForStores(&buf, []preparedStore{
		{Plan: runtimePlan{ComponentName: "csv"}, Store: csvStore},
		{Plan: runtimePlan{ComponentName: "sqlite"}, Store: sqliteStore},
	})
	if err != nil {
		t.Fatalf("printAnalyseOutputsForStores: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"[csv]", "[sqlite]", "analyse.csv", "results.sqlite:bench_000000_run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("analyse output missing %q:\n%s", want, out)
		}
	}

	missing := NewStore(t.TempDir(), statsCommandManifest("missing", true), nil)
	if err := printAnalyseOutputsForStores(&buf, []preparedStore{{Plan: runtimePlan{ComponentName: "missing"}, Store: missing}}); err == nil {
		t.Fatal("expected missing CSV error")
	}
}

func TestPrintTreeForPlans(t *testing.T) {
	if err := printTreeForPlans(nil, nil); err != nil {
		t.Fatalf("nil writer returned error: %v", err)
	}

	var single bytes.Buffer
	if err := printTreeForPlans(&single, []runtimePlan{{ComponentName: "bench", Manifest: statusSummaryManifest()}}); err != nil {
		t.Fatalf("printTreeForPlans single: %v", err)
	}
	if out := single.String(); strings.Contains(out, "[bench]") || !strings.Contains(out, "total:") {
		t.Fatalf("unexpected single tree output:\n%s", out)
	}

	var multi bytes.Buffer
	err := printTreeForPlans(&multi, []runtimePlan{
		{ComponentName: "small", Manifest: statusSummaryManifest()},
		{ComponentName: "large", Manifest: Manifest{
			Steps: []ManifestStep{{Name: "run", Dir: "run"}},
			Work:  []ManifestWork{{Step: "run", Row: 0, Dir: "000000"}},
		}},
	})
	if err != nil {
		t.Fatalf("printTreeForPlans multi: %v", err)
	}
	out := multi.String()
	for _, want := range []string{"[small]", "[large]", "step1", "run"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree output missing %q:\n%s", want, out)
		}
	}
}

func statsCommandOptionsFromSource(t *testing.T, source string, stdout *bytes.Buffer) Options {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	diags := &diag.Diagnostics{}
	source = strings.TrimSpace(source) + "\n"
	loadRes, err := imports.LoadAndExpandSource("stats.jbs", source, cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: filepath.Join(cwd, "stats.jbs"),
		Stdout:      stdout,
	}
}

func createStatsCommandRuns(t *testing.T, opts Options) []*Store {
	t.Helper()
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	stores := make([]*Store, 0, len(suite.Plans))
	for _, plan := range suite.Plans {
		store, _, err := CreateRunDirectoryWithInitial(plan.RootDir, plan, StatusNotStarted)
		if err != nil {
			t.Fatal(err)
		}
		stores = append(stores, store)
	}
	return stores
}

func changeWorkingDirForStatsCommandTest(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func statsCommandManifest(component string, withCSV bool) Manifest {
	step := ManifestStep{Name: "run", Dir: "run", NProc: 1}
	if withCSV {
		step.AnalyseCSV = "analyse.csv"
	}
	return Manifest{
		Schema:             1,
		SourceHash:         "hash",
		BenchmarkName:      "bench",
		BenchmarkComponent: component,
		RunID:              "000000",
		GlobalNProc:        1,
		Steps:              []ManifestStep{step},
		Work:               []ManifestWork{{Step: "run", Row: 0, Dir: "000000", Values: map[string]string{"x": "1"}}},
	}
}

func writeStatsCommandInspectionRun(t *testing.T, root, runID string, manifest Manifest, csv string) *Store {
	t.Helper()
	manifest.RunID = runID
	writeInspectionRun(t, root, runID, manifest)
	store := NewStore(filepath.Join(root, runID), manifest, nil)
	for _, work := range manifest.Work {
		workDir := store.WorkDir(work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: StatusNotStarted, Step: work.Step, Row: work.Row}); err != nil {
			t.Fatal(err)
		}
	}
	if csv != "" {
		for _, step := range manifest.Steps {
			if step.AnalyseCSV == "" {
				continue
			}
			stepDir := filepath.Join(store.RunDir, step.Dir)
			if err := os.MkdirAll(stepDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stepDir, step.AnalyseCSV), []byte(csv), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return store
}

func writeAllWorkStatusesForStatsCommandTest(t *testing.T, store *Store, status Status) {
	t.Helper()
	for _, work := range store.Manifest.Work {
		workDir := store.WorkDir(work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: status, Step: work.Step, Row: work.Row}); err != nil {
			t.Fatal(err)
		}
	}
}

func statsCommandStatusStore(t *testing.T, status Status) *Store {
	t.Helper()
	store := NewStore(t.TempDir(), statusSummaryManifest(), nil)
	writeStatusSummaryStatuses(t, store, map[string]Status{
		"step1/000000": status,
		"step1/000001": StatusFinished,
		"step2/000000": StatusFinished,
		"step2/000001": StatusFinished,
		"step3/000000": StatusFinished,
		"step4/000000": StatusFinished,
	})
	return store
}

func statsCommandSQLiteStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "results.sqlite")
	db, err := openAnalyseDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceAnalyseTable(
		tx,
		"bench_000000_run",
		[]string{"run_id", "x"},
		[]AnalyseValueKind{analyseValueString, analyseValueString},
		analyseRowsFromStrings([][]string{{"000000", "1"}}),
	); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return NewStore(filepath.Join(dir, "run"), Manifest{
		Schema:              1,
		RunID:               "000000",
		BenchmarkName:       "bench",
		AnalyseDatabase:     "results.sqlite",
		AnalyseDatabasePath: dbPath,
		Steps:               []ManifestStep{{Name: "run", Dir: "run", NProc: 1, AnalyseTable: "bench_000000_run"}},
	}, nil)
}
