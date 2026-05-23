package run

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/benchmarks"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	jbsimports "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

func TestRuntimeBenchmarkConfigValidationBranches(t *testing.T) {
	if _, err := runtimeBenchmarkConfig(nil); err == nil || !strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("expected nil result error, got %v", err)
	}

	cfg, err := runtimeBenchmarkConfig(&sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{}}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Configured || len(cfg.Specs) != 0 || len(cfg.ByName) != 0 {
		t.Fatalf("empty config = %#v", cfg)
	}

	_, err = runtimeBenchmarkConfig(&sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{
		"jbs_benchmarks": eval.String("bad"),
	}}})
	if err == nil || !strings.Contains(err.Error(), "must be a dictionary") {
		t.Fatalf("expected non-dictionary config error, got %v", err)
	}

	_, err = runtimeBenchmarkConfig(&sema.Result{Globals: sema.GlobalState{Values: map[string]eval.Value{
		"jbs_benchmarks": eval.DictValue([]eval.DictEntry{
			{Key: eval.DictKey{Kind: eval.DictKeyInt, I: 1}, Value: eval.String("run")},
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "***"}, Value: eval.String("run")},
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "bad"}, Value: eval.Int(1)},
		}),
	}}})
	if err == nil || !strings.Contains(err.Error(), "key must be a string") ||
		!strings.Contains(err.Error(), "must produce a valid directory name") ||
		!strings.Contains(err.Error(), "must be a string or a list of strings") ||
		strings.Count(err.Error(), ";") < 2 {
		t.Fatalf("expected joined benchmark config problems, got %v", err)
	}
}

func TestParameterComponentSelectionsBranches(t *testing.T) {
	inputs := runtimeInputs{RootName: "bench"}
	if _, _, err := parameterComponentSelections(inputs, benchmarks.Config{}, "small"); err == nil ||
		!strings.Contains(err.Error(), "--benchmark requires non-empty jbs_benchmarks") {
		t.Fatalf("expected selected benchmark without config error, got %v", err)
	}

	all := benchmarks.Spec{Name: "all", DirName: "all", AllSteps: true}
	small := benchmarks.Spec{Name: "small", DirName: "small", Targets: []string{"run"}}
	cfg := benchmarks.Config{
		Configured: true,
		Specs:      []benchmarks.Spec{all, small},
		ByName: map[string]benchmarks.Spec{
			"all":   all,
			"small": small,
		},
	}

	if _, _, err := parameterComponentSelections(inputs, cfg, "missing"); err == nil ||
		!strings.Contains(err.Error(), "unknown benchmark") {
		t.Fatalf("expected unknown selected benchmark error, got %v", err)
	}

	sel, configured, err := parameterComponentSelections(inputs, benchmarks.Config{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if configured || len(sel) != 1 || sel[0].ComponentName != "bench" || sel[0].Configured {
		t.Fatalf("unconfigured selections = configured %v %#v", configured, sel)
	}

	sel, configured, err = parameterComponentSelections(inputs, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if !configured || len(sel) != 2 {
		t.Fatalf("configured selections = configured %v %#v", configured, sel)
	}
	if sel[0].RootDir != filepath.Join("bench", "all") ||
		sel[0].ComponentName != "all" ||
		sel[0].ComponentDir != "all" ||
		sel[0].TablePrefix != "bench_all" ||
		!sel[0].Spec.AllSteps {
		t.Fatalf("wildcard selection = %#v", sel[0])
	}
}

func TestBuildParameterPlanSuiteSelectedBenchmarkAndErrors(t *testing.T) {
	if _, err := BuildParameterPlanSuite(Options{}, &diag.Diagnostics{}); err == nil ||
		!strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("expected build input error, got %v", err)
	}

	_, err := BuildParameterPlanSuite(Options{
		Result: &sema.Result{
			Globals: sema.GlobalState{Values: map[string]eval.Value{
				"jbs_name":       eval.String("bench"),
				"jbs_benchmarks": eval.String("bad"),
			}},
			DoBlocks:  []ast.DoBlock{{Name: "run"}},
			StepOrder: []string{"run"},
		},
		Sources:     map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		ProgramFile: "<test>",
	}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "jbs_benchmarks must be a dictionary") {
		t.Fatalf("expected benchmark config error, got %v", err)
	}

	_, err = BuildParameterPlanSuite(Options{
		Result: &sema.Result{
			Globals:   sema.GlobalState{Values: map[string]eval.Value{"jbs_name": eval.String("bench")}},
			DoBlocks:  []ast.DoBlock{{Name: "run"}},
			StepOrder: []string{"run"},
		},
		Sources:     map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		ProgramFile: "<test>",
		Benchmark:   "small",
	}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "--benchmark requires non-empty jbs_benchmarks") {
		t.Fatalf("expected benchmark selection error, got %v", err)
	}

	suite := buildParameterSuiteFromSourceWithOptions(t, `
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
`, Options{Benchmark: "large"})
	if !suite.Configured || len(suite.Plans) != 1 || suite.Plans[0].ComponentName != "large" {
		t.Fatalf("selected parameter suite = %#v", suite)
	}
	if got := planWorkForStep(suite.Plans[0].WorkPlan.Work, "run_large"); len(got) != 1 {
		t.Fatalf("selected parameter work for run_large = %#v", got)
	}
	if got := planWorkForStep(suite.Plans[0].WorkPlan.Work, "run_small"); len(got) != 0 {
		t.Fatalf("unselected run_small work retained: %#v", got)
	}

	_, err = BuildParameterPlanSuite(Options{
		Result: &sema.Result{
			Globals: sema.GlobalState{Values: map[string]eval.Value{
				"jbs_name": eval.String("bench"),
				"jbs_benchmarks": eval.DictValue([]eval.DictEntry{{
					Key:   eval.DictKey{Kind: eval.DictKeyString, S: "small"},
					Value: eval.String("missing"),
				}}),
			}},
			DoBlocks:  []ast.DoBlock{{Name: "run"}},
			StepOrder: []string{"run"},
		},
		Sources:     map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		ProgramFile: "<test>",
		Benchmark:   "small",
	}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "unknown benchmark target step") {
		t.Fatalf("expected selected benchmark target error, got %v", err)
	}

	_, err = applyWorkLimit(workplan.Plan{
		Steps: []workplan.Step{{Name: "run"}},
		Work: []workplan.WorkPackage{{
			ID:       workplan.WorkID{Step: "run", Row: 0},
			StepName: "run",
			Deps:     []workplan.WorkID{{Step: "missing", Row: 0}},
		}},
	}, nil, 1)
	if err == nil || !strings.Contains(err.Error(), "depends on missing workpackage") {
		t.Fatalf("expected work-limit missing ancestor error, got %v", err)
	}
}

func TestBuildRuntimeInputsErrorBranches(t *testing.T) {
	if _, err := buildRuntimeInputs(Options{}, &diag.Diagnostics{}); err == nil ||
		!strings.Contains(err.Error(), "missing analysis result") {
		t.Fatalf("expected missing result error, got %v", err)
	}

	_, err := buildRuntimeInputs(Options{Result: &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{"jbs_name": eval.Int(1)}},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "jbs_name must be a string") {
		t.Fatalf("expected benchmark name error, got %v", err)
	}

	_, err = buildRuntimeInputs(Options{Result: &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{"jbs_name": eval.String("bench")}},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "requires at least one do block") {
		t.Fatalf("expected missing do-block error, got %v", err)
	}

	_, err = buildRuntimeInputs(Options{Result: &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name":  eval.String("bench"),
			"jbs_nproc": eval.Int(-1),
		}},
		DoBlocks:  []ast.DoBlock{{Name: "run"}},
		StepOrder: []string{"run"},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "jbs_nproc must be >=") {
		t.Fatalf("expected negative global nproc error, got %v", err)
	}

	_, err = buildRuntimeInputs(Options{Result: &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name":  eval.String("bench"),
			"jbs_nproc": eval.String("bad"),
		}},
		DoBlocks:  []ast.DoBlock{{Name: "run"}},
		StepOrder: []string{"run"},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "jbs_nproc must be an integer") {
		t.Fatalf("expected invalid global nproc error, got %v", err)
	}

	neg := -1
	_, err = buildRuntimeInputs(Options{Result: &sema.Result{
		Globals:   sema.GlobalState{Values: map[string]eval.Value{"jbs_name": eval.String("bench")}},
		DoBlocks:  []ast.DoBlock{{Name: "run", NProc: &neg}},
		StepOrder: []string{"run"},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), `do step "run" has invalid nproc=-1`) {
		t.Fatalf("expected invalid step nproc error, got %v", err)
	}

	_, err = buildRuntimeInputs(Options{Result: &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"jbs_name":     eval.String("bench"),
			"jbs_database": eval.Int(1),
		}},
		DoBlocks:  []ast.DoBlock{{Name: "run"}},
		StepOrder: []string{"run"},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "jbs_database must be a string") {
		t.Fatalf("expected analyse database error, got %v", err)
	}

	_, err = buildRuntimeInputs(Options{Result: &sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{"jbs_name": eval.String("bench")}},
		DoBlocks: []ast.DoBlock{{
			Name: "run",
			FSubs: []ast.FileSubstitution{{
				Path: "template.in",
				Rules: []ast.FileSubstitutionRule{{
					Pattern: "[",
					Expr:    ast.StringExpr{Value: "x"},
				}},
			}},
		}},
		StepOrder: []string{"run"},
	}}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "invalid fsub regex") {
		t.Fatalf("expected fsub planning error, got %v", err)
	}

	res, sources := analyzeRuntimeSource(t, `
jbs_name = "bench"
do run {
    echo run
}
`)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	removedWD := t.TempDir()
	if err := os.Chdir(removedWD); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	if err := os.RemoveAll(removedWD); err != nil {
		t.Fatal(err)
	}
	_, err = buildRuntimeInputs(Options{Result: res, Sources: sources, ProgramFile: "<repl>"}, &diag.Diagnostics{})
	if err == nil || !strings.Contains(err.Error(), "determine source directory") {
		t.Fatalf("expected source-directory error, got %v", err)
	}
}

func TestBuildComponentRuntimePlanFSubErrorAndConfiguredManifest(t *testing.T) {
	_, err := buildComponentRuntimePlan(runtimeInputs{
		RootName: "bench",
		Sources:  map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		WorkPlan: simpleRuntimeWorkPlan(),
	}, componentSelection{
		Configured: true,
		Spec:       benchmarks.Spec{Name: "small", DirName: "small", Targets: []string{"missing"}},
		RootDir:    filepath.Join("bench", "small"),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown benchmark target step") {
		t.Fatalf("expected component target error, got %v", err)
	}

	_, err = buildComponentRuntimePlan(runtimeInputs{
		RootName: "bench",
		Sources:  map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		Limit:    1,
		WorkPlan: workplan.Plan{
			BenchmarkName: "bench",
			GlobalNProc:   1,
			Steps:         []workplan.Step{{Name: "run", Kind: "do", NProc: 1}},
			Work: []workplan.WorkPackage{{
				ID:       workplan.WorkID{Step: "run", Row: 0},
				StepName: "run",
				StepKind: "do",
				Deps:     []workplan.WorkID{{Step: "missing", Row: 0}},
			}},
		},
	}, componentSelection{RootDir: "bench", ComponentName: "bench"})
	if err == nil || !strings.Contains(err.Error(), "depends on missing workpackage") {
		t.Fatalf("expected component limit error, got %v", err)
	}

	inputs := runtimeInputs{
		RootName: "bench",
		Sources:  map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		WorkPlan: simpleRuntimeWorkPlan(),
		FileSubs: map[string][]FileSubstitutionPlan{
			"run": {{SourcePath: filepath.Join(t.TempDir(), "missing.tpl"), DestName: "missing.tpl"}},
		},
	}
	_, err = buildComponentRuntimePlan(inputs, componentSelection{RootDir: "bench", ComponentName: "bench", TablePrefix: "bench"})
	if err == nil || !strings.Contains(err.Error(), "fsub template") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected fsub snapshot error, got %v", err)
	}

	plan, err := buildComponentRuntimePlan(runtimeInputs{
		RootName:  "bench",
		Sources:   map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		WorkPlan:  simpleRuntimeWorkPlan(),
		SourceDir: t.TempDir(),
	}, componentSelection{
		Configured:    true,
		Spec:          benchmarks.Spec{Name: "all", DirName: "all", AllSteps: true},
		RootDir:       filepath.Join("bench", "all"),
		ComponentName: "all",
		ComponentDir:  "all",
		TablePrefix:   "bench_all",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Manifest.BenchmarkComponent != "all" || plan.Manifest.AnalyseTablePrefix != "bench_all" {
		t.Fatalf("configured manifest fields missing: %#v", plan.Manifest)
	}

	_, err = buildComponentRuntimePlan(runtimeInputs{
		RootName: "bench",
		Sources:  map[string]string{"test.jbs": "do run {\n echo run\n}\n"},
		WorkPlan: workplan.Plan{
			BenchmarkName: "bench",
			GlobalNProc:   1,
			Steps:         []workplan.Step{{Name: "run", Kind: "do", NProc: 1}},
			Work: []workplan.WorkPackage{{
				ID:       workplan.WorkID{Step: "run", Row: 0},
				StepName: "run",
				StepKind: "do",
				Values:   map[string]eval.Value{"bad-name": eval.Int(1)},
			}},
		},
	}, componentSelection{RootDir: "bench", ComponentName: "bench"})
	if err == nil || !strings.Contains(err.Error(), "cannot be emitted as a shell assignment") {
		t.Fatalf("expected shell assignment variable error, got %v", err)
	}

	plan, err = buildComponentRuntimePlan(runtimeInputs{
		RootName: "bench",
		Sources:  map[string]string{"test.jbs": "do dep {\n:\n}\ndo run after dep {\n:\n}\n"},
		WorkPlan: workplan.Plan{
			BenchmarkName: "bench",
			GlobalNProc:   1,
			Steps: []workplan.Step{
				{Name: "!", Kind: "do", NProc: 1},
				{Name: "run", Kind: "do", NProc: 1},
			},
			Work: []workplan.WorkPackage{
				{ID: workplan.WorkID{Step: "!", Row: 0}, StepName: "!", StepKind: "do"},
				{ID: workplan.WorkID{Step: "run", Row: 0}, StepName: "run", StepKind: "do", Deps: []workplan.WorkID{
					{Step: "!", Row: 0},
					{Step: "!", Row: 0},
				}},
			},
		},
	}, componentSelection{RootDir: "bench", ComponentName: "bench"})
	if err != nil {
		t.Fatal(err)
	}
	var depLinks []string
	for _, work := range plan.Manifest.Work {
		if work.Step == "run" {
			for _, dep := range work.Deps {
				depLinks = append(depLinks, dep.Link)
			}
		}
	}
	if strings.Join(depLinks, ",") != "dep,dep_1_dep" {
		t.Fatalf("dependency links = %#v", depLinks)
	}
}

func TestNormalizeLockRuntimeDefaultsAndPreservesProvided(t *testing.T) {
	defaulted := normalizeLockRuntime(lockRuntime{})
	if defaulted.pid() <= 0 {
		t.Fatalf("default pid = %d", defaulted.pid())
	}
	if host, err := defaulted.hostname(); err != nil || host == "" {
		t.Fatalf("default hostname = %q, %v", host, err)
	}
	if defaulted.now().IsZero() {
		t.Fatal("default time is zero")
	}
	if alive, err := defaulted.processAlive(os.Getpid()); err != nil || !alive {
		t.Fatalf("default processAlive(current) = %v, %v", alive, err)
	}

	wantTime := time.Unix(123, 0).UTC()
	custom := normalizeLockRuntime(lockRuntime{
		pid:      func() int { return 42 },
		hostname: func() (string, error) { return "node", nil },
		now:      func() time.Time { return wantTime },
		processAlive: func(pid int) (bool, error) {
			return pid == 7, nil
		},
	})
	if custom.pid() != 42 {
		t.Fatalf("custom pid = %d", custom.pid())
	}
	if host, err := custom.hostname(); err != nil || host != "node" {
		t.Fatalf("custom hostname = %q, %v", host, err)
	}
	if !custom.now().Equal(wantTime) {
		t.Fatalf("custom time = %s", custom.now())
	}
	if alive, err := custom.processAlive(7); err != nil || !alive {
		t.Fatalf("custom processAlive = %v, %v", alive, err)
	}
}

func TestLocalProcessAliveBranches(t *testing.T) {
	for _, pid := range []int{0, -1} {
		alive, err := localProcessAlive(pid)
		if err != nil || alive {
			t.Fatalf("localProcessAlive(%d) = %v, %v", pid, alive, err)
		}
	}
	alive, err := localProcessAlive(os.Getpid())
	if err != nil || !alive {
		t.Fatalf("localProcessAlive(current) = %v, %v", alive, err)
	}
	alive, err = localProcessAlive(1 << 30)
	if err != nil || alive {
		t.Fatalf("localProcessAlive(missing) = %v, %v", alive, err)
	}
}

func TestInspectLockClassifiesMalformedMetadataFields(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	valid := lockInfo{Schema: 1, PID: 99, Hostname: "node", CreatedAt: time.Unix(100, 0).UTC()}
	cases := []lockInfo{
		{Schema: 2, PID: valid.PID, Hostname: valid.Hostname, CreatedAt: valid.CreatedAt},
		{Schema: valid.Schema, PID: 0, Hostname: valid.Hostname, CreatedAt: valid.CreatedAt},
		{Schema: valid.Schema, PID: valid.PID, Hostname: "", CreatedAt: valid.CreatedAt},
		{Schema: valid.Schema, PID: valid.PID, Hostname: valid.Hostname},
	}
	for _, info := range cases {
		writeLockInfoForTest(t, lockPath, info)
		if got := inspectLock(lockPath, testLockRuntime("node", 1, nil)); got.Class != lockMalformed {
			t.Fatalf("inspectLock(%#v) class = %v", info, got.Class)
		}
	}
}

func TestMaybeReclaimStaleLockBranches(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	rt := testLockRuntime("node", 777, func(pid int) (bool, error) {
		return pid == 777, nil
	})

	if err := maybeReclaimStaleLock(lockPath, rt); err != nil {
		t.Fatalf("missing lock reclaim = %v", err)
	}

	live := lockInfo{Schema: 1, PID: 777, Hostname: "node", CreatedAt: time.Unix(100, 0).UTC()}
	writeLockInfoForTest(t, lockPath, live)
	if err := maybeReclaimStaleLock(lockPath, rt); err == nil || !strings.Contains(err.Error(), "is locked") {
		t.Fatalf("expected live lock error, got %v", err)
	}

	if err := os.WriteFile(lockPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := maybeReclaimStaleLock(lockPath, rt); err == nil || !strings.Contains(err.Error(), "invalid lock metadata") {
		t.Fatalf("expected malformed lock error, got %v", err)
	}

	stale := lockInfo{Schema: 1, PID: 123, Hostname: "node", CreatedAt: time.Unix(100, 0).UTC()}
	writeLockInfoForTest(t, lockPath, stale)
	oldRemove := lockRemove
	lockRemove = func(string) error { return errors.New("remove denied") }
	t.Cleanup(func() { lockRemove = oldRemove })
	if err := maybeReclaimStaleLock(lockPath, rt); err == nil || !strings.Contains(err.Error(), "could not be reclaimed") {
		t.Fatalf("expected stale delete error, got %v", err)
	}
}

func TestAvailableNProcFallbacks(t *testing.T) {
	oldMax := runtimeGOMAXPROCS
	oldCPU := runtimeNumCPU
	t.Cleanup(func() {
		runtimeGOMAXPROCS = oldMax
		runtimeNumCPU = oldCPU
	})

	runtimeGOMAXPROCS = func(int) int { return 0 }
	runtimeNumCPU = func() int { return 3 }
	if got := availableNProc(); got != 3 {
		t.Fatalf("availableNProc NumCPU fallback = %d, want 3", got)
	}

	runtimeNumCPU = func() int { return 0 }
	if got := availableNProc(); got != 1 {
		t.Fatalf("availableNProc defensive fallback = %d, want 1", got)
	}
}

func TestWithSignalsStopAndInterruptPaths(t *testing.T) {
	ctx, stop := withSignals(context.Background(), nil)
	stop()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not cancel signal context")
	}

	oldExit := signalExit
	exitCodes := make(chan int, 1)
	signalExit = func(code int) {
		exitCodes <- code
	}
	t.Cleanup(func() { signalExit = oldExit })

	before := make(chan struct{}, 1)
	ctx, stop = withSignals(context.Background(), func() {
		before <- struct{}{}
	})
	defer stop()
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("interrupt did not cancel signal context")
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-before:
	case <-time.After(2 * time.Second):
		t.Fatal("second interrupt did not run hard-exit hook")
	}
	select {
	case code := <-exitCodes:
		if code != 130 {
			t.Fatalf("signal exit code = %d, want 130", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second interrupt did not call signalExit")
	}
}

func TestRewriteArchiveWithSnapshotCopiesExistingAndReturnsSnapshot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bench")
	createArchiveRun(t, root, "000000", StatusFinished, map[string]string{
		"step/000000/stdout": "new\n",
	})
	archive := filepath.Join(dir, "bench.tar.gz")
	writeTarGzEntries(t, archive, []archiveTestEntry{{
		Header: tar.Header{Name: "old/file.txt", Mode: 0o644, Size: int64(len("old\n")), Typeflag: tar.TypeReg},
		Body:   "old\n",
	}})

	snapshot, err := rewriteArchiveWithSnapshot(root, archive, "stamp", []string{"000000"})
	if err != nil {
		t.Fatal(err)
	}
	entries := readTarGzEntries(t, archive)
	if got := entries["old/file.txt"].Body; got != "old\n" {
		t.Fatalf("copied existing entry body = %q", got)
	}
	if _, ok := entries["stamp/bench/000000/status"]; !ok {
		t.Fatalf("new run status missing from archive entries: %#v", sortedArchiveNames(entries))
	}
	if !slices.ContainsFunc(snapshot.Paths, func(item archivedPath) bool {
		return filepath.Clean(item.FSPath) == filepath.Join(root, "000000", "step", "000000", "stdout") && item.Kind == archivedPathFile
	}) {
		t.Fatalf("snapshot does not contain archived stdout path: %#v", snapshot.Paths)
	}
}

func TestRewriteArchiveWithSnapshotReportsFinalizationErrors(t *testing.T) {
	t.Run("chmod", func(t *testing.T) {
		withArchiveTempFile(t, &fakeArchiveTemp{name: filepath.Join(t.TempDir(), "tmp"), chmodErr: errors.New("chmod denied")})
		_, err := rewriteArchiveWithSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "bench.tar.gz"), "stamp", nil)
		if err == nil || !strings.Contains(err.Error(), "chmod denied") {
			t.Fatalf("expected chmod error, got %v", err)
		}
	})

	t.Run("sync", func(t *testing.T) {
		withArchiveTempFile(t, &fakeArchiveTemp{name: filepath.Join(t.TempDir(), "tmp"), syncErr: errors.New("sync denied")})
		_, err := rewriteArchiveWithSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "bench.tar.gz"), "stamp", nil)
		if err == nil || !strings.Contains(err.Error(), "sync denied") {
			t.Fatalf("expected sync error, got %v", err)
		}
	})

	t.Run("close", func(t *testing.T) {
		withArchiveTempFile(t, &fakeArchiveTemp{name: filepath.Join(t.TempDir(), "tmp"), closeErr: errors.New("close denied")})
		_, err := rewriteArchiveWithSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "bench.tar.gz"), "stamp", nil)
		if err == nil || !strings.Contains(err.Error(), "close denied") {
			t.Fatalf("expected close error, got %v", err)
		}
	})

	t.Run("rename", func(t *testing.T) {
		oldRename := archiveRename
		archiveRename = func(string, string) error { return errors.New("rename denied") }
		t.Cleanup(func() { archiveRename = oldRename })
		_, err := rewriteArchiveWithSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "bench.tar.gz"), "stamp", nil)
		if err == nil || !strings.Contains(err.Error(), "rename denied") {
			t.Fatalf("expected rename error, got %v", err)
		}
	})

	t.Run("sync dir", func(t *testing.T) {
		oldSyncDir := archiveSyncDir
		archiveSyncDir = func(string) error { return errors.New("sync dir denied") }
		t.Cleanup(func() { archiveSyncDir = oldSyncDir })
		_, err := rewriteArchiveWithSnapshot(t.TempDir(), filepath.Join(t.TempDir(), "bench.tar.gz"), "stamp", nil)
		if err == nil || !strings.Contains(err.Error(), "sync dir denied") {
			t.Fatalf("expected sync-dir error, got %v", err)
		}
	})
}

func TestRemoveArchivedFileBranches(t *testing.T) {
	removed, err := removeArchivedFile(filepath.Join(t.TempDir(), "missing"))
	if err != nil || removed {
		t.Fatalf("missing file removal = %v, %v", removed, err)
	}

	dir := t.TempDir()
	removed, err = removeArchivedFile(dir)
	if err != nil || removed {
		t.Fatalf("directory-at-file removal = %v, %v", removed, err)
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRemove := archiveRemove
	archiveRemove = func(string) error { return os.ErrNotExist }
	removed, err = removeArchivedFile(file)
	if err != nil || removed {
		t.Fatalf("file removal racing with missing file = %v, %v", removed, err)
	}

	archiveRemove = func(string) error { return errors.New("permission denied") }
	removed, err = removeArchivedFile(file)
	if err == nil || removed || !strings.Contains(err.Error(), "failed to remove archived file") {
		t.Fatalf("file removal permission error = %v, %v", removed, err)
	}

	archiveRemove = oldRemove
	removed, err = removeArchivedFile(file)
	if err != nil || !removed {
		t.Fatalf("successful file removal = %v, %v", removed, err)
	}
}

func TestRemoveArchivedDirIfEmptyBranches(t *testing.T) {
	removed, err := removeArchivedDirIfEmpty(filepath.Join(t.TempDir(), "missing"))
	if err != nil || removed {
		t.Fatalf("missing dir removal = %v, %v", removed, err)
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err = removeArchivedDirIfEmpty(file)
	if err != nil || removed {
		t.Fatalf("file-at-dir removal = %v, %v", removed, err)
	}

	nonEmpty := filepath.Join(t.TempDir(), "nonempty")
	if err := os.Mkdir(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err = removeArchivedDirIfEmpty(nonEmpty)
	if err != nil || removed {
		t.Fatalf("non-empty dir removal = %v, %v", removed, err)
	}

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	oldRemove := archiveRemove
	archiveRemove = func(string) error { return os.ErrNotExist }
	removed, err = removeArchivedDirIfEmpty(empty)
	if err != nil || removed {
		t.Fatalf("dir removal racing with missing dir = %v, %v", removed, err)
	}

	archiveRemove = func(string) error { return syscall.ENOTEMPTY }
	removed, err = removeArchivedDirIfEmpty(empty)
	if err != nil || removed {
		t.Fatalf("dir removal racing with non-empty dir = %v, %v", removed, err)
	}

	archiveRemove = func(string) error { return errors.New("permission denied") }
	removed, err = removeArchivedDirIfEmpty(empty)
	if err == nil || removed || !strings.Contains(err.Error(), "failed to remove archived directory") {
		t.Fatalf("dir removal permission error = %v, %v", removed, err)
	}

	archiveRemove = oldRemove
	removed, err = removeArchivedDirIfEmpty(empty)
	if err != nil || !removed {
		t.Fatalf("successful dir removal = %v, %v", removed, err)
	}
}

func analyzeRuntimeSource(t *testing.T, source string) (*sema.Result, map[string]string) {
	t.Helper()
	diags := &diag.Diagnostics{}
	cwd := t.TempDir()
	loadRes, err := jbsimports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return res, loadRes.Sources
}

func simpleRuntimeWorkPlan() workplan.Plan {
	return workplan.Plan{
		BenchmarkName: "bench",
		SourceHash:    "hash",
		GlobalNProc:   1,
		Steps: []workplan.Step{{
			Name:  "run",
			Kind:  "do",
			NProc: 1,
			Body:  "echo run",
		}},
		Work: []workplan.WorkPackage{{
			ID:       workplan.WorkID{Step: "run", Row: 0},
			StepName: "run",
			StepKind: "do",
			Values:   map[string]eval.Value{},
		}},
	}
}

type fakeArchiveTemp struct {
	bytes.Buffer
	name     string
	chmodErr error
	syncErr  error
	closeErr error
}

func (f *fakeArchiveTemp) Chmod(fs.FileMode) error {
	return f.chmodErr
}

func (f *fakeArchiveTemp) Sync() error {
	return f.syncErr
}

func (f *fakeArchiveTemp) Close() error {
	return f.closeErr
}

func (f *fakeArchiveTemp) Name() string {
	return f.name
}

func withArchiveTempFile(t *testing.T, tmp archiveTempFile) {
	t.Helper()
	oldCreateTemp := archiveCreateTemp
	archiveCreateTemp = func(string, string) (archiveTempFile, error) {
		return tmp, nil
	}
	t.Cleanup(func() { archiveCreateTemp = oldCreateTemp })
}
