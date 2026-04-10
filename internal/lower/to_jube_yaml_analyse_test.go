package lower_test

import (
	"strings"
	"testing"
)

func TestLetAndAnalyseLowering(t *testing.T) {
	src := `
param params {
  x = (1,2,3)
  a = ("a","b","c")
  a + x
}

do write with params {
  echo "Number: ${x}" > en
  echo "Letter: ${a}" >> en
  echo "Zahl: ${x}" > de
}

let p {
  number = "Number: %d"
  zahl = "Zahl: %d"
  letter = "Letter: %w"
}

analyse write with p {
  p0 = number in "en"
  p1 = zahl in "de"
  p2 = letter in "en"
  (
    a,
    x,
    p0,
    p1 as "de zahl",
    p2,
  )
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.PatternSet) != 1 {
		t.Fatalf("expected one grouped patternset, got %#v", doc.PatternSet)
	}
	ps := doc.PatternSet[0]
	if ps.Name != "p" {
		t.Fatalf("expected grouped patternset named p, got %#v", ps.Name)
	}
	if len(ps.Pattern) != 3 {
		t.Fatalf("expected 3 alias patterns, got %#v", ps.Pattern)
	}
	for _, p := range ps.Pattern {
		if !strings.Contains(p.Name, "__write__") {
			t.Fatalf("expected analyse alias pattern naming only, got %#v", p.Name)
		}
		if !p.Meta.IsAnalyseAlias || p.Meta.AnalyseStep != "write" {
			t.Fatalf("expected analyse alias pattern metadata, got %#v", p.Meta)
		}
	}
	if len(doc.Analyser) != 1 {
		t.Fatalf("expected one analyser, got %#v", doc.Analyser)
	}
	an := doc.Analyser[0]
	if an.Use != "p" {
		t.Fatalf("expected compact analyser use 'p', got %#v", an.Use)
	}
	if an.Analyse[0].Step != "write" {
		t.Fatalf("unexpected analyse step: %#v", an.Analyse[0])
	}
	if len(an.Analyse[0].File) != 2 {
		t.Fatalf("expected deduplicated analyse files, got %#v", an.Analyse[0].File)
	}
	if an.Analyse[0].File[0].Use != "p" || an.Analyse[0].File[0].Value != "en" {
		t.Fatalf("unexpected first analyse file: %#v", an.Analyse[0].File[0])
	}
	if an.Analyse[0].File[1].Use != "p" || an.Analyse[0].File[1].Value != "de" {
		t.Fatalf("unexpected second analyse file: %#v", an.Analyse[0].File[1])
	}
	if doc.Result == nil {
		t.Fatalf("expected result object")
	}
	if len(doc.Result.Use) != 1 || doc.Result.Use[0] != an.Name {
		t.Fatalf("unexpected result use list: %#v", doc.Result.Use)
	}
	if len(doc.Result.Table) != 1 {
		t.Fatalf("expected one result table, got %#v", doc.Result.Table)
	}
	table := doc.Result.Table[0]
	if table.Style != "csv" {
		t.Fatalf("expected csv style, got %#v", table.Style)
	}
	if len(table.Column) != 5 {
		t.Fatalf("unexpected columns: %#v", table.Column)
	}
	if table.Column[2].Title != "p0" || table.Column[2].Expr != "_jp__p_number__write__p0" {
		t.Fatalf("unexpected first analyse result column: %#v", table.Column[2])
	}
	if table.Column[3].Title != "de zahl" || table.Column[3].Expr != "_jp__p_zahl__write__p1" {
		t.Fatalf("unexpected aliased result column: %#v", table.Column[3])
	}
	if table.Column[4].Title != "p2" || table.Column[4].Expr != "_jp__p_letter__write__p2" {
		t.Fatalf("unexpected second analyse result column: %#v", table.Column[4])
	}
}

func TestAnalyseAliasPatternsetMaterialization(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "Number: ${a}" > en
  echo "Number: ${a}" > de
}
let g {
  number = "Number: %d"
}
analyse write with g {
  p0 = number in "en"
  p1 = number in "de"
  (a, p0, p1)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Analyser) != 1 {
		t.Fatalf("expected one analyser")
	}
	if doc.Analyser[0].Use != "g" {
		t.Fatalf("expected compact analyser use 'g', got %#v", doc.Analyser[0].Use)
	}
	files := doc.Analyser[0].Analyse[0].File
	if len(files) != 2 {
		t.Fatalf("expected two analyse file entries, got %#v", files)
	}
	if files[0].Use != "g" || files[1].Use != "g" {
		t.Fatalf("expected grouped patternset use for both files, got %#v", files)
	}
	if len(doc.PatternSet) != 1 || doc.PatternSet[0].Name != "g" {
		t.Fatalf("expected one grouped patternset g, got %#v", doc.PatternSet)
	}
	if len(doc.PatternSet[0].Pattern) != 2 {
		t.Fatalf("expected only alias patterns in grouped set, got %#v", doc.PatternSet[0].Pattern)
	}
	names := make(map[string]struct{}, len(doc.PatternSet[0].Pattern))
	for _, pat := range doc.PatternSet[0].Pattern {
		names[pat.Name] = struct{}{}
	}
	if _, ok := names["_jp__g_number__write__p0"]; !ok {
		t.Fatalf("expected alias pattern p0 in grouped set: %#v", doc.PatternSet[0].Pattern)
	}
	if _, ok := names["_jp__g_number__write__p1"]; !ok {
		t.Fatalf("expected alias pattern p1 in grouped set: %#v", doc.PatternSet[0].Pattern)
	}
}

func TestAnalyseResultColumnUsesAliasPatternName(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "Number: ${a}" > en
}
let g {
  number = "Number: %d"
}
analyse write with g {
  number = number in "en"
  (a, number)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if doc.Result == nil || len(doc.Result.Table) != 1 || len(doc.Result.Table[0].Column) != 2 {
		t.Fatalf("unexpected result shape: %#v", doc.Result)
	}
	col := doc.Result.Table[0].Column[1]
	if col.Title != "number" || col.Expr != "_jp__g_number__write__number" {
		t.Fatalf("unexpected analyse column mapping: %#v", col)
	}
}

func TestAnalyseCompactUseAcrossMultiplePatternGroups(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "A 1" > a.out
  echo "B 1" > b.out
}
let g1 {
  x = "A %d"
}
let g2 {
  y = "B %d"
}
analyse write with g1, g2 {
  ax = x in "a.out"
  by = y in "b.out"
  (a, ax, by)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Analyser) != 1 {
		t.Fatalf("expected one analyser")
	}
	if doc.Analyser[0].Use != "g1, g2" {
		t.Fatalf("expected compact analyser use 'g1, g2', got %#v", doc.Analyser[0].Use)
	}
}

func TestAnalyseInlineExpressionsUseDistinctSyntheticIds(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do write with p {
  echo "A 1" > a.out
  echo "B 1" > b.out
}
analyse write {
  ax = "A %d" in "a.out"
  by = "B %d" in "b.out"
  (a, ax, by)
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.PatternSet) != 2 {
		t.Fatalf("expected two synthetic pattern sets, got %#v", doc.PatternSet)
	}
	names := map[string]struct{}{}
	for _, ps := range doc.PatternSet {
		names[ps.Name] = struct{}{}
	}
	if _, ok := names["_ja_write_ax"]; !ok {
		t.Fatalf("missing synthetic inline pattern set for ax: %#v", doc.PatternSet)
	}
	if _, ok := names["_ja_write_by"]; !ok {
		t.Fatalf("missing synthetic inline pattern set for by: %#v", doc.PatternSet)
	}
	if doc.Result == nil || len(doc.Result.Table) != 1 {
		t.Fatalf("missing result table")
	}
	cols := doc.Result.Table[0].Column
	if len(cols) != 3 {
		t.Fatalf("unexpected result columns: %#v", cols)
	}
	if cols[1].Expr == cols[2].Expr {
		t.Fatalf("expected distinct synthetic ids for inline expressions, got %#v", cols)
	}
}
