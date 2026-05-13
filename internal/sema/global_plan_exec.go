package sema

import (
	"maps"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type globalExecOptions struct {
	CollectPrints bool
	ShellRunner   eval.ShellRunner
	ShellMode     eval.ShellMode
	Environ       func() []string
}

func execGlobalPlan(plan *globalPlan, generalSeed map[string]eval.Value, scalarSeed map[string]eval.Value, diags *diag.Diagnostics) *globalExecResult {
	return execGlobalPlanWithOptions(plan, generalSeed, scalarSeed, globalExecOptions{}, diags)
}

func execGlobalPlanWithOptions(plan *globalPlan, generalSeed map[string]eval.Value, scalarSeed map[string]eval.Value, opts globalExecOptions, diags *diag.Diagnostics) *globalExecResult {
	if plan == nil {
		plan = &globalPlan{
			Steps:             make([]globalInputStep, 0),
			StepByName:        make(map[string]int),
			LocalVisibleNames: make([]string, 0),
		}
	}
	engine := newGlobalSeqEngine(plan, generalSeed, scalarSeed, opts, diags)
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
	collectPrints       bool
	shellRunner         eval.ShellRunner
	shellMode           eval.ShellMode
	environ             func() []string
	shellUses           []string
	outputSeq           int
	res                 *globalExecResult
}

func newGlobalSeqEngine(plan *globalPlan, generalSeed map[string]eval.Value, scalarSeed map[string]eval.Value, opts globalExecOptions, diags *diag.Diagnostics) *globalSeqEngine {
	values := cloneValueMap(generalSeed)
	if values == nil {
		values = map[string]eval.Value{}
	}
	scalars := cloneValueMap(scalarSeed)
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
		PrintEvents:           make([]PrintEvent, 0),
		ScalarGlobals:         GlobalState{Values: cloneValueMap(scalars), Spans: make(map[string]diag.Span)},
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
		collectPrints:       opts.CollectPrints,
		shellRunner:         opts.ShellRunner,
		shellMode:           opts.ShellMode,
		environ:             opts.Environ,
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
	e.res.UserGlobals.Values = cloneValueMap(e.values)
	e.res.UserGlobals.Spans = maps.Clone(e.spans)
	e.res.UserGlobalVarByName, e.res.UserGlobalOrder = cloneGlobalVars(e.globalVars, e.globalOrder)
	e.res.ScalarGlobals = GlobalState{
		Values: cloneValueMap(e.scalarSeed),
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

func (e *globalSeqEngine) evalOptions(step globalInputStep) eval.ExprOptions {
	opts := eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           e.currentNameCatalog(),
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.rootFrame,
		ShellRunner:                     e.shellRunner,
		ShellMode:                       e.shellMode,
		ShellUse:                        e.recordShellUse,
		Environ:                         e.environ,
		DeleteName:                      e.deleteGlobalName,
	}
	if e.collectPrints {
		opts.Print = e.recordPrintEvent
		opts.PrintIndex = step.Index
		opts.NextPrintSeq = e.nextOutputSeq
	}
	return opts
}

func (e *globalSeqEngine) nextOutputSeq() int {
	e.outputSeq++
	return e.outputSeq
}

func (e *globalSeqEngine) recordPrintEvent(event eval.PrintEvent) {
	e.res.PrintEvents = append(e.res.PrintEvents, PrintEvent{
		Index:  event.Index,
		Seq:    event.Seq,
		Span:   event.Span,
		Values: eval.CloneValues(event.Values),
	})
}

func (e *globalSeqEngine) recordShellUse(event eval.ShellUseEvent) {
	if event.Name == "" {
		return
	}
	e.shellUses = append(e.shellUses, event.Name)
}

func (e *globalSeqEngine) takeShellUses() []string {
	uses := uniqueSortedNamesExcept(e.shellUses, "")
	e.shellUses = nil
	return uses
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
	prevSelfDeps := e.previousBindingDependencySnapshot(assign.Name)
	exprReadsSelf := exprEvalTimeReadsName(effective, assign.Name)
	if assign.Op != "" && assign.Op != ast.AssignEq {
		if _, ok := e.rootFrame.Read(assign.Name, assign.Span, e.diags); !ok {
			return
		}
	}

	before := errorCount(e.diags)
	value := eval.EvalExprWithOptions(effective, nil, e.diags, e.evalOptions(step))
	shellDeps := e.takeShellUses()
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
	directDeps = append(directDeps, shellDeps...)
	directDeps = append(directDeps, guardDeps...)
	directDeps = uniqueSortedNamesExcept(directDeps, assign.Name)
	depNames := e.expandGlobalDeps(directDeps, assign.Name)
	depKeys := e.expandGlobalDepKeys(directDeps, assign.Name)
	if exprReadsSelf || slices.Contains(shellDeps, assign.Name) || slices.Contains(guardDeps, assign.Name) {
		depNames = uniqueSortedNamesExcept(append(depNames, prevSelfDeps.Names...), assign.Name)
		depKeys = uniqueSortedBindingVersionKeys(append(depKeys, prevSelfDeps.Keys...))
	}
	orderNames, vars := globalVarSeries(assign.Name, value)
	gv := &GlobalVar{
		Name:          assign.Name,
		Value:         value,
		Span:          assign.Span,
		Order:         orderNames,
		Vars:          vars,
		DependsOn:     depNames,
		DependsOnKeys: depKeys,
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
	cond, ok := eval.EvalBoolCondition(step.IfStmt.Cond, nil, e.diags, e.evalOptions(step))
	shellDeps := e.takeShellUses()
	if !ok {
		return globalStepResult{}
	}
	checkedDeps := append([]string(nil), guardDeps...)
	checkedDeps = append(checkedDeps, globalExprDependencies(step.IfStmt.Cond, "")...)
	checkedDeps = append(checkedDeps, shellDeps...)
	checkedDeps = uniqueSortedNamesExcept(checkedDeps, "")
	if cond {
		return e.executeSteps(step.Then, checkedDeps)
	}
	for _, branch := range step.Elifs {
		branchCond, ok := eval.EvalBoolConditionFor("elif", branch.Cond, nil, e.diags, e.evalOptions(step))
		branchShellDeps := e.takeShellUses()
		if !ok {
			return globalStepResult{}
		}
		checkedDeps = append(checkedDeps, globalExprDependencies(branch.Cond, "")...)
		checkedDeps = append(checkedDeps, branchShellDeps...)
		checkedDeps = uniqueSortedNamesExcept(checkedDeps, "")
		if branchCond {
			return e.executeSteps(branch.Body, checkedDeps)
		}
	}
	return e.executeSteps(step.Else, checkedDeps)
}

func (e *globalSeqEngine) evalForStep(step globalInputStep, guardDeps []string) globalStepResult {
	if step.ForStmt == nil {
		return globalStepResult{}
	}
	iterable := eval.EvalExprWithOptions(step.ForStmt.Iterable, nil, e.diags, e.evalOptions(step))
	shellDeps := e.takeShellUses()
	items, ok := eval.IterableElements(iterable, exprSpan(step.ForStmt.Iterable), e.diags)
	if !ok {
		return globalStepResult{}
	}
	loopDeps := append([]string(nil), guardDeps...)
	loopDeps = append(loopDeps, globalExprDependencies(step.ForStmt.Iterable, "")...)
	loopDeps = append(loopDeps, shellDeps...)
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
		cond, ok := eval.EvalBoolConditionFor("while", step.WhileStmt.Cond, nil, e.diags, e.evalOptions(step))
		shellDeps := e.takeShellUses()
		if !ok || !cond {
			return globalStepResult{}
		}
		iterDeps := append([]string(nil), loopDeps...)
		iterDeps = append(iterDeps, shellDeps...)
		iterDeps = uniqueSortedNamesExcept(iterDeps, "")
		result := e.executeSteps(step.Body, iterDeps)
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
	beforeSeq := e.outputSeq
	value := eval.EvalExprWithOptions(step.ExprStmt.Expr, nil, e.diags, e.evalOptions(step))
	e.takeShellUses()
	echo := true
	if value.Kind == eval.KindNull && (e.outputSeq > beforeSeq || isDeleteCallExpr(step.ExprStmt.Expr)) {
		echo = false
	}
	e.res.TopLevelExprs = append(e.res.TopLevelExprs, TopLevelExprResult{
		Index: step.Index,
		Seq:   e.nextOutputSeq(),
		Span:  step.ExprStmt.Span,
		Value: value,
		Echo:  echo,
	})
}
