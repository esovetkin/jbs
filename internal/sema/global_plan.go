package sema

import (
	"maps"
	"slices"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

type globalReadRef struct {
	Name    string
	SeedAlt string
}

type globalInputKind string

const (
	globalInputAssign          globalInputKind = "assign"
	globalInputExpr            globalInputKind = "expr"
	globalInputProjectedImport globalInputKind = "projected_import"
)

type projectedImport struct {
	LocalName     string
	SourceName    string
	SourceBinding *GlobalBinding
	Span          diag.Span
}

type globalInputStep struct {
	ID                int
	Kind              globalInputKind
	Name              string
	Assign            *ast.GlobalAssign
	ExprStmt          *ast.ExprStmt
	Import            *projectedImport
	EffectiveExpr     ast.Expr
	Reads             []globalReadRef
	Index             int
	IsSimple          bool
	SeedEnv           map[string]eval.Value
	VisibleNamespaces map[string]*Namespace
	Names             *eval.NameCatalog
}

type globalPlan struct {
	Steps              []globalInputStep
	StepsByName        map[string][]int
	SimpleWritesByName map[string][]int
}

type globalExecResult struct {
	UserGlobals         GlobalState
	UserGlobalVarByName map[string]*GlobalVar
	UserGlobalOrder     []string
	TopLevelExprs       []TopLevelExprResult
	ScalarGlobals       GlobalState
}

func buildGlobalPlan(prog ast.Program, baseSeed map[string]eval.Value) *globalPlan {
	plan := &globalPlan{
		Steps:              make([]globalInputStep, 0),
		StepsByName:        make(map[string][]int),
		SimpleWritesByName: make(map[string][]int),
	}
	for index, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			assign := n
			assignCopy := assign
			effectiveExpr := assignmentExpr(assign.Name, assign.Op, assign.Expr, assign.Span)
			id := len(plan.Steps)
			step := globalInputStep{
				ID:            id,
				Kind:          globalInputAssign,
				Name:          assign.Name,
				Assign:        &assignCopy,
				EffectiveExpr: effectiveExpr,
				Reads:         globalExprReadRefs(effectiveExpr),
				Index:         index,
				IsSimple:      !isCompoundAssignOp(assign.Op),
			}
			plan.Steps = append(plan.Steps, step)
			plan.StepsByName[step.Name] = append(plan.StepsByName[step.Name], id)
			if step.IsSimple {
				plan.SimpleWritesByName[step.Name] = append(plan.SimpleWritesByName[step.Name], id)
			}
		case ast.ExprStmt:
			exprStmt := n
			exprCopy := exprStmt
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:            id,
				Kind:          globalInputExpr,
				ExprStmt:      &exprCopy,
				EffectiveExpr: exprStmt.Expr,
				Reads:         globalExprReadRefs(exprStmt.Expr),
				Index:         index,
			})
		}
	}
	assignGlobalPlanNameCatalogs(plan, baseSeed)
	return plan
}

func assignGlobalPlanNameCatalogs(plan *globalPlan, seed map[string]eval.Value) {
	if plan == nil {
		return
	}
	activeSet := make(map[int]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		activeSet[step.ID] = struct{}{}
	}
	prevWrite := make(map[string]int, len(plan.StepsByName))
	for i := range plan.Steps {
		step := &plan.Steps[i]
		visibleSet := make(map[string]struct{}, len(seed)+len(prevWrite)+len(plan.SimpleWritesByName))
		for name := range seed {
			if isUnqualifiedVisibleName(name) {
				visibleSet[name] = struct{}{}
			}
		}
		for name := range prevWrite {
			if isUnqualifiedVisibleName(name) {
				visibleSet[name] = struct{}{}
			}
		}
		for name := range plan.SimpleWritesByName {
			if !isUnqualifiedVisibleName(name) {
				continue
			}
			if _, ok := visibleSet[name]; ok {
				continue
			}
			if _, ok := uniqueForwardSimpleWrite(name, step.ID, plan, activeSet); ok {
				visibleSet[name] = struct{}{}
			}
		}
		visible := make([]string, 0, len(visibleSet))
		for name := range visibleSet {
			visible = append(visible, name)
		}
		step.Names = scopeNameCatalog(visible, step.VisibleNamespaces)
		if step.Name != "" {
			prevWrite[step.Name] = step.ID
		}
	}
}

func isCompoundAssignOp(op ast.AssignOp) bool {
	_, ok := mapAssignOpToBinary(op)
	return ok
}

func execGlobalPlan(plan *globalPlan, generalSeed map[string]eval.Value, scalarSeed map[string]eval.Value, diags *diag.Diagnostics) *globalExecResult {
	if plan == nil {
		plan = &globalPlan{
			Steps:              make([]globalInputStep, 0),
			StepsByName:        make(map[string][]int),
			SimpleWritesByName: make(map[string][]int),
		}
	}
	res := &globalExecResult{
		UserGlobals: GlobalState{
			Values: make(map[string]eval.Value),
			Modes:  make(map[string]string),
			Spans:  make(map[string]diag.Span),
		},
		UserGlobalVarByName: make(map[string]*GlobalVar),
		UserGlobalOrder:     make([]string, 0),
		TopLevelExprs:       make([]TopLevelExprResult, 0),
		ScalarGlobals: GlobalState{
			Values: maps.Clone(scalarSeed),
			Modes:  make(map[string]string),
			Spans:  make(map[string]diag.Span),
		},
	}
	if generalSeed == nil {
		generalSeed = map[string]eval.Value{}
	}
	if scalarSeed == nil {
		scalarSeed = map[string]eval.Value{}
	}
	userInclude := func(step globalInputStep) bool {
		if step.Kind == globalInputExpr {
			return true
		}
		_, known := generalSeed[step.Name]
		return !known
	}
	scalarInclude := func(step globalInputStep) bool {
		if step.Kind == globalInputExpr {
			return false
		}
		_, known := scalarSeed[step.Name]
		return known
	}
	userOrder := buildGlobalSchedule(plan, generalSeed, userInclude)
	execUserGlobalSteps(plan, userOrder, generalSeed, userInclude, res, diags)
	scalarOrder := buildGlobalSchedule(plan, scalarSeed, scalarInclude)
	execScalarGlobalSteps(plan, scalarOrder, scalarSeed, diags, &res.ScalarGlobals)
	return res
}

func execUserGlobalSteps(plan *globalPlan, order []int, seed map[string]eval.Value, include func(globalInputStep) bool, res *globalExecResult, diags *diag.Diagnostics) {
	env := maps.Clone(seed)
	seenOrder := make(map[string]bool)
	for _, step := range plan.Steps {
		if !include(step) || step.Name == "" || seenOrder[step.Name] {
			continue
		}
		seenOrder[step.Name] = true
		res.UserGlobalOrder = append(res.UserGlobalOrder, step.Name)
	}
	for _, id := range order {
		step := plan.Steps[id]
		switch step.Kind {
		case globalInputExpr:
			if step.ExprStmt == nil || step.EffectiveExpr == nil {
				continue
			}
			evalEnv := mergeValueEnv(step.SeedEnv, env)
			value := eval.EvalExprWithOptions(step.EffectiveExpr, evalEnv, diags, eval.ExprOptions{
				GlobalAssignmentTupleArithmetic: true,
				Context:                         eval.EvalCtxBindingAssign,
				Names:                           step.Names,
			})
			res.TopLevelExprs = append(res.TopLevelExprs, TopLevelExprResult{
				Index: step.Index,
				Span:  step.ExprStmt.Span,
				Value: value,
			})
		case globalInputProjectedImport:
			if step.Import == nil || step.Import.SourceBinding == nil {
				continue
			}
			gv := globalVarFromImportedBinding(step.Name, step.Import.SourceBinding, step.Import.Span)
			if gv == nil {
				continue
			}
			res.UserGlobalVarByName[step.Name] = gv
			res.UserGlobals.Values[step.Name] = gv.Value
			res.UserGlobals.Spans[step.Name] = gv.Span
			if gv.Mode != "" {
				res.UserGlobals.Modes[step.Name] = gv.Mode
			} else {
				delete(res.UserGlobals.Modes, step.Name)
			}
			env[step.Name] = gv.Value
		case globalInputAssign:
			if step.Assign == nil {
				continue
			}
			warnModeExprInCollections(step.EffectiveExpr, diags)
			mode, inner, isModeExpr := unwrapModeExpr(step.EffectiveExpr)
			expr := step.EffectiveExpr
			if isModeExpr {
				expr = inner
			}
			evalEnv := mergeValueEnv(step.SeedEnv, env)
			value := eval.EvalExprWithOptions(expr, evalEnv, diags, eval.ExprOptions{
				GlobalAssignmentTupleArithmetic: true,
				Context:                         eval.EvalCtxBindingAssign,
				Names:                           step.Names,
			})
			if isModeExpr {
				value = coerceModeValue(mode, value, step.Assign.Span, diags)
			} else {
				mode = ""
			}
			if hasNestedList(value) {
				diags.AddError(
					diag.CodeE305,
					"nested tuple/list value is not allowed for global variable '"+step.Name+"'",
					step.Assign.Span,
					"use flat tuple/list values only",
				)
			}
			orderNames, vars := globalVarSeries(step.Name, value)
			res.UserGlobalVarByName[step.Name] = &GlobalVar{
				Name:      step.Name,
				Value:     value,
				Mode:      mode,
				Span:      step.Assign.Span,
				Order:     orderNames,
				Vars:      vars,
				DependsOn: globalExprDependencies(step.EffectiveExpr, step.Name),
			}
			res.UserGlobals.Values[step.Name] = value
			res.UserGlobals.Spans[step.Name] = step.Assign.Span
			if mode != "" {
				res.UserGlobals.Modes[step.Name] = mode
			} else {
				delete(res.UserGlobals.Modes, step.Name)
			}
			env[step.Name] = value
		}
	}
}

func execScalarGlobalSteps(plan *globalPlan, order []int, seed map[string]eval.Value, diags *diag.Diagnostics, out *GlobalState) {
	env := maps.Clone(seed)
	for _, id := range order {
		step := plan.Steps[id]
		switch step.Kind {
		case globalInputProjectedImport:
			if step.Import == nil || step.Import.SourceBinding == nil {
				continue
			}
			gv := globalVarFromImportedBinding(step.Name, step.Import.SourceBinding, step.Import.Span)
			if gv == nil {
				continue
			}
			if step.Name == "jbs_name" || step.Name == "jbs_outpath" {
				if gv.Mode != "" {
					diags.AddError(
						diag.CodeE303,
						step.Name+" must be a simple string, not shell()/python()",
						gv.Span,
						"assign a plain string literal",
					)
					continue
				}
				if gv.Value.Kind != eval.KindString {
					code := diag.CodeE301
					if step.Name == "jbs_outpath" {
						code = diag.CodeE302
					}
					diags.AddError(
						code,
						step.Name+" must be a simple string literal",
						gv.Span,
						"assign a plain quoted string",
					)
					continue
				}
			}
			if !isScalarGlobalValue(gv.Value) {
				diags.AddError(
					diag.CodeE304,
					"global variable '"+step.Name+"' must be scalar; tuples/lists are not allowed",
					gv.Span,
					"use string/int/float/bool or shell()/python() scalar values",
				)
				continue
			}
			env[step.Name] = gv.Value
			out.Values[step.Name] = gv.Value
			out.Spans[step.Name] = gv.Span
			if gv.Mode != "" {
				out.Modes[step.Name] = gv.Mode
			} else {
				delete(out.Modes, step.Name)
			}
		case globalInputAssign:
			if step.Assign == nil {
				continue
			}
			warnModeExprInCollections(step.EffectiveExpr, diags)
			assign := *step.Assign
			if assign.Name == "jbs_name" || assign.Name == "jbs_outpath" {
				if _, isMode := assign.Expr.(ast.ModeExpr); isMode {
					diags.AddError(
						diag.CodeE303,
						assign.Name+" must be a simple string, not shell()/python()",
						assign.Span,
						"assign a plain string literal",
					)
					continue
				}
				if _, ok := assign.Expr.(ast.StringExpr); !ok {
					code := diag.CodeE301
					if assign.Name == "jbs_outpath" {
						code = diag.CodeE302
					}
					diags.AddError(
						code,
						assign.Name+" must be a simple string literal",
						assign.Span,
						"assign a plain quoted string",
					)
					continue
				}
				evalEnv := mergeValueEnv(step.SeedEnv, env)
				value := eval.EvalExprWithOptions(assign.Expr, evalEnv, diags, eval.ExprOptions{Context: eval.EvalCtxScalarGlobalAssign, Names: step.Names})
				env[assign.Name] = value
				out.Values[assign.Name] = value
				delete(out.Modes, assign.Name)
				out.Spans[assign.Name] = assign.Span
				continue
			}
			mode, inner, isModeExpr := unwrapModeExpr(step.EffectiveExpr)
			expr := step.EffectiveExpr
			if isModeExpr {
				expr = inner
			}
			evalEnv := mergeValueEnv(step.SeedEnv, env)
			value := eval.EvalExprWithOptions(expr, evalEnv, diags, eval.ExprOptions{Context: eval.EvalCtxScalarGlobalAssign, Names: step.Names})
			if isModeExpr {
				value = coerceModeValue(mode, value, assign.Span, diags)
				out.Modes[assign.Name] = mode
			} else {
				delete(out.Modes, assign.Name)
			}
			if !isScalarGlobalValue(value) {
				diags.AddError(
					diag.CodeE304,
					"global variable '"+assign.Name+"' must be scalar; tuples/lists are not allowed",
					assign.Span,
					"use string/int/float/bool or shell()/python() scalar values",
				)
				continue
			}
			env[assign.Name] = value
			out.Values[assign.Name] = value
			out.Spans[assign.Name] = assign.Span
		}
	}
}

func buildGlobalSchedule(plan *globalPlan, seed map[string]eval.Value, include func(globalInputStep) bool) []int {
	active := make([]int, 0, len(plan.Steps))
	activeSet := make(map[int]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		if !include(step) {
			continue
		}
		active = append(active, step.ID)
		activeSet[step.ID] = struct{}{}
	}
	if len(active) == 0 {
		return nil
	}
	depsOf := make(map[int]map[int]struct{}, len(active))
	dependents := make(map[int][]int, len(active))
	indegree := make(map[int]int, len(active))
	prevWrite := make(map[string]int, len(plan.StepsByName))
	for _, id := range active {
		indegree[id] = 0
	}
	addDep := func(depID int, stepID int) {
		if depID == stepID {
			return
		}
		if _, ok := activeSet[depID]; !ok {
			return
		}
		if depsOf[stepID] == nil {
			depsOf[stepID] = make(map[int]struct{})
		}
		if _, exists := depsOf[stepID][depID]; exists {
			return
		}
		depsOf[stepID][depID] = struct{}{}
		dependents[depID] = append(dependents[depID], stepID)
		indegree[stepID]++
	}
	for _, id := range active {
		step := plan.Steps[id]
		if step.Name != "" {
			if prevID, ok := prevWrite[step.Name]; ok {
				addDep(prevID, id)
			}
		}
		for _, read := range step.Reads {
			depID, ok := bindGlobalReadDependency(step, read, id, prevWrite, plan, activeSet, seed)
			if ok {
				addDep(depID, id)
			}
		}
		if step.Name != "" {
			prevWrite[step.Name] = id
		}
	}
	remaining := make(map[int]struct{}, len(active))
	for _, id := range active {
		remaining[id] = struct{}{}
	}
	order := make([]int, 0, len(active))
	for {
		ready := -1
		readyIndex := 0
		for _, id := range active {
			if _, ok := remaining[id]; !ok || indegree[id] != 0 {
				continue
			}
			if ready == -1 || plan.Steps[id].Index < readyIndex {
				ready = id
				readyIndex = plan.Steps[id].Index
			}
		}
		if ready == -1 {
			break
		}
		delete(remaining, ready)
		order = append(order, ready)
		for _, dep := range dependents[ready] {
			indegree[dep]--
		}
	}
	if len(remaining) == 0 {
		return order
	}
	for _, id := range active {
		if _, ok := remaining[id]; !ok {
			continue
		}
		order = append(order, id)
	}
	return order
}

func bindGlobalReadDependency(step globalInputStep, read globalReadRef, stepID int, prevWrite map[string]int, plan *globalPlan, activeSet map[int]struct{}, seed map[string]eval.Value) (int, bool) {
	if read.Name != "" {
		if depID, ok := prevWrite[read.Name]; ok {
			return depID, true
		}
		if stepHasSeed(step, read.Name, seed) {
			return 0, false
		}
		if read.Name != step.Name {
			if depID, ok := uniqueForwardSimpleWrite(read.Name, stepID, plan, activeSet); ok {
				return depID, true
			}
		}
	}
	if read.SeedAlt != "" && stepHasSeed(step, read.SeedAlt, seed) {
		return 0, false
	}
	return 0, false
}

func stepHasSeed(step globalInputStep, name string, base map[string]eval.Value) bool {
	if step.SeedEnv != nil {
		if _, ok := step.SeedEnv[name]; ok {
			return true
		}
	}
	if base == nil {
		return false
	}
	_, ok := base[name]
	return ok
}

func uniqueForwardSimpleWrite(name string, stepID int, plan *globalPlan, activeSet map[int]struct{}) (int, bool) {
	writes := plan.SimpleWritesByName[name]
	found := -1
	for _, id := range writes {
		if id == stepID {
			continue
		}
		if _, ok := activeSet[id]; !ok {
			continue
		}
		if !stepEligibleForForwardBinding(plan.Steps[id]) {
			continue
		}
		if found >= 0 {
			return 0, false
		}
		found = id
	}
	if found < 0 {
		return 0, false
	}
	return found, true
}

func stepEligibleForForwardBinding(step globalInputStep) bool {
	return step.Kind == globalInputAssign && step.IsSimple
}

func globalExprReadNames(expr ast.Expr) []string {
	refs := globalExprReadRefs(expr)
	out := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs)*2)
	for _, ref := range refs {
		if ref.Name != "" {
			if _, ok := seen[ref.Name]; !ok {
				seen[ref.Name] = struct{}{}
				out = append(out, ref.Name)
			}
		}
		if ref.SeedAlt != "" {
			if _, ok := seen[ref.SeedAlt]; !ok {
				seen[ref.SeedAlt] = struct{}{}
				out = append(out, ref.SeedAlt)
			}
		}
	}
	return out
}

func globalExprReadRefs(expr ast.Expr) []globalReadRef {
	out := make([]globalReadRef, 0)
	seen := make(map[globalReadRef]struct{})
	var walk func(ast.Expr)
	appendRef := func(ref globalReadRef) {
		if ref.Name == "" && ref.SeedAlt == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.IdentExpr:
			appendRef(globalReadRef{Name: n.Name})
		case ast.QualifiedIdentExpr:
			if n.Namespace != "" {
				seedAlt := ""
				if n.Name != "" {
					seedAlt = n.Namespace + "." + n.Name
				}
				appendRef(globalReadRef{Name: n.Namespace, SeedAlt: seedAlt})
			}
		case ast.MemberExpr:
			walk(n.Base)
		case ast.ModeExpr:
			walk(n.Expr)
		case ast.ListExpr:
			for _, item := range n.Items {
				walk(item)
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				walk(item)
			}
		case ast.ConvertExpr:
			walk(n.Expr)
		case ast.CallExpr:
			for _, arg := range n.Args {
				walk(arg)
			}
		case ast.AliasExpr:
			walk(n.Expr)
		case ast.IndexExpr:
			walk(n.Base)
		case ast.UnaryExpr:
			walk(n.Expr)
		case ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.CompareExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.ConditionalExpr:
			walk(n.Then)
			walk(n.Cond)
			walk(n.Else)
		}
	}
	walk(expr)
	return out
}

func globalVarsFromExec(exec *globalExecResult) (map[string]*GlobalVar, []string) {
	if exec == nil {
		return map[string]*GlobalVar{}, nil
	}
	out := make(map[string]*GlobalVar, len(exec.UserGlobalVarByName))
	for name, gv := range exec.UserGlobalVarByName {
		if gv == nil {
			continue
		}
		copyGV := *gv
		copyGV.Order = append([]string(nil), gv.Order...)
		copyGV.Vars = cloneSeriesMap(gv.Vars)
		copyGV.DependsOn = append([]string(nil), gv.DependsOn...)
		out[name] = &copyGV
	}
	return out, slices.Clone(exec.UserGlobalOrder)
}
