package lower

import (
	"reflect"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func newStepUseContext(res *sema.Result) *lowerContext {
	if res == nil {
		res = &sema.Result{}
	}
	if res.BindingsByName == nil {
		res.BindingsByName = map[string]*sema.GlobalBinding{}
	}
	if res.StepScopeByName == nil {
		res.StepScopeByName = map[string]*sema.StepScopePlan{}
	}
	if res.SubmitByName == nil {
		res.SubmitByName = map[string]*sema.SubmitSpec{}
	}
	return &lowerContext{
		res:                       res,
		diags:                     &diag.Diagnostics{},
		names:                     map[string]struct{}{},
		sourceParameterSetEmitted: map[string]struct{}{},
		subsetNames:               map[subsetKey]subsetInfo{},
		stepSourceRows:            map[string]map[sourceRowKey]sourceRowContext{},
	}
}

func TestResolveStepUsesForStepNoPlan(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{})
	got := ctx.resolveStepUsesForStep("run", nil)
	if len(got.Use) != 0 {
		t.Fatalf("expected no use entries without a step plan, got %#v", got.Use)
	}
	if len(got.SourceRows) != 0 {
		t.Fatalf("expected no source row mapping without a step plan, got %#v", got.SourceRows)
	}
}

func TestResolveStepUsesForStepFullSourceDirectAndAliasedSubset(t *testing.T) {
	binding := &sema.GlobalBinding{
		Name:  "p",
		Shape: sema.BindingTable,
		Order: []string{"x"},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1), eval.Int(2)},
		},
		Origins: map[string]diag.Span{},
		Modes:   map[string]string{},
	}
	plan := &sema.StepScopePlan{
		StepName:      "run",
		ExplicitDelta: []sema.ScopeImport{{Source: "p", Full: true}},
	}

	directCtx := newStepUseContext(&sema.Result{
		BindingsByName:  map[string]*sema.GlobalBinding{"p": binding},
		StepScopeByName: map[string]*sema.StepScopePlan{"run": plan},
	})
	direct := directCtx.resolveStepUsesForStep("run", map[string]string{})
	if len(direct.Use) != 1 || direct.Use[0] != "p" {
		t.Fatalf("expected direct use of full table source, got %#v", direct.Use)
	}
	if len(directCtx.doc.ParameterSet) != 1 || directCtx.doc.ParameterSet[0].Name != "p" {
		t.Fatalf("expected source parameter set to be emitted once, got %#v", directCtx.doc.ParameterSet)
	}

	aliasedCtx := newStepUseContext(&sema.Result{
		BindingsByName:  map[string]*sema.GlobalBinding{"p": binding},
		StepScopeByName: map[string]*sema.StepScopePlan{"run": plan},
	})
	aliased := aliasedCtx.resolveStepUsesForStep("run", map[string]string{"x": "_ja__x"})
	if len(aliased.Use) != 1 {
		t.Fatalf("expected aliased full import to lower as one subset, got %#v", aliased.Use)
	}
	name, ok := aliased.Use[0].(string)
	if !ok || !strings.HasPrefix(name, "_js__") {
		t.Fatalf("expected aliased full import to produce a synthetic subset, got %#v", aliased.Use[0])
	}
	if len(aliasedCtx.doc.ParameterSet) != 1 || aliasedCtx.doc.ParameterSet[0].Meta.Kind != ParameterSetKindSubset {
		t.Fatalf("expected one subset parameter set, got %#v", aliasedCtx.doc.ParameterSet)
	}
}

func TestResolveStepUsesForStepScalarBindingSubset(t *testing.T) {
	binding := &sema.GlobalBinding{
		Name:  "defaults",
		Shape: sema.BindingScalar,
		Order: []string{"queue"},
		Vars: map[string][]eval.Value{
			"queue": {eval.String("batch")},
		},
		Modes: map[string]string{"queue": "python"},
	}
	ctx := newStepUseContext(&sema.Result{
		BindingsByName: map[string]*sema.GlobalBinding{"defaults": binding},
		StepScopeByName: map[string]*sema.StepScopePlan{
			"run": {
				StepName:      "run",
				ExplicitDelta: []sema.ScopeImport{{Source: "defaults", Full: true}},
			},
		},
	})

	got := ctx.resolveStepUsesForStep("run", nil)
	if len(got.Use) != 1 {
		t.Fatalf("expected one scalar subset use entry, got %#v", got.Use)
	}
	if len(got.SourceRows) != 0 {
		t.Fatalf("scalar subset should not carry row state, got %#v", got.SourceRows)
	}
	if len(ctx.doc.ParameterSet) != 1 || ctx.doc.ParameterSet[0].Meta.Kind != ParameterSetKindSubset {
		t.Fatalf("expected one generated scalar subset parameterset, got %#v", ctx.doc.ParameterSet)
	}
	if value, ok := ctx.doc.ParameterSet[0].Parameter[0].Value.(SingleQuoted); !ok || string(value) != "batch" {
		t.Fatalf("expected scalar python subset value, got %#v", ctx.doc.ParameterSet[0].Parameter)
	}
}

func TestResolveStepUsesForStepRowVaryingScalarTracksRows(t *testing.T) {
	binding := &sema.GlobalBinding{
		Name:       "_js__1__x",
		PublicName: "x",
		VersionID:  "x:v1",
		Shape:      sema.BindingScalar,
		Order:      []string{"x"},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(0), eval.Int(1), eval.Int(2)},
		},
	}
	ctx := newStepUseContext(&sema.Result{
		BindingsByName: map[string]*sema.GlobalBinding{"_js__1__x": binding},
		StepScopeByName: map[string]*sema.StepScopePlan{
			"run": {
				StepName:      "run",
				ExplicitDelta: []sema.ScopeImport{{Source: "_js__1__x", Full: true}},
			},
		},
	})

	got := ctx.resolveStepUsesForStep("run", nil)
	if len(got.Use) != 1 {
		t.Fatalf("expected one indexed subset use entry, got %#v", got.Use)
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected one generated subset parameterset, got %#v", ctx.doc.ParameterSet)
	}
	ps := ctx.doc.ParameterSet[0]
	if len(ps.Parameter) < 3 {
		t.Fatalf("expected index, row-helper, and payload parameters, got %#v", ps.Parameter)
	}
	if ps.Parameter[0].Type != "int" || ps.Parameter[0].Value != "0,1,2" {
		t.Fatalf("expected index parameter for all scalar rows, got %#v", ps.Parameter[0])
	}
	if ps.Parameter[1].Separator != ReservedSeparator {
		t.Fatalf("expected row-helper parameter with reserved separator, got %#v", ps.Parameter[1])
	}
	if ps.Parameter[2].Name != "x" || ps.Parameter[2].Mode != "python" {
		t.Fatalf("expected python payload parameter for x, got %#v", ps.Parameter[2])
	}

	key := sourceRowKey{Public: "x", Version: "x:v1"}
	rowContext, ok := got.SourceRows[key]
	if !ok {
		t.Fatalf("expected version-aware row context for x:v1, got %#v", got.SourceRows)
	}
	if rowContext.VarName != ps.Parameter[1].Name || !reflect.DeepEqual(rowContext.Groups, []string{"0", "1", "2"}) {
		t.Fatalf("unexpected row context: %#v", rowContext)
	}
}

func TestResolveStepUsesDedupAndMissingSources(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{
		BindingsByName: map[string]*sema.GlobalBinding{
			"p": {
				Name:  "p",
				Shape: sema.BindingTable,
				Order: []string{"x"},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1), eval.Int(2)},
				},
			},
		},
	})

	got := ctx.resolveStepUses(
		"run",
		nil,
		[]sema.ScopeImport{
			{Source: "missing_full", Full: true},
			{Source: "missing_partial", Visible: "z", SourceVar: "z"},
			{Source: "p", Visible: "x"},
			{Source: "p", Visible: "x"},
		},
		nil,
	)
	if len(got.Use) != 1 {
		t.Fatalf("expected one subset use entry from the valid partial import, got %#v", got.Use)
	}
	name, ok := got.Use[0].(string)
	if !ok || !strings.HasPrefix(name, "_js__") {
		t.Fatalf("expected a synthetic subset name, got %#v", got.Use[0])
	}
	if len(ctx.doc.ParameterSet) != 1 {
		t.Fatalf("expected exactly one generated subset parameter set, got %#v", ctx.doc.ParameterSet)
	}
}

func TestStepAliasMaps(t *testing.T) {
	ctx := newStepUseContext(&sema.Result{
		StepScopeByName: map[string]*sema.StepScopePlan{
			"run": {
				Effective: map[string]sema.VisibleBinding{
					"nodes": {Source: "jobs"},
					"queue": {Source: "defaults"},
					"foo":   {Source: "jobs"},
					"bare":  {},
				},
			},
			"plain": {
				Effective: map[string]sema.VisibleBinding{
					"nodes": {Source: "jobs"},
					"queue": {Source: "defaults"},
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
	})

	if got := ctx.stepAliasMap("run", false); len(got) != 0 {
		t.Fatalf("expected no aliases when forSubmit=false, got %#v", got)
	}
	if got := ctx.stepAliasMap("missing", true); len(got) != 0 {
		t.Fatalf("expected no aliases for missing step plan, got %#v", got)
	}

	aliases := ctx.stepAliasMap("run", true)
	if aliases["nodes"] != "_ja__nodes" || aliases["queue"] != "_ja__queue" {
		t.Fatalf("expected submit-key aliases for nodes and queue, got %#v", aliases)
	}
	if _, ok := aliases["foo"]; ok {
		t.Fatalf("did not expect non-submit key alias for foo: %#v", aliases)
	}
	if _, ok := aliases["bare"]; ok {
		t.Fatalf("did not expect source-less binding to be aliased: %#v", aliases)
	}

	valueAliases := ctx.submitValueAliasMap("run")
	if valueAliases["helper"] != "_jk__run_helper" {
		t.Fatalf("expected helper alias merged into submit value aliases, got %#v", valueAliases)
	}
	if _, ok := valueAliases["ignored"]; ok {
		t.Fatalf("did not expect helper with empty alias to be included: %#v", valueAliases)
	}
	plainAliases := ctx.submitValueAliasMap("plain")
	if plainAliases["nodes"] != "_ja__nodes" || plainAliases["queue"] != "_ja__queue" {
		t.Fatalf("expected submit key aliases without helper merge, got %#v", plainAliases)
	}
}

func TestSourceNeedsAlias(t *testing.T) {
	src := &sema.GlobalBinding{
		Order: []string{"a", "b"},
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1)},
			"b": {eval.Int(2)},
		},
	}
	if sourceNeedsAlias(nil, map[string]string{"a": "_ja__a"}) {
		t.Fatalf("nil source must not need aliasing")
	}
	if sourceNeedsAlias(src, nil) {
		t.Fatalf("empty alias map must not require aliasing")
	}
	if !sourceNeedsAlias(src, map[string]string{"b": "_ja__b"}) {
		t.Fatalf("expected aliasing when one source var is remapped")
	}
	if sourceNeedsAlias(src, map[string]string{"z": "_ja__z"}) {
		t.Fatalf("did not expect aliasing when no source vars are remapped")
	}
}

func TestInheritedRowsForStepAndStepSpanFallback(t *testing.T) {
	depSpan1 := diag.NewSpan("step.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	depSpan2 := diag.NewSpan("step.jbs", diag.NewPos(2, 2, 1), diag.NewPos(3, 2, 2))
	runSpan := diag.NewSpan("step.jbs", diag.NewPos(4, 3, 1), diag.NewPos(5, 3, 2))
	ctx := newStepUseContext(&sema.Result{
		DoBlocks: []ast.DoBlock{{Name: "dep1", Span: depSpan1}, {Name: "run", Span: runSpan}},
		Submits:  []ast.SubmitBlock{{Name: "dep2", Span: depSpan2}},
	})
	sameKey := sourceRowKey{Public: "same", Version: "same-v1"}
	conflictKey := sourceRowKey{Public: "conflict", Version: "conflict-v1"}
	emptyKey := sourceRowKey{Public: "empty", Version: "empty-v1"}
	ctx.stepSourceRows = map[string]map[sourceRowKey]sourceRowContext{
		"dep1": {
			sameKey:     {VarName: "rows_same", Groups: []string{"0,1"}},
			conflictKey: {VarName: "rows_a", Groups: []string{"0,1"}},
		},
		"dep2": {
			sameKey:     {VarName: "rows_same", Groups: []string{"0,1"}},
			conflictKey: {VarName: "rows_b", Groups: []string{"0,1"}},
			emptyKey:    {},
		},
	}

	got := ctx.inheritedRowsForStep("run", []string{"dep1", "dep2"})
	want := map[sourceRowKey]sourceRowContext{sameKey: {VarName: "rows_same", Groups: []string{"0,1"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected inherited row map: got=%#v want=%#v", got, want)
	}
	if countLowerDiag(ctx.diags, diag.CodeE232) != 1 {
		t.Fatalf("expected one E232 for conflicting inherited rows, got %d: %s", countLowerDiag(ctx.diags, diag.CodeE232), ctx.diags.String())
	}
	if !strings.Contains(ctx.diags.String(), "source 'conflict'") {
		t.Fatalf("expected conflict diagnostic to use public source name, got: %s", ctx.diags.String())
	}
	if ctx.stepSpan("run") != runSpan || ctx.stepSpan("dep2") != depSpan2 {
		t.Fatalf("expected stepSpan to find do and submit block spans")
	}
	if !ctx.stepSpan("missing").IsZero() {
		t.Fatalf("expected missing step span to be zero")
	}
}

func TestCloneSourceRowContextMap(t *testing.T) {
	key := sourceRowKey{Public: "a", Version: "a-v1"}
	src := map[sourceRowKey]sourceRowContext{key: {VarName: "rows_a", Groups: []string{"0,1"}}}
	clone := cloneSourceRowContextMap(src)
	if !reflect.DeepEqual(clone, src) {
		t.Fatalf("expected cloneSourceRowContextMap to copy content, got %#v want %#v", clone, src)
	}
	clone[key] = sourceRowContext{VarName: "rows_b", Groups: []string{"2,3"}}
	clone[sourceRowKey{Public: "b", Version: "b-v1"}] = sourceRowContext{VarName: "rows_c", Groups: []string{"4,5"}}
	if src[key].VarName != "rows_a" || len(src) != 1 {
		t.Fatalf("expected cloneSourceRowContextMap to produce an independent map, src=%#v clone=%#v", src, clone)
	}
	if got := cloneSourceRowContextMap(nil); got != nil {
		t.Fatalf("expected nil clone for nil map, got %#v", got)
	}
	if !equalSourceRowContext(sourceRowContext{VarName: "rows", Groups: []string{"0,1"}}, sourceRowContext{VarName: "rows", Groups: []string{"0,1"}}) {
		t.Fatalf("expected equalSourceRowContext to match identical contexts")
	}
	if equalSourceRowContext(sourceRowContext{VarName: "rows", Groups: []string{"0,1"}}, sourceRowContext{VarName: "rows", Groups: []string{"0"}}) {
		t.Fatalf("expected equalSourceRowContext to reject mismatched groups")
	}
}
