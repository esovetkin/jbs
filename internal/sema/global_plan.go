package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type globalReadRef struct {
	Name    string
	SeedAlt string
}

type globalInputKind string

const (
	globalInputAssign          globalInputKind = "assign"
	globalInputExpr            globalInputKind = "expr"
	globalInputIf              globalInputKind = "if"
	globalInputFor             globalInputKind = "for"
	globalInputWhile           globalInputKind = "while"
	globalInputBreak           globalInputKind = "break"
	globalInputContinue        globalInputKind = "continue"
	globalInputProjectedImport globalInputKind = "projected_import"
	globalInputNamespaceImport globalInputKind = "namespace_import"
	globalInputDo              globalInputKind = "do"
	globalInputAnalyse         globalInputKind = "analyse"
)

type projectedImport struct {
	LocalName    string
	SourceName   string
	SourceGlobal *GlobalVar
	Span         diag.Span
}

type globalInputStep struct {
	ID             int
	Kind           globalInputKind
	Name           string
	Assign         *ast.GlobalAssign
	ExprStmt       *ast.ExprStmt
	IfStmt         *ast.IfStmt
	ForStmt        *ast.ForStmt
	WhileStmt      *ast.WhileStmt
	Then           []globalInputStep
	Else           []globalInputStep
	Body           []globalInputStep
	BreakStmt      *ast.BreakStmt
	ContinueStmt   *ast.ContinueStmt
	Import         *projectedImport
	NamespaceScope *moduleScope
	DoBlock        *ast.DoBlock
	AnalyseBlock   *ast.AnalyseBlock
	EffectiveExpr  ast.Expr
	Reads          []globalReadRef
	Index          int
	Names          *eval.NameCatalog
	ForwardVisible bool
	BaseDir        string
}

type globalPlan struct {
	Steps      []globalInputStep
	StepByName map[string]int
	// Precomputed before execution so expression name catalogs can expose
	// locals that may be defined later in sequential or control-flow execution.
	LocalVisibleNames []string
	NextID            int
}

type globalPlanContext struct {
	InControlBody bool
	LoopDepth     int
}

func (ctx globalPlanContext) nestedControl() globalPlanContext {
	ctx.InControlBody = true
	return ctx
}

func (ctx globalPlanContext) nestedLoop() globalPlanContext {
	ctx.InControlBody = true
	ctx.LoopDepth++
	return ctx
}

type globalExecResult struct {
	UserGlobals           GlobalState
	UserGlobalVarByName   map[string]*GlobalVar
	UserGlobalOrder       []string
	TopLevelExprs         []TopLevelExprResult
	ScalarGlobals         GlobalState
	SnapshotBindings      []*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
}

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

func execGlobalPlan(plan *globalPlan, generalSeed map[string]eval.Value, scalarSeed map[string]eval.Value, diags *diag.Diagnostics) *globalExecResult {
	if plan == nil {
		plan = &globalPlan{
			Steps:             make([]globalInputStep, 0),
			StepByName:        make(map[string]int),
			LocalVisibleNames: make([]string, 0),
		}
	}
	engine := newGlobalSeqEngine(plan, generalSeed, scalarSeed, diags)
	engine.execute()
	return engine.res
}

type globalSeqEngine struct {
	plan                *globalPlan
	diags               *diag.Diagnostics
	rootFrame           *eval.Frame
	values              map[string]eval.Value
	spans               map[string]diag.Span
	scalarSeed          map[string]eval.Value
	scalarSpans         map[string]diag.Span
	globalVars          map[string]*GlobalVar
	globalOrder         []string
	globalOrderSeen     map[string]struct{}
	currentBindings     map[string]*GlobalBinding
	currentBindingOrder []string
	currentBindingSeen  map[string]struct{}
	namespaces          map[string]*Namespace
	snapshotNames       map[string]struct{}
	res                 *globalExecResult
}

func newGlobalSeqEngine(plan *globalPlan, generalSeed map[string]eval.Value, scalarSeed map[string]eval.Value, diags *diag.Diagnostics) *globalSeqEngine {
	values := maps.Clone(generalSeed)
	if values == nil {
		values = map[string]eval.Value{}
	}
	scalars := maps.Clone(scalarSeed)
	if scalars == nil {
		scalars = map[string]eval.Value{}
	}
	res := &globalExecResult{
		UserGlobals: GlobalState{
			Values: make(map[string]eval.Value),
			Spans:  make(map[string]diag.Span),
		},
		UserGlobalVarByName:   make(map[string]*GlobalVar),
		UserGlobalOrder:       make([]string, 0),
		TopLevelExprs:         make([]TopLevelExprResult, 0),
		ScalarGlobals:         GlobalState{Values: maps.Clone(scalars), Spans: make(map[string]diag.Span)},
		SnapshotBindings:      make([]*GlobalBinding, 0),
		ScopeSnapshotsByIndex: make(map[int]*ScopeSnapshot),
		ScopeSnapshotsByBlock: make(map[string]*ScopeSnapshot),
	}
	return &globalSeqEngine{
		plan:                plan,
		diags:               diags,
		rootFrame:           eval.NewRootFrame(values),
		values:              values,
		spans:               make(map[string]diag.Span),
		scalarSeed:          scalars,
		scalarSpans:         make(map[string]diag.Span),
		globalVars:          make(map[string]*GlobalVar),
		globalOrder:         make([]string, 0),
		globalOrderSeen:     make(map[string]struct{}),
		currentBindings:     make(map[string]*GlobalBinding),
		currentBindingOrder: make([]string, 0),
		currentBindingSeen:  make(map[string]struct{}),
		namespaces:          make(map[string]*Namespace),
		snapshotNames:       make(map[string]struct{}),
		res:                 res,
	}
}

func (e *globalSeqEngine) execute() {
	if e == nil || e.plan == nil {
		return
	}
	if result := e.executeSteps(e.plan.Steps, nil); result.active() {
		e.diags.AddError(diag.CodeE080, "'break' and 'continue' are only allowed inside loops", result.Span, "move the statement into a for/while body")
	}
	e.res.UserGlobals.Values = maps.Clone(e.values)
	e.res.UserGlobals.Spans = maps.Clone(e.spans)
	e.res.UserGlobalVarByName, e.res.UserGlobalOrder = cloneGlobalVars(e.globalVars, e.globalOrder)
	e.res.ScalarGlobals = GlobalState{
		Values: maps.Clone(e.scalarSeed),
		Spans:  maps.Clone(e.scalarSpans),
	}
}

type globalLoopSignal int

const (
	globalLoopNone globalLoopSignal = iota
	globalLoopBreak
	globalLoopContinue
)

type globalStepResult struct {
	Signal globalLoopSignal
	Span   diag.Span
}

func (r globalStepResult) active() bool {
	return r.Signal != globalLoopNone
}

func (e *globalSeqEngine) executeSteps(steps []globalInputStep, guardDeps []string) globalStepResult {
	for _, step := range steps {
		var result globalStepResult
		switch step.Kind {
		case globalInputAssign:
			e.evalAssignStep(step, guardDeps)
		case globalInputProjectedImport:
			e.evalProjectedImportStep(step)
		case globalInputNamespaceImport:
			e.evalNamespaceImportStep(step)
		case globalInputExpr:
			e.evalExprStep(step)
		case globalInputIf:
			result = e.evalIfStep(step, guardDeps)
		case globalInputFor:
			result = e.evalForStep(step, guardDeps)
		case globalInputWhile:
			result = e.evalWhileStep(step, guardDeps)
		case globalInputBreak:
			return globalStepResult{Signal: globalLoopBreak, Span: globalStepSpan(step)}
		case globalInputContinue:
			return globalStepResult{Signal: globalLoopContinue, Span: globalStepSpan(step)}
		case globalInputDo, globalInputAnalyse:
			e.recordDeclarationSnapshot(step)
		}
		if result.active() {
			return result
		}
	}
	return globalStepResult{}
}

func (e *globalSeqEngine) evalAssignStep(step globalInputStep, guardDeps []string) {
	if step.Assign == nil {
		return
	}
	assign := *step.Assign
	effective := assignmentExpr(assign.Name, assign.Op, assign.Expr, assign.Span)
	if assign.Op != "" && assign.Op != ast.AssignEq {
		if _, ok := e.rootFrame.Read(assign.Name, assign.Span, e.diags); !ok {
			return
		}
	}

	before := errorCount(e.diags)
	value := eval.EvalExprWithOptions(effective, nil, e.diags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           e.currentNameCatalog(),
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.rootFrame,
	})
	if errorCount(e.diags) > before {
		return
	}
	if hasNestedList(value) {
		e.diags.AddError(
			diag.CodeE305,
			"nested tuple/list value is not allowed for global variable '"+assign.Name+"'",
			assign.Span,
			"use flat tuple/list values only",
		)
		return
	}

	directDeps := globalExprDependencies(effective, assign.Name)
	directDeps = append(directDeps, guardDeps...)
	directDeps = uniqueSortedNamesExcept(directDeps, assign.Name)
	orderNames, vars := globalVarSeries(assign.Name, value)
	gv := &GlobalVar{
		Name:          assign.Name,
		Value:         value,
		Span:          assign.Span,
		Order:         orderNames,
		Vars:          vars,
		DependsOn:     e.expandGlobalDeps(directDeps, assign.Name),
		DependsOnKeys: e.expandGlobalDepKeys(directDeps, assign.Name),
		VersionID:     bindingVersionID(step),
	}
	if !e.acceptGlobalVar(gv) {
		return
	}
	e.publishGlobalVar(gv)
}

func (e *globalSeqEngine) evalIfStep(step globalInputStep, guardDeps []string) globalStepResult {
	if step.IfStmt == nil {
		return globalStepResult{}
	}
	cond, ok := eval.EvalBoolCondition(step.IfStmt.Cond, nil, e.diags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           e.currentNameCatalog(),
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.rootFrame,
	})
	if !ok {
		return globalStepResult{}
	}
	nextGuardDeps := append([]string(nil), guardDeps...)
	nextGuardDeps = append(nextGuardDeps, globalExprDependencies(step.IfStmt.Cond, "")...)
	nextGuardDeps = uniqueSortedNamesExcept(nextGuardDeps, "")
	if cond {
		return e.executeSteps(step.Then, nextGuardDeps)
	}
	return e.executeSteps(step.Else, nextGuardDeps)
}

func (e *globalSeqEngine) evalForStep(step globalInputStep, guardDeps []string) globalStepResult {
	if step.ForStmt == nil {
		return globalStepResult{}
	}
	iterable := eval.EvalExprWithOptions(step.ForStmt.Iterable, nil, e.diags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           e.currentNameCatalog(),
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.rootFrame,
	})
	items, ok := eval.IterableElements(iterable, exprSpan(step.ForStmt.Iterable), e.diags)
	if !ok {
		return globalStepResult{}
	}
	loopDeps := append([]string(nil), guardDeps...)
	loopDeps = append(loopDeps, globalExprDependencies(step.ForStmt.Iterable, "")...)
	loopDeps = uniqueSortedNamesExcept(loopDeps, "")
	for i, item := range items {
		if i >= eval.MaxLoopIterations {
			e.diags.AddError(diag.CodeE106, "loop exceeded 100000 iterations", step.ForStmt.Span, "check the iterable size")
			return globalStepResult{}
		}
		if !e.publishLoopVariable(step.ForStmt.Target, item, step.ForStmt.Span, loopDeps, step) {
			return globalStepResult{}
		}
		result := e.executeSteps(step.Body, loopDeps)
		switch result.Signal {
		case globalLoopBreak:
			return globalStepResult{}
		case globalLoopContinue:
			continue
		default:
			if result.active() {
				return result
			}
		}
	}
	return globalStepResult{}
}

func (e *globalSeqEngine) evalWhileStep(step globalInputStep, guardDeps []string) globalStepResult {
	if step.WhileStmt == nil {
		return globalStepResult{}
	}
	loopDeps := append([]string(nil), guardDeps...)
	loopDeps = append(loopDeps, globalExprDependencies(step.WhileStmt.Cond, "")...)
	loopDeps = uniqueSortedNamesExcept(loopDeps, "")
	for i := 0; ; i++ {
		if i >= eval.MaxLoopIterations {
			e.diags.AddError(diag.CodeE106, "loop exceeded 100000 iterations", step.WhileStmt.Span, "check the while condition")
			return globalStepResult{}
		}
		cond, ok := eval.EvalBoolConditionFor("while", step.WhileStmt.Cond, nil, e.diags, eval.ExprOptions{
			GlobalAssignmentTupleArithmetic: true,
			Context:                         eval.EvalCtxBindingAssign,
			Names:                           e.currentNameCatalog(),
			Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
			Frame:                           e.rootFrame,
		})
		if !ok || !cond {
			return globalStepResult{}
		}
		result := e.executeSteps(step.Body, loopDeps)
		switch result.Signal {
		case globalLoopBreak:
			return globalStepResult{}
		case globalLoopContinue:
			continue
		default:
			if result.active() {
				return result
			}
		}
	}
}

func (e *globalSeqEngine) publishLoopVariable(name string, value eval.Value, span diag.Span, deps []string, step globalInputStep) bool {
	if name == "" {
		return false
	}
	if hasNestedList(value) {
		e.diags.AddError(
			diag.CodeE305,
			"nested tuple/list value is not allowed for global variable '"+name+"'",
			span,
			"use flat tuple/list values only",
		)
		return false
	}
	directDeps := uniqueSortedNamesExcept(deps, name)
	orderNames, vars := globalVarSeries(name, value)
	gv := &GlobalVar{
		Name:          name,
		Value:         value,
		Span:          span,
		Order:         orderNames,
		Vars:          vars,
		DependsOn:     e.expandGlobalDeps(directDeps, name),
		DependsOnKeys: e.expandGlobalDepKeys(directDeps, name),
		VersionID:     bindingVersionID(step),
	}
	if !e.acceptGlobalVar(gv) {
		return false
	}
	e.publishGlobalVar(gv)
	return true
}

func (e *globalSeqEngine) evalProjectedImportStep(step globalInputStep) {
	if step.Import == nil || step.Import.SourceGlobal == nil {
		return
	}
	gv := globalVarFromImportedGlobal(step.Name, step.Import.SourceGlobal, step.Import.Span)
	if gv == nil || !e.acceptGlobalVar(gv) {
		return
	}
	gv.VersionID = bindingVersionID(step)
	gv.DependsOn = []string{step.Import.SourceName}
	if key := BindingVersionKeyForGlobalVar(step.Import.SourceGlobal, step.Import.SourceName); key != (BindingVersionKey{}) {
		gv.DependsOnKeys = []BindingVersionKey{key}
	}
	e.publishGlobalVar(gv)
}

func (e *globalSeqEngine) evalNamespaceImportStep(step globalInputStep) {
	scope := step.NamespaceScope
	if scope == nil {
		return
	}
	for name, ns := range scope.Namespaces {
		if ns == nil {
			continue
		}
		current := e.namespaces[name]
		if current == nil {
			e.namespaces[name] = &Namespace{
				Name:     ns.Name,
				Members:  append([]string(nil), ns.Members...),
				Bindings: append([]string(nil), ns.Bindings...),
				Steps:    append([]string(nil), ns.Steps...),
			}
			continue
		}
		current.Members = mergeUniqueStrings(current.Members, ns.Members)
		current.Bindings = mergeUniqueStrings(current.Bindings, ns.Bindings)
		current.Steps = mergeUniqueStrings(current.Steps, ns.Steps)
	}
	for name, gv := range scope.ExportsByName {
		next := cloneGlobalVar(gv)
		if next == nil {
			continue
		}
		next.Name = name
		next.Namespace = namespaceHead(name)
		e.publishGlobalVar(next)
	}
	for _, binding := range scope.Bindings {
		next := cloneBinding(binding)
		if next == nil {
			continue
		}
		e.publishBinding(next)
	}
}

func (e *globalSeqEngine) evalExprStep(step globalInputStep) {
	if step.ExprStmt == nil || step.ExprStmt.Expr == nil {
		return
	}
	value := eval.EvalExprWithOptions(step.ExprStmt.Expr, nil, e.diags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           e.currentNameCatalog(),
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.rootFrame,
	})
	e.res.TopLevelExprs = append(e.res.TopLevelExprs, TopLevelExprResult{
		Index: step.Index,
		Span:  step.ExprStmt.Span,
		Value: value,
	})
}

func (e *globalSeqEngine) recordDeclarationSnapshot(step globalInputStep) {
	snap := e.cloneSnapshot(step.Index)
	if snap == nil {
		return
	}
	e.res.ScopeSnapshotsByIndex[step.Index] = snap
	if key := globalStepBlockKey(step); key != "" {
		e.res.ScopeSnapshotsByBlock[key] = snap
	}
	for _, binding := range snap.Bindings {
		e.res.SnapshotBindings = append(e.res.SnapshotBindings, cloneBinding(binding))
	}
}

func (e *globalSeqEngine) cloneSnapshot(index int) *ScopeSnapshot {
	snap := &ScopeSnapshot{
		Index: index,
		Globals: GlobalState{
			Values: maps.Clone(e.values),
			Spans:  maps.Clone(e.spans),
		},
		Bindings:       make([]*GlobalBinding, 0, len(e.currentBindings)),
		BindingsByName: make(map[string]*GlobalBinding, len(e.currentBindings)*2),
		Namespaces:     cloneVisibleNamespaces(e.namespaces),
	}
	snap.GlobalVarByName, snap.GlobalVarOrder = cloneGlobalVars(e.globalVars, e.globalOrder)
	for _, public := range e.currentBindingOrder {
		binding := e.currentBindings[public]
		if binding == nil {
			continue
		}
		next := cloneBinding(binding)
		next.PublicName = bindingDisplayName(next)
		next.Name = e.snapshotBindingName(next.PublicName, index)
		next.SyntheticGlobal = true
		snap.Bindings = append(snap.Bindings, next)
		snap.BindingsByName[next.Name] = next
		if next.PublicName != "" {
			snap.BindingsByName[next.PublicName] = next
		}
	}
	return snap
}

func (e *globalSeqEngine) snapshotBindingName(public string, index int) string {
	base := "_js__" + fmt.Sprint(index) + "__" + sanitizeSnapshotName(public)
	name := base
	for i := 1; ; i++ {
		if _, exists := e.snapshotNames[name]; !exists {
			if _, collides := e.currentBindings[name]; !collides {
				e.snapshotNames[name] = struct{}{}
				return name
			}
		}
		name = fmt.Sprintf("%s_%d", base, i)
	}
}

func bindingVersionID(step globalInputStep) string {
	span := globalStepSpan(step)
	if !span.IsZero() {
		return fmt.Sprintf("%s:%d:%d", span.File, span.Start.Offset, span.End.Offset)
	}
	return fmt.Sprintf("%s:%d:%s", step.Kind, step.ID, step.Name)
}

func sanitizeSnapshotName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "binding"
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "binding"
	}
	return b.String()
}

func (e *globalSeqEngine) acceptGlobalVar(gv *GlobalVar) bool {
	if gv == nil {
		return false
	}
	if gv.Name == "jbs_name" {
		if gv.Value.Kind != eval.KindString {
			e.diags.AddError(
				diag.CodeE301,
				gv.Name+" must be a simple string literal",
				gv.Span,
				"assign a plain quoted string",
			)
			return false
		}
	}
	if _, ok := e.scalarSeed[gv.Name]; ok && !isScalarGlobalValue(gv.Value) {
		e.diags.AddError(
			diag.CodeE304,
			"global variable '"+gv.Name+"' must be scalar; tuples/lists are not allowed",
			gv.Span,
			"use string/int/float/bool scalar values",
		)
		return false
	}
	return true
}

func (e *globalSeqEngine) expandGlobalDeps(deps []string, self string) []string {
	if len(deps) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(deps))
	queue := append([]string(nil), deps...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if name == "" || name == self {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if gv := e.globalVars[name]; gv != nil {
			queue = append(queue, gv.DependsOn...)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(seen))
}

func (e *globalSeqEngine) expandGlobalDepKeys(deps []string, self string) []BindingVersionKey {
	if len(deps) == 0 {
		return nil
	}
	seen := make(map[BindingVersionKey]struct{}, len(deps))
	seenNames := make(map[string]struct{}, len(deps))
	addKey := func(key BindingVersionKey) {
		if key == (BindingVersionKey{}) || key.Public == self {
			return
		}
		seen[key] = struct{}{}
	}
	var addName func(string)
	addName = func(name string) {
		if name == "" || name == self {
			return
		}
		if _, exists := seenNames[name]; exists {
			return
		}
		seenNames[name] = struct{}{}
		if key, ok := e.bindingKeyForCurrentName(name); ok {
			addKey(key)
		}
		if gv := e.globalVars[name]; gv != nil {
			if len(gv.DependsOnKeys) > 0 {
				for _, dep := range gv.DependsOnKeys {
					addKey(dep)
				}
				return
			}
			for _, depName := range gv.DependsOn {
				addName(depName)
			}
		}
	}
	for _, dep := range deps {
		addName(dep)
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]BindingVersionKey, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	slices.SortFunc(out, compareBindingVersionKey)
	return out
}

func (e *globalSeqEngine) bindingKeyForCurrentName(name string) (BindingVersionKey, bool) {
	if e == nil || name == "" {
		return BindingVersionKey{}, false
	}
	if binding := e.currentBindings[name]; binding != nil {
		return BindingVersionKeyForBinding(binding, name), true
	}
	if gv := e.globalVars[name]; gv != nil {
		return BindingVersionKeyForGlobalVar(gv, name), true
	}
	return BindingVersionKey{}, false
}

func (e *globalSeqEngine) publishGlobalVar(gv *GlobalVar) {
	if e == nil || gv == nil || gv.Name == "" {
		return
	}
	e.values[gv.Name] = gv.Value
	e.spans[gv.Name] = gv.Span
	e.rootFrame.AssignLocal(gv.Name, gv.Value, gv.Span)

	if _, ok := e.scalarSeed[gv.Name]; ok {
		e.scalarSeed[gv.Name] = gv.Value
		e.scalarSpans[gv.Name] = gv.Span
	}

	e.globalVars[gv.Name] = cloneGlobalVar(gv)
	if _, seen := e.globalOrderSeen[gv.Name]; !seen {
		e.globalOrderSeen[gv.Name] = struct{}{}
		e.globalOrder = append(e.globalOrder, gv.Name)
	}

	binding := bindingFromGlobalVar(gv.Name, gv)
	if binding == nil || isBuiltinGlobalName(gv.Name) {
		delete(e.currentBindings, gv.Name)
		return
	}
	binding.PublicName = gv.Name
	e.publishBinding(binding)
}

func (e *globalSeqEngine) publishBinding(binding *GlobalBinding) {
	if e == nil || binding == nil || binding.Name == "" {
		return
	}
	if binding.PublicName == "" {
		binding.PublicName = binding.Name
	}
	e.currentBindings[binding.Name] = cloneBinding(binding)
	if _, seen := e.currentBindingSeen[binding.Name]; !seen {
		e.currentBindingSeen[binding.Name] = struct{}{}
		e.currentBindingOrder = append(e.currentBindingOrder, binding.Name)
	}
}

func (e *globalSeqEngine) currentNameCatalog() *eval.NameCatalog {
	if e == nil {
		return nil
	}
	return scopeNameCatalog(visibleNamesFromEnv(e.values), e.namespaces)
}

func namespaceHead(name string) string {
	head, _, ok := strings.Cut(name, ".")
	if !ok {
		return ""
	}
	return head
}

func isBuiltinGlobalName(name string) bool {
	return name == "jbs_name" || name == "jbs_nproc"
}

func bindingDisplayName(binding *GlobalBinding) string {
	if binding == nil {
		return ""
	}
	if binding.PublicName != "" {
		return binding.PublicName
	}
	return binding.Name
}

func globalStepBlockKey(step globalInputStep) string {
	switch step.Kind {
	case globalInputDo:
		if step.DoBlock != nil {
			return doBlockSnapshotKey(*step.DoBlock)
		}
	case globalInputAnalyse:
		if step.AnalyseBlock != nil {
			return analyseBlockSnapshotKey(*step.AnalyseBlock)
		}
	}
	return ""
}

func doBlockSnapshotKey(block ast.DoBlock) string {
	return blockSnapshotKey("do", block.Name, block.Span)
}

func analyseBlockSnapshotKey(block ast.AnalyseBlock) string {
	return blockSnapshotKey("analyse", block.StepName, block.Span)
}

func blockSnapshotKey(kind string, name string, span diag.Span) string {
	return kind + "|" + name + "|" + span.File + "|" + fmt.Sprint(span.Start.Offset)
}

func errorCount(diags *diag.Diagnostics) int {
	if diags == nil {
		return 0
	}
	count := 0
	for _, item := range diags.Items {
		if item.Severity == diag.SeverityError {
			count++
		}
	}
	return count
}

func exprSpan(expr ast.Expr) diag.Span {
	if expr == nil {
		return diag.Span{}
	}
	return expr.GetSpan()
}

func uniqueSortedNamesExcept(names []string, except string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" || name == except {
			continue
		}
		seen[name] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(seen))
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
	case globalInputIf:
		if step.IfStmt != nil {
			return step.IfStmt.Span
		}
	case globalInputFor:
		if step.ForStmt != nil {
			return step.ForStmt.Span
		}
	case globalInputWhile:
		if step.WhileStmt != nil {
			return step.WhileStmt.Span
		}
	case globalInputBreak:
		if step.BreakStmt != nil {
			return step.BreakStmt.Span
		}
	case globalInputContinue:
		if step.ContinueStmt != nil {
			return step.ContinueStmt.Span
		}
	case globalInputProjectedImport:
		if step.Import != nil {
			return step.Import.Span
		}
	case globalInputDo:
		if step.DoBlock != nil {
			return step.DoBlock.Span
		}
	case globalInputAnalyse:
		if step.AnalyseBlock != nil {
			return step.AnalyseBlock.Span
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
		case ast.ListExpr:
			for _, item := range n.Items {
				walk(item)
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				walk(item)
			}
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			walkFuncBodyExprRefs(n.Body, walk)
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

func walkFuncBodyExprRefs(body []ast.FuncBodyStmt, walk func(ast.Expr)) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			walk(node.Expr)
		case ast.ReturnStmt:
			walk(node.Expr)
		case ast.ExprStmt:
			walk(node.Expr)
		case ast.FuncIfStmt:
			walk(node.Cond)
			walkFuncBodyExprRefs(node.Then, walk)
			walkFuncBodyExprRefs(node.Else, walk)
		case ast.FuncForStmt:
			walk(node.Iterable)
			walkFuncBodyExprRefs(node.Body, walk)
		case ast.FuncWhileStmt:
			walk(node.Cond)
			walkFuncBodyExprRefs(node.Body, walk)
		}
	}
}

func globalVarsFromExec(exec *globalExecResult) (map[string]*GlobalVar, []string) {
	if exec == nil {
		return map[string]*GlobalVar{}, nil
	}
	return cloneGlobalVars(exec.UserGlobalVarByName, exec.UserGlobalOrder)
}
