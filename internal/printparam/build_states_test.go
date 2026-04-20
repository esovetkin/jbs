package printparam

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestInheritParentStatesConflicts(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}

	a := wpState{Values: map[string]eval.Value{"x": eval.Int(1)}, SourceRows: map[string][]int{"p": {0}}}
	b := wpState{Values: map[string]eval.Value{"x": eval.Int(2)}, SourceRows: map[string][]int{"q": {1}}}
	_, ok := mergeParentStates(a, b, span, diags)
	if ok {
		t.Fatalf("expected value conflict merge to fail")
	}
	if countBuildDiag(diags, diag.CodeE500) != 1 {
		t.Fatalf("expected one E500, got %d: %s", countBuildDiag(diags, diag.CodeE500), diags.String())
	}

	diags = &diag.Diagnostics{}
	a = wpState{Values: map[string]eval.Value{"x": eval.Int(1)}, SourceRows: map[string][]int{"p": {0}}}
	b = wpState{Values: map[string]eval.Value{"y": eval.Int(2)}, SourceRows: map[string][]int{"p": {1}}}
	_, ok = mergeParentStates(a, b, span, diags)
	if ok {
		t.Fatalf("expected source-row conflict merge to fail")
	}
	if countBuildDiag(diags, diag.CodeE501) != 1 {
		t.Fatalf("expected one E501, got %d: %s", countBuildDiag(diags, diag.CodeE501), diags.String())
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

	if got := buildChoices(emptyState(), sourceGroup{Source: "missing"}, sources); got != nil {
		t.Fatalf("expected nil choices for missing source, got %#v", got)
	}

	state := emptyState()
	state.SourceRows["p"] = []int{1, 5}
	choices := buildChoices(state, sourceGroup{Source: "p", Vars: []sourceVar{{Visible: "a", SourceVar: "a"}}}, sources)
	if len(choices) != 1 {
		t.Fatalf("expected invalid row indices to be skipped, got %#v", choices)
	}
	if choices[0].Rows[0] != 1 || choices[0].Values["a"].I != 1 {
		t.Fatalf("unexpected constrained choice: %#v", choices[0])
	}

	choices = buildChoices(emptyState(), sourceGroup{Source: "p", Full: true}, sources)
	if len(choices) != 3 {
		t.Fatalf("expected full-import choices per row, got %#v", choices)
	}
	if choices[2].Values["a"].I != 2 || choices[2].Values["b"].S != "y" {
		t.Fatalf("unexpected full-import row values: %#v", choices[2].Values)
	}

	choices = buildChoices(emptyState(), sourceGroup{Source: "p", Vars: []sourceVar{{Visible: "a", SourceVar: "a"}}}, sources)
	if len(choices) != 2 {
		t.Fatalf("expected grouped choices for a=[1,1,2], got %#v", choices)
	}
	if !reflect.DeepEqual(choices[0].Rows, []int{0, 1}) || choices[0].Values["a"].I != 1 {
		t.Fatalf("unexpected first grouped choice: %#v", choices[0])
	}
	if !reflect.DeepEqual(choices[1].Rows, []int{2}) || choices[1].Values["a"].I != 2 {
		t.Fatalf("unexpected second grouped choice: %#v", choices[1])
	}

	choices = buildChoices(emptyState(), sourceGroup{Source: "empty", Full: true}, sources)
	if len(choices) != 1 {
		t.Fatalf("expected rowCount fallback of 1 for empty source, got %#v", choices)
	}
}

func TestBuildChoicesRegroupsInheritedProjection(t *testing.T) {
	sources := map[string]*sema.GlobalBinding{"p0": hiddenProjectionBinding()}
	state := emptyState()
	state.SourceRows["p0"] = []int{0, 1, 12, 13}

	choices := buildChoices(state, sourceGroup{
		Source: "p0",
		Vars: []sourceVar{
			{Visible: "b", SourceVar: "b"},
			{Visible: "c", SourceVar: "c"},
		},
	}, sources)
	want := []sourceChoice{
		{
			Rows: []int{0, 1},
			Values: map[string]eval.Value{
				"b": eval.String("a"),
				"c": eval.String("x"),
			},
		},
		{
			Rows: []int{12, 13},
			Values: map[string]eval.Value{
				"b": eval.String("a"),
				"c": eval.String("z"),
			},
		},
	}
	if !reflect.DeepEqual(choices, want) {
		t.Fatalf("expected inherited regrouping by projected values, got %#v want %#v", choices, want)
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

	if got := expandStep(nil, nil, sources, span, diags); got != nil {
		t.Fatalf("expected nil expansion for empty parent states, got %#v", got)
	}

	parent := wpState{Values: map[string]eval.Value{"a": eval.Int(1)}, SourceRows: map[string][]int{}}
	groups := []sourceGroup{{Source: "p", Vars: []sourceVar{{Visible: "a", SourceVar: "a"}}}}
	got := expandStep([]wpState{parent}, groups, sources, span, diags)
	if len(got) != 1 {
		t.Fatalf("expected one expanded state after conflict filtering, got %#v", got)
	}
	if got[0].Values["a"].I != 1 {
		t.Fatalf("unexpected remaining state value: %#v", got[0].Values)
	}
	if countBuildDiag(diags, diag.CodeE502) != 1 {
		t.Fatalf("expected one E502 from conflicting choice, got %d: %s", countBuildDiag(diags, diag.CodeE502), diags.String())
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
	if countBuildDiag(diags, diag.CodeE502) != 1 {
		t.Fatalf("expected one E502 from mergeWithChoice conflict, got %d: %s", countBuildDiag(diags, diag.CodeE502), diags.String())
	}
}

func TestBuildEndToEnd(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &sema.Result{
		Globals: sema.GlobalState{
			Values: map[string]eval.Value{
				"helper_fn": eval.Function(&eval.FunctionValue{}),
			},
		},
		GlobalVarByName: map[string]*sema.GlobalVar{
			"helper_fn": {
				Name:  "helper_fn",
				Value: eval.Function(&eval.FunctionValue{}),
				Span:  span,
			},
		},
		GlobalVarOrder: []string{"helper_fn"},
		StepOrder:      []string{"s0", "s1", "s2"},
		DoBlocks:       []ast.DoBlock{{Name: "s0", Span: span}, {Name: "s1", After: []string{"s0"}, Span: span}},
		Submits:        []ast.SubmitBlock{{Name: "s2", After: []string{"s1"}, Span: span}},
		BindingsByName: map[string]*sema.GlobalBinding{
			"p": {
				Name:  "p",
				Shape: sema.BindingTable,
				Order: []string{"x", "y"},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1), eval.Int(2)},
					"y": {eval.String("a"), eval.String("b")},
				},
			},
			"q": {
				Name:  "q",
				Shape: sema.BindingTable,
				Order: []string{"z"},
				Vars: map[string][]eval.Value{
					"z": {eval.Int(9), eval.Int(9)},
				},
			},
		},
		StepScopeByName: map[string]*sema.StepScopePlan{
			"s0": {
				StepName: "s0",
				ExplicitDelta: []sema.ScopeImport{
					{Source: "p", Visible: "x", SourceVar: "x", Span: span},
					{Source: "p", Visible: "yy", SourceVar: "y", Span: span},
				},
				Effective: map[string]sema.VisibleBinding{
					"x":  {Name: "x", Source: "p", SourceVar: "x", Span: span},
					"yy": {Name: "yy", Source: "p", SourceVar: "y", Span: span},
				},
			},
			"s1": {
				StepName:      "s1",
				ExplicitDelta: []sema.ScopeImport{{Source: "q", Visible: "z", SourceVar: "z", Span: span}},
				Effective: map[string]sema.VisibleBinding{
					"x":  {Name: "x", Source: "p", SourceVar: "x", Span: span},
					"yy": {Name: "yy", Source: "p", SourceVar: "y", Span: span},
					"z":  {Name: "z", Source: "q", SourceVar: "z", Span: span},
				},
			},
			// s2 deliberately omitted to cover the nil-plan path in Build.
		},
	}

	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	wantCols := []string{"p.x", "p.y", "q.z"}
	if !reflect.DeepEqual(table.Columns, wantCols) {
		t.Fatalf("unexpected columns: got=%#v want=%#v", table.Columns, wantCols)
	}
	for _, col := range table.Columns {
		if col == "helper_fn.helper_fn" || col == "helper_fn" {
			t.Fatalf("did not expect function-valued globals to appear in printparam columns: %#v", table.Columns)
		}
	}
	if len(table.Rows) != 6 {
		t.Fatalf("expected six rows, got %d: %#v", len(table.Rows), table.Rows)
	}
	if table.Rows[0].StepKind != "do" || table.Rows[0].StepName != "s0" {
		t.Fatalf("unexpected first row step label: %#v", table.Rows[0])
	}
	if table.Rows[0].Values["p.x"] != "1" || table.Rows[0].Values["p.y"] != "a" {
		t.Fatalf("unexpected first row values: %#v", table.Rows[0].Values)
	}
	if table.Rows[1].Values["p.x"] != "2" || table.Rows[1].Values["p.y"] != "b" {
		t.Fatalf("unexpected second row values: %#v", table.Rows[1].Values)
	}
	if table.Rows[2].StepName != "s1" || table.Rows[2].Values["q.z"] != "9" {
		t.Fatalf("unexpected third row values: %#v", table.Rows[2])
	}
	if table.Rows[4].StepKind != "submit" || table.Rows[4].StepName != "s2" {
		t.Fatalf("unexpected submit row: %#v", table.Rows[4])
	}
	if len(table.Rows[4].Values) != 0 || len(table.Rows[5].Values) != 0 {
		t.Fatalf("expected empty values for nil-plan submit rows, got %#v %#v", table.Rows[4].Values, table.Rows[5].Values)
	}
}

func TestBuildEndToEndRegroupsHiddenDimensions(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &sema.Result{
		StepOrder: []string{"step0", "step1"},
		DoBlocks: []ast.DoBlock{
			{Name: "step0", Span: span},
			{Name: "step1", After: []string{"step0"}, Span: span},
		},
		BindingsByName: map[string]*sema.GlobalBinding{
			"p0": hiddenProjectionBinding(),
		},
		StepScopeByName: map[string]*sema.StepScopePlan{
			"step0": {
				StepName: "step0",
				ExplicitDelta: []sema.ScopeImport{
					{Source: "p0", Visible: "a", SourceVar: "a", Span: span},
				},
				Effective: map[string]sema.VisibleBinding{
					"a": {Name: "a", Source: "p0", SourceVar: "a", Span: span},
				},
			},
			"step1": {
				StepName:       "step1",
				InheritedSteps: []string{"step0"},
				Inherited: map[string]sema.VisibleBinding{
					"a": {Name: "a", Source: "p0", SourceVar: "a", ViaStep: "step0", Span: span},
				},
				ExplicitDelta: []sema.ScopeImport{
					{Source: "p0", Visible: "b", SourceVar: "b", Span: span},
					{Source: "p0", Visible: "c", SourceVar: "c", Span: span},
				},
				Effective: map[string]sema.VisibleBinding{
					"a": {Name: "a", Source: "p0", SourceVar: "a", ViaStep: "step0", Span: span},
					"b": {Name: "b", Source: "p0", SourceVar: "b", Span: span},
					"c": {Name: "c", Source: "p0", SourceVar: "c", Span: span},
				},
			},
		},
	}

	diags := &diag.Diagnostics{}
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got := countRowsForStep(table.Rows, "step0"); got != 6 {
		t.Fatalf("expected 6 step0 rows, got %d", got)
	}
	if got := countRowsForStep(table.Rows, "step1"); got != 12 {
		t.Fatalf("expected 12 step1 rows, got %d", got)
	}
	if got := countVisibleTuple(table.Rows, "step1", "0", "a", "x"); got != 1 {
		t.Fatalf("expected one visible tuple for step1 (0,a,x), got %d", got)
	}
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

func countRowsForStep(rows []Row, stepName string) int {
	count := 0
	for _, row := range rows {
		if row.StepName == stepName {
			count++
		}
	}
	return count
}

func countVisibleTuple(rows []Row, stepName, a, b, c string) int {
	count := 0
	for _, row := range rows {
		if row.StepName != stepName {
			continue
		}
		if row.Values["p0.a"] == a && row.Values["p0.b"] == b && row.Values["p0.c"] == c {
			count++
		}
	}
	return count
}
