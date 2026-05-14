package run

import (
	"encoding/csv"
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
	plan.Header = []string{"run_id", "x", "number"}
	rows, err := analyseWorkPackage(dir, ManifestWork{Dir: "000000", Values: map[string]string{"x": "a"}}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertCellRows(t, rows, [][]string{{"000000", "a", "42"}})
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
	assertCellRows(t, rows, [][]string{
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
	assertCellRows(t, rows, [][]string{{"000000", "AA", "17"}})

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
	assertCellRows(t, rows, [][]string{{"000000", "a", ""}})
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

func TestCollectPatternMatchesRejectsUnsafePath(t *testing.T) {
	_, err := collectPatternMatches(t.TempDir(), map[string]AnalysePatternPlan{
		"number": testPattern("number", "../out.log", `Number: ([0-9]+)`),
	})
	if err == nil || !strings.Contains(err.Error(), "escapes the workpackage directory") {
		t.Fatalf("expected unsafe path error, got %v", err)
	}
}

func TestValuesForColumnUnknownKind(t *testing.T) {
	got, err := valuesForColumn(ManifestWork{}, nil, AnalyseColumnPlan{Kind: AnalyseColumnKind("other")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CSVString() != "" {
		t.Fatalf("unknown column values = %#v", got)
	}
}

func TestAnalyseWorkPackageTypedPatternCaptures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "out.log"), []byte("id=7 ratio=1.25 word=abc manual=42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := testAnalysePlan([]AnalyseColumnPlan{
		{
			Kind:       analyseColumnPattern,
			Source:     "mixed",
			GroupCount: 4,
			GroupTypes: []AnalyseValueKind{analyseValueInt, analyseValueFloat, analyseValueString, analyseValueString},
		},
	}, map[string]AnalysePatternPlan{
		"mixed": testPatternWithTypes(
			"mixed",
			"out.log",
			`id=([0-9]+) ratio=([0-9.]+) word=([A-Za-z]+) manual=([0-9]+)`,
			[]AnalyseValueKind{analyseValueInt, analyseValueFloat, analyseValueString, analyseValueString},
		),
	})
	rows, err := analyseWorkPackage(dir, ManifestWork{Dir: "000000"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertCellRows(t, rows, [][]string{{"000000", "7", "1.25", "abc", "42"}})
	if got, ok := rows[0][1].SQLiteValue().(int64); !ok || got != 7 {
		t.Fatalf("int sqlite value = %#v", rows[0][1].SQLiteValue())
	}
	if got, ok := rows[0][2].SQLiteValue().(float64); !ok || got != 1.25 {
		t.Fatalf("float sqlite value = %#v", rows[0][2].SQLiteValue())
	}
	if got, ok := rows[0][4].SQLiteValue().(string); !ok || got != "42" {
		t.Fatalf("manual capture sqlite value = %#v", rows[0][4].SQLiteValue())
	}
}

func TestAnalyseCellFromWorkValue(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		kind       AnalyseValueKind
		wantCSV    string
		wantSQLite any
		wantErr    string
	}{
		{name: "int", text: "7", kind: analyseValueInt, wantCSV: "7", wantSQLite: int64(7)},
		{name: "float", text: "1.25", kind: analyseValueFloat, wantCSV: "1.25", wantSQLite: 1.25},
		{name: "bool true", text: "true", kind: analyseValueBool, wantCSV: "true", wantSQLite: int64(1)},
		{name: "bool false", text: "false", kind: analyseValueBool, wantCSV: "false", wantSQLite: int64(0)},
		{name: "string", text: "x", kind: analyseValueString, wantCSV: "x", wantSQLite: "x"},
		{name: "bad int", text: "x", kind: analyseValueInt, wantErr: "parse integer value"},
		{name: "bad bool", text: "x", kind: analyseValueBool, wantErr: "parse boolean value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := analyseCellFromWorkValue(tt.text, tt.kind)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.CSVString() != tt.wantCSV {
				t.Fatalf("CSVString() = %q, want %q", got.CSVString(), tt.wantCSV)
			}
			if got.SQLiteValue() != tt.wantSQLite {
				t.Fatalf("SQLiteValue() = %#v, want %#v", got.SQLiteValue(), tt.wantSQLite)
			}
		})
	}
}

func TestSubmatchGroupsReportsTypedConversionError(t *testing.T) {
	_, err := submatchGroups([][]string{{"match", "9223372036854775808"}}, []AnalyseValueKind{analyseValueInt})
	if err == nil || !strings.Contains(err.Error(), "parse integer capture") {
		t.Fatalf("expected integer parse error, got %v", err)
	}
}

func TestValuesForColumnMissingTypedPatternCells(t *testing.T) {
	got, err := valuesForColumn(
		ManifestWork{},
		patternMatches{},
		AnalyseColumnPlan{Kind: analyseColumnPattern, Source: "missing", GroupCount: 2, GroupTypes: []AnalyseValueKind{analyseValueInt, analyseValueFloat}},
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Valid || got[1].Valid || got[0].CSVString() != "" || got[1].SQLiteValue() != nil {
		t.Fatalf("missing typed cells = %#v", got)
	}
}

func TestRunAnalysesCSVWritesTable(t *testing.T) {
	store, plan := testCSVAnalyseStore(t, StatusFinished)
	if err := RunAnalyses(store, map[string]AnalysePlan{"run": plan}); err != nil {
		t.Fatal(err)
	}

	rows := readCSVRows(t, filepath.Join(store.RunDir, "run", "analyse.csv"))
	assertRows(t, rows, [][]string{
		{"run_id", "x", "number"},
		{"000000", "a", "7"},
		{"000001", "b", "8"},
	})
}

func TestRunAnalysesCSVSkipsStepsWithoutAnalyseCSV(t *testing.T) {
	runDir := t.TempDir()
	manifest := Manifest{
		Steps: []ManifestStep{{Name: "prep", Dir: "prep"}},
		Work: []ManifestWork{{
			Step: "prep",
			Row:  0,
			Dir:  "000000",
		}},
	}
	store := NewStore(runDir, manifest, nil)
	if err := RunAnalyses(store, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunAnalysesCSVReportsMissingPlan(t *testing.T) {
	store, _ := testCSVAnalyseStore(t, StatusFinished)
	err := RunAnalyses(store, nil)
	if err == nil || !strings.Contains(err.Error(), `missing analyse plan for step "run"`) {
		t.Fatalf("expected missing plan error, got %v", err)
	}
}

func TestRunAnalysesCSVPropagatesWorkPackageAnalyseError(t *testing.T) {
	store, plan := testCSVAnalyseStore(t, StatusFinished)
	work := store.Manifest.Work[0]
	if err := os.Remove(filepath.Join(store.WorkDir(work), "out.log")); err != nil {
		t.Fatal(err)
	}

	err := RunAnalyses(store, map[string]AnalysePlan{"run": plan})
	if err == nil || !strings.Contains(err.Error(), "analyse run/000000") || !strings.Contains(err.Error(), "read analyse file") {
		t.Fatalf("expected analyse file error, got %v", err)
	}
}

func TestCollectStepAnalyseRowsReportsStatusReadError(t *testing.T) {
	store, plan := testCSVAnalyseStore(t, StatusFinished)
	work := store.Manifest.Work[0]
	if err := os.Remove(store.WorkStatusPath(work)); err != nil {
		t.Fatal(err)
	}

	_, err := collectStepAnalyseRows(store, store.Manifest.Steps[0], plan)
	if err == nil || !strings.Contains(err.Error(), "analyse run/000000") {
		t.Fatalf("expected status read error, got %v", err)
	}
}

func TestCollectStepAnalyseRowsRejectsUnfinishedWork(t *testing.T) {
	store, plan := testCSVAnalyseStore(t, StatusInterrupted)
	_, err := collectStepAnalyseRows(store, store.Manifest.Steps[0], plan)
	if err == nil || !strings.Contains(err.Error(), "status is INTERRUPTED") {
		t.Fatalf("expected unfinished status error, got %v", err)
	}
}

func TestRunAnalysesSQLiteWritesAndReplacesRows(t *testing.T) {
	runDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "results.sqlite")
	manifest := Manifest{
		BenchmarkName:       "bench",
		RunID:               "000000",
		AnalyseDatabase:     "results.sqlite",
		AnalyseDatabasePath: dbPath,
		Steps: []ManifestStep{{
			Name:         "run",
			Dir:          "run",
			AnalyseTable: "bench_000000_run",
		}},
		Work: []ManifestWork{{
			Step:   "run",
			Row:    0,
			Dir:    "000000",
			Values: map[string]string{"x": "a"},
		}},
	}
	store := NewStore(runDir, manifest, nil)
	work := manifest.Work[0]
	workDir := store.WorkDir(work)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "out.log"), []byte("Number: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := WorkStatus{Schema: 1, Status: StatusFinished, Step: work.Step, Row: work.Row}
	if err := store.WriteWorkStatus(work, status); err != nil {
		t.Fatal(err)
	}

	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnWorkValue, Source: "x", GroupCount: 1},
		{Kind: analyseColumnPattern, Source: "number", GroupCount: 1},
	}, map[string]AnalysePatternPlan{
		"number": testPattern("number", "out.log", `Number: ([0-9]+)`),
	})
	plan.Header = []string{"run_id", "x", "number"}
	if err := RunAnalyses(store, map[string]AnalysePlan{"run": plan}); err != nil {
		t.Fatal(err)
	}
	assertAnalyseTable(t, dbPath, "bench_000000_run", []string{"run_id", "x", "number"}, [][]string{{"000000", "a", "7"}})

	if err := os.WriteFile(filepath.Join(workDir, "out.log"), []byte("Number: 8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunAnalyses(store, map[string]AnalysePlan{"run": plan}); err != nil {
		t.Fatal(err)
	}
	assertAnalyseTable(t, dbPath, "bench_000000_run", []string{"run_id", "x", "number"}, [][]string{{"000000", "a", "8"}})
}

func TestRunAnalysesSQLiteWritesTypedPatternColumns(t *testing.T) {
	runDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "typed.sqlite")
	manifest := Manifest{
		AnalyseDatabasePath: dbPath,
		Steps: []ManifestStep{{
			Name:         "run",
			Dir:          "run",
			AnalyseTable: "bench_000000_run",
		}},
		Work: []ManifestWork{{
			Step:   "run",
			Row:    0,
			Dir:    "000000",
			Values: map[string]string{"x": "case"},
		}},
	}
	store := NewStore(runDir, manifest, nil)
	work := manifest.Work[0]
	workDir := store.WorkDir(work)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "out.log"), []byte("Number: 7\nRatio: 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: StatusFinished, Step: work.Step, Row: work.Row}); err != nil {
		t.Fatal(err)
	}

	plan := AnalysePlan{
		Step:        "run",
		Header:      []string{"run_id", "x", "number", "ratio"},
		ColumnTypes: []AnalyseValueKind{analyseValueString, analyseValueString, analyseValueInt, analyseValueFloat},
		Columns: []AnalyseColumnPlan{
			{Kind: analyseColumnWorkValue, Source: "x", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueString}},
			{Kind: analyseColumnPattern, Source: "number", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueInt}},
			{Kind: analyseColumnPattern, Source: "ratio", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueFloat}},
		},
		Patterns: map[string]AnalysePatternPlan{
			"number": testPatternWithTypes("number", "out.log", `Number: ([0-9]+)`, []AnalyseValueKind{analyseValueInt}),
			"ratio":  testPatternWithTypes("ratio", "out.log", `Ratio: ([0-9.]+)`, []AnalyseValueKind{analyseValueFloat}),
		},
	}
	if err := RunAnalyses(store, map[string]AnalysePlan{"run": plan}); err != nil {
		t.Fatal(err)
	}
	assertAnalyseTable(t, dbPath, "bench_000000_run", plan.Header, [][]string{{"000000", "case", "7", "1.25"}})
	assertRows(t, readSQLiteColumnTypes(t, dbPath, "bench_000000_run"), [][]string{
		{"run_id", "TEXT"},
		{"x", "TEXT"},
		{"number", "INTEGER"},
		{"ratio", "REAL"},
	})
	db, err := openAnalyseDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var numberType, ratioType string
	if err := db.QueryRow(`SELECT typeof("number"), typeof("ratio") FROM "bench_000000_run"`).Scan(&numberType, &ratioType); err != nil {
		t.Fatal(err)
	}
	if numberType != "integer" || ratioType != "real" {
		t.Fatalf("sqlite value types = %s/%s, want integer/real", numberType, ratioType)
	}
}

func TestRunAnalysesSQLiteWritesTypedWorkValueColumns(t *testing.T) {
	runDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "typed-work.sqlite")
	manifest := Manifest{
		AnalyseDatabasePath: dbPath,
		Steps: []ManifestStep{{
			Name:         "run",
			Dir:          "run",
			AnalyseTable: "bench_000000_run",
		}},
		Work: []ManifestWork{{
			Step: "run",
			Row:  0,
			Dir:  "000000",
			Values: map[string]string{
				"i": "7",
				"f": "1.25",
				"b": "true",
				"s": "label",
			},
		}},
	}
	store := NewStore(runDir, manifest, nil)
	work := manifest.Work[0]
	workDir := store.WorkDir(work)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: StatusFinished, Step: work.Step, Row: work.Row}); err != nil {
		t.Fatal(err)
	}

	plan := AnalysePlan{
		Step:        "run",
		Header:      []string{"run_id", "i", "f", "b", "s"},
		ColumnTypes: []AnalyseValueKind{analyseValueString, analyseValueInt, analyseValueFloat, analyseValueBool, analyseValueString},
		Columns: []AnalyseColumnPlan{
			{Kind: analyseColumnWorkValue, Source: "i", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueInt}},
			{Kind: analyseColumnWorkValue, Source: "f", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueFloat}},
			{Kind: analyseColumnWorkValue, Source: "b", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueBool}},
			{Kind: analyseColumnWorkValue, Source: "s", GroupCount: 1, GroupTypes: []AnalyseValueKind{analyseValueString}},
		},
		Patterns: map[string]AnalysePatternPlan{},
	}
	if err := RunAnalyses(store, map[string]AnalysePlan{"run": plan}); err != nil {
		t.Fatal(err)
	}
	assertAnalyseTable(t, dbPath, "bench_000000_run", plan.Header, [][]string{{"000000", "7", "1.25", "1", "label"}})
	assertRows(t, readSQLiteColumnTypes(t, dbPath, "bench_000000_run"), [][]string{
		{"run_id", "TEXT"},
		{"i", "INTEGER"},
		{"f", "REAL"},
		{"b", "INTEGER"},
		{"s", "TEXT"},
	})
	db, err := openAnalyseDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var iType, fType, bType, sType string
	if err := db.QueryRow(`SELECT typeof("i"), typeof("f"), typeof("b"), typeof("s") FROM "bench_000000_run"`).Scan(&iType, &fType, &bType, &sType); err != nil {
		t.Fatal(err)
	}
	if iType != "integer" || fType != "real" || bType != "integer" || sType != "text" {
		t.Fatalf("sqlite value types = %s/%s/%s/%s, want integer/real/integer/text", iType, fType, bType, sType)
	}
}

func TestRunAnalysesSQLiteQuotesIdentifiers(t *testing.T) {
	runDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "quoted.sqlite")
	manifest := Manifest{
		BenchmarkName:       "bench",
		RunID:               "000000",
		AnalyseDatabase:     "quoted.sqlite",
		AnalyseDatabasePath: dbPath,
		Steps: []ManifestStep{{
			Name:         "select",
			Dir:          "select",
			AnalyseTable: "bench_000000_select",
		}},
		Work: []ManifestWork{{
			Step:   "select",
			Row:    0,
			Dir:    "000000",
			Values: map[string]string{"name": "case"},
		}},
	}
	store := NewStore(runDir, manifest, nil)
	work := manifest.Work[0]
	workDir := store.WorkDir(work)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	status := WorkStatus{Schema: 1, Status: StatusFinished, Step: work.Step, Row: work.Row}
	if err := store.WriteWorkStatus(work, status); err != nil {
		t.Fatal(err)
	}

	plan := AnalysePlan{
		Step:   "select",
		Header: []string{"run_id", "name of a column", "Pair.0"},
		Columns: []AnalyseColumnPlan{
			{Kind: analyseColumnWorkValue, Source: "name", GroupCount: 1},
			{Kind: analyseColumnWorkValue, Source: "missing", GroupCount: 1},
		},
		Patterns: map[string]AnalysePatternPlan{},
	}
	if err := RunAnalyses(store, map[string]AnalysePlan{"select": plan}); err != nil {
		t.Fatal(err)
	}
	assertAnalyseTable(t, dbPath, "bench_000000_select", plan.Header, [][]string{{"000000", "case", ""}})
}

func TestRunAnalysesSQLiteRejectsEmptyDatabasePath(t *testing.T) {
	err := runAnalysesSQLite(NewStore(t.TempDir(), Manifest{}, nil), nil)
	if err == nil || !strings.Contains(err.Error(), "analyse database path is empty") {
		t.Fatalf("expected empty database path error, got %v", err)
	}
}

func TestRunAnalysesSQLiteReportsParentCreationError(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir(), Manifest{
		AnalyseDatabasePath: filepath.Join(parent, "results.sqlite"),
	}, nil)
	err := runAnalysesSQLite(store, nil)
	if err == nil {
		t.Fatal("expected parent creation error")
	}
}

func TestRunAnalysesSQLiteSkipsStepsWithoutAnalyseTable(t *testing.T) {
	store := NewStore(t.TempDir(), Manifest{
		AnalyseDatabasePath: filepath.Join(t.TempDir(), "results.sqlite"),
		Steps:               []ManifestStep{{Name: "prep", Dir: "prep"}},
	}, nil)
	if err := runAnalysesSQLite(store, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunAnalysesSQLiteReportsMissingPlan(t *testing.T) {
	store, _ := testSQLiteAnalyseStore(t, StatusFinished)
	err := runAnalysesSQLite(store, nil)
	if err == nil || !strings.Contains(err.Error(), `missing analyse plan for step "run"`) {
		t.Fatalf("expected missing plan error, got %v", err)
	}
}

func TestRunAnalysesSQLitePropagatesCollectError(t *testing.T) {
	store, plan := testSQLiteAnalyseStore(t, StatusInterrupted)
	err := runAnalysesSQLite(store, map[string]AnalysePlan{"run": plan})
	if err == nil || !strings.Contains(err.Error(), "status is INTERRUPTED") {
		t.Fatalf("expected collect error, got %v", err)
	}
}

func TestRunAnalysesSQLiteReportsTableWriteError(t *testing.T) {
	store, plan := testSQLiteAnalyseStore(t, StatusFinished)
	plan.Header = nil
	err := runAnalysesSQLite(store, map[string]AnalysePlan{"run": plan})
	if err == nil || !strings.Contains(err.Error(), `write analyse table "run"`) {
		t.Fatalf("expected table write error, got %v", err)
	}
}

func TestOpenAnalyseDBReportsPragmaError(t *testing.T) {
	db, err := openAnalyseDB(t.TempDir())
	if db != nil {
		db.Close()
	}
	if err == nil {
		t.Fatal("expected database open pragma error")
	}
}

func TestEnsureAnalyseDatabaseParentAllowsEmptyPath(t *testing.T) {
	if err := ensureAnalyseDatabaseParent(""); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceAnalyseTableCreatesEmptyTable(t *testing.T) {
	db, err := openAnalyseDB(filepath.Join(t.TempDir(), "results.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceAnalyseTable(tx, "empty", []string{"run_id"}, []AnalyseValueKind{analyseValueString}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	header, rows, err := readAnalyseTable(db, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(header, "\x00") != "run_id" || len(rows) != 0 {
		t.Fatalf("empty table = header %#v rows %#v", header, rows)
	}
}

func TestReplaceAnalyseTableReportsClosedTransaction(t *testing.T) {
	db, err := openAnalyseDB(filepath.Join(t.TempDir(), "results.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := replaceAnalyseTable(tx, "closed", []string{"run_id"}, []AnalyseValueKind{analyseValueString}, nil); err == nil {
		t.Fatal("expected closed transaction error")
	}
}

func TestReplaceAnalyseTableReportsCreateError(t *testing.T) {
	db, err := openAnalyseDB(filepath.Join(t.TempDir(), "results.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := replaceAnalyseTable(tx, "bad", nil, nil, nil); err == nil {
		t.Fatal("expected create table error")
	}
}

func TestReadAnalyseTableReportsMissingTable(t *testing.T) {
	db, err := openAnalyseDB(filepath.Join(t.TempDir(), "results.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _, err = readAnalyseTable(db, "missing")
	if err == nil || !strings.Contains(err.Error(), `analyse table "missing" does not exist`) {
		t.Fatalf("expected missing table error, got %v", err)
	}
}

func TestSQLiteTableColumnsReportsClosedDatabase(t *testing.T) {
	db, err := openAnalyseDB(filepath.Join(t.TempDir(), "results.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sqliteTableColumns(db, "missing"); err == nil {
		t.Fatal("expected closed database error")
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
	return testPatternWithTypes(name, file, expr, nil)
}

func testPatternWithTypes(name, file, expr string, groupTypes []AnalyseValueKind) AnalysePatternPlan {
	re := regexp.MustCompile(expr)
	if groupTypes == nil {
		groupTypes = groupTypesForCount(re.NumSubexp(), nil)
	}
	return AnalysePatternPlan{
		Name:         name,
		File:         file,
		Regex:        expr,
		GroupCount:   re.NumSubexp(),
		GroupTypes:   groupTypes,
		CompiledExpr: re,
	}
}

func testCSVAnalyseStore(t *testing.T, status Status) (*Store, AnalysePlan) {
	t.Helper()
	runDir := t.TempDir()
	manifest := Manifest{
		Steps: []ManifestStep{
			{Name: "run", Dir: "run", AnalyseCSV: "analyse.csv"},
			{Name: "prep", Dir: "prep"},
		},
		Work: []ManifestWork{
			{Step: "run", Row: 0, Dir: "000000", Values: map[string]string{"x": "a"}},
			{Step: "run", Row: 1, Dir: "000001", Values: map[string]string{"x": "b"}},
			{Step: "prep", Row: 0, Dir: "000000", Values: map[string]string{"x": "ignored"}},
		},
	}
	store := NewStore(runDir, manifest, nil)
	for _, step := range manifest.Steps {
		if err := os.MkdirAll(filepath.Join(runDir, step.Dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, work := range manifest.Work {
		workDir := store.WorkDir(work)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if work.Step == "run" {
			number := map[int]string{0: "7", 1: "8"}[work.Row]
			if err := os.WriteFile(filepath.Join(workDir, "out.log"), []byte("Number: "+number+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: status, Step: work.Step, Row: work.Row}); err != nil {
			t.Fatal(err)
		}
	}

	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnWorkValue, Source: "x", GroupCount: 1},
		{Kind: analyseColumnPattern, Source: "number", GroupCount: 1},
	}, map[string]AnalysePatternPlan{
		"number": testPattern("number", "out.log", `Number: ([0-9]+)`),
	})
	plan.Header = []string{"run_id", "x", "number"}
	return store, plan
}

func testSQLiteAnalyseStore(t *testing.T, status Status) (*Store, AnalysePlan) {
	t.Helper()
	runDir := t.TempDir()
	manifest := Manifest{
		AnalyseDatabasePath: filepath.Join(t.TempDir(), "results.sqlite"),
		Steps: []ManifestStep{
			{Name: "run", Dir: "run", AnalyseTable: "bench_000000_run"},
		},
		Work: []ManifestWork{{
			Step:   "run",
			Row:    0,
			Dir:    "000000",
			Values: map[string]string{"x": "a"},
		}},
	}
	store := NewStore(runDir, manifest, nil)
	work := manifest.Work[0]
	workDir := store.WorkDir(work)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "out.log"), []byte("Number: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteWorkStatus(work, WorkStatus{Schema: 1, Status: status, Step: work.Step, Row: work.Row}); err != nil {
		t.Fatal(err)
	}
	plan := testAnalysePlan([]AnalyseColumnPlan{
		{Kind: analyseColumnWorkValue, Source: "x", GroupCount: 1},
		{Kind: analyseColumnPattern, Source: "number", GroupCount: 1},
	}, map[string]AnalysePatternPlan{
		"number": testPattern("number", "out.log", `Number: ([0-9]+)`),
	})
	plan.Header = []string{"run_id", "x", "number"}
	return store, plan
}

func readCSVRows(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
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

func assertCellRows(t *testing.T, got [][]AnalyseCell, want [][]string) {
	t.Helper()
	assertRows(t, analyseCellsToCSVRows(got), want)
}

func analyseRowsFromStrings(rows [][]string) [][]AnalyseCell {
	out := make([][]AnalyseCell, 0, len(rows))
	for _, row := range rows {
		cells := make([]AnalyseCell, len(row))
		for i, value := range row {
			cells[i] = stringAnalyseCell(value)
		}
		out = append(out, cells)
	}
	return out
}

func assertAnalyseTable(t *testing.T, dbPath, table string, wantHeader []string, wantRows [][]string) {
	t.Helper()
	db, err := openAnalyseDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	header, rows, err := readAnalyseTable(db, table)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(header, "\x00") != strings.Join(wantHeader, "\x00") {
		t.Fatalf("header = %#v, want %#v", header, wantHeader)
	}
	assertRows(t, rows, wantRows)
}

func readSQLiteColumnTypes(t *testing.T, dbPath, table string) [][]string {
	t.Helper()
	db, err := openAnalyseDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := make([][]string, 0)
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		out = append(out, []string{name, typ})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
