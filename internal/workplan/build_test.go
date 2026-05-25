package workplan

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestBuildPreservesImmediateDependenciesOnly(t *testing.T) {
	src := `
cases = table(x=[1, 2])

do step1 with cases {
echo "$x"
}

do step2 after step1 {
echo step2
}

do step3 after step2 {
echo step3
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("x.jbs", src, diags)
	res := sema.Analyze(prog, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	plan := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected workplan diagnostics: %s", diags.String())
	}
	var step3 []WorkPackage
	for _, work := range plan.Work {
		if work.StepName == "step3" {
			step3 = append(step3, work)
		}
	}
	if len(step3) != 2 {
		t.Fatalf("expected two step3 workpackages, got %d", len(step3))
	}
	for _, work := range step3 {
		if len(work.Deps) != 1 {
			t.Fatalf("expected one direct dependency for %#v, got %#v", work.ID, work.Deps)
		}
		if work.Deps[0].Step != "step2" {
			t.Fatalf("expected step3 to depend only on step2, got %#v", work.Deps)
		}
	}
}

func TestBuildWithAliasesUsesVisibleNamesOnly(t *testing.T) {
	src := `
x = [1, 2]
cases = table(long = ["a", "b"])

do first with x as y {
echo "$y"
}

do second after first with cases["long"] as short {
echo "$y $short"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("alias.jbs", src, diags)
	res := sema.Analyze(prog, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	plan := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected workplan diagnostics: %s", diags.String())
	}
	first := workByStep(plan.Work, "first")
	if len(first) != 2 {
		t.Fatalf("expected two first workpackages, got %#v", first)
	}
	for _, work := range first {
		if _, ok := work.Values["y"]; !ok {
			t.Fatalf("expected alias y in work values, got %#v", work.Values)
		}
		if _, ok := work.Values["x"]; ok {
			t.Fatalf("did not expect original x in work values, got %#v", work.Values)
		}
	}
	second := workByStep(plan.Work, "second")
	if len(second) != 4 {
		t.Fatalf("expected cartesian expansion for inherited y and short, got %#v", second)
	}
	for _, work := range second {
		if _, ok := work.Values["short"]; !ok {
			t.Fatalf("expected alias short in work values, got %#v", work.Values)
		}
		if _, ok := work.Values["long"]; ok {
			t.Fatalf("did not expect original long in work values, got %#v", work.Values)
		}
		if _, ok := work.Values["y"]; !ok {
			t.Fatalf("expected inherited alias y in work values, got %#v", work.Values)
		}
	}
}

func TestBuildWithRowIndexedTableImport(t *testing.T) {
	src := `
cases = t(x=[10,20,30], y=["a","b","c"])
subset = cases[[2,0]]

do run with subset {
echo "$x $y"
}
`
	plan := buildPlanFromSourceForTest(t, "row_indexed_subset.jbs", src)

	run := workByStep(plan.Work, "run")
	if len(run) != 2 {
		t.Fatalf("expected two run workpackages, got %#v", run)
	}
	wantX := []int64{30, 10}
	wantY := []string{"c", "a"}
	for i, work := range run {
		if work.Values["x"].I != wantX[i] || work.Values["y"].S != wantY[i] {
			t.Fatalf("run row %d values=%#v want x=%d y=%q", i, work.Values, wantX[i], wantY[i])
		}
	}
}

func TestBuildProjectedImportsUseProjectionIdentity(t *testing.T) {
	src := `
z = t(x=(1,2,3)*2) * t(y=sort(("a","b")*2)) * t(z=("x","y"))

do xonly with z["x"] {
echo "$x"
}

do yz with z["y", "z"] {
echo "$y $z"
}

do aliased with z["x"] as xx {
echo "$xx"
}
`
	plan := buildPlanFromSourceForTest(t, "projection_identity.jbs", src)

	xonly := workByStep(plan.Work, "xonly")
	if len(xonly) != 6 {
		t.Fatalf("expected 6 x-only workpackages, got %d", len(xonly))
	}
	wantX := []int64{1, 2, 3, 1, 2, 3}
	for i, work := range xonly {
		if work.Values["x"].I != wantX[i] {
			t.Fatalf("xonly row %d x=%#v want %d", i, work.Values["x"], wantX[i])
		}
		if rows := onlySourceRowsForTest(t, work); len(rows) != 8 {
			t.Fatalf("xonly row %d source rows=%#v want 8 rows", i, rows)
		}
	}

	yz := workByStep(plan.Work, "yz")
	if len(yz) != 8 {
		t.Fatalf("expected 8 y/z workpackages, got %d", len(yz))
	}
	for i, work := range yz {
		if rows := onlySourceRowsForTest(t, work); len(rows) != 6 {
			t.Fatalf("yz row %d source rows=%#v want 6 rows", i, rows)
		}
	}

	aliased := workByStep(plan.Work, "aliased")
	if len(aliased) != 6 {
		t.Fatalf("expected 6 aliased workpackages, got %d", len(aliased))
	}
	for _, work := range aliased {
		if _, ok := work.Values["xx"]; !ok {
			t.Fatalf("expected alias xx in work values, got %#v", work.Values)
		}
		if _, ok := work.Values["x"]; ok {
			t.Fatalf("did not expect original x in aliased work values, got %#v", work.Values)
		}
	}
}

func TestBuildDependentProjectedImportsUseInheritedSourceRows(t *testing.T) {
	src := `
z = t(x=(1,2,3)*2) * t(y=sort(("a","b")*2)) * t(z=("x","y"))

do s0 with z["x"] {
echo "$x"
}

do s1 after s0 with z["y"] {
echo "$x $y"
}
`
	plan := buildPlanFromSourceForTest(t, "dependent_projection_identity.jbs", src)

	s0 := workByStep(plan.Work, "s0")
	if len(s0) != 6 {
		t.Fatalf("expected 6 s0 workpackages, got %d", len(s0))
	}
	s1 := workByStep(plan.Work, "s1")
	if len(s1) != 24 {
		t.Fatalf("expected 24 s1 workpackages, got %d", len(s1))
	}
	for _, work := range s1 {
		if _, ok := work.Values["x"]; !ok {
			t.Fatalf("expected inherited x in s1 work values, got %#v", work.Values)
		}
		if _, ok := work.Values["y"]; !ok {
			t.Fatalf("expected projected y in s1 work values, got %#v", work.Values)
		}
		constraints := onlySourceConstraintsForTest(t, work)
		if len(constraints) != 2 {
			t.Fatalf("expected inherited and explicit source constraints, got %#v", constraints)
		}
		if len(constraints[0].Rows) != 8 || len(constraints[1].Rows) != 2 {
			t.Fatalf("unexpected dependent source constraints: %#v", constraints)
		}
	}
}

func buildPlanFromSourceForTest(t *testing.T, filename, src string) Plan {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse(filename, src, diags)
	res := sema.Analyze(prog, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	plan := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected workplan diagnostics: %s", diags.String())
	}
	return plan
}

func onlySourceRowsForTest(t *testing.T, work WorkPackage) []int {
	t.Helper()
	constraints := onlySourceConstraintsForTest(t, work)
	if len(constraints) != 1 {
		t.Fatalf("expected one source constraint, got %#v", constraints)
	}
	return constraints[0].Rows
}

func onlySourceConstraintsForTest(t *testing.T, work WorkPackage) []SourceRowConstraint {
	t.Helper()
	if len(work.SourceRows) != 1 {
		t.Fatalf("expected one source in source rows, got %#v", work.SourceRows)
	}
	for _, constraints := range work.SourceRows {
		return constraints
	}
	return nil
}

func workByStep(work []WorkPackage, step string) []WorkPackage {
	out := make([]WorkPackage, 0)
	for _, item := range work {
		if item.StepName == step {
			out = append(out, item)
		}
	}
	return out
}

func TestCollectStepsInResultOrderAndDeps(t *testing.T) {
	s0 := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	s1 := diag.NewSpan("in.jbs", diag.NewPos(1, 2, 1), diag.NewPos(2, 2, 2))
	s2 := diag.NewSpan("in.jbs", diag.NewPos(2, 3, 1), diag.NewPos(3, 3, 2))
	nproc := 4
	res := &sema.Result{
		StepOrder: []string{"step0", "step1", "step2", "missing"},
		DoBlocks: []ast.DoBlock{
			{Name: "step0", Span: s0, Body: "echo step0"},
			{Name: "step1", After: []string{"step0"}, NProc: &nproc, Span: s1, Body: "echo step1"},
			{Name: "step2", After: []string{"step0", "step1"}, Span: s2, Body: "echo step2"},
		},
	}

	got := collectStepsInResultOrder(res)
	if len(got) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(got))
	}
	if got[0].Name != "step0" || got[0].Kind != "do" || got[1].NProc != 4 || got[1].Body != "echo step1" || got[2].Name != "step2" {
		t.Fatalf("unexpected collected step order: %#v", got)
	}
	deps := stepDeps(map[string]stepDef{"step0": got[0], "step1": got[1], "step2": got[2]})
	wantDeps := map[string][]string{"step0": nil, "step1": {"step0"}, "step2": {"step0", "step1"}}
	if !reflect.DeepEqual(deps, wantDeps) {
		t.Fatalf("unexpected step dependency map: got=%#v want=%#v", deps, wantDeps)
	}
}

func TestGroupExplicitDeltaByItem(t *testing.T) {
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
			{ItemID: 0, Source: "p", Full: true, Span: span},
			{ItemID: 0, Source: "p", Visible: "a", SourceVar: "a", Span: span},
			{ItemID: 1, Source: "q", Visible: "", SourceVar: "x", Span: span},
			{ItemID: 1, Source: "q", Visible: "ren", SourceVar: "", Span: span},
		},
	}

	got := groupExplicitDeltaByItem(plan, sources, nil)
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
	if len(empty.Values) != 0 || len(empty.SourceRows) != 0 || len(empty.Parents) != 0 {
		t.Fatalf("expected emptyState to initialize empty maps and no parents, got %#v", empty)
	}

	st := state{
		ID:         WorkID{Step: "s0", Row: 3},
		Values:     map[string]eval.Value{"a": eval.Int(1)},
		SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{{Public: "p", Version: "p:v1"}: sourceRows(0, 1)},
		Parents:    []WorkID{{Step: "parent", Row: 2}},
	}
	pKey := sema.BindingVersionKey{Public: "p", Version: "p:v1"}
	cloned := cloneState(st)
	if !reflect.DeepEqual(cloned, st) {
		t.Fatalf("cloneState mismatch: got=%#v want=%#v", cloned, st)
	}
	cloned.Values["a"] = eval.Int(2)
	cloned.SourceRows[pKey][0].Rows[0] = 9
	cloned.Parents[0].Row = 7
	if st.Values["a"].I != 1 || st.SourceRows[pKey][0].Rows[0] != 0 || st.Parents[0].Row != 2 {
		t.Fatalf("cloneState must deep copy maps, row slices, and parents, state=%#v clone=%#v", st, cloned)
	}

	sliceClone := cloneStateSlice([]state{st})
	if len(sliceClone) != 1 || !reflect.DeepEqual(sliceClone[0], st) {
		t.Fatalf("cloneStateSlice mismatch: got=%#v want %#v", sliceClone, st)
	}
	sliceClone[0].SourceRows[pKey][0].Rows[1] = 7
	if st.SourceRows[pKey][0].Rows[1] != 1 {
		t.Fatalf("cloneStateSlice must deep copy nested row slices, state=%#v clone=%#v", st, sliceClone)
	}
}

func TestInheritParentStates(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}

	got := inheritParentStates(nil, nil, span, diags)
	if len(got) != 1 || len(got[0].Values) != 0 || len(got[0].SourceRows) != 0 || len(got[0].Parents) != 0 {
		t.Fatalf("expected a single empty state for no dependencies, got %#v", got)
	}

	p := sema.BindingVersionKey{Public: "p", Version: "p:v1"}
	q := sema.BindingVersionKey{Public: "q", Version: "q:v1"}
	byStep := map[string][]state{
		"s0": {
			{ID: WorkID{Step: "s0", Row: 0}, Values: map[string]eval.Value{"a": eval.Int(1)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p: sourceRows(0)}},
			{ID: WorkID{Step: "s0", Row: 1}, Values: map[string]eval.Value{"a": eval.Int(2)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p: sourceRows(1)}},
		},
		"s1": {
			{ID: WorkID{Step: "s1", Row: 0}, Values: map[string]eval.Value{"b": eval.String("x")}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{q: sourceRows(0)}},
		},
	}
	got = inheritParentStates([]string{"s0", "s0", "s1"}, byStep, span, diags)
	if len(got) != 2 {
		t.Fatalf("expected deduped parent dependencies to produce two states, got %#v", got)
	}
	if got[0].Values["a"].I != 1 || got[0].Values["b"].S != "x" {
		t.Fatalf("unexpected first inherited state: %#v", got[0])
	}
	if !reflect.DeepEqual(got[0].Parents, []WorkID{{Step: "s0", Row: 0}, {Step: "s1", Row: 0}}) {
		t.Fatalf("unexpected first inherited parents: %#v", got[0].Parents)
	}
	if got[1].Values["a"].I != 2 || got[1].Values["b"].S != "x" {
		t.Fatalf("unexpected second inherited state: %#v", got[1])
	}
	if !reflect.DeepEqual(got[1].Parents, []WorkID{{Step: "s0", Row: 1}, {Step: "s1", Row: 0}}) {
		t.Fatalf("unexpected second inherited parents: %#v", got[1].Parents)
	}

	if got := inheritParentStates([]string{"missing"}, byStep, span, diags); got != nil {
		t.Fatalf("expected nil when a dependency has no states, got %#v", got)
	}
}

func TestInheritParentStatesConflicts(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	p := sema.BindingVersionKey{Public: "p", Version: "p:v1"}
	q := sema.BindingVersionKey{Public: "q", Version: "q:v1"}

	diags := &diag.Diagnostics{}
	a := state{Values: map[string]eval.Value{"x": eval.Int(1)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p: sourceRows(0)}}
	b := state{Values: map[string]eval.Value{"x": eval.Int(2)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{q: sourceRows(1)}}
	_, ok := mergeParentStates(a, b, span, diags)
	if ok {
		t.Fatalf("expected value conflict merge to fail")
	}
	if countWorkplanDiag(diags, diag.CodeE500) != 1 {
		t.Fatalf("expected one E500, got %d: %s", countWorkplanDiag(diags, diag.CodeE500), diags.String())
	}

	diags = &diag.Diagnostics{}
	a = state{Values: map[string]eval.Value{"x": eval.Int(1)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p: sourceRows(0)}}
	b = state{Values: map[string]eval.Value{"y": eval.Int(2)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p: sourceRows(1)}}
	merged, ok := mergeParentStates(a, b, span, diags)
	if !ok {
		t.Fatalf("expected same-source row constraints to merge: %s", diags.String())
	}
	if len(merged.SourceRows[p]) != 2 {
		t.Fatalf("expected both same-source row constraints to survive, got %#v", merged.SourceRows[p])
	}

	diags = &diag.Diagnostics{}
	p2 := sema.BindingVersionKey{Public: "p", Version: "p:v2"}
	a = state{Values: map[string]eval.Value{}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p: sourceRows(0)}}
	b = state{Values: map[string]eval.Value{}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{p2: sourceRows(1)}}
	merged, ok = mergeParentStates(a, b, span, diags)
	if !ok {
		t.Fatalf("same public source with different versions should not conflict: %s", diags.String())
	}
	if len(merged.SourceRows) != 2 {
		t.Fatalf("expected both row contexts to survive, got %#v", merged.SourceRows)
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
		eval.DictValue([]eval.DictEntry{{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a"}, Value: eval.Int(1)}}),
		{Kind: "weird"},
	}
	for _, value := range values {
		if got := eval.StableValueKey(value); got == "" {
			t.Fatalf("StableValueKey must not be empty for %#v", value)
		}
	}
	if got := uniqueStrings([]string{"a", "b", "a", "c", "b"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("uniqueStrings did not preserve first appearance order: %#v", got)
	}
}

func TestBuildChoicesBranches(t *testing.T) {
	sources := map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Shape: sema.BindingTable,
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1), eval.Int(1), eval.Int(2)},
				"b": {eval.String("x"), eval.String("x"), eval.String("y")},
			},
		},
		"empty": {Name: "empty", Shape: sema.BindingTable, Vars: map[string][]eval.Value{}},
	}

	if got := buildChoices(emptyState(), sourceGroup{Source: "missing"}, sources, nil); got != nil {
		t.Fatalf("expected nil choices for missing source, got %#v", got)
	}

	st := emptyState()
	st.SourceRows[sema.BindingVersionKeyForSource(sources, "p")] = inheritedSourceRows(1, 5)
	choices := buildChoices(st, sourceGroup{Source: "p", Vars: []sourceVar{{Visible: "a", SourceVar: "a"}}}, sources, nil)
	if len(choices) != 1 {
		t.Fatalf("expected invalid row indices to be skipped, got %#v", choices)
	}
	if choices[0].Rows[0] != 1 || choices[0].Values["a"].I != 1 {
		t.Fatalf("unexpected constrained choice: %#v", choices[0])
	}

	choices = buildChoices(emptyState(), sourceGroup{Source: "p", Full: true}, sources, nil)
	if len(choices) != 3 {
		t.Fatalf("expected full-import choices per row, got %#v", choices)
	}
	if choices[2].Values["a"].I != 2 || choices[2].Values["b"].S != "y" {
		t.Fatalf("unexpected full-import row values: %#v", choices[2].Values)
	}

	choices = buildChoices(emptyState(), sourceGroup{Source: "p", Vars: []sourceVar{{Visible: "a", SourceVar: "a"}}}, sources, nil)
	if len(choices) != 3 {
		t.Fatalf("expected fallback projection identity to keep each row, got %#v", choices)
	}
	if !reflect.DeepEqual(choices[0].Rows, []int{0}) || choices[0].Values["a"].I != 1 {
		t.Fatalf("unexpected first projected choice: %#v", choices[0])
	}
	if !reflect.DeepEqual(choices[1].Rows, []int{1}) || choices[1].Values["a"].I != 1 || !reflect.DeepEqual(choices[2].Rows, []int{2}) || choices[2].Values["a"].I != 2 {
		t.Fatalf("unexpected projected choices: %#v", choices)
	}

	choices = buildChoices(emptyState(), sourceGroup{Source: "empty", Full: true}, sources, nil)
	if len(choices) != 1 {
		t.Fatalf("expected rowCount fallback of 1 for empty source, got %#v", choices)
	}
}

func TestBuildChoicesKeepsFallbackProjectionRows(t *testing.T) {
	sources := map[string]*sema.GlobalBinding{"p0": hiddenProjectionBinding()}
	st := emptyState()
	st.SourceRows[sema.BindingVersionKeyForSource(sources, "p0")] = inheritedSourceRows(0, 1, 12, 13)

	choices := buildChoices(st, sourceGroup{
		Source: "p0",
		Vars: []sourceVar{
			{Visible: "b", SourceVar: "b"},
			{Visible: "c", SourceVar: "c"},
		},
	}, sources, nil)
	want := []sourceChoice{
		{Rows: []int{0}, Values: map[string]eval.Value{"b": eval.String("a"), "c": eval.String("x")}},
		{Rows: []int{1}, Values: map[string]eval.Value{"b": eval.String("a"), "c": eval.String("x")}},
		{Rows: []int{12}, Values: map[string]eval.Value{"b": eval.String("a"), "c": eval.String("z")}},
		{Rows: []int{13}, Values: map[string]eval.Value{"b": eval.String("a"), "c": eval.String("z")}},
	}
	if !reflect.DeepEqual(choices, want) {
		t.Fatalf("expected fallback projected rows, got %#v want %#v", choices, want)
	}
}

func TestExpandStepAndMergeWithChoiceConflict(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	sources := map[string]*sema.GlobalBinding{
		"p": {
			Name:  "p",
			Shape: sema.BindingTable,
			Vars:  map[string][]eval.Value{"a": {eval.Int(1), eval.Int(2)}},
		},
	}
	diags := &diag.Diagnostics{}

	if got := expandStep(nil, nil, sources, nil, span, diags); got != nil {
		t.Fatalf("expected nil expansion for empty parent states, got %#v", got)
	}

	parent := state{Values: map[string]eval.Value{"a": eval.Int(1)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{}}
	groups := []sourceGroup{{Source: "p", Vars: []sourceVar{{Visible: "a", SourceVar: "a"}}}}
	got := expandStep([]state{parent}, groups, sources, nil, span, diags)
	if len(got) != 1 {
		t.Fatalf("expected one expanded state after conflict filtering, got %#v", got)
	}
	if got[0].Values["a"].I != 1 {
		t.Fatalf("unexpected remaining state value: %#v", got[0].Values)
	}
	if countWorkplanDiag(diags, diag.CodeE502) != 1 {
		t.Fatalf("expected one E502 from conflicting choice, got %d: %s", countWorkplanDiag(diags, diag.CodeE502), diags.String())
	}

	diags = &diag.Diagnostics{}
	merged, ok := mergeWithChoice(
		state{Values: map[string]eval.Value{"x": eval.Int(1)}, SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{}},
		sourceGroup{Source: "p", DisplaySource: "p"},
		sourceChoice{Rows: []int{0}, Values: map[string]eval.Value{"x": eval.Int(2)}},
		span,
		diags,
	)
	if ok {
		t.Fatalf("expected mergeWithChoice conflict to fail, got %#v", merged)
	}
	if countWorkplanDiag(diags, diag.CodeE502) != 1 {
		t.Fatalf("expected one E502 from mergeWithChoice conflict, got %d: %s", countWorkplanDiag(diags, diag.CodeE502), diags.String())
	}
}

func countWorkplanDiag(diags *diag.Diagnostics, code diag.Code) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == string(code) {
			count++
		}
	}
	return count
}

func sourceRows(rows ...int) []SourceRowConstraint {
	return []SourceRowConstraint{{Rows: append([]int(nil), rows...)}}
}

func inheritedSourceRows(rows ...int) []SourceRowConstraint {
	return []SourceRowConstraint{{Rows: append([]int(nil), rows...), Inherited: true}}
}

func hiddenProjectionBinding() *sema.GlobalBinding {
	aVals := make([]eval.Value, 0, 24)
	bVals := make([]eval.Value, 0, 24)
	cVals := make([]eval.Value, 0, 24)
	dVals := make([]eval.Value, 0, 24)
	pairs := []struct {
		a int64
		b string
	}{
		{a: 0, b: "a"},
		{a: 1, b: "b"},
		{a: 2, b: "c"},
		{a: 3, b: "a"},
		{a: 4, b: "b"},
		{a: 5, b: "c"},
	}
	for _, c := range []string{"x", "z"} {
		for _, pair := range pairs {
			for _, d := range []bool{true, false} {
				aVals = append(aVals, eval.Int(pair.a))
				bVals = append(bVals, eval.String(pair.b))
				cVals = append(cVals, eval.String(c))
				dVals = append(dVals, eval.Bool(d))
			}
		}
	}
	return &sema.GlobalBinding{
		Name:  "p0",
		Shape: sema.BindingTable,
		Order: []string{"a", "b", "c", "d"},
		Vars: map[string][]eval.Value{
			"a": aVals,
			"b": bVals,
			"c": cVals,
			"d": dVals,
		},
	}
}
