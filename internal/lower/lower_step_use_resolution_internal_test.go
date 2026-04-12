package lower

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestResolveStepUsesForStep_NoPlan(t *testing.T) {
	ctx := &lowerContext{
		res: &sema.Result{
			ImportSourceByName: map[string]*sema.ImportSource{},
			StepImportByName:   map[string]*sema.StepImportPlan{},
		},
		diags:          &diag.Diagnostics{},
		names:          map[string]struct{}{},
		subsetNames:    map[subsetKey]subsetInfo{},
		stepSourceRows: map[string]map[string]string{},
	}

	got := ctx.resolveStepUsesForStep("run", nil)
	if len(got.Use) != 0 {
		t.Fatalf("expected no use entries without import plan, got %#v", got.Use)
	}
	if len(got.SourceRows) != 0 {
		t.Fatalf("expected no source rows without import plan, got %#v", got.SourceRows)
	}
}

func TestResolveStepUsesForStep_FullParamDirectAndAliasedSubset(t *testing.T) {
	src := &sema.ImportSource{
		Name:  "p",
		Kind:  sema.SourceKindParam,
		Order: []string{"x"},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1), eval.Int(2)},
		},
		Origins: map[string]diag.Span{},
		Modes:   map[string]string{},
	}
	plan := &sema.StepImportPlan{
		StepName: "run",
		ExplicitDelta: []sema.PlannedImport{
			{Source: "p", Kind: sema.SourceKindParam, Full: true},
		},
	}
	baseCtx := &lowerContext{
		res: &sema.Result{
			ImportSourceByName: map[string]*sema.ImportSource{"p": src},
			StepImportByName:   map[string]*sema.StepImportPlan{"run": plan},
		},
		diags:          &diag.Diagnostics{},
		names:          map[string]struct{}{},
		subsetNames:    map[subsetKey]subsetInfo{},
		stepSourceRows: map[string]map[string]string{},
	}

	direct := baseCtx.resolveStepUsesForStep("run", map[string]string{})
	if len(direct.Use) != 1 || direct.Use[0] != "p" {
		t.Fatalf("expected direct use of full param source, got %#v", direct.Use)
	}
	if len(baseCtx.doc.ParameterSet) != 0 {
		t.Fatalf("expected no synthetic subset for direct full source use, got %#v", baseCtx.doc.ParameterSet)
	}

	aliasedCtx := &lowerContext{
		res: &sema.Result{
			ImportSourceByName: map[string]*sema.ImportSource{"p": src},
			StepImportByName:   map[string]*sema.StepImportPlan{"run": plan},
		},
		doc:            Document{},
		diags:          &diag.Diagnostics{},
		names:          map[string]struct{}{},
		subsetNames:    map[subsetKey]subsetInfo{},
		stepSourceRows: map[string]map[string]string{},
	}
	aliased := aliasedCtx.resolveStepUsesForStep("run", map[string]string{"x": "_ja__x"})
	if len(aliased.Use) != 1 {
		t.Fatalf("expected one synthetic subset use entry, got %#v", aliased.Use)
	}
	name, ok := aliased.Use[0].(string)
	if !ok || !strings.HasPrefix(name, "_js__") {
		t.Fatalf("expected synthetic subset name, got %#v", aliased.Use[0])
	}
	if len(aliasedCtx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one synthetic subset parameterset, got %#v", aliasedCtx.doc.ParameterSet)
	}
}

func TestResolveStepUsesForStep_FullLetCreatesScalarSubset(t *testing.T) {
	ctx := &lowerContext{
		res: &sema.Result{
			ImportSourceByName: map[string]*sema.ImportSource{
				"l": {
					Name:  "l",
					Kind:  sema.SourceKindLet,
					Order: []string{"queue"},
					Vars: map[string][]eval.Value{
						"queue": {eval.String("batch")},
					},
					Origins: map[string]diag.Span{},
					Modes: map[string]string{
						"queue": "python",
					},
				},
			},
			StepImportByName: map[string]*sema.StepImportPlan{
				"run": {
					StepName: "run",
					ExplicitDelta: []sema.PlannedImport{
						{Source: "l", Kind: sema.SourceKindLet, Full: true},
					},
				},
			},
		},
		diags:          &diag.Diagnostics{},
		names:          map[string]struct{}{},
		subsetNames:    map[subsetKey]subsetInfo{},
		stepSourceRows: map[string]map[string]string{},
	}

	got := ctx.resolveStepUsesForStep("run", nil)
	if len(got.Use) != 1 {
		t.Fatalf("expected one synthetic let subset use entry, got %#v", got.Use)
	}
	if len(got.SourceRows) != 0 {
		t.Fatalf("let subset should not carry inherited row variable, got %#v", got.SourceRows)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one generated let subset parameterset, got %#v", ctx.doc.ParameterSet)
	}
}

func TestResolveStepUses_PartialDedupAndMissingSources(t *testing.T) {
	ctx := &lowerContext{
		res: &sema.Result{
			ImportSourceByName: map[string]*sema.ImportSource{
				"p": {
					Name:  "p",
					Kind:  sema.SourceKindParam,
					Order: []string{"x"},
					Vars: map[string][]eval.Value{
						"x": {eval.Int(1), eval.Int(2)},
					},
					Origins: map[string]diag.Span{},
					Modes:   map[string]string{},
				},
			},
		},
		diags:          &diag.Diagnostics{},
		names:          map[string]struct{}{},
		subsetNames:    map[subsetKey]subsetInfo{},
		stepSourceRows: map[string]map[string]string{},
	}

	got := ctx.resolveStepUses(
		"run",
		nil,
		[]sema.PlannedImport{
			{Source: "missing_full", Kind: sema.SourceKindParam, Full: true},
			{Source: "missing_partial", Kind: sema.SourceKindParam, Visible: "z", SourceVar: "z"},
			{Source: "p", Kind: sema.SourceKindParam, Visible: "x", SourceVar: ""},
			{Source: "p", Kind: sema.SourceKindParam, Visible: "x", SourceVar: ""},
		},
		nil,
	)

	if len(got.Use) != 1 {
		t.Fatalf("expected one subset use entry from valid partial import, got %#v", got.Use)
	}
	name, ok := got.Use[0].(string)
	if !ok || !strings.HasPrefix(name, "_js__") {
		t.Fatalf("expected synthetic subset name, got %#v", got.Use[0])
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected exactly one generated subset parameterset, got %#v", ctx.doc.ParameterSet)
	}
}

func TestStepAliasMapAndSubmitValueAliasMap(t *testing.T) {
	ctx := &lowerContext{
		res: &sema.Result{
			StepImportByName: map[string]*sema.StepImportPlan{
				"run": {
					Effective: map[string]sema.VarOrigin{
						"nodes": {Kind: sema.SourceKindParam},
						"queue": {Kind: sema.SourceKindLet},
						"foo":   {Kind: sema.SourceKindParam},
						"other": {Kind: sema.SourceKind("other")},
					},
				},
				"plain": {
					Effective: map[string]sema.VarOrigin{
						"nodes": {Kind: sema.SourceKindParam},
						"queue": {Kind: sema.SourceKindLet},
					},
				},
			},
			SubmitByName: map[string]*sema.SubmitSpec{
				"run": {
					Helpers: []sema.SubmitHelper{
						{Original: "helper", Aliased: "_jk__run_helper"},
						{Original: "", Aliased: "_ignored"},
						{Original: "ignored", Aliased: ""},
					},
				},
			},
		},
	}

	if got := ctx.stepAliasMap("run", false); len(got) != 0 {
		t.Fatalf("expected no aliases when forSubmit=false, got %#v", got)
	}
	if got := ctx.stepAliasMap("missing", true); len(got) != 0 {
		t.Fatalf("expected empty alias map for missing step plan, got %#v", got)
	}

	aliases := ctx.stepAliasMap("run", true)
	if aliases["nodes"] != "_ja__nodes" || aliases["queue"] != "_ja__queue" {
		t.Fatalf("expected submit-key aliases for nodes/queue, got %#v", aliases)
	}
	if _, ok := aliases["foo"]; ok {
		t.Fatalf("did not expect non-submit key alias for foo: %#v", aliases)
	}
	if _, ok := aliases["other"]; ok {
		t.Fatalf("did not expect non-param/let origin alias: %#v", aliases)
	}

	valueAliases := ctx.submitValueAliasMap("run")
	if valueAliases["helper"] != "_jk__run_helper" {
		t.Fatalf("expected helper alias merged into submit value aliases, got %#v", valueAliases)
	}
	if _, ok := valueAliases["ignored"]; ok {
		t.Fatalf("did not expect helper with empty alias to be included: %#v", valueAliases)
	}
	noHelperAliases := ctx.submitValueAliasMap("plain")
	if noHelperAliases["nodes"] != "_ja__nodes" || noHelperAliases["queue"] != "_ja__queue" {
		t.Fatalf("expected submit aliases without helper merge for missing submit spec, got %#v", noHelperAliases)
	}
}

func TestSourceNeedsAlias(t *testing.T) {
	src := &sema.ImportSource{
		Order: []string{"a", "b"},
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1)},
			"b": {eval.Int(2)},
		},
	}

	if sourceNeedsAlias(nil, map[string]string{"a": "_ja__a"}) {
		t.Fatalf("nil source should not need alias")
	}
	if sourceNeedsAlias(src, nil) {
		t.Fatalf("empty alias map should not require aliasing")
	}
	if sourceNeedsAlias(src, map[string]string{"x": "_ja__x"}) {
		t.Fatalf("non-overlapping aliases should not require aliasing")
	}
	if !sourceNeedsAlias(src, map[string]string{"b": "_ja__b"}) {
		t.Fatalf("overlapping alias must require aliasing")
	}
}

func TestInheritedRowsForStepAndStepSpan(t *testing.T) {
	targetSpan := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 5))
	dep1Span := diag.NewSpan("in.jbs", diag.NewPos(0, 2, 1), diag.NewPos(0, 2, 5))
	dep2Span := diag.NewSpan("in.jbs", diag.NewPos(0, 3, 1), diag.NewPos(0, 3, 5))
	submitSpan := diag.NewSpan("in.jbs", diag.NewPos(0, 4, 1), diag.NewPos(0, 4, 7))

	ctx := &lowerContext{
		res: &sema.Result{
			DoBlocks: []ast.DoBlock{
				{Name: "target", Span: targetSpan},
				{Name: "dep1", Span: dep1Span},
				{Name: "dep2", Span: dep2Span},
			},
			Submits: []ast.SubmitBlock{
				{Name: "sub", Span: submitSpan},
			},
		},
		diags: &diag.Diagnostics{},
		stepSourceRows: map[string]map[string]string{
			"dep1": {"p": "rows_a", "empty": ""},
			"dep2": {"p": "rows_b", "q": "rows_q"},
			"dep3": {},
			"dep4": {"p": "rows_c"},
		},
	}

	if got := ctx.stepSpan("target"); got != targetSpan {
		t.Fatalf("expected do step span for target, got %#v", got)
	}
	if got := ctx.stepSpan("sub"); got != submitSpan {
		t.Fatalf("expected submit step span for sub, got %#v", got)
	}
	if got := ctx.stepSpan("missing"); !got.IsZero() {
		t.Fatalf("expected zero span for missing step, got %#v", got)
	}

	rows := ctx.inheritedRowsForStep("target", []string{"dep3", "dep1", "dep2", "dep4"})
	if rows["q"] != "rows_q" {
		t.Fatalf("expected non-conflicting inherited rows for q, got %#v", rows)
	}
	if _, ok := rows["p"]; ok {
		t.Fatalf("expected conflicting source p to be removed, got %#v", rows)
	}

	e232 := 0
	for _, item := range ctx.diags.Items {
		if item.Code == string(diag.CodeE232) {
			e232++
		}
	}
	if e232 != 1 {
		t.Fatalf("expected exactly one E232 conflict diagnostic, got %d: %#v", e232, ctx.diags.Items)
	}
}
