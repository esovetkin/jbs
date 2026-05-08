package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func buildGlobalPlan(prog ast.Program, baseSeed map[string]eval.Value, baseDir string) *globalPlan {
	visibleNames := collectProgramVisibleNames(prog)
	plan := &globalPlan{
		Steps:             make([]globalInputStep, 0, len(prog.Stmts)),
		StepByName:        make(map[string]int),
		LocalVisibleNames: append([]string(nil), visibleNames...),
	}
	appendGlobalPlanSteps(plan, prog.Stmts, baseDir, globalPlanContext{})
	assignGlobalPlanNameCatalogs(plan, baseSeed)
	return plan
}

func appendGlobalPlanSteps(plan *globalPlan, stmts []ast.Stmt, baseDir string, ctx globalPlanContext) []globalInputStep {
	steps := make([]globalInputStep, 0, len(stmts))
	for index, stmt := range stmts {
		step, ok := buildGlobalInputStep(plan, stmt, index, baseDir, ctx)
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

func buildGlobalInputStep(plan *globalPlan, stmt ast.Stmt, index int, baseDir string, ctx globalPlanContext) (globalInputStep, bool) {
	switch n := stmt.(type) {
	case ast.IfStmt:
		stmtCopy := n
		id := nextGlobalStepID(plan)
		return globalInputStep{
			ID:      id,
			Kind:    globalInputIf,
			IfStmt:  &stmtCopy,
			Then:    appendGlobalPlanSteps(plan, stmtCopy.Then, baseDir, ctx.nestedControl()),
			Else:    appendGlobalPlanSteps(plan, stmtCopy.Else, baseDir, ctx.nestedControl()),
			Index:   index,
			BaseDir: baseDir,
		}, true
	case ast.ForStmt:
		stmtCopy := n
		id := nextGlobalStepID(plan)
		step := globalInputStep{
			ID:            id,
			Kind:          globalInputFor,
			Name:          stmtCopy.Target,
			ForStmt:       &stmtCopy,
			Body:          appendGlobalPlanSteps(plan, stmtCopy.Body, baseDir, ctx.nestedLoop()),
			EffectiveExpr: stmtCopy.Iterable,
			Reads:         globalExprReadRefs(stmtCopy.Iterable),
			Index:         index,
			BaseDir:       baseDir,
		}
		if stmtCopy.Target != "" {
			plan.StepByName[stmtCopy.Target] = id
		}
		return step, true
	case ast.WhileStmt:
		stmtCopy := n
		return globalInputStep{
			ID:            nextGlobalStepID(plan),
			Kind:          globalInputWhile,
			WhileStmt:     &stmtCopy,
			Body:          appendGlobalPlanSteps(plan, stmtCopy.Body, baseDir, ctx.nestedLoop()),
			EffectiveExpr: stmtCopy.Cond,
			Reads:         globalExprReadRefs(stmtCopy.Cond),
			Index:         index,
			BaseDir:       baseDir,
		}, true
	case ast.BreakStmt:
		stmtCopy := n
		return globalInputStep{
			ID:        nextGlobalStepID(plan),
			Kind:      globalInputBreak,
			BreakStmt: &stmtCopy,
			Index:     index,
			BaseDir:   baseDir,
		}, true
	case ast.ContinueStmt:
		stmtCopy := n
		return globalInputStep{
			ID:           nextGlobalStepID(plan),
			Kind:         globalInputContinue,
			ContinueStmt: &stmtCopy,
			Index:        index,
			BaseDir:      baseDir,
		}, true
	case ast.GlobalAssign:
		assignCopy := n
		effective := assignmentExpr(assignCopy.Name, assignCopy.Op, assignCopy.Expr, assignCopy.Span)
		id := nextGlobalStepID(plan)
		step := globalInputStep{
			ID:            id,
			Kind:          globalInputAssign,
			Name:          assignCopy.Name,
			Assign:        &assignCopy,
			EffectiveExpr: effective,
			Reads:         globalExprReadRefs(effective),
			Index:         index,
			BaseDir:       baseDir,
		}
		plan.StepByName[assignCopy.Name] = id
		return step, true
	case ast.ExprStmt:
		exprCopy := n
		return globalInputStep{
			ID:            nextGlobalStepID(plan),
			Kind:          globalInputExpr,
			ExprStmt:      &exprCopy,
			EffectiveExpr: exprCopy.Expr,
			Reads:         globalExprReadRefs(exprCopy.Expr),
			Index:         index,
			BaseDir:       baseDir,
		}, true
	case ast.DoBlock:
		if ctx.InControlBody {
			return globalInputStep{}, false
		}
		blockCopy := n
		return globalInputStep{
			ID:      nextGlobalStepID(plan),
			Kind:    globalInputDo,
			Name:    blockCopy.Name,
			DoBlock: &blockCopy,
			Index:   index,
			BaseDir: baseDir,
		}, true
	case ast.AnalyseBlock:
		if ctx.InControlBody {
			return globalInputStep{}, false
		}
		blockCopy := n
		return globalInputStep{
			ID:           nextGlobalStepID(plan),
			Kind:         globalInputAnalyse,
			Name:         blockCopy.StepName,
			AnalyseBlock: &blockCopy,
			Index:        index,
			BaseDir:      baseDir,
		}, true
	case ast.UseStmt:
		return globalInputStep{}, false
	default:
		return globalInputStep{}, false
	}
}

func nextGlobalStepID(plan *globalPlan) int {
	if plan == nil {
		return 0
	}
	id := plan.NextID
	plan.NextID++
	return id
}

func collectProgramVisibleNames(prog ast.Program) []string {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	for _, stmt := range prog.Stmts {
		collectProgramVisibleNameStmt(stmt, &names, seen)
	}
	return names
}

func collectProgramVisibleNameStmt(stmt ast.Stmt, names *[]string, seen map[string]struct{}) {
	switch n := stmt.(type) {
	case ast.GlobalAssign:
		appendVisibleName(names, seen, n.Name)
	case ast.IfStmt:
		for _, child := range n.Then {
			collectProgramVisibleNameStmt(child, names, seen)
		}
		for _, child := range n.Else {
			collectProgramVisibleNameStmt(child, names, seen)
		}
	case ast.ForStmt:
		appendVisibleName(names, seen, n.Target)
		for _, child := range n.Body {
			collectProgramVisibleNameStmt(child, names, seen)
		}
	case ast.WhileStmt:
		for _, child := range n.Body {
			collectProgramVisibleNameStmt(child, names, seen)
		}
	}
}

func appendVisibleName(names *[]string, seen map[string]struct{}, name string) {
	if name == "" {
		return
	}
	if _, exists := seen[name]; exists {
		return
	}
	seen[name] = struct{}{}
	*names = append(*names, name)
}

func assignGlobalPlanNameCatalogs(plan *globalPlan, seed map[string]eval.Value) {
	if plan == nil {
		return
	}
	visible := make(map[string]struct{}, len(seed)+len(plan.LocalVisibleNames))
	for _, name := range visibleNamesFromEnv(seed) {
		visible[name] = struct{}{}
	}
	for _, name := range plan.LocalVisibleNames {
		if isUnqualifiedVisibleName(name) {
			visible[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(visible))
	for name := range visible {
		names = append(names, name)
	}
	catalog := scopeNameCatalog(names, nil)
	assignGlobalStepNameCatalogs(plan.Steps, catalog)
}

func assignGlobalStepNameCatalogs(steps []globalInputStep, catalog *eval.NameCatalog) {
	for i := range steps {
		steps[i].Reads = globalExprReadRefs(steps[i].EffectiveExpr)
		steps[i].Names = catalog
		assignGlobalStepNameCatalogs(steps[i].Then, catalog)
		assignGlobalStepNameCatalogs(steps[i].Else, catalog)
		assignGlobalStepNameCatalogs(steps[i].Body, catalog)
	}
}
