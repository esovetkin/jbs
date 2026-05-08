package workplan

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestRequiredStepsForAnalysesIncludesTransitiveDeps(t *testing.T) {
	plan := Plan{Steps: []Step{
		{Name: "prep"},
		{Name: "build", After: []string{"prep"}},
		{Name: "run", After: []string{"build"}},
		{Name: "unused"},
	}}
	keep, err := RequiredStepsForAnalyses(plan, []string{"run"})
	if err != nil {
		t.Fatalf("RequiredStepsForAnalyses failed: %v", err)
	}
	for _, name := range []string{"prep", "build", "run"} {
		if _, ok := keep[name]; !ok {
			t.Fatalf("missing kept step %q in %#v", name, keep)
		}
	}
	if _, ok := keep["unused"]; ok {
		t.Fatalf("unexpected unused step in %#v", keep)
	}
}

func TestRequiredStepsForAnalysesRejectsUnknownTarget(t *testing.T) {
	_, err := RequiredStepsForAnalyses(Plan{Steps: []Step{{Name: "run"}}}, []string{"missing"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestFilterRemovesUnrelatedStepsAndWork(t *testing.T) {
	plan := Plan{
		BenchmarkName: "bench",
		SourceHash:    "hash",
		GlobalNProc:   2,
		Steps: []Step{
			{Name: "prep"},
			{Name: "run", After: []string{"prep"}},
			{Name: "unused"},
		},
		Work: []WorkPackage{
			{ID: WorkID{Step: "prep"}, StepName: "prep", Values: map[string]eval.Value{"x": eval.Int(1)}},
			{ID: WorkID{Step: "run"}, StepName: "run", Deps: []WorkID{{Step: "prep"}}, Values: map[string]eval.Value{"x": eval.Int(1)}},
			{ID: WorkID{Step: "unused"}, StepName: "unused"},
		},
	}
	keep := map[string]struct{}{"prep": {}, "run": {}}
	got, err := Filter(plan, keep)
	if err != nil {
		t.Fatalf("Filter failed: %v", err)
	}
	if got.BenchmarkName != "bench" || got.SourceHash != "hash" || got.GlobalNProc != 2 {
		t.Fatalf("metadata not preserved: %#v", got)
	}
	if len(got.Steps) != 2 || got.Steps[0].Name != "prep" || got.Steps[1].Name != "run" {
		t.Fatalf("unexpected steps: %#v", got.Steps)
	}
	if len(got.Work) != 2 || got.Work[0].StepName != "prep" || got.Work[1].StepName != "run" {
		t.Fatalf("unexpected work: %#v", got.Work)
	}
}

func TestFilterRejectsRemovedDependency(t *testing.T) {
	plan := Plan{
		Steps: []Step{{Name: "prep"}, {Name: "run", After: []string{"prep"}}},
		Work:  []WorkPackage{{ID: WorkID{Step: "run"}, StepName: "run", Deps: []WorkID{{Step: "prep"}}}},
	}
	_, err := Filter(plan, map[string]struct{}{"run": {}})
	if err == nil {
		t.Fatalf("expected error")
	}
}
