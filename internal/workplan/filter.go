package workplan

import (
	"fmt"
	"slices"
)

func RequiredStepsForAnalyses(plan Plan, analyseSteps []string) (map[string]struct{}, error) {
	deps := make(map[string][]string, len(plan.Steps))
	for _, step := range plan.Steps {
		deps[step.Name] = slices.Clone(step.After)
	}

	keep := make(map[string]struct{})
	var visit func(string) error
	visit = func(name string) error {
		if _, ok := keep[name]; ok {
			return nil
		}
		after, ok := deps[name]
		if !ok {
			return fmt.Errorf("unknown analyse target step %q", name)
		}
		keep[name] = struct{}{}
		for _, dep := range after {
			if err := visit(dep); err != nil {
				return err
			}
		}
		return nil
	}

	for _, name := range analyseSteps {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return keep, nil
}

func Filter(plan Plan, keep map[string]struct{}) (Plan, error) {
	out := Plan{
		BenchmarkName: plan.BenchmarkName,
		SourceHash:    plan.SourceHash,
		GlobalNProc:   plan.GlobalNProc,
	}
	for _, step := range plan.Steps {
		if _, ok := keep[step.Name]; !ok {
			continue
		}
		next := step
		next.After = slices.Clone(step.After)
		out.Steps = append(out.Steps, next)
	}
	for _, work := range plan.Work {
		if _, ok := keep[work.StepName]; !ok {
			continue
		}
		next := work
		next.Deps = slices.Clone(work.Deps)
		for _, dep := range next.Deps {
			if _, ok := keep[dep.Step]; !ok {
				return Plan{}, fmt.Errorf("filtered workpackage %s depends on removed step %s", work.ID.Step, dep.Step)
			}
		}
		out.Work = append(out.Work, next)
	}
	return out, nil
}
