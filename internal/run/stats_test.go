package run

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildStatusSummaryCountsAndTree(t *testing.T) {
	runDir := t.TempDir()
	manifest := statusSummaryManifest()
	store := NewStore(runDir, manifest, nil)
	writeStatusSummaryStatuses(t, store, map[string]Status{
		"step1/000000": StatusFinished,
		"step1/000001": StatusError,
		"step2/000000": StatusBlocked,
		"step2/000001": StatusNotStarted,
		"step3/000000": StatusInterrupted,
		"step4/000000": StatusRunning,
	})

	summary, err := BuildStatusSummary(store)
	if err != nil {
		t.Fatalf("BuildStatusSummary: %v", err)
	}
	if summary.Total != (StatusCounts{Finished: 1, Error: 1, Blocked: 1, NotStarted: 1, Running: 1, Interrupted: 1}) {
		t.Fatalf("total = %#v", summary.Total)
	}
	gotNames := make([]string, 0, len(summary.Rows))
	for _, row := range summary.Rows {
		gotNames = append(gotNames, treeLabelName(row.Label))
	}
	wantNames := []string{"step1", "step2", "step3", "step4"}
	if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("row names = %v, want %v", gotNames, wantNames)
	}
	if summary.Rows[2].Counts.Interrupted != 1 {
		t.Fatalf("step3 counts = %#v, want interrupted", summary.Rows[2].Counts)
	}
}

func TestPrintStatusSummaryRendersTable(t *testing.T) {
	summary := RunStatusSummary{
		Rows: []StatusSummaryRow{
			{Label: "└── step1", Counts: StatusCounts{Finished: 2, Error: 1}, DurationSeconds: 1.25},
			{Label: "    └── step2", Counts: StatusCounts{Blocked: 3}, DurationSeconds: 3},
		},
		Total:                StatusCounts{Finished: 2, Error: 1, Blocked: 3},
		TotalDurationSeconds: 4.25,
	}

	var buf bytes.Buffer
	PrintStatusSummary(&buf, summary)
	out := buf.String()
	for _, want := range []string{"| step", "BLOCKED", "duration_s", "└── step1", "total:", "|       3 |", "1.25", "4.25"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary output missing %q:\n%s", want, out)
		}
	}
}

func TestBuildStatusSummarySumsDurations(t *testing.T) {
	runDir := t.TempDir()
	manifest := statusSummaryManifest()
	store := NewStore(runDir, manifest, nil)
	writeStatusSummaryWorkStatuses(t, store, map[string]WorkStatus{
		"step1/000000": {Schema: 1, Status: StatusFinished, Step: "step1", Row: 0, Duration: durationPtr(1.25)},
		"step1/000001": {Schema: 1, Status: StatusError, Step: "step1", Row: 1, Duration: durationPtr(2)},
		"step2/000000": {Schema: 1, Status: StatusBlocked, Step: "step2", Row: 0, Duration: durationPtr(0.5)},
		"step2/000001": {Schema: 1, Status: StatusNotStarted, Step: "step2", Row: 1},
		"step3/000000": {Schema: 1, Status: StatusInterrupted, Step: "step3", Row: 0, Duration: durationPtr(0.25)},
		"step4/000000": {Schema: 1, Status: StatusRunning, Step: "step4", Row: 0},
	})

	summary, err := BuildStatusSummary(store)
	if err != nil {
		t.Fatalf("BuildStatusSummary: %v", err)
	}
	if summary.TotalDurationSeconds != 4 {
		t.Fatalf("total duration = %v, want 4", summary.TotalDurationSeconds)
	}
	got := make(map[string]float64, len(summary.Rows))
	for _, row := range summary.Rows {
		got[treeLabelName(row.Label)] = row.DurationSeconds
	}
	want := map[string]float64{"step1": 3.25, "step2": 0.5, "step3": 0.25, "step4": 0}
	for step, wantDuration := range want {
		if got[step] != wantDuration {
			t.Fatalf("%s duration = %v, want %v; all durations=%#v", step, got[step], wantDuration, got)
		}
	}
}

func TestBuildStatusSummaryTreatsMissingDurationAsZero(t *testing.T) {
	runDir := t.TempDir()
	manifest := statusSummaryManifest()
	store := NewStore(runDir, manifest, nil)
	writeStatusSummaryStatuses(t, store, map[string]Status{
		"step1/000000": StatusNotStarted,
		"step1/000001": StatusRunning,
		"step2/000000": StatusFinished,
		"step2/000001": StatusError,
		"step3/000000": StatusInterrupted,
		"step4/000000": StatusBlocked,
	})

	summary, err := BuildStatusSummary(store)
	if err != nil {
		t.Fatalf("BuildStatusSummary: %v", err)
	}
	if summary.TotalDurationSeconds != 0 {
		t.Fatalf("total duration = %v, want 0", summary.TotalDurationSeconds)
	}
	for _, row := range summary.Rows {
		if row.DurationSeconds != 0 {
			t.Fatalf("%s duration = %v, want 0", row.Label, row.DurationSeconds)
		}
	}
}

func TestBuildJobTreeSummaryCountsWorkpackages(t *testing.T) {
	summary := BuildJobTreeSummary(statusSummaryManifest())
	if summary.Total != 6 {
		t.Fatalf("total = %d, want 6", summary.Total)
	}
	got := make(map[string]int, len(summary.Rows))
	for _, row := range summary.Rows {
		got[treeLabelName(row.Label)] = row.Count
	}
	if got["step1"] != 2 {
		t.Fatalf("step1 count = %d, want 2", got["step1"])
	}
	if got["step2"] != 2 {
		t.Fatalf("step2 count = %d, want 2", got["step2"])
	}
	if got["step3"] != 1 || got["step4"] != 1 {
		t.Fatalf("leaf counts = %#v, want step3=1 and step4=1", got)
	}
}

func TestPrintJobTreeSummaryRendersTable(t *testing.T) {
	summary := JobTreeSummary{
		Rows: []JobTreeRow{
			{Label: "└── step1", Count: 2},
			{Label: "    └── step2", Count: 3},
		},
		Total: 5,
	}

	var buf bytes.Buffer
	PrintJobTreeSummary(&buf, summary)
	out := buf.String()
	for _, want := range []string{"| step", "| #", "└── step1", "total:", "| 5 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("job tree output missing %q:\n%s", want, out)
		}
	}
}

func TestBuildStatusSummaryCollectsAbsoluteFailedWorkDirectories(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	manifest := statusSummaryManifest()
	store := NewStore(filepath.Join("bench", "000000"), manifest, nil)
	writeStatusSummaryStatuses(t, store, map[string]Status{
		"step1/000000": StatusFinished,
		"step1/000001": StatusError,
		"step2/000000": StatusBlocked,
		"step2/000001": StatusNotStarted,
		"step3/000000": StatusFinished,
		"step4/000000": StatusFinished,
	})

	summary, err := BuildStatusSummary(store)
	if err != nil {
		t.Fatalf("BuildStatusSummary: %v", err)
	}
	if len(summary.FailedWork) != 1 {
		t.Fatalf("failed paths = %#v, want one ERROR workpackage", summary.FailedWork)
	}
	want := filepath.Join(cwd, "bench", "000000", "step1", "000001")
	if got := summary.FailedWork[0].Path; got != want || !filepath.IsAbs(got) {
		t.Fatalf("failed path = %q, want absolute %q", got, want)
	}
	if summary.FailedWork[0].Step != "step1" || summary.FailedWork[0].Row != 1 {
		t.Fatalf("failed work metadata = %#v", summary.FailedWork[0])
	}
}

func TestPrintFailedWorkDirectories(t *testing.T) {
	var empty bytes.Buffer
	PrintFailedWorkDirectories(&empty, nil)
	if empty.Len() != 0 {
		t.Fatalf("empty failed list printed %q", empty.String())
	}

	var buf bytes.Buffer
	PrintFailedWorkDirectories(&buf, []FailedWorkDirectory{
		{Step: "s", Row: 1, Path: "bench/000000/s/000001"},
	})
	out := buf.String()
	for _, want := range []string{
		"failed workpackage directories:",
		"bench/000000/s/000001",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBuildStatusSummaryDoesNotDuplicateMultiParentStep(t *testing.T) {
	runDir := t.TempDir()
	manifest := Manifest{
		Steps: []ManifestStep{
			{Name: "a", Dir: "a"},
			{Name: "b", Dir: "b"},
			{Name: "c", Dir: "c"},
		},
		Work: []ManifestWork{
			{Step: "a", Row: 0, Dir: "000000"},
			{Step: "b", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "a", Row: 0}}},
			{Step: "c", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "a", Row: 0}, {Step: "b", Row: 0}}},
		},
	}
	store := NewStore(runDir, manifest, nil)
	writeStatusSummaryStatuses(t, store, map[string]Status{
		"a/000000": StatusFinished,
		"b/000000": StatusFinished,
		"c/000000": StatusFinished,
	})

	summary, err := BuildStatusSummary(store)
	if err != nil {
		t.Fatalf("BuildStatusSummary: %v", err)
	}
	if len(summary.Rows) != 3 {
		t.Fatalf("rows = %#v, want one row per step", summary.Rows)
	}
	if summary.Total.Finished != 3 {
		t.Fatalf("total = %#v, want three finished rows", summary.Total)
	}
}

func TestBuildStatusSummaryReportsStatusReadError(t *testing.T) {
	runDir := t.TempDir()
	manifest := statusSummaryManifest()
	store := NewStore(runDir, manifest, nil)
	writeStatusSummaryStatuses(t, store, map[string]Status{
		"step1/000000": StatusFinished,
	})

	_, err := BuildStatusSummary(store)
	if err == nil || !strings.Contains(err.Error(), "read step1/000001 status") {
		t.Fatalf("error = %v, want contextual read error", err)
	}
}

func TestOpenLatestStoresForBenchmarkDirSingleRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	writeInspectionRun(t, root, "000000", Manifest{
		Schema:        1,
		RunID:         "000000",
		BenchmarkName: "bench",
	})

	prepared, err := openLatestStoresForBenchmarkDir(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 1 {
		t.Fatalf("prepared stores = %d, want 1", len(prepared))
	}
	if prepared[0].Plan.ComponentName != "bench" {
		t.Fatalf("component label = %q, want bench", prepared[0].Plan.ComponentName)
	}
	if prepared[0].Store.RunDir != filepath.Join(root, "000000") {
		t.Fatalf("run dir = %q, want latest run", prepared[0].Store.RunDir)
	}
}

func TestOpenLatestStoresForBenchmarkDirComponentRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench", "small")
	writeInspectionRun(t, root, "000000", Manifest{
		Schema:             1,
		RunID:              "000000",
		BenchmarkName:      "bench",
		BenchmarkComponent: "small",
	})

	prepared, err := openLatestStoresForBenchmarkDir(root, "small")
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 1 || prepared[0].Plan.ComponentName != "small" {
		t.Fatalf("prepared = %#v, want selected small component", prepared)
	}
}

func TestOpenLatestStoresForBenchmarkDirSelectedComponent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	writeInspectionRun(t, filepath.Join(root, "small"), "000000", Manifest{
		Schema:             1,
		RunID:              "000000",
		BenchmarkName:      "bench",
		BenchmarkComponent: "small",
	})
	writeInspectionRun(t, filepath.Join(root, "large"), "000000", Manifest{
		Schema:             1,
		RunID:              "000000",
		BenchmarkName:      "bench",
		BenchmarkComponent: "large",
	})

	prepared, err := openLatestStoresForBenchmarkDir(root, "small")
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 1 || prepared[0].Plan.ComponentName != "small" {
		t.Fatalf("prepared = %#v, want only small component", prepared)
	}
}

func TestOpenLatestStoresForBenchmarkDirRejectsUnknownSelectedComponent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench")
	writeInspectionRun(t, filepath.Join(root, "small"), "000000", Manifest{
		Schema:             1,
		RunID:              "000000",
		BenchmarkName:      "bench",
		BenchmarkComponent: "small",
	})

	_, err := openLatestStoresForBenchmarkDir(root, "large")
	if err == nil || !strings.Contains(err.Error(), `unknown benchmark "large"`) {
		t.Fatalf("error = %v, want unknown selected component", err)
	}
}

func statusSummaryManifest() Manifest {
	return Manifest{
		Steps: []ManifestStep{
			{Name: "step1", Dir: "step1"},
			{Name: "step2", Dir: "step2"},
			{Name: "step3", Dir: "step3"},
			{Name: "step4", Dir: "step4"},
		},
		Work: []ManifestWork{
			{Step: "step1", Row: 0, Dir: "000000"},
			{Step: "step1", Row: 1, Dir: "000001"},
			{Step: "step2", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "step1", Row: 0}}},
			{Step: "step2", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "step1", Row: 1}}},
			{Step: "step3", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "step2", Row: 0}}},
			{Step: "step4", Row: 0, Dir: "000000", Deps: []ManifestWorkRef{{Step: "step1", Row: 0}}},
		},
	}
}

func writeInspectionRun(t *testing.T, root, runID string, manifest Manifest) {
	t.Helper()
	runDir := filepath.Join(root, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeStatusSummaryStatuses(t *testing.T, store *Store, statuses map[string]Status) {
	t.Helper()
	for _, work := range store.Manifest.Work {
		status, ok := statuses[workKey(work.Step, work.Row)]
		if !ok {
			continue
		}
		workDir := store.WorkDir(work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: status, Step: work.Step, Row: work.Row}); err != nil {
			t.Fatal(err)
		}
	}
}

func writeStatusSummaryWorkStatuses(t *testing.T, store *Store, statuses map[string]WorkStatus) {
	t.Helper()
	for _, work := range store.Manifest.Work {
		status, ok := statuses[workKey(work.Step, work.Row)]
		if !ok {
			continue
		}
		workDir := store.WorkDir(work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteWorkStatus(work, status); err != nil {
			t.Fatal(err)
		}
	}
}

func treeLabelName(label string) string {
	fields := strings.Fields(label)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
