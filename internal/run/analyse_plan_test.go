package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestAnalysePlansByStepBuildsHeaders(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
cases = table(x=[1])
do run with cases {
    echo "pair=AA-17" > out.log
}
analyse run {
    pair = "pair=([A-Z]+)-([0-9]+)" in "out.log"
    n = "n=%d" in "out.log"
    (x as "X", pair as "Pair", n)
}
`)
	analysis := plan.Analyses["run"]
	if analysis.Step != "run" || analysis.CSV != "analyse.csv" {
		t.Fatalf("unexpected analyse plan identity: %#v", analysis)
	}
	want := []string{"run_id", "X", "Pair.0", "Pair.1", "n"}
	if strings.Join(analysis.Header, "|") != strings.Join(want, "|") {
		t.Fatalf("header = %#v, want %#v", analysis.Header, want)
	}
	if len(analysis.Columns) != 3 {
		t.Fatalf("columns = %#v", analysis.Columns)
	}
	if analysis.Patterns["pair"].GroupCount != 2 {
		t.Fatalf("pair group count = %d, want 2", analysis.Patterns["pair"].GroupCount)
	}
	if analysis.Patterns["n"].GroupCount != 1 {
		t.Fatalf("n group count = %d, want 1", analysis.Patterns["n"].GroupCount)
	}
	assertAnalyseKinds(t, analysis.ColumnTypes, []AnalyseValueKind{
		analyseValueString,
		analyseValueInt,
		analyseValueString,
		analyseValueString,
		analyseValueInt,
	})
	if plan.Manifest.Steps[0].AnalyseCSV != "analyse.csv" {
		t.Fatalf("manifest analyse csv = %q", plan.Manifest.Steps[0].AnalyseCSV)
	}
}

func TestAnalysePlansByStepTracksMixedPatternTypes(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    mixed = "id=%d label=([A-Z]+) ratio=%f word=%w" in "out.log"
    (mixed)
}
`)
	analysis := plan.Analyses["run"]
	wantHeader := []string{"run_id", "mixed.0", "mixed.1", "mixed.2", "mixed.3"}
	if strings.Join(analysis.Header, "|") != strings.Join(wantHeader, "|") {
		t.Fatalf("header = %#v, want %#v", analysis.Header, wantHeader)
	}
	wantTypes := []AnalyseValueKind{
		analyseValueString,
		analyseValueInt,
		analyseValueString,
		analyseValueFloat,
		analyseValueString,
	}
	assertAnalyseKinds(t, analysis.ColumnTypes, wantTypes)
	assertAnalyseKinds(t, analysis.Patterns["mixed"].GroupTypes, wantTypes[1:])
}

func TestAnalysePlansByStepUsesSQLiteBackend(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	plan := buildPlanFromSource(t, `
jbs_name = "bench"
jbs_database = "results.sqlite"
cases = table(x=[1])
do run with cases {
    echo "$x"
}
analyse run {
    (x)
}
`)
	if plan.AnalyseDatabase != "results.sqlite" {
		t.Fatalf("runtime plan database = %q", plan.AnalyseDatabase)
	}
	if plan.AnalyseDatabasePath != filepath.Join(cwd, "results.sqlite") {
		t.Fatalf("runtime plan database path = %q", plan.AnalyseDatabasePath)
	}
	if plan.Manifest.AnalyseDatabase != "results.sqlite" {
		t.Fatalf("manifest database = %q", plan.Manifest.AnalyseDatabase)
	}
	if plan.Manifest.AnalyseDatabasePath != filepath.Join(cwd, "results.sqlite") {
		t.Fatalf("manifest database path = %q", plan.Manifest.AnalyseDatabasePath)
	}
	step := plan.Manifest.Steps[0]
	if step.AnalyseCSV != "" || step.AnalyseTable != "run" {
		t.Fatalf("unexpected analyse backend fields: %#v", step)
	}
}

func TestAnalysePlansByStepTypesWorkValueColumns(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
jbs_database = "results.sqlite"
cases = table(i = [1], f = [1.5], b = [true], s = ["x"])
do run with cases {
    echo ok
}
analyse run {
    (i, f, b, s)
}
`)
	analysis := plan.Analyses["run"]
	assertAnalyseKinds(t, analysis.ColumnTypes, []AnalyseValueKind{
		analyseValueString,
		analyseValueInt,
		analyseValueFloat,
		analyseValueBool,
		analyseValueString,
	})
}

func TestAnalysePlansByStepBuildsInlinePatternColumns(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    (
        "Runtime %f" in "job.out",
        "Runtime %f" in "job.out" as "runtime",
        "Point %d %f" in "job.out" as "point",
    )
}
`)
	analysis := plan.Analyses["run"]
	wantHeader := []string{"run_id", "Runtime %f", "runtime", "point.0", "point.1"}
	if strings.Join(analysis.Header, "|") != strings.Join(wantHeader, "|") {
		t.Fatalf("header = %#v, want %#v", analysis.Header, wantHeader)
	}
	assertAnalyseKinds(t, analysis.ColumnTypes, []AnalyseValueKind{
		analyseValueString,
		analyseValueFloat,
		analyseValueFloat,
		analyseValueInt,
		analyseValueFloat,
	})
	if len(analysis.Columns) != 3 {
		t.Fatalf("columns = %#v", analysis.Columns)
	}
	for _, col := range analysis.Columns {
		if col.Kind != analyseColumnPattern {
			t.Fatalf("expected inline pattern columns, got %#v", analysis.Columns)
		}
	}
	if len(analysis.Patterns) != 3 {
		t.Fatalf("patterns = %#v", analysis.Patterns)
	}
}

func TestAnalysePlansByStepInlineZeroCaptureUsesPatternTextInError(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    ("literal" in "out.log")
}
`)
	if err == nil || !strings.Contains(err.Error(), "literal") || strings.Contains(err.Error(), "__analyse_inline") {
		t.Fatalf("expected inline zero-capture error with display pattern, got %v", err)
	}
}

func TestMergeAnalyseValueKinds(t *testing.T) {
	tests := []struct {
		name string
		a    AnalyseValueKind
		b    AnalyseValueKind
		want AnalyseValueKind
	}{
		{name: "empty", b: analyseValueInt, want: analyseValueInt},
		{name: "int", a: analyseValueInt, b: analyseValueInt, want: analyseValueInt},
		{name: "float", a: analyseValueFloat, b: analyseValueFloat, want: analyseValueFloat},
		{name: "int float", a: analyseValueInt, b: analyseValueFloat, want: analyseValueFloat},
		{name: "float int", a: analyseValueFloat, b: analyseValueInt, want: analyseValueFloat},
		{name: "bool", a: analyseValueBool, b: analyseValueBool, want: analyseValueBool},
		{name: "int string", a: analyseValueInt, b: analyseValueString, want: analyseValueString},
		{name: "bool int", a: analyseValueBool, b: analyseValueInt, want: analyseValueString},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeAnalyseValueKinds(tt.a, tt.b); got != tt.want {
				t.Fatalf("mergeAnalyseValueKinds(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestGlobalAnalyseDatabasePathResolution(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	tests := []struct {
		name        string
		value       eval.Value
		wantDisplay string
		wantPath    string
		wantErr     string
	}{
		{name: "missing", value: eval.Value{}},
		{name: "empty", value: eval.String("")},
		{name: "relative", value: eval.String("results.sqlite"), wantDisplay: "results.sqlite", wantPath: filepath.Join(cwd, "results.sqlite")},
		{name: "nested", value: eval.String("out/results.sqlite"), wantDisplay: filepath.Join("out", "results.sqlite"), wantPath: filepath.Join(cwd, "out", "results.sqlite")},
		{name: "parent", value: eval.String("../results.sqlite"), wantDisplay: filepath.Join("..", "results.sqlite"), wantPath: filepath.Clean(filepath.Join(cwd, "..", "results.sqlite"))},
		{name: "absolute", value: eval.String(filepath.Join(cwd, "abs.sqlite")), wantDisplay: filepath.Join(cwd, "abs.sqlite"), wantPath: filepath.Join(cwd, "abs.sqlite")},
		{name: "dot", value: eval.String("."), wantErr: "must name a database file"},
		{name: "dotdot", value: eval.String(".."), wantErr: "must name a database file"},
		{name: "non-string", value: eval.Int(1), wantErr: "must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := globalAnalyseDatabase(&sema.Result{
				Globals: sema.GlobalState{Values: map[string]eval.Value{"jbs_database": tt.value}},
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Display != tt.wantDisplay || cfg.Path != tt.wantPath {
				t.Fatalf("config = %#v, want display=%q path=%q", cfg, tt.wantDisplay, tt.wantPath)
			}
		})
	}
}

func TestAnalysePlansByStepRejectsDuplicateSQLiteHeaders(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
jbs_database = "results.sqlite"
cases = table(x=[1])
do run with cases {
    echo ok
}
analyse run {
    (x, x as "x")
}
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate result column") {
		t.Fatalf("expected duplicate SQLite column error, got %v", err)
	}
}

func TestAnalysePlansByStepRejectsDuplicateSQLiteInlineDefaultHeaders(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
jbs_database = "results.sqlite"
do run {
    echo ok
}
analyse run {
    ("Runtime %f" in "a.out", "Runtime %f" in "b.out")
}
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate result column") {
		t.Fatalf("expected duplicate SQLite inline column error, got %v", err)
	}
}

func TestAnalysePlansByStepRejectsDuplicateTarget(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    ()
}
analyse run {
    ()
}
`)
	if err == nil || !strings.Contains(err.Error(), "multiple analyse blocks target step") {
		t.Fatalf("expected duplicate analyse error, got %v", err)
	}
}

func TestAnalysePlansByStepRejectsInvalidRegex(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    bad = "(" in "out.log"
    (bad)
}
`)
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestAnalysePlansByStepRejectsZeroCapturePattern(t *testing.T) {
	_, err := buildPlanFromSourceErr(t, `
jbs_name = "bench"
do run {
    echo ok
}
analyse run {
    bad = "literal" in "out.log"
    (bad)
}
`)
	if err == nil || !strings.Contains(err.Error(), "at least one capture group") {
		t.Fatalf("expected zero-capture error, got %v", err)
	}
}

func TestAnalysePlansByStepIgnoresUnusedExtractionAssignments(t *testing.T) {
	plan := buildPlanFromSource(t, `
jbs_name = "bench"
cases = table(x=[1])
do run with cases {
    echo ok
}
analyse run {
    unused = "literal" in "missing.log"
    (x)
}
`)
	analysis := plan.Analyses["run"]
	if len(analysis.Patterns) != 0 {
		t.Fatalf("unused extraction should not be compiled, got %#v", analysis.Patterns)
	}
	want := []string{"run_id", "x"}
	if strings.Join(analysis.Header, "|") != strings.Join(want, "|") {
		t.Fatalf("header = %#v, want %#v", analysis.Header, want)
	}
}

func buildPlanFromSourceErr(t *testing.T, source string) (runtimePlan, error) {
	t.Helper()
	diags := &diag.Diagnostics{}
	cwd := t.TempDir()
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return buildRuntimePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: "test.jbs",
	}, diags)
}

func assertAnalyseKinds(t *testing.T, got, want []AnalyseValueKind) {
	t.Helper()
	if strings.Join(analyseKindStrings(got), "|") != strings.Join(analyseKindStrings(want), "|") {
		t.Fatalf("kinds = %#v, want %#v", got, want)
	}
}

func analyseKindStrings(kinds []AnalyseValueKind) []string {
	out := make([]string, len(kinds))
	for i, kind := range kinds {
		out[i] = string(kind)
	}
	return out
}
