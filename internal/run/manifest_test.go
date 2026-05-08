package run

import (
	"strings"
	"testing"
)

func TestFinalizeRunManifestPrefixesAnalyseTables(t *testing.T) {
	manifest := Manifest{
		BenchmarkName:       "bench",
		AnalyseDatabasePath: "/tmp/results.sqlite",
		Steps: []ManifestStep{
			{Name: "s", Dir: "s", AnalyseTable: "s"},
			{Name: "plain", Dir: "plain"},
		},
	}
	got, err := finalizeRunManifest(manifest, "000042")
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "000042" {
		t.Fatalf("RunID = %q", got.RunID)
	}
	if got.Steps[0].AnalyseTable != "bench_000042_s" {
		t.Fatalf("AnalyseTable = %q", got.Steps[0].AnalyseTable)
	}
	if got.Steps[1].AnalyseTable != "" {
		t.Fatalf("non-analysed step got table %q", got.Steps[1].AnalyseTable)
	}
}

func TestFinalizeRunManifestKeepsCSVMode(t *testing.T) {
	manifest := Manifest{
		BenchmarkName: "bench",
		Steps:         []ManifestStep{{Name: "s", Dir: "s", AnalyseCSV: "analyse.csv"}},
	}
	got, err := finalizeRunManifest(manifest, "000000")
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "000000" {
		t.Fatalf("RunID = %q", got.RunID)
	}
	if got.Steps[0].AnalyseCSV != "analyse.csv" || got.Steps[0].AnalyseTable != "" {
		t.Fatalf("unexpected analyse fields: %#v", got.Steps[0])
	}
}

func TestValidateRunManifestRejectsMissingRunID(t *testing.T) {
	err := validateRunManifest(Manifest{BenchmarkName: "bench"})
	if err == nil || !strings.Contains(err.Error(), "missing run_id") {
		t.Fatalf("expected missing run_id error, got %v", err)
	}
}

func TestValidateRunManifestRejectsMismatchedSQLiteTable(t *testing.T) {
	err := validateRunManifest(Manifest{
		BenchmarkName:       "bench",
		RunID:               "000000",
		AnalyseDatabasePath: "/tmp/results.sqlite",
		Steps: []ManifestStep{{
			Name:         "run",
			Dir:          "run",
			AnalyseTable: "run",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "want \"bench_000000_run\"") {
		t.Fatalf("expected mismatched table error, got %v", err)
	}
}

func TestValidateRunManifestAcceptsPrefixedSQLiteTable(t *testing.T) {
	err := validateRunManifest(Manifest{
		BenchmarkName:       "bench",
		RunID:               "000000",
		AnalyseDatabasePath: "/tmp/results.sqlite",
		Steps: []ManifestStep{{
			Name:         "run",
			Dir:          "run",
			AnalyseTable: "bench_000000_run",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
