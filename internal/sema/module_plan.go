package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

func buildModuleGlobalPlan(info *imports.ModuleInfo, childByIndex map[int]*moduleScope, prefixedByIndex map[int]*moduleScope, baseSeed map[string]eval.Value, diags *diag.Diagnostics) *globalPlan {
	prep := prepareModuleBindings(info, childByIndex, diags)
	plan := &globalPlan{
		Steps:             make([]globalInputStep, 0),
		StepByName:        make(map[string]int),
		LocalVisibleNames: append([]string(nil), prep.LocalVisibleNames...),
	}
	if info == nil {
		return plan
	}
	useByIndex := make(map[int]imports.ResolvedUse, len(info.Uses))
	for _, use := range info.Uses {
		useByIndex[use.Index] = use
	}
	appendModuleGlobalPlanSteps(plan, info.Program.Stmts, info.BaseDir, globalPlanContext{}, useByIndex, prep, prefixedByIndex)
	assignGlobalPlanNameCatalogs(plan, baseSeed)
	return plan
}

func appendModuleGlobalPlanSteps(plan *globalPlan, stmts []ast.Stmt, baseDir string, ctx globalPlanContext, useByIndex map[int]imports.ResolvedUse, prep moduleBindingPrep, prefixedByIndex map[int]*moduleScope) []globalInputStep {
	steps := make([]globalInputStep, 0, len(stmts))
	for index, stmt := range stmts {
		sourceIndex := index
		if ctx.InControlBody {
			sourceIndex = ctx.OriginIndex
		}
		if use, ok := useByIndex[index]; ok {
			if ctx.InControlBody {
				continue
			}
			if use.Kind == imports.UseNamespace {
				step := globalInputStep{
					ID:             nextGlobalStepID(plan),
					Kind:           globalInputNamespaceImport,
					NamespaceScope: prefixedByIndex[index],
					Index:          sourceIndex,
					BaseDir:        baseDir,
				}
				plan.Steps = append(plan.Steps, step)
				continue
			}
			for _, name := range use.Names {
				imp := prep.AcceptedImports[projectedImportDecisionKey{Index: index, Name: name}]
				if imp == nil {
					continue
				}
				id := nextGlobalStepID(plan)
				plan.Steps = append(plan.Steps, globalInputStep{
					ID:      id,
					Kind:    globalInputProjectedImport,
					Name:    name,
					Import:  imp,
					Index:   sourceIndex,
					BaseDir: baseDir,
				})
				plan.StepByName[name] = id
			}
			continue
		}
		if ifStmt, ok := stmt.(ast.IfStmt); ok {
			stmtCopy := ifStmt
			step := globalInputStep{
				ID:      nextGlobalStepID(plan),
				Kind:    globalInputIf,
				IfStmt:  &stmtCopy,
				Then:    appendModuleGlobalPlanSteps(plan, stmtCopy.Then, baseDir, ctx.nestedControl(sourceIndex), nil, prep, prefixedByIndex),
				Else:    appendModuleGlobalPlanSteps(plan, stmtCopy.Else, baseDir, ctx.nestedControl(sourceIndex), nil, prep, prefixedByIndex),
				Index:   sourceIndex,
				BaseDir: baseDir,
			}
			if ctx.InControlBody {
				steps = append(steps, step)
			} else {
				plan.Steps = append(plan.Steps, step)
			}
			continue
		}
		if forStmt, ok := stmt.(ast.ForStmt); ok {
			stmtCopy := forStmt
			id := nextGlobalStepID(plan)
			step := globalInputStep{
				ID:            id,
				Kind:          globalInputFor,
				Name:          stmtCopy.Target,
				ForStmt:       &stmtCopy,
				Body:          appendModuleGlobalPlanSteps(plan, stmtCopy.Body, baseDir, ctx.nestedLoop(sourceIndex), nil, prep, prefixedByIndex),
				EffectiveExpr: stmtCopy.Iterable,
				Reads:         globalExprReadRefs(stmtCopy.Iterable),
				Index:         sourceIndex,
				BaseDir:       baseDir,
			}
			if stmtCopy.Target != "" {
				plan.StepByName[stmtCopy.Target] = id
			}
			if ctx.InControlBody {
				steps = append(steps, step)
			} else {
				plan.Steps = append(plan.Steps, step)
			}
			continue
		}
		if whileStmt, ok := stmt.(ast.WhileStmt); ok {
			stmtCopy := whileStmt
			step := globalInputStep{
				ID:            nextGlobalStepID(plan),
				Kind:          globalInputWhile,
				WhileStmt:     &stmtCopy,
				Body:          appendModuleGlobalPlanSteps(plan, stmtCopy.Body, baseDir, ctx.nestedLoop(sourceIndex), nil, prep, prefixedByIndex),
				EffectiveExpr: stmtCopy.Cond,
				Reads:         globalExprReadRefs(stmtCopy.Cond),
				Index:         sourceIndex,
				BaseDir:       baseDir,
			}
			if ctx.InControlBody {
				steps = append(steps, step)
			} else {
				plan.Steps = append(plan.Steps, step)
			}
			continue
		}
		step, ok := buildGlobalInputStep(plan, stmt, sourceIndex, baseDir, ctx)
		if !ok {
			continue
		}
		if ctx.InControlBody {
			steps = append(steps, step)
		} else {
			plan.Steps = append(plan.Steps, step)
		}
	}
	return steps
}
