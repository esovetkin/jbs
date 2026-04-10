package printparam

import (
	"slices"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func compileForPrintParam(t *testing.T, src string) *sema.Result {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	return res
}

func stepRows(table Table, step string) []Row {
	out := make([]Row, 0)
	for _, row := range table.Rows {
		if row.StepName == step {
			out = append(out, row)
		}
	}
	return out
}

func assertNoHeaderOnlyColumns(t *testing.T, table Table) {
	t.Helper()
	for _, col := range table.Columns {
		present := false
		for _, row := range table.Rows {
			if _, ok := row.Values[col]; ok {
				present = true
				break
			}
		}
		if !present {
			t.Fatalf("column %q is header-only in table: %#v", col, table.Columns)
		}
	}
}

func TestBuildFullImport(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}

do s with p {
  echo ${a} ${b}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	if len(table.Columns) != 2 || table.Columns[0] != "p.a" || table.Columns[1] != "p.b" {
		t.Fatalf("unexpected columns: %#v", table.Columns)
	}
	rows := stepRows(table, "s")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Values["p.a"] != "1" || rows[0].Values["p.b"] != "x" {
		t.Fatalf("unexpected row 0: %#v", rows[0].Values)
	}
	if rows[1].Values["p.a"] != "2" || rows[1].Values["p.b"] != "y" {
		t.Fatalf("unexpected row 1: %#v", rows[1].Values)
	}
}

func TestBuildSubsetGrouping(t *testing.T) {
	src := `
param p {
  a = (1,1,2)
  b = ("x","x","y")
  a + b
}

do s with a from p {
  echo ${a}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	rows := stepRows(table, "s")
	if len(rows) != 2 {
		t.Fatalf("expected grouped subset to produce 2 rows, got %d", len(rows))
	}
	if rows[0].Values["p.a"] != "1" || rows[1].Values["p.a"] != "2" {
		t.Fatalf("unexpected grouped values: %#v %#v", rows[0].Values, rows[1].Values)
	}
}

func TestBuildAfterInheritanceNarrowing(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("a","b","c")
  c = ("x","y","z")
  a * (b + c)
}

do s0 with a from p {
  echo ${a}
}

do s1 after s0 with (b,c) from p {
  echo ${a} ${b} ${c}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	s0 := stepRows(table, "s0")
	s1 := stepRows(table, "s1")
	if len(s0) != 2 {
		t.Fatalf("expected 2 rows for s0, got %d", len(s0))
	}
	if len(s1) != 6 {
		t.Fatalf("expected 6 rows for s1, got %d", len(s1))
	}
	expected := [][3]string{
		{"1", "a", "x"},
		{"1", "b", "y"},
		{"1", "c", "z"},
		{"2", "a", "x"},
		{"2", "b", "y"},
		{"2", "c", "z"},
	}
	for i, row := range s1 {
		if row.Values["p.a"] != expected[i][0] || row.Values["p.b"] != expected[i][1] || row.Values["p.c"] != expected[i][2] {
			t.Fatalf("unexpected s1 row %d: %#v", i, row.Values)
		}
	}
}

func TestBuildTransitiveInheritance(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y","z")
  c = ("u","v","w")
  a * (b + c)
}

do s0 with a from p {
  echo ${a}
}

do s1 after s0 with b from p {
  echo ${a} ${b}
}

do s2 after s1 with c from p {
  echo ${a} ${b} ${c}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	if got := len(stepRows(table, "s0")); got != 2 {
		t.Fatalf("expected 2 rows for s0, got %d", got)
	}
	if got := len(stepRows(table, "s1")); got != 6 {
		t.Fatalf("expected 6 rows for s1, got %d", got)
	}
	s2 := stepRows(table, "s2")
	if len(s2) != 6 {
		t.Fatalf("expected 6 rows for s2, got %d", len(s2))
	}
	for i, row := range s2 {
		if row.Values["p.a"] == "" || row.Values["p.b"] == "" || row.Values["p.c"] == "" {
			t.Fatalf("row %d missing inherited values: %#v", i, row.Values)
		}
	}
}

func TestBuildMixedSourcesCartesian(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

param q {
  b = ("x","y")
  b
}

do s with p, q {
  echo ${a} ${b}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	rows := stepRows(table, "s")
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	expected := [][2]string{{"1", "x"}, {"1", "y"}, {"2", "x"}, {"2", "y"}}
	for i, row := range rows {
		if row.Values["p.a"] != expected[i][0] || row.Values["q.b"] != expected[i][1] {
			t.Fatalf("unexpected row %d: %#v", i, row.Values)
		}
	}
}

func TestBuildTupleArithmeticAssignmentRows(t *testing.T) {
	src := `
param p {
  x = tuple([1,2,3]) * 2
  x
}

do s with p {
  echo ${x}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	rows := stepRows(table, "s")
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(rows))
	}
	expected := []string{"1", "2", "3", "1", "2", "3"}
	for i, row := range rows {
		if row.Values["p.x"] != expected[i] {
			t.Fatalf("unexpected row %d: %#v", i, row.Values)
		}
	}
}

func TestBuildLetSourceImport(t *testing.T) {
	src := `
let l {
  x = 1
  y = "a"
}
do s with l {
  echo ${x} ${y}
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	if len(table.Columns) != 2 || table.Columns[0] != "l.x" || table.Columns[1] != "l.y" {
		t.Fatalf("unexpected columns for step-imported let namespace: %#v", table.Columns)
	}
	rows := stepRows(table, "s")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Values["l.x"] != "1" || rows[0].Values["l.y"] != "a" {
		t.Fatalf("unexpected row 0 values: %#v", rows[0].Values)
	}
	assertNoHeaderOnlyColumns(t, table)
}

func TestBuildSubmitUseDefaultsDoesNotCreateColumns(t *testing.T) {
	src := `
let submit_defaults {
  queue = "batch"
  gres = "gpu:4"
}

submit run
  use submit_defaults
{
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	if len(table.Columns) != 0 {
		t.Fatalf("expected no columns for submit-header defaults only, got %#v", table.Columns)
	}
	rows := stepRows(table, "run")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertNoHeaderOnlyColumns(t, table)
}

func TestBuildSubmitUseDefaultsExcludedButWithLetAndParamIncluded(t *testing.T) {
	src := `
let submit_defaults {
  queue = "batch"
  gres = "gpu:4"
}

let l {
  x = 1
}

param p {
  a = (1,2,3)
  b = ("a","b")
  a*b
}

submit run
  use submit_defaults
  with p, l
{
  preprocess = {
    echo ${a} $b $x
  }
}
`
	res := compileForPrintParam(t, src)
	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected build errors: %s", diags.String())
	}
	want := []string{"l.x", "p.a", "p.b"}
	if !slices.Equal(table.Columns, want) {
		t.Fatalf("unexpected columns: got=%#v want=%#v", table.Columns, want)
	}
	for _, col := range table.Columns {
		if len(col) >= len("submit_defaults.") && col[:len("submit_defaults.")] == "submit_defaults." {
			t.Fatalf("unexpected submit_defaults column in table: %#v", table.Columns)
		}
	}
	rows := stepRows(table, "run")
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(rows))
	}
	for i, row := range rows {
		if _, ok := row.Values["l.x"]; !ok {
			t.Fatalf("row %d missing l.x: %#v", i, row.Values)
		}
		if _, ok := row.Values["p.a"]; !ok {
			t.Fatalf("row %d missing p.a: %#v", i, row.Values)
		}
		if _, ok := row.Values["p.b"]; !ok {
			t.Fatalf("row %d missing p.b: %#v", i, row.Values)
		}
	}
	assertNoHeaderOnlyColumns(t, table)
}
