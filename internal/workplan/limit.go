package workplan

import (
	"fmt"
	"slices"
)

type LimitOptions struct {
	Limit       int
	TargetSteps []string
}

func LimitBranches(plan Plan, opts LimitOptions) (Plan, error) {
	if opts.Limit <= 0 {
		return clonePlan(plan), nil
	}
	targetSteps := slices.Clone(opts.TargetSteps)
	if len(targetSteps) == 0 {
		targetSteps = TerminalSteps(plan)
	}
	return limitBranches(plan, opts.Limit, targetSteps)
}

func TerminalSteps(plan Plan) []string {
	hasChild := make(map[string]bool)
	known := make(map[string]bool)
	for _, step := range plan.Steps {
		known[step.Name] = true
	}
	for _, step := range plan.Steps {
		for _, dep := range step.After {
			if known[dep] && dep != step.Name {
				hasChild[dep] = true
			}
		}
	}
	out := make([]string, 0)
	for _, step := range plan.Steps {
		if !hasChild[step.Name] {
			out = append(out, step.Name)
		}
	}
	return out
}

func limitBranches(plan Plan, limit int, targetSteps []string) (Plan, error) {
	byID := make(map[WorkID]WorkPackage, len(plan.Work))
	for _, work := range plan.Work {
		byID[work.ID] = work
	}

	targets := make(map[string]struct{}, len(targetSteps))
	for _, step := range targetSteps {
		targets[step] = struct{}{}
	}

	keptWork := make(map[WorkID]struct{})
	counts := make(map[string]int)
	for _, work := range plan.Work {
		if _, ok := targets[work.StepName]; !ok {
			continue
		}
		if counts[work.StepName] >= limit {
			continue
		}
		if err := keepWithAncestors(work.ID, byID, keptWork); err != nil {
			return Plan{}, err
		}
		counts[work.StepName]++
	}

	return filterPlanToKeptWork(plan, keptWork, targets), nil
}

func keepWithAncestors(id WorkID, byID map[WorkID]WorkPackage, kept map[WorkID]struct{}) error {
	if _, ok := kept[id]; ok {
		return nil
	}
	work, ok := byID[id]
	if !ok {
		return fmt.Errorf("limited workpackage %s/%06d depends on missing workpackage", id.Step, id.Row)
	}
	kept[id] = struct{}{}
	for _, dep := range work.Deps {
		if err := keepWithAncestors(dep, byID, kept); err != nil {
			return err
		}
	}
	return nil
}

func filterPlanToKeptWork(plan Plan, keptWork map[WorkID]struct{}, targetSteps map[string]struct{}) Plan {
	keptSteps := make(map[string]struct{})
	for id := range keptWork {
		keptSteps[id.Step] = struct{}{}
	}
	for step := range targetSteps {
		keptSteps[step] = struct{}{}
	}

	out := Plan{
		BenchmarkName: plan.BenchmarkName,
		SourceHash:    plan.SourceHash,
		GlobalNProc:   plan.GlobalNProc,
	}
	for _, step := range plan.Steps {
		if _, ok := keptSteps[step.Name]; !ok {
			continue
		}
		next := step
		next.After = slices.Clone(step.After)
		out.Steps = append(out.Steps, next)
	}
	for _, work := range plan.Work {
		if _, ok := keptWork[work.ID]; !ok {
			continue
		}
		out.Work = append(out.Work, cloneWorkPackage(work))
	}
	return out
}

func clonePlan(plan Plan) Plan {
	out := Plan{
		BenchmarkName: plan.BenchmarkName,
		SourceHash:    plan.SourceHash,
		GlobalNProc:   plan.GlobalNProc,
		Steps:         make([]Step, 0, len(plan.Steps)),
		Work:          make([]WorkPackage, 0, len(plan.Work)),
	}
	for _, step := range plan.Steps {
		next := step
		next.After = slices.Clone(step.After)
		out.Steps = append(out.Steps, next)
	}
	for _, work := range plan.Work {
		out.Work = append(out.Work, cloneWorkPackage(work))
	}
	return out
}

func cloneWorkPackage(work WorkPackage) WorkPackage {
	next := work
	next.Values = cloneValues(work.Values)
	next.SourceRows = cloneSourceRows(work.SourceRows)
	next.Deps = slices.Clone(work.Deps)
	return next
}
