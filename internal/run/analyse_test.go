package run

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAnalyseWorkPackageOneMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.log"), []byte("Number: 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnWorkValue, Source: "x", GroupCount: 1},
		{Kind: analyseColumnPattern, Source: "number", GroupCount: 1},
	}, map[string]AnalysePatternPlan{
		"number": testPattern("number", "out.log", `Number: ([0-9]+)`),
	})
	rows, err := analyseWorkPackage(dir, ManifestWork{Dir: "000000", Values: map[string]string{"x": "a"}}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, rows, [][]string{{"000000", "a", "42"}})
}

func TestAnalyseWorkPackageMultipleMatchesAndShorterMatchLists(t *testing.T) {
	dir := t.TempDir()
	data := "Number: 1\nLetter: a\nNumber: 2\n"
	if err := os.WriteFile(filepath.Join(dir, "out.log"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnPattern, Source: "number", GroupCount: 1},
		{Kind: analyseColumnPattern, Source: "letter", GroupCount: 1},
	}, map[string]AnalysePatternPlan{
		"number": testPattern("number", "out.log", `Number: ([0-9]+)`),
		"letter": testPattern("letter", "out.log", `Letter: ([A-Za-z]+)`),
	})
	rows, err := analyseWorkPackage(dir, ManifestWork{Dir: "000000"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, rows, [][]string{
		{"000000", "1", "a"},
		{"000000", "2", ""},
	})
}

func TestAnalyseWorkPackageMultiCaptureAndNoMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.log"), []byte("Pair: AA-17\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnPattern, Source: "pair", GroupCount: 2},
	}, map[string]AnalysePatternPlan{
		"pair": testPattern("pair", "out.log", `Pair: ([A-Z]+)-([0-9]+)`),
	})
	rows, err := analyseWorkPackage(dir, ManifestWork{Dir: "000000"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, rows, [][]string{{"000000", "AA", "17"}})

	plan.Patterns["pair"] = testPattern("pair", "out.log", `Missing: ([0-9]+)`)
	rows, err = analyseWorkPackage(dir, ManifestWork{Dir: "000000"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no rows for no matches, got %#v", rows)
	}
}

func TestAnalyseWorkPackageOnlyWorkValues(t *testing.T) {
	dir := t.TempDir()
	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnWorkValue, Source: "x", GroupCount: 1},
		{Kind: analyseColumnWorkValue, Source: "missing", GroupCount: 1},
	}, nil)
	rows, err := analyseWorkPackage(dir, ManifestWork{Dir: "000000", Values: map[string]string{"x": "a"}}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, rows, [][]string{{"000000", "a", ""}})
}

func TestAnalyseFilePathValidation(t *testing.T) {
	dir := t.TempDir()
	if got, err := analyseFilePath(dir, "logs/out.txt"); err != nil || got != filepath.Join(dir, "logs", "out.txt") {
		t.Fatalf("nested path = %q, %v", got, err)
	}
	for _, rel := range []string{"", "/tmp/out.txt", ".", "..", "../out.txt"} {
		if _, err := analyseFilePath(dir, rel); err == nil {
			t.Fatalf("expected %q to be rejected", rel)
		}
	}
}

func TestCollectPatternMatchesMissingFile(t *testing.T) {
	_, err := collectPatternMatches(t.TempDir(), map[string]AnalysePatternPlan{
		"number": testPattern("number", "missing.log", `Number: ([0-9]+)`),
	})
	if err == nil || !strings.Contains(err.Error(), "missing.log") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestWriteCSVAtomicReplacesAndEscapes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analyse.csv")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := [][]string{{"run_id", "value"}, {"000000", "a,b \"c\"\nnext"}}
	if err := writeCSVAtomic(path, rows); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "old") {
		t.Fatalf("old content was not replaced: %q", got)
	}
	if !strings.Contains(got, "\"a,b \"\"c\"\"\nnext\"") {
		t.Fatalf("csv escaping missing from %q", got)
	}
}

func testAnalysePlan(columns []AnalyseColumnPlan, patterns map[string]AnalysePatternPlan) AnalysePlan {
	if patterns == nil {
		patterns = map[string]AnalysePatternPlan{}
	}
	return AnalysePlan{
		Step:     "run",
		CSV:      "analyse.csv",
		Header:   []string{"run_id"},
		Columns:  columns,
		Patterns: patterns,
	}
}

func testPattern(name, file, expr string) AnalysePatternPlan {
	re := regexp.MustCompile(expr)
	return AnalysePatternPlan{
		Name:         name,
		File:         file,
		Regex:        expr,
		GroupCount:   re.NumSubexp(),
		CompiledExpr: re,
	}
}

func assertRows(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}
	for i := range got {
		if strings.Join(got[i], "\x00") != strings.Join(want[i], "\x00") {
			t.Fatalf("row %d = %#v, want %#v (all rows %#v)", i, got[i], want[i], got)
		}
	}
}
