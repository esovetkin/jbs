package lower

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/sema"
)

func newAnalyseLowerCtx(res *sema.Result) *lowerContext {
	return &lowerContext{
		res:                    res,
		diags:                  &diag.Diagnostics{},
		names:                  map[string]struct{}{},
		sourceParamsetEmitted:  map[string]struct{}{},
		subsetNames:            map[subsetKey]subsetInfo{},
		stepSourceRows:         map[string]map[string]string{},
		patternSetIndexByGroup: map[string]int{},
		analyserNames:          map[string]string{},
	}
}

func TestPatternTemplateKey(t *testing.T) {
	if got := patternTemplateKey("g", "p"); got != "g.p" {
		t.Fatalf("unexpected pattern template key: %q", got)
	}
}

func TestEnsurePatternSetBranches(t *testing.T) {
	res := &sema.Result{
		LetByName: map[string]*sema.LetNamespace{
			"let_group": {Name: "let_group"},
		},
	}
	ctx := newAnalyseLowerCtx(res)

	ctx.ensurePatternSet("inline_group", "step0")
	if len(ctx.doc.PatternSet) != 1 {
		t.Fatalf("expected one pattern set, got %#v", ctx.doc.PatternSet)
	}
	if ctx.doc.PatternSet[0].Meta.Kind != PatternSetKindInline || ctx.doc.PatternSet[0].Meta.Source != "step0" {
		t.Fatalf("unexpected inline patternset meta: %#v", ctx.doc.PatternSet[0].Meta)
	}

	// Existing index branch should short-circuit.
	ctx.ensurePatternSet("inline_group", "step1")
	if len(ctx.doc.PatternSet) != 1 {
		t.Fatalf("expected existing patternset to be reused")
	}

	// Let-backed patternset branch.
	ctx.ensurePatternSet("let_group", "stepX")
	if len(ctx.doc.PatternSet) != 2 {
		t.Fatalf("expected second patternset for let-backed group")
	}
	if ctx.doc.PatternSet[1].Meta.Kind != PatternSetKindLet || ctx.doc.PatternSet[1].Meta.Source != "let_group" {
		t.Fatalf("unexpected let patternset meta: %#v", ctx.doc.PatternSet[1].Meta)
	}

	// Invalid cached index should be repaired by creating a fresh entry.
	ctx.patternSetIndexByGroup["broken"] = 99
	ctx.ensurePatternSet("broken", "step2")
	if len(ctx.doc.PatternSet) != 3 {
		t.Fatalf("expected broken index branch to append new patternset, got %#v", ctx.doc.PatternSet)
	}
}

func TestAppendAliasPatternBranches(t *testing.T) {
	res := &sema.Result{LetByName: map[string]*sema.LetNamespace{}}
	ctx := newAnalyseLowerCtx(res)

	tmpl := sema.PatternTemplate{
		Group: "g",
		Name:  "p",
		Regex: "N: $jube_pat_int",
		Type:  "int",
	}

	// Missing group index branch: no-op.
	ctx.appendAliasPattern("step0", "n", "_jp__g_p__step0__n", tmpl)
	if len(ctx.doc.PatternSet) != 0 {
		t.Fatalf("expected no pattern sets when group index is missing")
	}

	ctx.ensurePatternSet("g", "step0")
	ctx.appendAliasPattern("step0", "n", "_jp__g_p__step0__n", tmpl)
	if len(ctx.doc.PatternSet[0].Pattern) != 1 {
		t.Fatalf("expected one alias pattern, got %#v", ctx.doc.PatternSet[0].Pattern)
	}
	meta := ctx.doc.PatternSet[0].Pattern[0].Meta
	if !meta.IsAnalyseAlias || meta.AnalyseStep != "step0" || meta.AliasName != "n" || meta.PatternRef != "g.p" {
		t.Fatalf("unexpected alias pattern meta: %#v", meta)
	}

	// Duplicate internal name should be ignored.
	ctx.appendAliasPattern("step0", "n", "_jp__g_p__step0__n", tmpl)
	if len(ctx.doc.PatternSet[0].Pattern) != 1 {
		t.Fatalf("expected duplicate alias pattern to be ignored, got %#v", ctx.doc.PatternSet[0].Pattern)
	}
}

func TestLowerAnalyseAndResult(t *testing.T) {
	spec := &sema.AnalyseSpec{
		Name: "analyse_run",
		Block: ast.AnalyseBlock{
			StepName: "run",
		},
		Assignments: []sema.AnalyseAssignmentSpec{
			{
				Name:    "n",
				Group:   "p",
				Pattern: "number",
				File:    "out.log",
				Template: sema.PatternTemplate{
					Group: "p",
					Name:  "number",
					Regex: "Number: $jube_pat_int",
					Type:  "int",
				},
			},
			{
				Name:    "w",
				Group:   "p",
				Pattern: "word",
				File:    "out.log",
				Template: sema.PatternTemplate{
					Group: "p",
					Name:  "word",
					Regex: "Word: $jube_pat_wrd",
					Type:  "string",
				},
			},
		},
		Columns: []sema.AnalyseColumnSpec{
			{Name: "n", Title: ""},
			{Name: "w", Title: "W"},
			{Name: "plain", Title: "", Source: "plain_expr"},
		},
	}

	res := &sema.Result{
		Analyse:   []*sema.AnalyseSpec{nil, spec},
		LetByName: map[string]*sema.LetNamespace{},
	}
	ctx := newAnalyseLowerCtx(res)

	ctx.lowerAnalyseAndResult()

	if ctx.doc.Result == nil {
		t.Fatalf("expected lowered result section")
	}
	if len(ctx.doc.Analyser) != 1 {
		t.Fatalf("expected one analyser entry, got %#v", ctx.doc.Analyser)
	}
	an := ctx.doc.Analyser[0]
	if an.Name == "" || an.Meta.Source != "run" {
		t.Fatalf("unexpected analyser metadata: %#v", an)
	}
	if an.Use != "p" {
		t.Fatalf("expected analyser use to include deduplicated group 'p', got %q", an.Use)
	}
	if len(an.Analyse) != 1 || len(an.Analyse[0].File) != 1 {
		t.Fatalf("expected one analyse item and deduplicated file list, got %#v", an.Analyse)
	}
	if an.Analyse[0].File[0].Use != "p" || an.Analyse[0].File[0].Value != "out.log" {
		t.Fatalf("unexpected analyse file item: %#v", an.Analyse[0].File[0])
	}

	if len(ctx.doc.PatternSet) != 1 || ctx.doc.PatternSet[0].Name != "p" {
		t.Fatalf("expected one patternset for group p, got %#v", ctx.doc.PatternSet)
	}
	if len(ctx.doc.PatternSet[0].Pattern) != 2 {
		t.Fatalf("expected two alias patterns, got %#v", ctx.doc.PatternSet[0].Pattern)
	}

	if len(ctx.doc.Result.Use) != 1 || ctx.doc.Result.Use[0] != an.Name {
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
	if len(table.Column) != 3 {
		t.Fatalf("unexpected result columns: %#v", table.Column)
	}
	if table.Column[0].Title != "n" || !strings.HasPrefix(table.Column[0].Expr, "_jp__p_number__run__n") {
		t.Fatalf("expected first column to map assignment alias pattern, got %#v", table.Column[0])
	}
	if table.Column[1].Title != "W" || !strings.HasPrefix(table.Column[1].Expr, "_jp__p_word__run__w") {
		t.Fatalf("expected second column to keep custom title and mapped expr, got %#v", table.Column[1])
	}
	if table.Column[2].Title != "plain" || table.Column[2].Expr != "plain_expr" {
		t.Fatalf("expected passthrough column expression, got %#v", table.Column[2])
	}
}
