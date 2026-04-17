package lower

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/sema"
)

func newAnalyseLowerContext(res *sema.Result) *lowerContext {
	return &lowerContext{
		res:                       res,
		diags:                     &diag.Diagnostics{},
		names:                     map[string]struct{}{},
		sourceParameterSetEmitted: map[string]struct{}{},
		subsetNames:               map[subsetKey]subsetInfo{},
		stepSourceRows:            map[string]map[string]string{},
		patternSetIndexByGroup:    map[string]int{},
		analyserNames:             map[string]string{},
	}
}

func TestPatternTemplateKeyAndAnalyseAliasPatternName(t *testing.T) {
	if got := patternTemplateKey("g", "p"); got != "g.p" {
		t.Fatalf("unexpected pattern template key: %q", got)
	}
	if got := analyseAliasPatternName("g-1", "p.2", "step-3", "alias.4"); got != "_jp__g_1_p_2__step_3__alias_4" {
		t.Fatalf("unexpected analyse alias pattern name: %q", got)
	}
}

func TestEnsurePatternSetBranches(t *testing.T) {
	res := &sema.Result{
		BindingsByName: map[string]*sema.GlobalBinding{
			"globals_group": {Name: "globals_group"},
		},
	}
	ctx := newAnalyseLowerContext(res)

	ctx.ensurePatternSet("inline_group", "step0")
	if len(ctx.doc.PatternSet) != 1 {
		t.Fatalf("expected one pattern set, got %#v", ctx.doc.PatternSet)
	}
	if ctx.doc.PatternSet[0].Meta.Kind != PatternSetKindInlineAnalyse || ctx.doc.PatternSet[0].Meta.Source != "step0" {
		t.Fatalf("unexpected inline patternset meta: %#v", ctx.doc.PatternSet[0].Meta)
	}

	ctx.ensurePatternSet("inline_group", "step1")
	if len(ctx.doc.PatternSet) != 1 {
		t.Fatalf("expected existing patternset to be reused, got %#v", ctx.doc.PatternSet)
	}

	ctx.ensurePatternSet("globals_group", "stepX")
	if len(ctx.doc.PatternSet) != 2 {
		t.Fatalf("expected imported-global patternset, got %#v", ctx.doc.PatternSet)
	}
	if ctx.doc.PatternSet[1].Meta.Kind != PatternSetKindImportedGlobals || ctx.doc.PatternSet[1].Meta.Source != "globals_group" {
		t.Fatalf("unexpected imported-global patternset meta: %#v", ctx.doc.PatternSet[1].Meta)
	}

	ctx.patternSetIndexByGroup["broken"] = 99
	ctx.ensurePatternSet("broken", "step2")
	if len(ctx.doc.PatternSet) != 3 {
		t.Fatalf("expected broken index branch to append new patternset, got %#v", ctx.doc.PatternSet)
	}
}

func TestAppendAliasPatternBranches(t *testing.T) {
	ctx := newAnalyseLowerContext(&sema.Result{BindingsByName: map[string]*sema.GlobalBinding{}})
	tmpl := sema.PatternTemplate{
		Group: "g",
		Name:  "p",
		Regex: "N: $jube_pat_int",
		Type:  "int",
	}

	ctx.appendAliasPattern("step0", "n", "_jp__g_p__step0__n", tmpl)
	if len(ctx.doc.PatternSet) != 0 {
		t.Fatalf("expected missing-group append to be a no-op, got %#v", ctx.doc.PatternSet)
	}

	ctx.ensurePatternSet("g", "step0")
	ctx.patternSetIndexByGroup["broken"] = 99
	ctx.appendAliasPattern("step0", "ignored", "_jp__g_p__step0__ignored", sema.PatternTemplate{Group: "broken", Name: "p", Regex: "x", Type: "string"})
	if len(ctx.doc.PatternSet[0].Pattern) != 0 {
		t.Fatalf("expected out-of-range group index to be ignored, got %#v", ctx.doc.PatternSet[0].Pattern)
	}

	ctx.appendAliasPattern("step0", "n", "_jp__g_p__step0__n", tmpl)
	if len(ctx.doc.PatternSet[0].Pattern) != 1 {
		t.Fatalf("expected one alias pattern, got %#v", ctx.doc.PatternSet[0].Pattern)
	}
	meta := ctx.doc.PatternSet[0].Pattern[0].Meta
	if !meta.IsAnalyseAlias || meta.AnalyseStep != "step0" || meta.AliasName != "n" || meta.PatternRef != "g.p" {
		t.Fatalf("unexpected alias pattern meta: %#v", meta)
	}

	ctx.appendAliasPattern("step0", "n", "_jp__g_p__step0__n", tmpl)
	if len(ctx.doc.PatternSet[0].Pattern) != 1 {
		t.Fatalf("expected duplicate alias pattern to be ignored, got %#v", ctx.doc.PatternSet[0].Pattern)
	}
}

func TestLowerAnalyseAndResult(t *testing.T) {
	res := &sema.Result{
		BindingsByName: map[string]*sema.GlobalBinding{
			"imported_patterns": {Name: "imported_patterns"},
		},
		Analyse: []*sema.AnalyseSpec{
			nil,
			{
				Name:  "analyse_run",
				Block: ast.AnalyseBlock{StepName: "run"},
				Assignments: []sema.AnalyseAssignmentSpec{
					{
						Name:     "n",
						Group:    "p",
						Pattern:  "number",
						File:     "out.log",
						Template: sema.PatternTemplate{Group: "p", Name: "number", Regex: "Number: $jube_pat_int", Type: "int"},
					},
					{
						Name:     "w",
						Group:    "p",
						Pattern:  "word",
						File:     "out.log",
						Template: sema.PatternTemplate{Group: "p", Name: "word", Regex: "Word: $jube_pat_wrd", Type: "string"},
					},
					{
						Name:     "ext",
						Group:    "imported_patterns",
						Pattern:  "external",
						File:     "extra.log",
						Template: sema.PatternTemplate{Group: "imported_patterns", Name: "external", Regex: "External: $jube_pat_wrd", Type: "string"},
					},
				},
				Columns: []sema.AnalyseColumnSpec{
					{Name: "n"},
					{Name: "w", Title: "W"},
					{Name: "plain", Source: "plain_expr"},
					{Name: "ext", Title: "External"},
				},
			},
		},
	}
	ctx := newAnalyseLowerContext(res)

	ctx.lowerAnalyseAndResult()
	if ctx.doc.Result == nil {
		t.Fatalf("expected lowered result section")
	}
	if len(ctx.doc.Analyser) != 1 {
		t.Fatalf("expected one analyser entry, got %#v", ctx.doc.Analyser)
	}
	analyser := ctx.doc.Analyser[0]
	if analyser.Name == "" || analyser.Meta.Source != "run" {
		t.Fatalf("unexpected analyser metadata: %#v", analyser)
	}
	if analyser.Use != "p, imported_patterns" {
		t.Fatalf("expected analyser use to preserve deduplicated group order, got %q", analyser.Use)
	}
	if len(analyser.Analyse) != 1 || len(analyser.Analyse[0].File) != 2 {
		t.Fatalf("expected one analyse item with deduplicated file list, got %#v", analyser.Analyse)
	}
	if analyser.Analyse[0].File[0].Use != "p" || analyser.Analyse[0].File[0].Value != "out.log" {
		t.Fatalf("unexpected first analyse file item: %#v", analyser.Analyse[0].File[0])
	}
	if analyser.Analyse[0].File[1].Use != "imported_patterns" || analyser.Analyse[0].File[1].Value != "extra.log" {
		t.Fatalf("unexpected second analyse file item: %#v", analyser.Analyse[0].File[1])
	}

	if len(ctx.doc.PatternSet) != 2 {
		t.Fatalf("expected two pattern sets, got %#v", ctx.doc.PatternSet)
	}
	if ctx.doc.PatternSet[0].Meta.Kind != PatternSetKindInlineAnalyse || ctx.doc.PatternSet[1].Meta.Kind != PatternSetKindImportedGlobals {
		t.Fatalf("unexpected patternset kinds: %#v", ctx.doc.PatternSet)
	}
	if len(ctx.doc.PatternSet[0].Pattern) != 2 || len(ctx.doc.PatternSet[1].Pattern) != 1 {
		t.Fatalf("expected alias patterns grouped by pattern set, got %#v", ctx.doc.PatternSet)
	}

	if len(ctx.doc.Result.Use) != 1 || ctx.doc.Result.Use[0] != analyser.Name {
		t.Fatalf("unexpected result use linkage: %#v", ctx.doc.Result.Use)
	}
	if len(ctx.doc.Result.Table) != 1 {
		t.Fatalf("expected one result table, got %#v", ctx.doc.Result.Table)
	}
	table := ctx.doc.Result.Table[0]
	if table.Style != "csv" || table.Meta.Source != "run" {
		t.Fatalf("unexpected result table metadata: %#v", table)
	}
	if !strings.HasPrefix(table.Name, "result_run") {
		t.Fatalf("expected result table name to start with result_run, got %q", table.Name)
	}
	if len(table.Column) != 4 {
		t.Fatalf("unexpected result columns: %#v", table.Column)
	}
	if table.Column[0].Title != "n" || !strings.HasPrefix(table.Column[0].Expr, "_jp__p_number__run__n") {
		t.Fatalf("expected first column to map alias expr, got %#v", table.Column[0])
	}
	if table.Column[1].Title != "W" || !strings.HasPrefix(table.Column[1].Expr, "_jp__p_word__run__w") {
		t.Fatalf("expected second column to preserve title and map alias expr, got %#v", table.Column[1])
	}
	if table.Column[2].Title != "plain" || table.Column[2].Expr != "plain_expr" {
		t.Fatalf("expected plain column passthrough, got %#v", table.Column[2])
	}
	if table.Column[3].Title != "External" || !strings.HasPrefix(table.Column[3].Expr, "_jp__imported_patterns_external__run__ext") {
		t.Fatalf("expected external column to map imported pattern alias, got %#v", table.Column[3])
	}
}

func TestLowerAnalyseAndResultNoAnalyseSpecs(t *testing.T) {
	ctx := newAnalyseLowerContext(&sema.Result{})
	ctx.lowerAnalyseAndResult()
	if ctx.doc.Result != nil || len(ctx.doc.Analyser) != 0 || len(ctx.doc.PatternSet) != 0 {
		t.Fatalf("expected no-op lowering without analyse specs, got %+v", ctx.doc)
	}
}
