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
	LocalName    string
	SourceName   string
	SourceGlobal *GlobalVar
	Span         diag.Span
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
	SeedEnv           map[string]eval.Value
	VisibleNamespaces map[string]*Namespace
	Names             *eval.NameCatalog
	BaseDir           string
	ForwardVisible    bool
}

type globalPlan struct {
	Steps             []globalInputStep
	StepByName        map[string]int
	LocalVisibleNames []string
}

type globalExecResult struct {
	UserGlobals         GlobalState
	UserGlobalVarByName map[string]*GlobalVar
	UserGlobalOrder     []string
	TopLevelExprs       []TopLevelExprResult
	ScalarGlobals       GlobalState
}

type globalForceState uint8

const (
	globalForceIdle globalForceState = iota
	globalForceForcing
	globalForceDone
)

type globalStepState struct {
	State globalForceState
	Value eval.Value
	Var   *GlobalVar
}

type activeGlobalEval struct {
	StepID int
	Name   string
	Deps   map[string]struct{}
}

type globalForceEngine struct {
	plan        *globalPlan
	include     func(globalInputStep) bool
	activeSet   map[int]struct{}
	generalSeed map[string]eval.Value
	diags       *diag.Diagnostics
	rootFrame   *eval.Frame
	res         *globalExecResult
	states      []globalStepState
	stack       []*activeGlobalEval
}

type programBindingPlan struct {
	AcceptedByIndex map[int]ast.GlobalAssign
	VisibleNames    []string
}

type topLevelBindingRef struct {
	Name string
	Span diag.Span
}

func buildGlobalPlan(prog ast.Program, baseSeed map[string]eval.Value, baseDir string, diags *diag.Diagnostics) *globalPlan {
	prep := planProgramBindings(prog, diags)
	plan := &globalPlan{
		Steps:             make([]globalInputStep, 0),
		StepByName:        make(map[string]int),
		LocalVisibleNames: append([]string(nil), prep.VisibleNames...),
	}
	for index, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			assign, ok := prep.AcceptedByIndex[index]
			if !ok {
				continue
			}
			assignCopy := assign
			id := len(plan.Steps)
			step := globalInputStep{
				ID:             id,
				Kind:           globalInputAssign,
				Name:           assign.Name,
				Assign:         &assignCopy,
				EffectiveExpr:  assign.Expr,
				Reads:          globalExprReadRefs(assign.Expr),
				Index:          index,
				BaseDir:        baseDir,
				ForwardVisible: true,
			}
			plan.Steps = append(plan.Steps, step)
			plan.StepByName[step.Name] = id
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
				BaseDir:       baseDir,
			})
		}
	}
	assignGlobalPlanNameCatalogs(plan, baseSeed)
	return plan
}

func planProgramBindings(prog ast.Program, diags *diag.Diagnostics) programBindingPlan {
	out := programBindingPlan{
		AcceptedByIndex: make(map[int]ast.GlobalAssign),
		VisibleNames:    make([]string, 0),
	}
	firstByName := make(map[string]topLevelBindingRef)
	for index, stmt := range prog.Stmts {
		assign, ok := stmt.(ast.GlobalAssign)
		if !ok {
			continue
		}
		if isCompoundAssignOp(assign.Op) {
			reportTopLevelCompoundAssign(diags, assign)
			continue
		}
		if prev, exists := firstByName[assign.Name]; exists {
			reportDuplicateTopLevelBinding(diags, assign.Name, assign.Span, prev.Span)
			continue
		}
		firstByName[assign.Name] = topLevelBindingRef{Name: assign.Name, Span: assign.Span}
		out.AcceptedByIndex[index] = assign
		out.VisibleNames = append(out.VisibleNames, assign.Name)
	}
	return out
}

func reportTopLevelCompoundAssign(diags *diag.Diagnostics, assign ast.GlobalAssign) {
	if diags == nil {
		return
	}
	diags.AddError(
		diag.CodeE307,
		"top-level binding '"+assign.Name+"' cannot use '"+string(assign.Op)+"'",
		assign.Span,
		"define a new name instead of mutating an existing global",
	)
}

func reportDuplicateTopLevelBinding(diags *diag.Diagnostics, name string, span diag.Span, firstSpan diag.Span) {
	if diags == nil {
		return
	}
	diags.AddError(
		diag.CodeE306,
		"duplicate top-level binding '"+name+"'",
		span,
		"define each top-level binding once; introduce a new name instead of rebinding or re-importing it",
		diag.RelatedSpan{Message: "first definition", Span: firstSpan},
	)
}

func assignGlobalPlanNameCatalogs(plan *globalPlan, seed map[string]eval.Value) {
	if plan == nil {
		return
	}
	baseVisible := make(map[string]struct{}, len(seed)+len(plan.LocalVisibleNames))
	for _, name := range visibleNamesFromEnv(seed) {
		baseVisible[name] = struct{}{}
	}
	for _, name := range plan.LocalVisibleNames {
		if isUnqualifiedVisibleName(name) {
			baseVisible[name] = struct{}{}
		}
	}
	visibleProjected := make(map[string]struct{})
	for i := range plan.Steps {
		step := &plan.Steps[i]
		visibleSet := make(map[string]struct{}, len(baseVisible)+len(visibleProjected))
		for name := range baseVisible {
			visibleSet[name] = struct{}{}
		}
		for name := range visibleProjected {
			visibleSet[name] = struct{}{}
		}
		visible := make([]string, 0, len(visibleSet))
		for name := range visibleSet {
			visible = append(visible, name)
		}
		step.Names = scopeNameCatalog(visible, step.VisibleNamespaces)
		if step.Kind == globalInputProjectedImport && isUnqualifiedVisibleName(step.Name) {
			visibleProjected[step.Name] = struct{}{}
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
			Steps:             make([]globalInputStep, 0),
			StepByName:        make(map[string]int),
			LocalVisibleNames: make([]string, 0),
		}
	}
	generalValues := maps.Clone(generalSeed)
	if generalValues == nil {
		generalValues = map[string]eval.Value{}
	}
	scalarValues := maps.Clone(scalarSeed)
	if scalarValues == nil {
		scalarValues = map[string]eval.Value{}
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
			Values: scalarValues,
			Modes:  make(map[string]string),
			Spans:  make(map[string]diag.Span),
		},
	}

	userInclude := func(step globalInputStep) bool {
		if step.Kind == globalInputExpr {
			return true
		}
		_, known := generalValues[step.Name]
		return !known
	}
	engine := newGlobalForceEngine(plan, generalValues, userInclude, diags, res)
	engine.execute()
	res.ScalarGlobals = execScalarGlobalPlan(plan, scalarValues, diags)
	return res
}

func execScalarGlobalPlan(plan *globalPlan, scalarSeed map[string]eval.Value, diags *diag.Diagnostics) GlobalState {
	out := GlobalState{
		Values: maps.Clone(scalarSeed),
		Modes:  make(map[string]string),
		Spans:  make(map[string]diag.Span),
	}
	if out.Values == nil {
		out.Values = map[string]eval.Value{}
	}
	if plan == nil || len(scalarSeed) == 0 {
		return out
	}
	include := func(step globalInputStep) bool {
		return step.Kind != globalInputExpr
	}
	dummy := &globalExecResult{
		UserGlobals: GlobalState{
			Values: make(map[string]eval.Value),
			Modes:  make(map[string]string),
			Spans:  make(map[string]diag.Span),
		},
		UserGlobalVarByName: make(map[string]*GlobalVar),
		UserGlobalOrder:     make([]string, 0),
		TopLevelExprs:       make([]TopLevelExprResult, 0),
	}
	engine := newGlobalForceEngine(plan, scalarSeed, include, diags, dummy)
	for _, step := range plan.Steps {
		if step.Name == "" {
			continue
		}
		if _, ok := scalarSeed[step.Name]; !ok {
			continue
		}
		if !scalarStepShouldForce(step, diags) {
			continue
		}
		engine.forceStep(step.ID)
	}
	for _, step := range plan.Steps {
		if step.Name == "" {
			continue
		}
		if _, ok := scalarSeed[step.Name]; !ok {
			continue
		}
		state := engine.states[step.ID]
		switch step.Kind {
		case globalInputProjectedImport:
			gv := state.Var
			if gv == nil {
				continue
			}
			if !acceptScalarGlobalImport(step.Name, gv, diags) {
				continue
			}
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
			if !acceptScalarGlobalAssign(*step.Assign, state.Var, diags) {
				continue
			}
			if state.Var == nil {
				continue
			}
			out.Values[step.Name] = state.Var.Value
			out.Spans[step.Name] = step.Assign.Span
			if state.Var.Mode != "" {
				out.Modes[step.Name] = state.Var.Mode
			} else {
				delete(out.Modes, step.Name)
			}
		}
	}
	return out
}

func scalarStepShouldForce(step globalInputStep, diags *diag.Diagnostics) bool {
	if step.Name != "jbs_name" && step.Name != "jbs_outpath" {
		return true
	}
	if step.Kind != globalInputAssign || step.Assign == nil {
		return true
	}
	if _, isMode := step.Assign.Expr.(ast.ModeExpr); isMode {
		diags.AddError(
			diag.CodeE303,
			step.Name+" must be a simple string, not shell()/python()",
			step.Assign.Span,
			"assign a plain string literal",
		)
		return false
	}
	if _, ok := step.Assign.Expr.(ast.StringExpr); !ok {
		code := diag.CodeE301
		if step.Name == "jbs_outpath" {
			code = diag.CodeE302
		}
		diags.AddError(
			code,
			step.Name+" must be a simple string literal",
			step.Assign.Span,
			"assign a plain quoted string",
		)
		return false
	}
	return true
}

func acceptScalarGlobalImport(name string, gv *GlobalVar, diags *diag.Diagnostics) bool {
	if gv == nil {
		return false
	}
	if name == "jbs_name" || name == "jbs_outpath" {
		if gv.Mode != "" {
			diags.AddError(
				diag.CodeE303,
				name+" must be a simple string, not shell()/python()",
				gv.Span,
				"assign a plain string literal",
			)
			return false
		}
		if gv.Value.Kind != eval.KindString {
			code := diag.CodeE301
			if name == "jbs_outpath" {
				code = diag.CodeE302
			}
			diags.AddError(
				code,
				name+" must be a simple string literal",
				gv.Span,
				"assign a plain quoted string",
			)
			return false
		}
	}
	if !isScalarGlobalValue(gv.Value) {
		diags.AddError(
			diag.CodeE304,
			"global variable '"+name+"' must be scalar; tuples/lists are not allowed",
			gv.Span,
			"use string/int/float/bool or shell()/python() scalar values",
		)
		return false
	}
	return true
}

func acceptScalarGlobalAssign(assign ast.GlobalAssign, gv *GlobalVar, diags *diag.Diagnostics) bool {
	if assign.Name == "jbs_name" || assign.Name == "jbs_outpath" {
		return true
	}
	if gv == nil {
		return false
	}
	if !isScalarGlobalValue(gv.Value) {
		diags.AddError(
			diag.CodeE304,
			"global variable '"+assign.Name+"' must be scalar; tuples/lists are not allowed",
			assign.Span,
			"use string/int/float/bool or shell()/python() scalar values",
		)
		return false
	}
	return true
}

func newGlobalForceEngine(plan *globalPlan, generalSeed map[string]eval.Value, include func(globalInputStep) bool, diags *diag.Diagnostics, res *globalExecResult) *globalForceEngine {
	if generalSeed == nil {
		generalSeed = map[string]eval.Value{}
	}
	if res == nil {
		res = &globalExecResult{
			UserGlobals: GlobalState{
				Values: make(map[string]eval.Value),
				Modes:  make(map[string]string),
				Spans:  make(map[string]diag.Span),
			},
			UserGlobalVarByName: make(map[string]*GlobalVar),
			UserGlobalOrder:     make([]string, 0),
			TopLevelExprs:       make([]TopLevelExprResult, 0),
		}
	}
	engine := &globalForceEngine{
		plan:        plan,
		include:     include,
		generalSeed: maps.Clone(generalSeed),
		diags:       diags,
		res:         res,
		states:      make([]globalStepState, len(plan.Steps)),
		activeSet:   make(map[int]struct{}, len(plan.Steps)),
	}
	for _, step := range plan.Steps {
		if include(step) {
			engine.activeSet[step.ID] = struct{}{}
		}
	}
	engine.rootFrame = eval.NewRootFrame(engine.generalSeed)
	engine.rootFrame.Resolve = engine.resolveName
	return engine
}

func (e *globalForceEngine) execute() {
	if e == nil || e.plan == nil {
		return
	}
	seenOrder := make(map[string]bool)
	for _, step := range e.plan.Steps {
		if !e.include(step) {
			continue
		}
		if step.Name != "" && !seenOrder[step.Name] {
			seenOrder[step.Name] = true
			e.res.UserGlobalOrder = append(e.res.UserGlobalOrder, step.Name)
		}
		switch step.Kind {
		case globalInputExpr:
			e.evalExprStep(step)
		case globalInputAssign, globalInputProjectedImport:
			e.forceStep(step.ID)
		}
	}
}

func (e *globalForceEngine) forceStep(id int) (eval.Value, bool) {
	if e == nil || e.plan == nil || id < 0 || id >= len(e.plan.Steps) {
		return eval.Null(), false
	}
	if _, ok := e.activeSet[id]; !ok {
		return eval.Null(), false
	}
	state := &e.states[id]
	switch state.State {
	case globalForceDone:
		return state.Value, true
	case globalForceForcing:
		step := e.plan.Steps[id]
		name := step.Name
		if name == "" && step.Assign != nil {
			name = step.Assign.Name
		}
		if name == "" {
			name = "<expr>"
		}
		e.diags.AddError(
			diag.CodeE100,
			"cyclic global reference involving '"+name+"'",
			globalStepSpan(step),
			"break the dependency cycle between top-level globals",
		)
		return eval.Null(), true
	}

	step := e.plan.Steps[id]
	state.State = globalForceForcing
	active := &activeGlobalEval{
		StepID: step.ID,
		Name:   step.Name,
		Deps:   make(map[string]struct{}),
	}
	e.stack = append(e.stack, active)
	defer func() {
		e.stack = e.stack[:len(e.stack)-1]
		state.State = globalForceDone
	}()

	switch step.Kind {
	case globalInputProjectedImport:
		if step.Import == nil || step.Import.SourceGlobal == nil {
			state.Value = eval.Null()
			return state.Value, true
		}
		gv := globalVarFromImportedGlobal(step.Name, step.Import.SourceGlobal, step.Import.Span)
		if gv == nil {
			state.Value = eval.Null()
			return state.Value, true
		}
		gv.DependsOn = sortedGlobalDeps(active.Deps, step.Name)
		state.Value = gv.Value
		state.Var = gv
		e.publishUserGlobal(step, gv)
		return state.Value, true
	case globalInputAssign:
		if step.Assign == nil {
			state.Value = eval.Null()
			return state.Value, true
		}
		warnModeExprInCollections(step.EffectiveExpr, e.diags)
		mode, inner, isModeExpr := unwrapModeExpr(step.EffectiveExpr)
		expr := step.EffectiveExpr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExprWithOptions(expr, nil, e.diags, eval.ExprOptions{
			GlobalAssignmentTupleArithmetic: true,
			Context:                         eval.EvalCtxBindingAssign,
			Names:                           step.Names,
			Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
			Frame:                           e.stepFrame(step),
		})
		if isModeExpr {
			value = coerceModeValue(mode, value, step.Assign.Span, e.diags)
		} else {
			mode = ""
		}
		if hasNestedList(value) {
			e.diags.AddError(
				diag.CodeE305,
				"nested tuple/list value is not allowed for global variable '"+step.Name+"'",
				step.Assign.Span,
				"use flat tuple/list values only",
			)
		}
		orderNames, vars := globalVarSeries(step.Name, value)
		gv := &GlobalVar{
			Name:      step.Name,
			Value:     value,
			Mode:      mode,
			Span:      step.Assign.Span,
			Order:     orderNames,
			Vars:      vars,
			DependsOn: sortedGlobalDeps(active.Deps, step.Name),
		}
		state.Value = value
		state.Var = gv
		e.publishUserGlobal(step, gv)
		return state.Value, true
	default:
		state.Value = eval.Null()
		return state.Value, false
	}
}

func (e *globalForceEngine) evalExprStep(step globalInputStep) {
	if step.ExprStmt == nil || step.EffectiveExpr == nil {
		return
	}
	e.stack = append(e.stack, &activeGlobalEval{StepID: step.ID})
	value := eval.EvalExprWithOptions(step.EffectiveExpr, nil, e.diags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           step.Names,
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.stepFrame(step),
	})
	e.stack = e.stack[:len(e.stack)-1]
	e.res.TopLevelExprs = append(e.res.TopLevelExprs, TopLevelExprResult{
		Index: step.Index,
		Span:  step.ExprStmt.Span,
		Value: value,
	})
}

func (e *globalForceEngine) stepFrame(step globalInputStep) *eval.Frame {
	frame := eval.NewChildFrame(e.rootFrame)
	for name, value := range step.SeedEnv {
		frame.AssignLocal(name, value, globalStepSpan(step))
	}
	return frame
}

func (e *globalForceEngine) resolveName(name string, at diag.Span, diags *diag.Diagnostics) (eval.Value, bool) {
	if e == nil || name == "" || len(e.stack) == 0 {
		return eval.Null(), false
	}
	current := e.stack[len(e.stack)-1]
	targetID, ok := e.resolveReadTarget(name, current.StepID)
	if !ok {
		return eval.Null(), false
	}
	value, forced := e.forceStep(targetID)
	if !forced {
		return eval.Null(), false
	}
	e.recordResolvedName(name)
	e.recordResolvedDeps(targetID)
	return value, true
}

func (e *globalForceEngine) resolveReadTarget(name string, consumerID int) (int, bool) {
	if e == nil || e.plan == nil || consumerID < 0 || consumerID >= len(e.plan.Steps) {
		return 0, false
	}
	targetID, ok := e.plan.StepByName[name]
	if !ok {
		return 0, false
	}
	if _, ok := e.activeSet[targetID]; !ok {
		return 0, false
	}
	consumer := e.plan.Steps[consumerID]
	target := e.plan.Steps[targetID]
	if !target.ForwardVisible && target.Index >= consumer.Index {
		return 0, false
	}
	return targetID, true
}

func (e *globalForceEngine) recordResolvedName(name string) {
	if e == nil || name == "" {
		return
	}
	for _, active := range e.stack {
		if active == nil || active.Name == "" || active.Name == name {
			continue
		}
		active.Deps[name] = struct{}{}
	}
}

func (e *globalForceEngine) recordResolvedDeps(stepID int) {
	if e == nil || stepID < 0 || stepID >= len(e.states) {
		return
	}
	gv := e.states[stepID].Var
	if gv == nil {
		return
	}
	for _, dep := range gv.DependsOn {
		e.recordResolvedName(dep)
	}
}

func (e *globalForceEngine) publishUserGlobal(step globalInputStep, gv *GlobalVar) {
	if e == nil || e.res == nil || gv == nil || step.Name == "" {
		return
	}
	e.res.UserGlobalVarByName[step.Name] = gv
	e.res.UserGlobals.Values[step.Name] = gv.Value
	e.res.UserGlobals.Spans[step.Name] = gv.Span
	if gv.Mode != "" {
		e.res.UserGlobals.Modes[step.Name] = gv.Mode
	} else {
		delete(e.res.UserGlobals.Modes, step.Name)
	}
}

func sortedGlobalDeps(deps map[string]struct{}, self string) []string {
	if len(deps) == 0 {
		return nil
	}
	out := make([]string, 0, len(deps))
	for name := range deps {
		if name == "" || name == self {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

func globalStepSpan(step globalInputStep) diag.Span {
	switch step.Kind {
	case globalInputAssign:
		if step.Assign != nil {
			return step.Assign.Span
		}
	case globalInputExpr:
		if step.ExprStmt != nil {
			return step.ExprStmt.Span
		}
	case globalInputProjectedImport:
		if step.Import != nil {
			return step.Import.Span
		}
	}
	return diag.Span{}
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
				walk(arg.Expr)
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
