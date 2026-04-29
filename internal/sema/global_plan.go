package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

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
	globalInputNamespaceImport globalInputKind = "namespace_import"
	globalInputDo              globalInputKind = "do"
	globalInputSubmit          globalInputKind = "submit"
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
	Import         *projectedImport
	NamespaceScope *moduleScope
	DoBlock        *ast.DoBlock
	SubmitBlock    *ast.SubmitBlock
	AnalyseBlock   *ast.AnalyseBlock
	EffectiveExpr  ast.Expr
	Reads          []globalReadRef
	Index          int
	Names          *eval.NameCatalog
	ForwardVisible bool
	BaseDir        string
}

type globalPlan struct {
	Steps             []globalInputStep
	StepByName        map[string]int
	LocalVisibleNames []string
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

type programBindingPlan struct {
	AcceptedByIndex map[int]ast.GlobalAssign
	VisibleNames    []string
}

func buildGlobalPlan(prog ast.Program, baseSeed map[string]eval.Value, baseDir string, diags *diag.Diagnostics) *globalPlan {
	_ = diags
	prep := planProgramBindings(prog, diags)
	plan := &globalPlan{
		Steps:             make([]globalInputStep, 0, len(prog.Stmts)),
		StepByName:        make(map[string]int),
		LocalVisibleNames: append([]string(nil), prep.VisibleNames...),
	}
	for index, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			assignCopy := n
			effective := assignmentExpr(assignCopy.Name, assignCopy.Op, assignCopy.Expr, assignCopy.Span)
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:            id,
				Kind:          globalInputAssign,
				Name:          assignCopy.Name,
				Assign:        &assignCopy,
				EffectiveExpr: effective,
				Reads:         globalExprReadRefs(effective),
				Index:         index,
				BaseDir:       baseDir,
			})
			plan.StepByName[assignCopy.Name] = id
		case ast.ExprStmt:
			exprCopy := n
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:            id,
				Kind:          globalInputExpr,
				ExprStmt:      &exprCopy,
				EffectiveExpr: exprCopy.Expr,
				Reads:         globalExprReadRefs(exprCopy.Expr),
				Index:         index,
				BaseDir:       baseDir,
			})
		case ast.DoBlock:
			blockCopy := n
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:      id,
				Kind:    globalInputDo,
				Name:    blockCopy.Name,
				DoBlock: &blockCopy,
				Index:   index,
				BaseDir: baseDir,
			})
		case ast.SubmitBlock:
			blockCopy := n
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:          id,
				Kind:        globalInputSubmit,
				Name:        blockCopy.Name,
				SubmitBlock: &blockCopy,
				Index:       index,
				BaseDir:     baseDir,
			})
		case ast.AnalyseBlock:
			blockCopy := n
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:           id,
				Kind:         globalInputAnalyse,
				Name:         blockCopy.StepName,
				AnalyseBlock: &blockCopy,
				Index:        index,
				BaseDir:      baseDir,
			})
		}
	}
	assignGlobalPlanNameCatalogs(plan, baseSeed)
	return plan
}

func planProgramBindings(prog ast.Program, diags *diag.Diagnostics) programBindingPlan {
	_ = diags
	out := programBindingPlan{
		AcceptedByIndex: make(map[int]ast.GlobalAssign),
		VisibleNames:    make([]string, 0),
	}
	seen := make(map[string]struct{})
	for index, stmt := range prog.Stmts {
		assign, ok := stmt.(ast.GlobalAssign)
		if !ok {
			continue
		}
		out.AcceptedByIndex[index] = assign
		if _, exists := seen[assign.Name]; exists {
			continue
		}
		seen[assign.Name] = struct{}{}
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
	for i := range plan.Steps {
		plan.Steps[i].Reads = globalExprReadRefs(plan.Steps[i].EffectiveExpr)
		plan.Steps[i].Names = catalog
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
	engine := newGlobalSeqEngine(plan, generalSeed, scalarSeed, diags)
	engine.execute()
	return engine.res
}

type globalSeqEngine struct {
	plan                *globalPlan
	diags               *diag.Diagnostics
	rootFrame           *eval.Frame
	values              map[string]eval.Value
	modes               map[string]string
	spans               map[string]diag.Span
	scalarSeed          map[string]eval.Value
	scalarModes         map[string]string
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
			Modes:  make(map[string]string),
			Spans:  make(map[string]diag.Span),
		},
		UserGlobalVarByName:   make(map[string]*GlobalVar),
		UserGlobalOrder:       make([]string, 0),
		TopLevelExprs:         make([]TopLevelExprResult, 0),
		ScalarGlobals:         GlobalState{Values: maps.Clone(scalars), Modes: make(map[string]string), Spans: make(map[string]diag.Span)},
		SnapshotBindings:      make([]*GlobalBinding, 0),
		ScopeSnapshotsByIndex: make(map[int]*ScopeSnapshot),
		ScopeSnapshotsByBlock: make(map[string]*ScopeSnapshot),
	}
	return &globalSeqEngine{
		plan:                plan,
		diags:               diags,
		rootFrame:           eval.NewRootFrame(values),
		values:              values,
		modes:               make(map[string]string),
		spans:               make(map[string]diag.Span),
		scalarSeed:          scalars,
		scalarModes:         make(map[string]string),
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
	for _, step := range e.plan.Steps {
		switch step.Kind {
		case globalInputAssign:
			e.evalAssignStep(step)
		case globalInputProjectedImport:
			e.evalProjectedImportStep(step)
		case globalInputNamespaceImport:
			e.evalNamespaceImportStep(step)
		case globalInputExpr:
			e.evalExprStep(step)
		case globalInputDo, globalInputSubmit, globalInputAnalyse:
			e.recordDeclarationSnapshot(step)
		}
	}
	e.res.UserGlobals.Values = maps.Clone(e.values)
	e.res.UserGlobals.Modes = maps.Clone(e.modes)
	e.res.UserGlobals.Spans = maps.Clone(e.spans)
	e.res.UserGlobalVarByName, e.res.UserGlobalOrder = cloneGlobalVars(e.globalVars, e.globalOrder)
	e.res.ScalarGlobals = GlobalState{
		Values: maps.Clone(e.scalarSeed),
		Modes:  maps.Clone(e.scalarModes),
		Spans:  maps.Clone(e.scalarSpans),
	}
}

func (e *globalSeqEngine) evalAssignStep(step globalInputStep) {
	if step.Assign == nil {
		return
	}
	assign := *step.Assign
	effective := assignmentExpr(assign.Name, assign.Op, assign.Expr, assign.Span)
	warnModeExprInCollections(effective, e.diags)

	mode := ""
	expr := effective
	if assign.Op == "" || assign.Op == ast.AssignEq {
		if foundMode, inner, ok := unwrapModeExpr(assign.Expr); ok {
			mode = foundMode
			expr = inner
		}
	} else if prev := e.modes[assign.Name]; prev != "" {
		if _, ok := e.rootFrame.Read(assign.Name, assign.Span, e.diags); !ok {
			return
		}
		mode = prev
	} else {
		if _, ok := e.rootFrame.Read(assign.Name, assign.Span, e.diags); !ok {
			return
		}
	}

	before := errorCount(e.diags)
	value := eval.EvalExprWithOptions(expr, nil, e.diags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
		Names:                           e.currentNameCatalog(),
		Files:                           &eval.FileAccess{BaseDir: step.BaseDir},
		Frame:                           e.rootFrame,
	})
	if (assign.Op == "" || assign.Op == ast.AssignEq) && mode != "" {
		value = coerceModeValue(mode, value, assign.Span, e.diags)
	}
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

	orderNames, vars := globalVarSeries(assign.Name, value)
	gv := &GlobalVar{
		Name:      assign.Name,
		Value:     value,
		Mode:      mode,
		Span:      assign.Span,
		Order:     orderNames,
		Vars:      vars,
		DependsOn: e.expandGlobalDeps(globalExprDependencies(effective, assign.Name), assign.Name),
		VersionID: bindingVersionID(step),
	}
	if !e.acceptGlobalVar(gv) {
		return
	}
	e.publishGlobalVar(gv)
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
			Modes:  maps.Clone(e.modes),
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
	if gv.Name == "jbs_name" || gv.Name == "jbs_outpath" {
		if gv.Mode != "" {
			e.diags.AddError(
				diag.CodeE303,
				gv.Name+" must be a simple string, not shell()/python()",
				gv.Span,
				"assign a plain string literal",
			)
			return false
		}
		if gv.Value.Kind != eval.KindString {
			code := diag.CodeE301
			if gv.Name == "jbs_outpath" {
				code = diag.CodeE302
			}
			e.diags.AddError(
				code,
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
			"use string/int/float/bool or shell()/python() scalar values",
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

func (e *globalSeqEngine) publishGlobalVar(gv *GlobalVar) {
	if e == nil || gv == nil || gv.Name == "" {
		return
	}
	e.values[gv.Name] = gv.Value
	e.spans[gv.Name] = gv.Span
	if gv.Mode != "" {
		e.modes[gv.Name] = gv.Mode
	} else {
		delete(e.modes, gv.Name)
	}
	e.rootFrame.AssignLocal(gv.Name, gv.Value, gv.Span)

	if _, ok := e.scalarSeed[gv.Name]; ok {
		e.scalarSeed[gv.Name] = gv.Value
		e.scalarSpans[gv.Name] = gv.Span
		if gv.Mode != "" {
			e.scalarModes[gv.Name] = gv.Mode
		} else {
			delete(e.scalarModes, gv.Name)
		}
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
	return name == "jbs_name" || name == "jbs_outpath" || name == "jbs_comment"
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
	case globalInputSubmit:
		if step.SubmitBlock != nil {
			return submitBlockSnapshotKey(*step.SubmitBlock)
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

func submitBlockSnapshotKey(block ast.SubmitBlock) string {
	return blockSnapshotKey("submit", block.Name, block.Span)
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
	case globalInputDo:
		if step.DoBlock != nil {
			return step.DoBlock.Span
		}
	case globalInputSubmit:
		if step.SubmitBlock != nil {
			return step.SubmitBlock.Span
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
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			for _, stmt := range n.Body {
				switch node := stmt.(type) {
				case ast.LocalAssignStmt:
					walk(node.Expr)
				case ast.ReturnStmt:
					walk(node.Expr)
				case ast.ExprStmt:
					walk(node.Expr)
				}
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
	return cloneGlobalVars(exec.UserGlobalVarByName, exec.UserGlobalOrder)
}
