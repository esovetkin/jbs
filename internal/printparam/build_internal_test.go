package printparam

import (
	"reflect"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func countDiag(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == code {
			count++
		}
	}
	return count
}

func TestFilterColumnsByUsage(t *testing.T) {
	candidates := []string{"p.a", "p.b", "p.a"}
	used := map[string]struct{}{
		"p.b": {},
		"q.c": {},
	}
	got := filterColumnsByUsage(candidates, used)
	want := []string{"p.b", "q.c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterColumnsByUsage mismatch: got=%#v want=%#v", got, want)
	}
}

func TestCollectQualifiedColumns(t *testing.T) {
	sources := map[string]*sema.ImportSource{
		"a": {
			Name:  "mod",
			Order: []string{"x", "y"},
			Vars: map[string][]eval.Value{
				"x": {eval.Int(1)},
				"y": {eval.Int(2)},
			},
		},
		"b": {
			Name:  "mod",
			Order: []string{"x"},
			Vars: map[string][]eval.Value{
				"x": {eval.Int(3)},
			},
		},
		"c": nil,
		"d": {
			Name: "other",
			Vars: map[string][]eval.Value{
				"z": {eval.Int(4)},
			},
		},
	}
	got := collectQualifiedColumns(sources)
	want := []string{"mod.x", "mod.y", "other.z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectQualifiedColumns mismatch: got=%#v want=%#v", got, want)
	}
}

func TestInheritParentStates(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}

	got := inheritParentStates(nil, nil, span, diags)
	if len(got) != 1 || len(got[0].Values) != 0 || len(got[0].SourceRows) != 0 {
		t.Fatalf("expected single empty state for no deps, got %#v", got)
	}

	byStep := map[string][]wpState{
		"s0": {
			{Values: map[string]eval.Value{"a": eval.Int(1)}, SourceRows: map[string][]int{"p": {0}}},
			{Values: map[string]eval.Value{"a": eval.Int(2)}, SourceRows: map[string][]int{"p": {1}}},
		},
		"s1": {
			{Values: map[string]eval.Value{"b": eval.String("x")}, SourceRows: map[string][]int{"q": {0}}},
		},
	}
	got = inheritParentStates([]string{"s0", "s0", "s1"}, byStep, span, diags)
	if len(got) != 2 {
		t.Fatalf("expected deduped parent deps to produce 2 states, got %d", len(got))
	}
	if got[0].Values["a"].I != 1 || got[0].Values["b"].S != "x" {
		t.Fatalf("unexpected first inherited state: %#v", got[0])
	}
	if got[1].Values["a"].I != 2 || got[1].Values["b"].S != "x" {
		t.Fatalf("unexpected second inherited state: %#v", got[1])
	}

	got = inheritParentStates([]string{"missing"}, byStep, span, diags)
	if got != nil {
		t.Fatalf("expected nil when dependency has no states, got %#v", got)
	}
}

func TestMergeParentStatesConflicts(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}

	a := wpState{
		Values:     map[string]eval.Value{"x": eval.Int(1)},
		SourceRows: map[string][]int{"p": {0}},
	}
	b := wpState{
		Values:     map[string]eval.Value{"x": eval.Int(2)},
		SourceRows: map[string][]int{"q": {1}},
	}
	_, ok := mergeParentStates(a, b, span, diags)
	if ok {
		t.Fatalf("expected value conflict merge to fail")
	}
	if countDiag(diags, "E500") != 1 {
		t.Fatalf("expected E500 once, got %d: %s", countDiag(diags, "E500"), diags.String())
	}

	diags = &diag.Diagnostics{}
	a = wpState{
		Values:     map[string]eval.Value{"x": eval.Int(1)},
		SourceRows: map[string][]int{"p": {0}},
	}
	b = wpState{
		Values:     map[string]eval.Value{"y": eval.Int(2)},
		SourceRows: map[string][]int{"p": {1}},
	}
	_, ok = mergeParentStates(a, b, span, diags)
	if ok {
		t.Fatalf("expected source-row conflict merge to fail")
	}
	if countDiag(diags, "E501") != 1 {
		t.Fatalf("expected E501 once, got %d: %s", countDiag(diags, "E501"), diags.String())
	}
}

func TestGroupExplicitDeltaBySource(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	sources := map[string]*sema.ImportSource{
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
	plan := &sema.StepImportPlan{
		ExplicitDelta: []sema.PlannedImport{
			{Source: "", Visible: "skip", Span: span},
			{Source: "p", Kind: sema.SourceKindParam, Full: true, Span: span},
			{Source: "p", Kind: sema.SourceKindParam, Visible: "a", SourceVar: "a", Span: span},
			{Source: "q", Kind: sema.SourceKindLet, Visible: "", SourceVar: "x", Span: span},
			{Source: "q", Kind: sema.SourceKindLet, Visible: "ren", SourceVar: "", Span: span},
		},
	}

	got := groupExplicitDeltaBySource(plan, sources)
	if len(got) != 2 {
		t.Fatalf("expected 2 source groups, got %d", len(got))
	}
	if got[0].Source != "p" || !got[0].Full {
		t.Fatalf("unexpected first group: %#v", got[0])
	}
	if len(got[0].Vars) != 2 || got[0].Vars[0].Visible != "a" || got[0].Vars[1].Visible != "b" {
		t.Fatalf("expected full group vars from source order, got %#v", got[0].Vars)
	}
	if got[1].Source != "q" || got[1].Full {
		t.Fatalf("unexpected second group: %#v", got[1])
	}
	if len(got[1].Vars) != 2 {
		t.Fatalf("expected two q vars entries, got %#v", got[1].Vars)
	}
	if got[1].Vars[0].Visible != "x" || got[1].Vars[0].SourceVar != "x" {
		t.Fatalf("unexpected first q var: %#v", got[1].Vars[0])
	}
	if got[1].Vars[1].Visible != "ren" || got[1].Vars[1].SourceVar != "ren" {
		t.Fatalf("expected empty SourceVar to fall back to visible name, got %#v", got[1].Vars[1])
	}
}

func TestBuildChoicesBranches(t *testing.T) {
	sources := map[string]*sema.ImportSource{
		"p": {
			Name:  "p",
			Order: []string{"a", "b"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1), eval.Int(1), eval.Int(2)},
				"b": {eval.String("x"), eval.String("x"), eval.String("y")},
			},
		},
		"empty": {
			Name: "empty",
			Vars: map[string][]eval.Value{},
		},
	}

	if got := buildChoices(emptyState(), sourceGroup{Source: "missing"}, sources); got != nil {
		t.Fatalf("expected nil choices for missing source, got %#v", got)
	}

	state := emptyState()
	state.SourceRows["p"] = []int{1, 5}
	choices := buildChoices(state, sourceGroup{
		Source: "p",
		Vars:   []sourceVar{{Visible: "a", SourceVar: "a"}},
	}, sources)
	if len(choices) != 1 {
		t.Fatalf("expected invalid row indices to be skipped, got %d choices", len(choices))
	}
	if choices[0].Rows[0] != 1 || choices[0].Values["a"].I != 1 {
		t.Fatalf("unexpected constrained choice: %#v", choices[0])
	}

	choices = buildChoices(emptyState(), sourceGroup{
		Source: "p",
		Full:   true,
	}, sources)
	if len(choices) != 3 {
		t.Fatalf("expected full import choices per row, got %d", len(choices))
	}
	if choices[2].Values["a"].I != 2 || choices[2].Values["b"].S != "y" {
		t.Fatalf("unexpected full import choice values: %#v", choices[2].Values)
	}

	choices = buildChoices(emptyState(), sourceGroup{
		Source: "p",
		Vars:   []sourceVar{{Visible: "a", SourceVar: "a"}},
	}, sources)
	if len(choices) != 2 {
		t.Fatalf("expected grouped choices for a=[1,1,2], got %d", len(choices))
	}
	if !reflect.DeepEqual(choices[0].Rows, []int{0, 1}) || choices[0].Values["a"].I != 1 {
		t.Fatalf("unexpected first grouped choice: %#v", choices[0])
	}
	if !reflect.DeepEqual(choices[1].Rows, []int{2}) || choices[1].Values["a"].I != 2 {
		t.Fatalf("unexpected second grouped choice: %#v", choices[1])
	}

	choices = buildChoices(emptyState(), sourceGroup{
		Source: "empty",
		Full:   true,
	}, sources)
	if len(choices) != 1 {
		t.Fatalf("expected fallback rowCount=1 for empty source, got %d", len(choices))
	}
}

func TestExpandStepAndMergeWithChoiceConflict(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	sources := map[string]*sema.ImportSource{
		"p": {
			Name: "p",
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1), eval.Int(2)},
			},
		},
	}
	diags := &diag.Diagnostics{}

	if got := expandStep(nil, nil, sources, span, diags); got != nil {
		t.Fatalf("expected nil expansion for empty parent states, got %#v", got)
	}

	parent := wpState{
		Values:     map[string]eval.Value{"a": eval.Int(1)},
		SourceRows: map[string][]int{},
	}
	groups := []sourceGroup{{
		Source: "p",
		Vars:   []sourceVar{{Visible: "a", SourceVar: "a"}},
	}}
	got := expandStep([]wpState{parent}, groups, sources, span, diags)
	if len(got) != 1 {
		t.Fatalf("expected one expanded state after conflict filtering, got %#v", got)
	}
	if got[0].Values["a"].I != 1 {
		t.Fatalf("unexpected remaining state value: %#v", got[0].Values)
	}
	if countDiag(diags, "E502") != 1 {
		t.Fatalf("expected one E502 conflict from second row choice, got %d: %s", countDiag(diags, "E502"), diags.String())
	}

	diags = &diag.Diagnostics{}
	merged, ok := mergeWithChoice(
		wpState{Values: map[string]eval.Value{"x": eval.Int(1)}, SourceRows: map[string][]int{}},
		"p",
		sourceChoice{Rows: []int{0}, Values: map[string]eval.Value{"x": eval.Int(2)}},
		span,
		diags,
	)
	if ok {
		t.Fatalf("expected mergeWithChoice conflict to fail, got %#v", merged)
	}
	if countDiag(diags, "E502") != 1 {
		t.Fatalf("expected E502 from mergeWithChoice conflict, got %d: %s", countDiag(diags, "E502"), diags.String())
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
	for _, v := range values {
		if got := valueKey(v); got == "" {
			t.Fatalf("valueKey must not be empty for %#v", v)
		}
	}
	if got := uniqueStrings([]string{"a", "b", "a", "c", "b"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("uniqueStrings did not preserve first-appearance order: %#v", got)
	}
}
