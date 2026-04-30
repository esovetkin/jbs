package printparam

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func countBuildDiag(diags *diag.Diagnostics, code diag.Code) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == string(code) {
			count++
		}
	}
	return count
}

func TestFilterColumnsByUsage(t *testing.T) {
	candidates := []string{"p.a", "p.b", "p.a"}
	used := map[string]struct{}{"p.b": {}, "q.c": {}}
	got := filterColumnsByUsage(candidates, used)
	want := []string{"p.b", "q.c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterColumnsByUsage mismatch: got=%#v want=%#v", got, want)
	}
}

func TestPruneHeaderOnlyColumns(t *testing.T) {
	cols := []string{"p.a", "p.b", "p.c"}
	rows := []Row{{Values: map[string]string{"p.a": "1"}}, {Values: map[string]string{"p.b": ""}}}
	got := pruneHeaderOnlyColumns(cols, rows)
	want := []string{"p.a", "p.b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pruneHeaderOnlyColumns mismatch: got=%#v want=%#v", got, want)
	}
}

func TestCollectQualifiedColumns(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"a": {Name: "mod", Order: []string{"x", "y"}, Vars: map[string][]eval.Value{"x": {eval.Int(1)}, "y": {eval.Int(2)}}},
		"b": {Name: "mod", Order: []string{"x"}, Vars: map[string][]eval.Value{"x": {eval.Int(3)}}},
		"c": nil,
		"d": {Name: "other", Vars: map[string][]eval.Value{"z": {eval.Int(4)}}},
	}
	got := collectQualifiedColumns(bindings)
	want := []string{"mod.x", "mod.y", "other.z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectQualifiedColumns mismatch: got=%#v want=%#v", got, want)
	}
}

func TestDisplayColumnKeyScalarIdentity(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"x": {
			Name:       "x",
			PublicName: "x",
			Shape:      sema.BindingScalar,
			Order:      []string{"x"},
			Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
		},
	}
	if got := displayColumnKey(bindings, "x", "x"); got != "x" {
		t.Fatalf("expected scalar identity column x, got %q", got)
	}
}

func TestDisplayColumnKeyNamespacedScalarIdentity(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"_js__1__x": {
			Name:       "_js__1__x",
			PublicName: "mod.x",
			Shape:      sema.BindingScalar,
			Order:      []string{"x"},
			Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
		},
	}
	if got := displayColumnKey(bindings, "_js__1__x", "x"); got != "mod.x" {
		t.Fatalf("expected namespaced scalar identity column mod.x, got %q", got)
	}
}

func TestDisplayColumnKeySingleColumnTableStaysQualified(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"cases": {
			Name:       "cases",
			PublicName: "cases",
			Shape:      sema.BindingTable,
			Order:      []string{"x"},
			Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
		},
	}
	if got := displayColumnKey(bindings, "cases", "x"); got != "cases.x" {
		t.Fatalf("expected qualified table column cases.x, got %q", got)
	}
}

func TestCollectStepsInResultOrderAndDeps(t *testing.T) {
	s0 := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	s1 := diag.NewSpan("in.jbs", diag.NewPos(1, 2, 1), diag.NewPos(2, 2, 2))
	s2 := diag.NewSpan("in.jbs", diag.NewPos(2, 3, 1), diag.NewPos(3, 3, 2))
	res := &sema.Result{
		StepOrder: []string{"step0", "step1", "step2", "missing"},
		DoBlocks: []ast.DoBlock{
			{Name: "step0", Span: s0},
			{Name: "step2", After: []string{"step0", "step1"}, Span: s2},
		},
		Submits: []ast.SubmitBlock{{Name: "step1", After: []string{"step0"}, Span: s1}},
	}
	got := collectStepsInResultOrder(res)
	if len(got) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(got))
	}
	if got[0].Name != "step0" || got[0].Kind != "do" || got[1].Kind != "submit" || got[2].Name != "step2" {
		t.Fatalf("unexpected collected step order: %#v", got)
	}
	deps := stepDeps(map[string]stepDef{"step0": got[0], "step1": got[1], "step2": got[2]})
	wantDeps := map[string][]string{"step0": nil, "step1": {"step0"}, "step2": {"step0", "step1"}}
	if !reflect.DeepEqual(deps, wantDeps) {
		t.Fatalf("unexpected step dependency map: got=%#v want=%#v", deps, wantDeps)
	}
}

func TestGroupExplicitDeltaBySource(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	sources := map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
				"b": {eval.Int(2)},
			},
		},
		"q": {
			Name: "q",
			Vars: map[string][]eval.Value{
				"x": {eval.String("x")},
			},
		},
	}
	plan := &sema.StepScopePlan{
		ExplicitDelta: []sema.ScopeImport{
			{Source: "", Visible: "skip", Span: span},
			{Source: "p", Full: true, Span: span},
			{Source: "p", Visible: "a", SourceVar: "a", Span: span},
			{Source: "q", Visible: "", SourceVar: "x", Span: span},
			{Source: "q", Visible: "ren", SourceVar: "", Span: span},
		},
	}

	got := groupExplicitDeltaBySource(plan, sources)
	if len(got) != 2 {
		t.Fatalf("expected two source groups, got %#v", got)
	}
	if got[0].Source != "p" || !got[0].Full {
		t.Fatalf("unexpected first group: %#v", got[0])
	}
	if len(got[0].Vars) != 2 || got[0].Vars[0].Visible != "a" || got[0].Vars[1].Visible != "b" {
		t.Fatalf("expected full-group vars in source order, got %#v", got[0].Vars)
	}
	if got[1].Source != "q" || got[1].Full {
		t.Fatalf("unexpected second group: %#v", got[1])
	}
	if len(got[1].Vars) != 2 || got[1].Vars[0].Visible != "x" || got[1].Vars[0].SourceVar != "x" || got[1].Vars[1].Visible != "ren" || got[1].Vars[1].SourceVar != "ren" {
		t.Fatalf("unexpected second-group vars: %#v", got[1].Vars)
	}
}

func TestStateCloneHelpers(t *testing.T) {
	empty := emptyState()
	if len(empty.Values) != 0 || len(empty.SourceRows) != 0 {
		t.Fatalf("expected emptyState to initialize empty maps, got %#v", empty)
	}

	state := wpState{
		Values:     map[string]eval.Value{"a": eval.Int(1)},
		SourceRows: map[sema.BindingVersionKey][]int{{Public: "p", Version: "p:v1"}: {0, 1}},
	}
	pKey := sema.BindingVersionKey{Public: "p", Version: "p:v1"}
	cloned := cloneState(state)
	if !reflect.DeepEqual(cloned, state) {
		t.Fatalf("cloneState mismatch: got=%#v want=%#v", cloned, state)
	}
	cloned.Values["a"] = eval.Int(2)
	cloned.SourceRows[pKey][0] = 9
	if state.Values["a"].I != 1 || state.SourceRows[pKey][0] != 0 {
		t.Fatalf("cloneState must deep copy maps and row slices, state=%#v clone=%#v", state, cloned)
	}

	sliceClone := cloneStateSlice([]wpState{state})
	if len(sliceClone) != 1 || !reflect.DeepEqual(sliceClone[0], state) {
		t.Fatalf("cloneStateSlice mismatch: got=%#v want %#v", sliceClone, state)
	}
	sliceClone[0].SourceRows[pKey][1] = 7
	if state.SourceRows[pKey][1] != 1 {
		t.Fatalf("cloneStateSlice must deep copy nested row slices, state=%#v clone=%#v", state, sliceClone)
	}
}

func TestInheritParentStates(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}

	got := inheritParentStates(nil, nil, span, diags)
	if len(got) != 1 || len(got[0].Values) != 0 || len(got[0].SourceRows) != 0 {
		t.Fatalf("expected a single empty state for no dependencies, got %#v", got)
	}

	byStep := map[string][]wpState{
		"s0": {
			{Values: map[string]eval.Value{"a": eval.Int(1)}, SourceRows: map[sema.BindingVersionKey][]int{{Public: "p", Version: "p:v1"}: {0}}},
			{Values: map[string]eval.Value{"a": eval.Int(2)}, SourceRows: map[sema.BindingVersionKey][]int{{Public: "p", Version: "p:v1"}: {1}}},
		},
		"s1": {
			{Values: map[string]eval.Value{"b": eval.String("x")}, SourceRows: map[sema.BindingVersionKey][]int{{Public: "q", Version: "q:v1"}: {0}}},
		},
	}
	got = inheritParentStates([]string{"s0", "s0", "s1"}, byStep, span, diags)
	if len(got) != 2 {
		t.Fatalf("expected deduped parent dependencies to produce two states, got %#v", got)
	}
	if got[0].Values["a"].I != 1 || got[0].Values["b"].S != "x" {
		t.Fatalf("unexpected first inherited state: %#v", got[0])
	}
	if got[1].Values["a"].I != 2 || got[1].Values["b"].S != "x" {
		t.Fatalf("unexpected second inherited state: %#v", got[1])
	}

	if got := inheritParentStates([]string{"missing"}, byStep, span, diags); got != nil {
		t.Fatalf("expected nil when a dependency has no states, got %#v", got)
	}
}

func TestValueKeyAndUniqueStrings(t *testing.T) {
	values := []eval.Value{
		eval.Null(),
		eval.Int(7),
		eval.Float(1.5),
		eval.String("abc"),
		eval.Bool(true),
		eval.List([]eval.Value{eval.Int(1), eval.String("x")}),
		eval.Tuple([]eval.Value{eval.Bool(false), eval.Float(2)}),
		{Kind: "weird"},
	}
	for _, value := range values {
		if got := valueKey(value); got == "" {
			t.Fatalf("valueKey must not be empty for %#v", value)
		}
	}
	if got := uniqueStrings([]string{"a", "b", "a", "c", "b"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("uniqueStrings did not preserve first appearance order: %#v", got)
	}
}
