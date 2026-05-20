package workplan

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestLimitBranchesKeepsTargetAndAncestors(t *testing.T) {
	plan := Plan{
		BenchmarkName: "bench",
		SourceHash:    "hash",
		GlobalNProc:   2,
		Steps: []Step{
			{Name: "prep"},
			{Name: "run", After: []string{"prep"}},
		},
		Work: []WorkPackage{
			{ID: WorkID{Step: "prep", Row: 0}, StepName: "prep"},
			{ID: WorkID{Step: "prep", Row: 1}, StepName: "prep"},
			{ID: WorkID{Step: "run", Row: 0}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 0}}},
			{ID: WorkID{Step: "run", Row: 1}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 1}}},
		},
	}
	got, err := LimitBranches(plan, LimitOptions{Limit: 1, TargetSteps: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.BenchmarkName != "bench" || got.SourceHash != "hash" || got.GlobalNProc != 2 {
		t.Fatalf("metadata not preserved: %#v", got)
	}
	if ids := workIDs(got.Work); !reflect.DeepEqual(ids, []WorkID{{Step: "prep", Row: 0}, {Step: "run", Row: 0}}) {
		t.Fatalf("limited work IDs = %#v", ids)
	}
	if !reflect.DeepEqual(got.Work[1].Deps, []WorkID{{Step: "prep", Row: 0}}) {
		t.Fatalf("target dependency not preserved: %#v", got.Work[1].Deps)
	}
}

func TestLimitBranchesKeepsTwoTargetsAndSharedAncestorOnce(t *testing.T) {
	plan := Plan{
		Steps: []Step{
			{Name: "prep"},
			{Name: "run", After: []string{"prep"}},
		},
		Work: []WorkPackage{
			{ID: WorkID{Step: "prep", Row: 0}, StepName: "prep"},
			{ID: WorkID{Step: "prep", Row: 1}, StepName: "prep"},
			{ID: WorkID{Step: "run", Row: 0}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 0}}},
			{ID: WorkID{Step: "run", Row: 1}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 0}}},
			{ID: WorkID{Step: "run", Row: 2}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 1}}},
		},
	}
	got, err := LimitBranches(plan, LimitOptions{Limit: 2, TargetSteps: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkID{
		{Step: "prep", Row: 0},
		{Step: "run", Row: 0},
		{Step: "run", Row: 1},
	}
	if ids := workIDs(got.Work); !reflect.DeepEqual(ids, want) {
		t.Fatalf("limited work IDs = %#v, want %#v", ids, want)
	}
}

func TestLimitBranchesLimitsEachTargetStep(t *testing.T) {
	plan := Plan{
		Steps: []Step{
			{Name: "prep"},
			{Name: "small", After: []string{"prep"}},
			{Name: "large", After: []string{"prep"}},
		},
		Work: []WorkPackage{
			{ID: WorkID{Step: "prep", Row: 0}, StepName: "prep"},
			{ID: WorkID{Step: "prep", Row: 1}, StepName: "prep"},
			{ID: WorkID{Step: "small", Row: 0}, StepName: "small", Deps: []WorkID{{Step: "prep", Row: 0}}},
			{ID: WorkID{Step: "small", Row: 1}, StepName: "small", Deps: []WorkID{{Step: "prep", Row: 1}}},
			{ID: WorkID{Step: "large", Row: 0}, StepName: "large", Deps: []WorkID{{Step: "prep", Row: 0}}},
			{ID: WorkID{Step: "large", Row: 1}, StepName: "large", Deps: []WorkID{{Step: "prep", Row: 1}}},
		},
	}
	got, err := LimitBranches(plan, LimitOptions{Limit: 1, TargetSteps: []string{"small", "large"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkID{
		{Step: "prep", Row: 0},
		{Step: "small", Row: 0},
		{Step: "large", Row: 0},
	}
	if ids := workIDs(got.Work); !reflect.DeepEqual(ids, want) {
		t.Fatalf("limited work IDs = %#v, want %#v", ids, want)
	}
}

func TestLimitBranchesUsesTerminalStepsWhenTargetsOmitted(t *testing.T) {
	plan := Plan{
		Steps: []Step{
			{Name: "prep"},
			{Name: "run", After: []string{"prep"}},
			{Name: "unused"},
		},
		Work: []WorkPackage{
			{ID: WorkID{Step: "prep", Row: 0}, StepName: "prep"},
			{ID: WorkID{Step: "prep", Row: 1}, StepName: "prep"},
			{ID: WorkID{Step: "run", Row: 0}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 0}}},
			{ID: WorkID{Step: "run", Row: 1}, StepName: "run", Deps: []WorkID{{Step: "prep", Row: 1}}},
			{ID: WorkID{Step: "unused", Row: 0}, StepName: "unused"},
			{ID: WorkID{Step: "unused", Row: 1}, StepName: "unused"},
		},
	}
	if got := TerminalSteps(plan); !reflect.DeepEqual(got, []string{"run", "unused"}) {
		t.Fatalf("terminal steps = %#v", got)
	}
	got, err := LimitBranches(plan, LimitOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkID{
		{Step: "prep", Row: 0},
		{Step: "run", Row: 0},
		{Step: "unused", Row: 0},
	}
	if ids := workIDs(got.Work); !reflect.DeepEqual(ids, want) {
		t.Fatalf("limited work IDs = %#v, want %#v", ids, want)
	}
}

func TestLimitBranchesDoesNotMutateInput(t *testing.T) {
	key := sema.BindingVersionKey{Public: "cases", Version: "cases:v1"}
	plan := Plan{
		Steps: []Step{{Name: "run"}},
		Work: []WorkPackage{{
			ID:         WorkID{Step: "run", Row: 0},
			StepName:   "run",
			Values:     map[string]eval.Value{"x": eval.Int(1)},
			SourceRows: map[sema.BindingVersionKey][]SourceRowConstraint{key: {{Rows: []int{0}}}},
		}},
	}
	got, err := LimitBranches(plan, LimitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got.Steps[0].Name = "changed"
	got.Work[0].Values["x"] = eval.Int(2)
	got.Work[0].SourceRows[key][0].Rows[0] = 9
	if plan.Steps[0].Name != "run" || plan.Work[0].Values["x"].I != 1 || plan.Work[0].SourceRows[key][0].Rows[0] != 0 {
		t.Fatalf("input plan was mutated: %#v", plan)
	}
}

func TestLimitBranchesRejectsMissingDependency(t *testing.T) {
	plan := Plan{
		Steps: []Step{{Name: "run"}},
		Work: []WorkPackage{{
			ID:       WorkID{Step: "run", Row: 0},
			StepName: "run",
			Deps:     []WorkID{{Step: "prep", Row: 0}},
		}},
	}
	if _, err := LimitBranches(plan, LimitOptions{Limit: 1, TargetSteps: []string{"run"}}); err == nil {
		t.Fatalf("expected missing dependency error")
	}
}

func TestLimitBranchesKeepsZeroWorkTargetStep(t *testing.T) {
	plan := Plan{
		Steps: []Step{{Name: "prep"}, {Name: "run", After: []string{"prep"}}},
	}
	got, err := LimitBranches(plan, LimitOptions{Limit: 1, TargetSteps: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	if ids := workIDs(got.Work); len(ids) != 0 {
		t.Fatalf("expected no work, got %#v", ids)
	}
	if names := stepNames(got.Steps); !reflect.DeepEqual(names, []string{"run"}) {
		t.Fatalf("expected target step only, got %#v", names)
	}
}

func workIDs(work []WorkPackage) []WorkID {
	out := make([]WorkID, 0, len(work))
	for _, item := range work {
		out = append(out, item.ID)
	}
	return out
}

func stepNames(steps []Step) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.Name)
	}
	return out
}
