package run

import (
	"bytes"
	"os"
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
			{Label: "└── step1", Counts: StatusCounts{Finished: 2, Error: 1}},
			{Label: "    └── step2", Counts: StatusCounts{Blocked: 3}},
		},
		Total: StatusCounts{Finished: 2, Error: 1, Blocked: 3},
	}

	var buf bytes.Buffer
	PrintStatusSummary(&buf, summary)
	out := buf.String()
	for _, want := range []string{"| step", "BLOCKED", "└── step1", "total:", "|       3 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary output missing %q:\n%s", want, out)
		}
	}
}

func TestBuildStatusSummaryCollectsFailedWorkDirectories(t *testing.T) {
	runDir := t.TempDir()
	manifest := statusSummaryManifest()
	store := NewStore(runDir, manifest, nil)
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
	want := store.WorkDir(manifest.Work[1])
	if summary.FailedWork[0].Path != want {
		t.Fatalf("failed path = %q, want %q", summary.FailedWork[0].Path, want)
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

func treeLabelName(label string) string {
	fields := strings.Fields(label)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
