package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSummarizeAnalyseCSV(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		content  string
		wantRows int
		wantCols int
	}{
		{name: "empty", content: "", wantRows: 0, wantCols: 0},
		{name: "header only", content: "run_id,x\n", wantRows: 0, wantCols: 2},
		{name: "data", content: "run_id,x\n000000,1\n000001,2\n", wantRows: 2, wantCols: 2},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".csv")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			rows, cols, err := summarizeAnalyseCSV(path)
			if err != nil {
				t.Fatalf("summarizeAnalyseCSV: %v", err)
			}
			if rows != tc.wantRows || cols != tc.wantCols {
				t.Fatalf("summary = (%d,%d), want (%d,%d)", rows, cols, tc.wantRows, tc.wantCols)
			}
		})
	}
}

func TestBuildAnalyseOutputSummariesCSV(t *testing.T) {
	runDir := t.TempDir()
	manifest := Manifest{
		Steps: []ManifestStep{{Name: "run", Dir: "run", AnalyseCSV: "analyse.csv"}},
	}
	store := NewStore(runDir, manifest, nil)
	if err := os.MkdirAll(filepath.Join(runDir, "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "run", "analyse.csv")
	if err := os.WriteFile(path, []byte("run_id,x\n000000,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summaries, err := BuildAnalyseOutputSummaries(store)
	if err != nil {
		t.Fatalf("BuildAnalyseOutputSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %#v, want one", summaries)
	}
	if summaries[0] != (AnalyseOutputSummary{Path: path, Rows: 1, Cols: 2}) {
		t.Fatalf("summary = %#v", summaries[0])
	}
}

func TestBuildAnalyseOutputSummariesSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "results.sqlite")
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
		analyseRowsFromStrings([][]string{{"000000", "1"}, {"000001", "2"}}),
	); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.Close()

	store := NewStore(t.TempDir(), Manifest{
		AnalyseDatabase:     "results.sqlite",
		AnalyseDatabasePath: dbPath,
		Steps:               []ManifestStep{{Name: "run", Dir: "run", AnalyseTable: "bench_000000_run"}},
	}, nil)
	summaries, err := BuildAnalyseOutputSummaries(store)
	if err != nil {
		t.Fatalf("BuildAnalyseOutputSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %#v, want one", summaries)
	}
	if summaries[0] != (AnalyseOutputSummary{Path: "results.sqlite:bench_000000_run", Rows: 2, Cols: 2}) {
		t.Fatalf("summary = %#v", summaries[0])
	}
}
