package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

func TestCreateRunDirectoryWithInitialBuildsAtomicRunTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	sourceDir := t.TempDir()
	plan := testRuntimePlan(sourceDir)

	store, warnings, err := CreateRunDirectoryWithInitial(root, plan, StatusRunning)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if store.RunDir != filepath.Join(root, "000000") {
		t.Fatalf("run dir = %q, want first run directory", store.RunDir)
	}
	if store.Manifest.RunID != "000000" {
		t.Fatalf("run id = %q, want 000000", store.Manifest.RunID)
	}
	if store.Manifest.CreatedAt.IsZero() {
		t.Fatal("manifest CreatedAt was not set")
	}
	if entries, err := filepath.Glob(filepath.Join(root, ".creating-*")); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("staging directories were left behind: %v", entries)
	}

	rootStatus, err := store.LoadRootStatus()
	if err != nil {
		t.Fatal(err)
	}
	if rootStatus.Status != StatusRunning {
		t.Fatalf("root status = %s, want %s", rootStatus.Status, StatusRunning)
	}
	if rootStatus.PID != os.Getpid() {
		t.Fatalf("root pid = %d, want %d", rootStatus.PID, os.Getpid())
	}
	if rootStatus.SourceHash != "hash" {
		t.Fatalf("source hash = %q, want hash", rootStatus.SourceHash)
	}

	work := store.Manifest.Work[1]
	runScript, err := os.ReadFile(filepath.Join(store.WorkDir(work), "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(runScript)
	if strings.Contains(text, ".creating-") {
		t.Fatalf("run.sh uses staging path:\n%s", text)
	}
	for _, want := range []string{
		"set -euo pipefail",
		"export JBS_RUN_DIR='" + filepath.Clean(store.RunDir) + "'",
		"export JBS_SRC_DIR='" + filepath.Clean(sourceDir) + "'",
		"export JBS_WORK_DIR='" + filepath.Clean(store.WorkDir(work)) + "'",
		"export x='2'",
		"echo child",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("run.sh missing %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(store.WorkDir(work), "stdout")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.WorkDir(work), "stderr")); err != nil {
		t.Fatal(err)
	}
	if status, err := store.LoadWorkStatus(work); err != nil {
		t.Fatal(err)
	} else if status.Status != StatusNotStarted {
		t.Fatalf("work status = %s, want %s", status.Status, StatusNotStarted)
	}
	if link, err := os.Readlink(filepath.Join(store.WorkDir(work), "parent")); err != nil {
		t.Fatal(err)
	} else if filepath.IsAbs(link) || filepath.Clean(filepath.Join(store.WorkDir(work), link)) != store.WorkDir(store.Manifest.Work[0]) {
		t.Fatalf("dependency link = %q, want relative path to parent work", link)
	}
	if data, err := os.ReadFile(filepath.Join(store.RunDir, "step", "analyse.csv")); err != nil {
		t.Fatal(err)
	} else if string(data) != "run_id,x\n" {
		t.Fatalf("analyse header = %q, want CSV header", string(data))
	}

	second, _, err := CreateRunDirectoryWithInitial(root, plan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	if second.RunDir != filepath.Join(root, "000001") {
		t.Fatalf("second run dir = %q, want incremented run directory", second.RunDir)
	}
	status, err := second.LoadRootStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusNotStarted || status.PID != 0 {
		t.Fatalf("dry root status = %#v, want NOTSTARTED without pid", status)
	}
}

func TestCreateRunDirectoryRejectsInvalidInitialStatus(t *testing.T) {
	_, _, err := CreateRunDirectoryWithInitial(t.TempDir(), runtimePlan{}, StatusFinished)
	if err == nil {
		t.Fatal("expected invalid initial status error")
	}
	if !strings.Contains(err.Error(), "invalid initial root status FINISHED") {
		t.Fatalf("error = %v, want invalid initial status", err)
	}
}

func TestCreateRunDirectoryReportsSetupAndPersistenceErrors(t *testing.T) {
	t.Run("root lock setup", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "root-file")
		if err := os.WriteFile(root, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := CreateRunDirectoryWithInitial(root, testRuntimePlan(t.TempDir()), StatusRunning); err == nil {
			t.Fatal("expected root setup error")
		}
	})

	t.Run("run id read failure", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permission-denied directory read is not reliable as root")
		}
		root := filepath.Join(t.TempDir(), "no-read")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o333); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(root, 0o755)
		})
		if _, _, err := CreateRunDirectoryWithInitial(root, testRuntimePlan(t.TempDir()), StatusRunning); err == nil {
			t.Fatal("expected run id read error")
		}
	})

	t.Run("duplicate sqlite analyse table", func(t *testing.T) {
		root := t.TempDir()
		plan := runtimePlan{
			Manifest: Manifest{
				Schema:              1,
				SourceHash:          "hash",
				BenchmarkName:       "bench",
				GlobalNProc:         1,
				AnalyseDatabasePath: filepath.Join(root, "analysis.sqlite"),
				Steps: []ManifestStep{
					{Name: "same", Dir: "one", NProc: 1, AnalyseTable: "pending"},
					{Name: "same", Dir: "two", NProc: 1, AnalyseTable: "pending"},
				},
			},
			SourceDir: t.TempDir(),
		}
		if _, _, err := CreateRunDirectoryWithInitial(root, plan, StatusRunning); err == nil ||
			!strings.Contains(err.Error(), "duplicate analyse table name") {
			t.Fatalf("error = %v, want duplicate table error", err)
		}
	})

	t.Run("staging path exists", func(t *testing.T) {
		root := t.TempDir()
		staging := filepath.Join(root, "bench", ".creating-000000-"+strconv.Itoa(os.Getpid()))
		if err := os.MkdirAll(filepath.Dir(staging), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(staging, []byte("occupied\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := CreateRunDirectoryWithInitial(filepath.Join(root, "bench"), testRuntimePlan(t.TempDir()), StatusRunning)
		if err == nil {
			t.Fatal("expected staging creation error")
		}
	})

	t.Run("populate failure cleans staging", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bench")
		plan := testRuntimePlan(t.TempDir())
		plan.Analyses = nil
		_, _, err := CreateRunDirectoryWithInitial(root, plan, StatusRunning)
		if err == nil || !strings.Contains(err.Error(), "missing analyse plan") {
			t.Fatalf("error = %v, want populate error", err)
		}
		if entries, err := filepath.Glob(filepath.Join(root, ".creating-*")); err != nil {
			t.Fatal(err)
		} else if len(entries) != 0 {
			t.Fatalf("staging directories were left behind: %v", entries)
		}
	})

	t.Run("manifest path is directory", func(t *testing.T) {
		root := t.TempDir()
		plan := runtimePlan{
			Manifest: Manifest{
				Schema:        1,
				SourceHash:    "hash",
				BenchmarkName: "bench",
				GlobalNProc:   1,
				Steps:         []ManifestStep{{Name: "step", Dir: "manifest.json", NProc: 1}},
			},
			SourceDir: t.TempDir(),
		}
		if _, _, err := CreateRunDirectoryWithInitial(root, plan, StatusRunning); err == nil {
			t.Fatal("expected manifest write error")
		}
	})

	t.Run("status path is directory", func(t *testing.T) {
		root := t.TempDir()
		plan := runtimePlan{
			Manifest: Manifest{
				Schema:        1,
				SourceHash:    "hash",
				BenchmarkName: "bench",
				GlobalNProc:   1,
				Steps:         []ManifestStep{{Name: "step", Dir: "status", NProc: 1}},
			},
			SourceDir: t.TempDir(),
		}
		if _, _, err := CreateRunDirectoryWithInitial(root, plan, StatusRunning); err == nil {
			t.Fatal("expected status write error")
		}
	})

	t.Run("final path exists as file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "000000"), []byte("occupied\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := CreateRunDirectoryWithInitial(root, testRuntimePlan(t.TempDir()), StatusRunning); err == nil {
			t.Fatal("expected final rename error")
		}
	})
}

func TestCreateRunDirectoryResolvesDefaultAndRelativeSourceDirs(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wd := t.TempDir()
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	root := filepath.Join(t.TempDir(), "bench")
	defaultPlan := testRuntimePlan("")
	defaultStore, _, err := CreateRunDirectoryWithInitial(root, defaultPlan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	defaultScript, err := os.ReadFile(filepath.Join(defaultStore.WorkDir(defaultStore.Manifest.Work[0]), "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(defaultScript), "export JBS_SRC_DIR='"+filepath.Clean(wd)+"'") {
		t.Fatalf("default source dir not written from working directory:\n%s", defaultScript)
	}

	relativePlan := testRuntimePlan("relative-src")
	relativeStore, _, err := CreateRunDirectoryWithInitial(root, relativePlan, StatusNotStarted)
	if err != nil {
		t.Fatal(err)
	}
	relativeScript, err := os.ReadFile(filepath.Join(relativeStore.WorkDir(relativeStore.Manifest.Work[0]), "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wd, "relative-src")
	if !strings.Contains(string(relativeScript), "export JBS_SRC_DIR='"+filepath.Clean(want)+"'") {
		t.Fatalf("relative source dir was not absolutized to %q:\n%s", want, relativeScript)
	}
}

func TestPopulateRunTreeUsesSQLiteAnalyseBackendWithoutCSVPlan(t *testing.T) {
	temp := t.TempDir()
	manifest := Manifest{
		Schema:              1,
		SourceHash:          "hash",
		BenchmarkName:       "bench",
		RunID:               "000000",
		GlobalNProc:         1,
		AnalyseDatabasePath: filepath.Join(temp, "analysis.sqlite"),
		Steps:               []ManifestStep{{Name: "step", Dir: "step", NProc: 1, AnalyseCSV: "analyse.csv"}},
		Work:                []ManifestWork{{Step: "step", Row: 0, Dir: "000000"}},
	}
	staging := filepath.Join(temp, "staging")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := populateRunTree(staging, filepath.Join(temp, "final"), temp, manifest, map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staging, "step", "analyse.csv")); !os.IsNotExist(err) {
		t.Fatalf("SQLite-backed analyse should not create CSV header, stat error: %v", err)
	}
	runScript, err := os.ReadFile(filepath.Join(staging, "step", "000000", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(runScript), "set -euo pipefail") {
		t.Fatalf("NoStrict run.sh should not contain strict mode:\n%s", runScript)
	}
}

func TestPopulateRunTreeReportsInvalidPlans(t *testing.T) {
	temp := t.TempDir()
	baseManifest := Manifest{
		Schema:        1,
		SourceHash:    "hash",
		BenchmarkName: "bench",
		RunID:         "000000",
		GlobalNProc:   1,
		Steps:         []ManifestStep{{Name: "step", Dir: "step", NProc: 1, AnalyseCSV: "analyse.csv"}},
		Work:          []ManifestWork{{Step: "step", Row: 0, Dir: "000000"}},
	}

	t.Run("missing analyse plan", func(t *testing.T) {
		staging := filepath.Join(temp, "missing-analyse")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, filepath.Join(temp, "final"), temp, baseManifest, nil, nil, workplan.Plan{}, nil, false)
		if err == nil || !strings.Contains(err.Error(), `missing analyse plan for step "step"`) {
			t.Fatalf("error = %v, want missing analyse plan", err)
		}
	})

	t.Run("unknown work step", func(t *testing.T) {
		staging := filepath.Join(temp, "unknown-work-step")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := baseManifest
		manifest.Steps = []ManifestStep{{Name: "known", Dir: "known", NProc: 1}}
		manifest.Work = []ManifestWork{{Step: "missing", Row: 0, Dir: "000000"}}
		_, err := populateRunTree(staging, filepath.Join(temp, "final"), temp, manifest, map[string]string{"missing": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil || !strings.Contains(err.Error(), `unknown step "missing"`) {
			t.Fatalf("error = %v, want unknown step", err)
		}
	})

	t.Run("unknown dependency step", func(t *testing.T) {
		staging := filepath.Join(temp, "unknown-dep-step")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := baseManifest
		manifest.Steps = []ManifestStep{{Name: "step", Dir: "step", NProc: 1}}
		manifest.Work = []ManifestWork{{Step: "step", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "missing", Row: 0, Link: "dep"}}}}
		_, err := populateRunTree(staging, filepath.Join(temp, "final"), temp, manifest, map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil || !strings.Contains(err.Error(), `unknown dependency step "missing"`) {
			t.Fatalf("error = %v, want unknown dependency step", err)
		}
	})

	t.Run("unknown dependency workpackage", func(t *testing.T) {
		staging := filepath.Join(temp, "unknown-dep-work")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := baseManifest
		manifest.Steps = []ManifestStep{{Name: "step", Dir: "step", NProc: 1}}
		manifest.Work = []ManifestWork{{Step: "step", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "step", Row: 99, Link: "dep"}}}}
		_, err := populateRunTree(staging, filepath.Join(temp, "final"), temp, manifest, map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil || !strings.Contains(err.Error(), "unknown dependency workpackage step/000099") {
			t.Fatalf("error = %v, want unknown dependency workpackage", err)
		}
	})

	t.Run("relative final path", func(t *testing.T) {
		staging := filepath.Join(temp, "relative-final")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := baseManifest
		manifest.Steps = []ManifestStep{{Name: "step", Dir: "step", NProc: 1}}
		_, err := populateRunTree(staging, "relative-final", temp, manifest, map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil || !strings.Contains(err.Error(), "must be absolute") {
			t.Fatalf("error = %v, want absolute path validation", err)
		}
	})
}

func TestPopulateRunTreeReportsFilesystemErrors(t *testing.T) {
	temp := t.TempDir()
	final := filepath.Join(temp, "final")
	source := t.TempDir()

	t.Run("step dir path is file", func(t *testing.T) {
		staging := filepath.Join(temp, "step-dir-file")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staging, "step"), []byte("occupied\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), nil, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected step directory creation error")
		}
	})

	t.Run("analyse header parent missing", func(t *testing.T) {
		staging := filepath.Join(temp, "analyse-header-error")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := testPopulateManifest()
		manifest.Steps[0].AnalyseCSV = filepath.Join("missing", "analyse.csv")
		analyses := map[string]AnalysePlan{"step": {Step: "step", CSV: manifest.Steps[0].AnalyseCSV, Header: []string{"x"}}}
		_, err := populateRunTree(staging, final, source, manifest, nil, nil, workplan.Plan{}, analyses, false)
		if err == nil {
			t.Fatal("expected analyse header write error")
		}
	})

	t.Run("work dir path is file", func(t *testing.T) {
		staging := filepath.Join(temp, "work-dir-file")
		if err := os.MkdirAll(filepath.Join(staging, "step"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staging, "step", "000000"), []byte("occupied\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected work directory creation error")
		}
	})

	t.Run("dependency link path exists", func(t *testing.T) {
		staging := filepath.Join(temp, "symlink-error")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := testPopulateManifest()
		manifest.Work = append(manifest.Work, ManifestWork{
			Step: "step",
			Row:  1,
			Dir:  "000001",
			Deps: []ManifestWorkRef{{Step: "step", Row: 0, Link: ""}},
		})
		_, err := populateRunTree(staging, final, source, manifest, map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected symlink creation error")
		}
	})

	t.Run("fsub template snapshot missing", func(t *testing.T) {
		staging := filepath.Join(temp, "fsub-error")
		if err := os.Mkdir(staging, 0o755); err != nil {
			t.Fatal(err)
		}
		fileSubs := map[string][]FileSubstitutionPlan{"step": {{SourcePath: filepath.Join(temp, "missing-template"), DestName: "out.txt"}}}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), map[string]string{"step": "true"}, fileSubs, workplan.Plan{}, nil, false)
		if err == nil || !strings.Contains(err.Error(), "was not snapshotted") {
			t.Fatalf("error = %v, want fsub missing-snapshot error", err)
		}
	})

	t.Run("run script path is directory", func(t *testing.T) {
		staging := filepath.Join(temp, "run-script-error")
		if err := os.MkdirAll(filepath.Join(staging, "step", "000000", "run.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected run.sh write error")
		}
	})

	t.Run("stdout path is directory", func(t *testing.T) {
		staging := filepath.Join(temp, "stdout-error")
		if err := os.MkdirAll(filepath.Join(staging, "step", "000000", "stdout"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected stdout write error")
		}
	})

	t.Run("stderr path is directory", func(t *testing.T) {
		staging := filepath.Join(temp, "stderr-error")
		if err := os.MkdirAll(filepath.Join(staging, "step", "000000", "stderr"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected stderr write error")
		}
	})

	t.Run("status path is directory", func(t *testing.T) {
		staging := filepath.Join(temp, "status-error")
		if err := os.MkdirAll(filepath.Join(staging, "step", "000000", "status"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := populateRunTree(staging, final, source, testPopulateManifest(), map[string]string{"step": "true"}, nil, workplan.Plan{}, nil, false)
		if err == nil {
			t.Fatal("expected work status write error")
		}
	})
}

func TestStoreRootStatusTransitionsAndStaleNormalization(t *testing.T) {
	store, running, finished := testStoreWithStatuses(t)

	if err := store.MarkRootRunning(); err != nil {
		t.Fatal(err)
	}
	status, err := store.LoadRootStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusRunning || status.PID != os.Getpid() || status.Error != "" {
		t.Fatalf("running root status = %#v, want RUNNING with pid and cleared error", status)
	}

	if err := store.MarkRootFinal(StatusError, "analysis failed"); err != nil {
		t.Fatal(err)
	}
	status, err = store.LoadRootStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusError || status.PID != 0 || status.Error != "analysis failed" {
		t.Fatalf("final root status = %#v, want ERROR with message", status)
	}

	if err := store.NormalizeStaleRunning(); err != nil {
		t.Fatal(err)
	}
	runningStatus, err := store.LoadWorkStatus(running)
	if err != nil {
		t.Fatal(err)
	}
	if runningStatus.Status != StatusInterrupted ||
		runningStatus.FinishedAt == nil ||
		runningStatus.Error != "stale RUNNING status from interrupted run" {
		t.Fatalf("normalized status = %#v, want interrupted stale status", runningStatus)
	}
	finishedStatus, err := store.LoadWorkStatus(finished)
	if err != nil {
		t.Fatal(err)
	}
	if finishedStatus.Status != StatusFinished {
		t.Fatalf("finished status = %s, want unchanged FINISHED", finishedStatus.Status)
	}
}

func TestStoreStatusMethodsReturnReadErrors(t *testing.T) {
	manifest := Manifest{
		Schema:        1,
		SourceHash:    "hash",
		BenchmarkName: "bench",
		RunID:         "000000",
		GlobalNProc:   1,
		Steps:         []ManifestStep{{Name: "step", Dir: "step", NProc: 1}},
		Work:          []ManifestWork{{Step: "step", Row: 0, Dir: "000000"}},
	}
	store := NewStore(t.TempDir(), manifest, nil)

	if _, err := store.LoadRootStatus(); err == nil {
		t.Fatal("expected missing root status read error")
	}
	if err := store.MarkRootRunning(); err == nil {
		t.Fatal("expected MarkRootRunning read error")
	}
	if err := store.MarkRootFinal(StatusFinished, "done"); err == nil {
		t.Fatal("expected MarkRootFinal read error")
	}
	if err := store.NormalizeStaleRunning(); err == nil {
		t.Fatal("expected NormalizeStaleRunning read error")
	}
}

func TestWriteAnalyseHeaderHandlesEmptyAndNonEmptyHeaders(t *testing.T) {
	temp := t.TempDir()
	empty := filepath.Join(temp, "empty.csv")
	if err := writeAnalyseHeader(empty, nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(empty); err != nil {
		t.Fatal(err)
	} else if len(data) != 0 {
		t.Fatalf("empty header file = %q, want empty", string(data))
	}

	nonEmpty := filepath.Join(temp, "header.csv")
	if err := writeAnalyseHeader(nonEmpty, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(nonEmpty); err != nil {
		t.Fatal(err)
	} else if string(data) != "a,b\n" {
		t.Fatalf("header file = %q, want CSV header", string(data))
	}
}

func testRuntimePlan(sourceDir string) runtimePlan {
	parent := ManifestWork{Step: "step", Row: 0, Dir: "000000", Values: map[string]string{"x": "1"}}
	child := ManifestWork{
		Step:   "step",
		Row:    1,
		Dir:    "000001",
		Deps:   []ManifestWorkRef{{Step: "step", Row: 0, Link: "parent"}},
		Values: map[string]string{"x": "2"},
	}
	return runtimePlan{
		RootDir: "bench",
		Manifest: Manifest{
			Schema:        1,
			SourceHash:    "hash",
			BenchmarkName: "bench",
			GlobalNProc:   1,
			Steps:         []ManifestStep{{Name: "step", Dir: "step", NProc: 1, AnalyseCSV: "analyse.csv"}},
			Work:          []ManifestWork{parent, child},
		},
		Bodies:    map[string]string{"step": "echo child"},
		Analyses:  map[string]AnalysePlan{"step": {Step: "step", CSV: "analyse.csv", Header: []string{"run_id", "x"}}},
		SourceDir: sourceDir,
	}
}

func testPopulateManifest() Manifest {
	return Manifest{
		Schema:        1,
		SourceHash:    "hash",
		BenchmarkName: "bench",
		RunID:         "000000",
		GlobalNProc:   1,
		Steps:         []ManifestStep{{Name: "step", Dir: "step", NProc: 1}},
		Work:          []ManifestWork{{Step: "step", Row: 0, Dir: "000000"}},
	}
}

func testStoreWithStatuses(t *testing.T) (*Store, ManifestWork, ManifestWork) {
	t.Helper()
	runDir := t.TempDir()
	running := ManifestWork{Step: "step", Row: 0, Dir: "000000"}
	finished := ManifestWork{Step: "step", Row: 1, Dir: "000001"}
	manifest := Manifest{
		Schema:        1,
		SourceHash:    "hash",
		BenchmarkName: "bench",
		RunID:         "000000",
		GlobalNProc:   1,
		Steps:         []ManifestStep{{Name: "step", Dir: "step", NProc: 1}},
		Work:          []ManifestWork{running, finished},
	}
	store := NewStore(runDir, manifest, nil)
	writeJSONForStoreTest(t, filepath.Join(runDir, "status"), RootStatus{
		Schema:     1,
		Status:     StatusInterrupted,
		SourceHash: "hash",
		PID:        123,
		Error:      "old error",
	})
	for _, item := range []struct {
		work   ManifestWork
		status Status
	}{
		{running, StatusRunning},
		{finished, StatusFinished},
	} {
		workDir := store.WorkDir(item.work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeJSONForStoreTest(t, store.WorkStatusPath(item.work), WorkStatus{
			Schema: 1,
			Status: item.status,
			Step:   item.work.Step,
			Row:    item.work.Row,
		})
	}
	return store, running, finished
}

func writeJSONForStoreTest(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
