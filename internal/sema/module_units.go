package sema

import (
	"maps"
	"slices"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/imports"
)

type moduleScope struct {
	Ref                   imports.ModuleRef
	Program               ast.Program
	BaseDirByFile         map[string]string
	Globals               GlobalState
	GlobalVarByName       map[string]*GlobalVar
	GlobalVarOrder        []string
	TopLevelExprs         []TopLevelExprResult
	LocalExportsByName    map[string]*GlobalVar
	ExportsByName         map[string]*GlobalVar
	LocalBindings         []*GlobalBinding
	LocalBindingsByName   map[string]*GlobalBinding
	Bindings              []*GlobalBinding
	BindingsByName        map[string]*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
	DoBlocks              []ast.DoBlock
	Submits               []ast.SubmitBlock
	AnalyseBlocks         []ast.AnalyseBlock
	StepOrder             []string
	Namespaces            map[string]*Namespace
	Env                   map[string]eval.Value
}

func buildEntryModuleScope(loadRes *imports.LoadResult, globals map[string]eval.Value, diags *diag.Diagnostics) *moduleScope {
	if loadRes == nil {
		return emptyModuleScope()
	}
	return compileModule(loadRes.Entry, loadRes, globals, diags, map[string]*moduleScope{}, map[string]bool{})
}

func compileModule(ref imports.ModuleRef, loadRes *imports.LoadResult, globals map[string]eval.Value, diags *diag.Diagnostics, cache map[string]*moduleScope, visiting map[string]bool) *moduleScope {
	if loadRes == nil || ref.ID == "" {
		return emptyModuleScope()
	}
	if cached := cache[ref.ID]; cached != nil {
		return cloneModuleScope(cached)
	}
	if visiting[ref.ID] {
		return emptyModuleScope()
	}
	info := loadRes.Modules[ref.ID]
	if info == nil {
		return emptyModuleScope()
	}

	visiting[ref.ID] = true
	defer delete(visiting, ref.ID)

	childByIndex := make(map[int]*moduleScope, len(info.Uses))
	prefixedByIndex := make(map[int]*moduleScope, len(info.Uses))
	for _, use := range info.Uses {
		child := compileModule(use.Source, loadRes, globals, diags, cache, visiting)
		childByIndex[use.Index] = child
		if use.Kind == imports.UseNamespace {
			prefixedByIndex[use.Index] = prefixModuleScope(child, use.Alias)
		}
	}

	exec := execGlobalPlan(buildModuleGlobalPlan(info, childByIndex, prefixedByIndex, globals, diags), globals, globals, diags)
	scope := emptyModuleScope()
	scope.Ref = ref
	scope.Program = info.Program
	if strings.TrimSpace(info.Program.File) != "" && strings.TrimSpace(info.BaseDir) != "" {
		scope.BaseDirByFile[info.Program.File] = info.BaseDir
	}
	scope.Globals = GlobalState{
		Values: maps.Clone(exec.ScalarGlobals.Values),
		Modes:  maps.Clone(exec.ScalarGlobals.Modes),
		Spans:  maps.Clone(exec.ScalarGlobals.Spans),
	}
	scope.GlobalVarByName, scope.GlobalVarOrder = globalVarsFromExec(exec)
	for _, name := range scope.GlobalVarOrder {
		gv := scope.GlobalVarByName[name]
		registerModuleExport(scope, name, gv, gv != nil && gv.Namespace == "" && !isBuiltinGlobalName(name))
		binding := bindingFromGlobalVar(name, gv)
		if binding == nil || isBuiltinGlobalName(name) {
			continue
		}
		registerModuleBinding(scope, binding, gv != nil && gv.Namespace == "")
	}
	registerSnapshotBindings(scope, exec.SnapshotBindings)
	scope.TopLevelExprs = cloneTopLevelExprResults(exec.TopLevelExprs)
	scope.ScopeSnapshotsByIndex = cloneScopeSnapshotsByIndex(exec.ScopeSnapshotsByIndex)
	scope.ScopeSnapshotsByBlock = cloneScopeSnapshotsByBlock(exec.ScopeSnapshotsByBlock)

	for _, use := range info.Uses {
		if use.Kind != imports.UseNamespace {
			continue
		}
		mergeModuleScope(scope, prefixedByIndex[use.Index])
	}

	for _, stmt := range info.Program.Stmts {
		switch n := stmt.(type) {
		case ast.DoBlock:
			scope.DoBlocks = append(scope.DoBlocks, n)
			scope.StepOrder = appendUniqueString(scope.StepOrder, n.Name)
		case ast.SubmitBlock:
			scope.Submits = append(scope.Submits, n)
			scope.StepOrder = appendUniqueString(scope.StepOrder, n.Name)
		case ast.AnalyseBlock:
			scope.AnalyseBlocks = append(scope.AnalyseBlocks, n)
		}
	}

	materializeModuleFunctionExports(scope)
	mergeGlobalVarsIntoState(&scope.Globals, scope.ExportsByName)
	cache[ref.ID] = cloneModuleScope(scope)
	return cloneModuleScope(scope)
}

func buildModuleGlobalPlan(info *imports.ModuleInfo, childByIndex map[int]*moduleScope, prefixedByIndex map[int]*moduleScope, baseSeed map[string]eval.Value, diags *diag.Diagnostics) *globalPlan {
	prep := planModuleBindings(info, childByIndex, diags)
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
	for index, stmt := range info.Program.Stmts {
		if use, ok := useByIndex[index]; ok {
			if use.Kind == imports.UseNamespace {
				id := len(plan.Steps)
				plan.Steps = append(plan.Steps, globalInputStep{
					ID:             id,
					Kind:           globalInputNamespaceImport,
					NamespaceScope: prefixedByIndex[index],
					Index:          index,
					BaseDir:        info.BaseDir,
				})
				continue
			}
			for _, name := range use.Names {
				imp := prep.AcceptedImports[projectedImportDecisionKey{Index: index, Name: name}]
				if imp == nil {
					continue
				}
				id := len(plan.Steps)
				plan.Steps = append(plan.Steps, globalInputStep{
					ID:      id,
					Kind:    globalInputProjectedImport,
					Name:    name,
					Import:  imp,
					Index:   index,
					BaseDir: info.BaseDir,
				})
				plan.StepByName[name] = id
			}
			continue
		}
		switch n := stmt.(type) {
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
				BaseDir:       info.BaseDir,
			})
		case ast.GlobalAssign:
			assign, ok := prep.AcceptedLocals[index]
			if !ok {
				continue
			}
			assignCopy := assign
			effective := assignmentExpr(assignCopy.Name, assignCopy.Op, assignCopy.Expr, assignCopy.Span)
			id := len(plan.Steps)
			step := globalInputStep{
				ID:            id,
				Kind:          globalInputAssign,
				Name:          assign.Name,
				Assign:        &assignCopy,
				EffectiveExpr: effective,
				Reads:         globalExprReadRefs(effective),
				Index:         index,
				BaseDir:       info.BaseDir,
			}
			plan.Steps = append(plan.Steps, step)
			plan.StepByName[step.Name] = id
		case ast.DoBlock:
			blockCopy := n
			id := len(plan.Steps)
			plan.Steps = append(plan.Steps, globalInputStep{
				ID:      id,
				Kind:    globalInputDo,
				Name:    blockCopy.Name,
				DoBlock: &blockCopy,
				Index:   index,
				BaseDir: info.BaseDir,
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
				BaseDir:     info.BaseDir,
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
				BaseDir:      info.BaseDir,
			})
		}
	}
	assignGlobalPlanNameCatalogs(plan, baseSeed)
	return plan
}

type projectedImportDecisionKey struct {
	Index int
	Name  string
}

type moduleBindingPlan struct {
	AcceptedLocals    map[int]ast.GlobalAssign
	AcceptedImports   map[projectedImportDecisionKey]*projectedImport
	LocalVisibleNames []string
}

func planModuleBindings(info *imports.ModuleInfo, childByIndex map[int]*moduleScope, diags *diag.Diagnostics) moduleBindingPlan {
	out := moduleBindingPlan{
		AcceptedLocals:    make(map[int]ast.GlobalAssign),
		AcceptedImports:   make(map[projectedImportDecisionKey]*projectedImport),
		LocalVisibleNames: make([]string, 0),
	}
	if info == nil {
		return out
	}
	useByIndex := make(map[int]imports.ResolvedUse, len(info.Uses))
	aliasSpans := make(map[string]diag.Span)
	for _, use := range info.Uses {
		useByIndex[use.Index] = use
		if use.Kind == imports.UseNamespace && strings.TrimSpace(use.Alias) != "" {
			if _, exists := aliasSpans[use.Alias]; !exists {
				aliasSpans[use.Alias] = use.Span
			}
		}
	}
	nonGlobalSymbols := collectModuleNonGlobalSymbols(info.Program)
	for index, stmt := range info.Program.Stmts {
		if use, ok := useByIndex[index]; ok {
			if use.Kind == imports.UseNamespace {
				continue
			}
			for _, name := range use.Names {
				if span, exists := aliasSpans[name]; exists {
					diags.AddError(
						diag.CodeE534,
						"import name collision: projected global '"+name+"' conflicts with module alias",
						use.Span,
						"rename the alias or imported global",
						diag.RelatedSpan{Message: "conflicting alias", Span: span},
					)
					continue
				}
				if span, exists := nonGlobalSymbols[name]; exists {
					diags.AddError(
						diag.CodeE534,
						"import name collision: projected global '"+name+"' conflicts with local step/submit symbol",
						use.Span,
						"rename the imported global or conflicting symbol",
						diag.RelatedSpan{Message: "conflicting symbol", Span: span},
					)
					continue
				}
				child := childByIndex[index]
				exported := (*GlobalVar)(nil)
				if child != nil {
					exported = child.LocalExportsByName[name]
				}
				if exported == nil {
					switch moduleLocalSymbolKind(moduleProgram(child, info, index), name) {
					case localSymbolDo, localSymbolSubmit, localSymbolAnalyse:
						diags.AddError(
							diag.CodeE533,
							"symbol '"+name+"' in module '"+use.Source.Label+"' is not importable",
							use.Span,
							"only globals are selectively importable",
						)
					default:
						if moduleLocalSymbolKind(moduleProgram(child, info, index), name) != localSymbolGlobal {
							diags.AddError(
								diag.CodeE532,
								"unknown symbol '"+name+"' in module '"+use.Source.Label+"'",
								use.Span,
								"import a global that exists in the source module",
							)
						}
					}
					continue
				}
				out.AcceptedImports[projectedImportDecisionKey{Index: index, Name: name}] = &projectedImport{
					LocalName:    name,
					SourceName:   name,
					SourceGlobal: exported,
					Span:         use.Span,
				}
			}
			continue
		}
		assign, ok := stmt.(ast.GlobalAssign)
		if !ok {
			continue
		}
		out.AcceptedLocals[index] = assign
		if !slices.Contains(out.LocalVisibleNames, assign.Name) {
			out.LocalVisibleNames = append(out.LocalVisibleNames, assign.Name)
		}
	}
	return out
}

type localSymbolKind int

const (
	localSymbolNone localSymbolKind = iota
	localSymbolGlobal
	localSymbolDo
	localSymbolSubmit
	localSymbolAnalyse
)

func moduleProgram(child *moduleScope, info *imports.ModuleInfo, index int) ast.Program {
	if child != nil {
		return child.Program
	}
	if info == nil {
		return ast.Program{}
	}
	useByIndex := make(map[int]imports.ResolvedUse, len(info.Uses))
	for _, use := range info.Uses {
		useByIndex[use.Index] = use
	}
	use := useByIndex[index]
	return ast.Program{File: use.Source.Label}
}

func moduleLocalSymbolKind(prog ast.Program, name string) localSymbolKind {
	if strings.TrimSpace(name) == "" {
		return localSymbolNone
	}
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			if n.Name == name {
				return localSymbolGlobal
			}
		case ast.DoBlock:
			if n.Name == name {
				return localSymbolDo
			}
		case ast.SubmitBlock:
			if n.Name == name {
				return localSymbolSubmit
			}
		case ast.AnalyseBlock:
			if n.StepName == name {
				return localSymbolAnalyse
			}
		}
	}
	return localSymbolNone
}

func collectModuleNonGlobalSymbols(prog ast.Program) map[string]diag.Span {
	out := make(map[string]diag.Span)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.DoBlock:
			if _, exists := out[n.Name]; !exists {
				out[n.Name] = n.Span
			}
		case ast.SubmitBlock:
			if _, exists := out[n.Name]; !exists {
				out[n.Name] = n.Span
			}
		}
	}
	return out
}

func prefixModuleScope(scope *moduleScope, prefix string) *moduleScope {
	if scope == nil || strings.TrimSpace(prefix) == "" {
		return cloneModuleScope(scope)
	}
	out := emptyModuleScope()
	out.BaseDirByFile = maps.Clone(scope.BaseDirByFile)
	for name, exported := range scope.ExportsByName {
		if exported == nil {
			continue
		}
		prefixedName := prefix + "." + name
		next := cloneGlobalVar(exported)
		next.Name = prefixedName
		next.DependsOn = prefixNames(prefix, exported.DependsOn)
		out.ExportsByName[prefixedName] = next
		out.Env[prefixedName] = next.Value
	}
	for name, exported := range scope.LocalExportsByName {
		if exported == nil {
			continue
		}
		prefixedName := prefix + "." + name
		next := cloneGlobalVar(exported)
		next.Name = prefixedName
		next.DependsOn = prefixNames(prefix, exported.DependsOn)
		out.LocalExportsByName[prefixedName] = next
	}
	for _, binding := range scope.Bindings {
		if binding == nil {
			continue
		}
		prefixedName := prefix + "." + binding.Name
		next := cloneBinding(binding)
		next.Name = prefixedName
		next.DependsOn = prefixNames(prefix, binding.DependsOn)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[prefixedName] = next
	}
	for _, block := range scope.DoBlocks {
		out.DoBlocks = append(out.DoBlocks, prefixDoBlock(block, prefix))
	}
	for _, block := range scope.Submits {
		out.Submits = append(out.Submits, prefixSubmitBlock(block, prefix))
	}
	for index, snap := range scope.ScopeSnapshotsByIndex {
		out.ScopeSnapshotsByIndex[index] = prefixScopeSnapshot(snap, prefix)
	}
	for key, snap := range scope.ScopeSnapshotsByBlock {
		out.ScopeSnapshotsByBlock[prefixSnapshotBlockKey(key, prefix)] = prefixScopeSnapshot(snap, prefix)
	}
	for _, stepName := range scope.StepOrder {
		out.StepOrder = append(out.StepOrder, prefix+"."+stepName)
	}
	out.Namespaces[prefix] = &Namespace{Name: prefix}
	for name, ns := range scope.Namespaces {
		q := prefix + "." + name
		out.Namespaces[q] = &Namespace{
			Name:     q,
			Members:  prefixNames(prefix, ns.Members),
			Bindings: prefixNames(prefix, ns.Bindings),
			Steps:    prefixNames(prefix, ns.Steps),
		}
	}
	for name := range out.ExportsByName {
		head, _, ok := strings.Cut(name, ".")
		if !ok {
			continue
		}
		ns := out.Namespaces[head]
		if ns == nil {
			ns = &Namespace{Name: head}
			out.Namespaces[head] = ns
		}
		ns.Members = appendUniqueString(ns.Members, name)
	}
	for _, binding := range out.Bindings {
		head, _, ok := strings.Cut(binding.Name, ".")
		if !ok {
			continue
		}
		ns := out.Namespaces[head]
		if ns == nil {
			ns = &Namespace{Name: head}
			out.Namespaces[head] = ns
		}
		ns.Bindings = appendUniqueString(ns.Bindings, binding.Name)
	}
	for _, stepName := range out.StepOrder {
		head, _, ok := strings.Cut(stepName, ".")
		if !ok {
			continue
		}
		ns := out.Namespaces[head]
		if ns == nil {
			ns = &Namespace{Name: head}
			out.Namespaces[head] = ns
		}
		ns.Steps = appendUniqueString(ns.Steps, stepName)
	}
	return out
}

func mergeModuleScope(dst *moduleScope, src *moduleScope) {
	if dst == nil || src == nil {
		return
	}
	for file, baseDir := range src.BaseDirByFile {
		if strings.TrimSpace(file) == "" || strings.TrimSpace(baseDir) == "" {
			continue
		}
		if _, exists := dst.BaseDirByFile[file]; !exists {
			dst.BaseDirByFile[file] = baseDir
		}
	}
	for name, exported := range src.ExportsByName {
		if exported == nil {
			continue
		}
		if _, exists := dst.ExportsByName[name]; exists {
			continue
		}
		next := cloneGlobalVar(exported)
		dst.ExportsByName[name] = next
		dst.Env[name] = next.Value
	}
	for _, binding := range src.Bindings {
		if binding == nil {
			continue
		}
		if _, exists := dst.BindingsByName[binding.Name]; exists {
			continue
		}
		next := cloneBinding(binding)
		dst.Bindings = append(dst.Bindings, next)
		dst.BindingsByName[next.Name] = next
		dst.Env[next.Name] = next.Value
	}
	for _, block := range src.DoBlocks {
		if containsStepName(dst.DoBlocks, block.Name) {
			continue
		}
		dst.DoBlocks = append(dst.DoBlocks, block)
	}
	for _, block := range src.Submits {
		if containsSubmitName(dst.Submits, block.Name) {
			continue
		}
		dst.Submits = append(dst.Submits, block)
	}
	for _, stepName := range src.StepOrder {
		dst.StepOrder = appendUniqueString(dst.StepOrder, stepName)
	}
	for name, ns := range src.Namespaces {
		current := dst.Namespaces[name]
		if current == nil {
			current = &Namespace{Name: name}
			dst.Namespaces[name] = current
		}
		current.Members = mergeUniqueStrings(current.Members, ns.Members)
		current.Bindings = mergeUniqueStrings(current.Bindings, ns.Bindings)
		current.Steps = mergeUniqueStrings(current.Steps, ns.Steps)
	}
	for index, snap := range src.ScopeSnapshotsByIndex {
		if _, exists := dst.ScopeSnapshotsByIndex[index]; !exists {
			dst.ScopeSnapshotsByIndex[index] = cloneScopeSnapshot(snap)
		}
	}
	for key, snap := range src.ScopeSnapshotsByBlock {
		if _, exists := dst.ScopeSnapshotsByBlock[key]; !exists {
			dst.ScopeSnapshotsByBlock[key] = cloneScopeSnapshot(snap)
		}
	}
}

func emptyModuleScope() *moduleScope {
	return &moduleScope{
		Globals:               GlobalState{Values: map[string]eval.Value{}, Modes: map[string]string{}, Spans: map[string]diag.Span{}},
		GlobalVarByName:       make(map[string]*GlobalVar),
		GlobalVarOrder:        make([]string, 0),
		TopLevelExprs:         make([]TopLevelExprResult, 0),
		LocalExportsByName:    make(map[string]*GlobalVar),
		ExportsByName:         make(map[string]*GlobalVar),
		LocalBindings:         make([]*GlobalBinding, 0),
		LocalBindingsByName:   make(map[string]*GlobalBinding),
		Bindings:              make([]*GlobalBinding, 0),
		BindingsByName:        make(map[string]*GlobalBinding),
		ScopeSnapshotsByIndex: make(map[int]*ScopeSnapshot),
		ScopeSnapshotsByBlock: make(map[string]*ScopeSnapshot),
		BaseDirByFile:         make(map[string]string),
		DoBlocks:              make([]ast.DoBlock, 0),
		Submits:               make([]ast.SubmitBlock, 0),
		AnalyseBlocks:         make([]ast.AnalyseBlock, 0),
		StepOrder:             make([]string, 0),
		Namespaces:            make(map[string]*Namespace),
		Env:                   make(map[string]eval.Value),
	}
}

func cloneModuleScope(scope *moduleScope) *moduleScope {
	if scope == nil {
		return emptyModuleScope()
	}
	out := emptyModuleScope()
	out.Ref = scope.Ref
	out.Program = scope.Program
	out.BaseDirByFile = maps.Clone(scope.BaseDirByFile)
	out.Globals = GlobalState{
		Values: maps.Clone(scope.Globals.Values),
		Modes:  maps.Clone(scope.Globals.Modes),
		Spans:  maps.Clone(scope.Globals.Spans),
	}
	out.GlobalVarByName, out.GlobalVarOrder = cloneGlobalVars(scope.GlobalVarByName, scope.GlobalVarOrder)
	out.TopLevelExprs = cloneTopLevelExprResults(scope.TopLevelExprs)
	out.DoBlocks = append([]ast.DoBlock(nil), scope.DoBlocks...)
	out.Submits = append([]ast.SubmitBlock(nil), scope.Submits...)
	out.AnalyseBlocks = append([]ast.AnalyseBlock(nil), scope.AnalyseBlocks...)
	out.StepOrder = append([]string(nil), scope.StepOrder...)
	out.Env = maps.Clone(scope.Env)
	for name, exported := range scope.LocalExportsByName {
		out.LocalExportsByName[name] = cloneGlobalVar(exported)
	}
	for name, exported := range scope.ExportsByName {
		out.ExportsByName[name] = cloneGlobalVar(exported)
	}
	for _, binding := range scope.LocalBindings {
		next := cloneBinding(binding)
		out.LocalBindings = append(out.LocalBindings, next)
		out.LocalBindingsByName[next.Name] = next
	}
	for _, binding := range scope.Bindings {
		next := cloneBinding(binding)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[next.Name] = next
	}
	out.ScopeSnapshotsByIndex = cloneScopeSnapshotsByIndex(scope.ScopeSnapshotsByIndex)
	out.ScopeSnapshotsByBlock = cloneScopeSnapshotsByBlock(scope.ScopeSnapshotsByBlock)
	for name, ns := range scope.Namespaces {
		out.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Members:  append([]string(nil), ns.Members...),
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	return out
}

func cloneGlobalVars(byName map[string]*GlobalVar, order []string) (map[string]*GlobalVar, []string) {
	out := make(map[string]*GlobalVar, len(byName))
	for name, gv := range byName {
		if gv == nil {
			continue
		}
		next := *gv
		next.Order = append([]string(nil), gv.Order...)
		next.Vars = cloneSeriesMap(gv.Vars)
		next.DependsOn = append([]string(nil), gv.DependsOn...)
		out[name] = &next
	}
	return out, slices.Clone(order)
}

func cloneBinding(binding *GlobalBinding) *GlobalBinding {
	if binding == nil {
		return nil
	}
	next := *binding
	next.Order = append([]string(nil), binding.Order...)
	next.Origins = maps.Clone(binding.Origins)
	next.Modes = maps.Clone(binding.Modes)
	next.Vars = cloneSeriesMap(binding.Vars)
	next.BaseVars = cloneSeriesMap(binding.BaseVars)
	next.Rows = cloneCombRows(binding.Rows, binding.Span)
	next.DependsOn = append([]string(nil), binding.DependsOn...)
	return &next
}

func cloneScopeSnapshot(snap *ScopeSnapshot) *ScopeSnapshot {
	if snap == nil {
		return nil
	}
	out := &ScopeSnapshot{
		Index: snap.Index,
		Globals: GlobalState{
			Values: maps.Clone(snap.Globals.Values),
			Modes:  maps.Clone(snap.Globals.Modes),
			Spans:  maps.Clone(snap.Globals.Spans),
		},
		Bindings:       make([]*GlobalBinding, 0, len(snap.Bindings)),
		BindingsByName: make(map[string]*GlobalBinding, len(snap.BindingsByName)),
		Namespaces:     make(map[string]*Namespace, len(snap.Namespaces)),
	}
	out.GlobalVarByName, out.GlobalVarOrder = cloneGlobalVars(snap.GlobalVarByName, snap.GlobalVarOrder)
	for _, binding := range snap.Bindings {
		next := cloneBinding(binding)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[next.Name] = next
		if next.PublicName != "" {
			out.BindingsByName[next.PublicName] = next
		}
	}
	for name, binding := range snap.BindingsByName {
		if binding == nil {
			continue
		}
		if existing := out.BindingsByName[binding.Name]; existing != nil {
			out.BindingsByName[name] = existing
			continue
		}
		out.BindingsByName[name] = cloneBinding(binding)
	}
	for name, ns := range snap.Namespaces {
		if ns == nil {
			continue
		}
		out.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Members:  append([]string(nil), ns.Members...),
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	return out
}

func cloneScopeSnapshotsByIndex(in map[int]*ScopeSnapshot) map[int]*ScopeSnapshot {
	out := make(map[int]*ScopeSnapshot, len(in))
	for index, snap := range in {
		out[index] = cloneScopeSnapshot(snap)
	}
	return out
}

func cloneScopeSnapshotsByBlock(in map[string]*ScopeSnapshot) map[string]*ScopeSnapshot {
	out := make(map[string]*ScopeSnapshot, len(in))
	for key, snap := range in {
		out[key] = cloneScopeSnapshot(snap)
	}
	return out
}

func cloneGlobalVar(gv *GlobalVar) *GlobalVar {
	if gv == nil {
		return nil
	}
	next := *gv
	next.Order = append([]string(nil), gv.Order...)
	next.Vars = cloneSeriesMap(gv.Vars)
	next.DependsOn = append([]string(nil), gv.DependsOn...)
	return &next
}

func cloneTopLevelExprResults(in []TopLevelExprResult) []TopLevelExprResult {
	if len(in) == 0 {
		return []TopLevelExprResult{}
	}
	out := make([]TopLevelExprResult, len(in))
	copy(out, in)
	return out
}

func mergeValueEnv(base map[string]eval.Value, extras map[string]eval.Value) map[string]eval.Value {
	out := make(map[string]eval.Value, len(base)+len(extras))
	for name, value := range base {
		out[name] = value
	}
	for name, value := range extras {
		out[name] = value
	}
	return out
}

func registerModuleExport(scope *moduleScope, name string, gv *GlobalVar, local bool) {
	if scope == nil || gv == nil || strings.TrimSpace(name) == "" {
		return
	}
	next := cloneGlobalVar(gv)
	next.Name = name
	if local {
		scope.LocalExportsByName[name] = next
	}
	if _, exists := scope.ExportsByName[name]; !exists {
		scope.ExportsByName[name] = next
	}
	scope.Env[name] = next.Value
}

func registerModuleBinding(scope *moduleScope, binding *GlobalBinding, local bool) {
	if scope == nil || binding == nil || strings.TrimSpace(binding.Name) == "" {
		return
	}
	next := cloneBinding(binding)
	if local {
		scope.LocalBindings = append(scope.LocalBindings, next)
		scope.LocalBindingsByName[next.Name] = next
	}
	if _, exists := scope.BindingsByName[next.Name]; exists {
		return
	}
	scope.Bindings = append(scope.Bindings, next)
	scope.BindingsByName[next.Name] = next
}

func registerSnapshotBindings(scope *moduleScope, bindings []*GlobalBinding) {
	if scope == nil {
		return
	}
	for _, binding := range bindings {
		if binding == nil || strings.TrimSpace(binding.Name) == "" {
			continue
		}
		next := cloneBinding(binding)
		next.SyntheticGlobal = true
		scope.Bindings = append(scope.Bindings, next)
		scope.BindingsByName[next.Name] = next
	}
}

func materializeModuleFunctionExports(scope *moduleScope) {
	if scope == nil {
		return
	}
	env := maps.Clone(scope.Globals.Values)
	mergeIntoValueEnv(env, scope.Env)
	root := eval.NewRootFrame(env)
	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}

	rewriteValue := func(value eval.Value) eval.Value {
		if value.Kind != eval.KindFunction || value.Fn == nil || !functionNeedsMaterialization(value.Fn) {
			return value
		}
		return eval.Function(materializeCapturedFunction(value.Fn, root, frameMemo, cellMemo, fnMemo))
	}
	rewriteGlobalVar := func(gv *GlobalVar) {
		if gv == nil || gv.Value.Kind != eval.KindFunction || gv.Value.Fn == nil {
			return
		}
		gv.Value = rewriteValue(gv.Value)
		gv.Order, gv.Vars = globalVarSeries(gv.Name, gv.Value)
	}

	for name, gv := range scope.ExportsByName {
		rewriteGlobalVar(gv)
		if gv != nil {
			scope.Env[name] = gv.Value
			root.AssignLocal(name, gv.Value, gv.Span)
		}
	}
	for _, gv := range scope.LocalExportsByName {
		rewriteGlobalVar(gv)
	}
	for _, gv := range scope.GlobalVarByName {
		rewriteGlobalVar(gv)
	}
}

func materializeCapturedFunction(fn *eval.FunctionValue, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) *eval.FunctionValue {
	if fn == nil {
		return nil
	}
	if next := fnMemo[fn]; next != nil {
		return next
	}
	next := *fn
	fnMemo[fn] = &next
	next.Capture = materializeCapturedFrame(fn.Capture, root, frameMemo, cellMemo, fnMemo)
	if len(fn.Defaults) > 0 {
		next.Defaults = make(map[int]eval.FunctionDefault, len(fn.Defaults))
		for index, defaultValue := range fn.Defaults {
			if defaultValue.PreEvaluated && defaultValue.Value.Kind == eval.KindFunction && defaultValue.Value.Fn != nil && functionNeedsMaterialization(defaultValue.Value.Fn) {
				defaultValue.Value = eval.Function(materializeCapturedFunction(defaultValue.Value.Fn, root, frameMemo, cellMemo, fnMemo))
			}
			next.Defaults[index] = defaultValue
		}
	}
	return &next
}

func functionNeedsMaterialization(fn *eval.FunctionValue) bool {
	if fn == nil {
		return false
	}
	for frame := fn.Capture; frame != nil; frame = frame.Parent {
		if frame.Resolve != nil {
			return true
		}
	}
	return false
}

func materializeCapturedFrame(frame *eval.Frame, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) *eval.Frame {
	if frame == nil || frame.Parent == nil {
		return root
	}
	if next := frameMemo[frame]; next != nil {
		return next
	}
	next := &eval.Frame{
		Parent: materializeCapturedFrame(frame.Parent, root, frameMemo, cellMemo, fnMemo),
		Values: make(map[string]*eval.Cell, len(frame.Values)),
	}
	frameMemo[frame] = next
	for name, cell := range frame.Values {
		next.Values[name] = materializeCapturedCell(cell, root, frameMemo, cellMemo, fnMemo)
	}
	return next
}

func materializeCapturedCell(cell *eval.Cell, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) *eval.Cell {
	if cell == nil {
		return nil
	}
	if next := cellMemo[cell]; next != nil {
		return next
	}
	next := &eval.Cell{
		Value:    cell.Value,
		Origin:   cell.Origin,
		Assigned: cell.Assigned,
	}
	cellMemo[cell] = next
	if next.Assigned && next.Value.Kind == eval.KindFunction && next.Value.Fn != nil && functionNeedsMaterialization(next.Value.Fn) {
		next.Value = eval.Function(materializeCapturedFunction(next.Value.Fn, root, frameMemo, cellMemo, fnMemo))
	}
	return next
}

func mergeIntoValueEnv(dst map[string]eval.Value, src map[string]eval.Value) {
	if dst == nil {
		return
	}
	for name, value := range src {
		dst[name] = value
	}
}

func mergeBindingValues(env map[string]eval.Value, bindings map[string]*GlobalBinding) {
	if env == nil {
		return
	}
	for name, binding := range bindings {
		if binding == nil {
			continue
		}
		env[name] = binding.Value
	}
}

func prefixDoBlock(block ast.DoBlock, prefix string) ast.DoBlock {
	block.Name = prefix + "." + block.Name
	block.After = prefixNames(prefix, block.After)
	block.WithItems = prefixWithItems(block.WithItems, prefix)
	return block
}

func prefixSubmitBlock(block ast.SubmitBlock, prefix string) ast.SubmitBlock {
	block.Name = prefix + "." + block.Name
	block.After = prefixNames(prefix, block.After)
	block.UseNames = prefixNames(prefix, block.UseNames)
	block.WithItems = prefixWithItems(block.WithItems, prefix)
	return block
}

func prefixWithItems(items []ast.WithItem, prefix string) []ast.WithItem {
	out := make([]ast.WithItem, len(items))
	for i, item := range items {
		next := item
		if next.Source != "" {
			next.Source = prefix + "." + next.Source
		}
		out[i] = next
	}
	return out
}

func prefixNames(prefix string, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, prefix+"."+name)
	}
	return out
}

func prefixScopeSnapshot(snap *ScopeSnapshot, prefix string) *ScopeSnapshot {
	if snap == nil || strings.TrimSpace(prefix) == "" {
		return cloneScopeSnapshot(snap)
	}
	out := cloneScopeSnapshot(snap)
	out.Globals.Values = prefixValueMap(prefix, out.Globals.Values)
	out.Globals.Modes = prefixStringMap(prefix, out.Globals.Modes)
	out.Globals.Spans = prefixSpanMap(prefix, out.Globals.Spans)
	out.GlobalVarByName = make(map[string]*GlobalVar, len(snap.GlobalVarByName))
	out.GlobalVarOrder = prefixNames(prefix, snap.GlobalVarOrder)
	for name, gv := range snap.GlobalVarByName {
		next := cloneGlobalVar(gv)
		if next == nil {
			continue
		}
		next.Name = prefix + "." + name
		next.DependsOn = prefixNames(prefix, next.DependsOn)
		out.GlobalVarByName[next.Name] = next
	}
	out.Bindings = make([]*GlobalBinding, 0, len(snap.Bindings))
	out.BindingsByName = make(map[string]*GlobalBinding, len(snap.BindingsByName))
	for _, binding := range snap.Bindings {
		next := cloneBinding(binding)
		if next == nil {
			continue
		}
		next.Name = prefix + "." + next.Name
		next.PublicName = prefix + "." + bindingDisplayName(binding)
		next.DependsOn = prefixNames(prefix, next.DependsOn)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[next.Name] = next
		out.BindingsByName[next.PublicName] = next
	}
	out.Namespaces = make(map[string]*Namespace, len(snap.Namespaces)+1)
	for name, ns := range snap.Namespaces {
		if ns == nil {
			continue
		}
		q := prefix + "." + name
		out.Namespaces[q] = &Namespace{
			Name:     q,
			Members:  prefixNames(prefix, ns.Members),
			Bindings: prefixNames(prefix, ns.Bindings),
			Steps:    prefixNames(prefix, ns.Steps),
		}
	}
	out.Namespaces[prefix] = &Namespace{Name: prefix}
	for _, binding := range out.Bindings {
		out.Namespaces[prefix].Bindings = appendUniqueString(out.Namespaces[prefix].Bindings, binding.PublicName)
	}
	for name := range out.GlobalVarByName {
		out.Namespaces[prefix].Members = appendUniqueString(out.Namespaces[prefix].Members, name)
	}
	return out
}

func prefixSnapshotBlockKey(key string, prefix string) string {
	parts := strings.Split(key, "|")
	if len(parts) < 4 || strings.TrimSpace(prefix) == "" {
		return key
	}
	parts[1] = prefix + "." + parts[1]
	return strings.Join(parts, "|")
}

func prefixValueMap(prefix string, in map[string]eval.Value) map[string]eval.Value {
	out := make(map[string]eval.Value, len(in))
	for name, value := range in {
		out[prefix+"."+name] = value
	}
	return out
}

func prefixStringMap(prefix string, in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for name, value := range in {
		out[prefix+"."+name] = value
	}
	return out
}

func prefixSpanMap(prefix string, in map[string]diag.Span) map[string]diag.Span {
	out := make(map[string]diag.Span, len(in))
	for name, value := range in {
		out[prefix+"."+name] = value
	}
	return out
}

func containsStepName(blocks []ast.DoBlock, name string) bool {
	for _, block := range blocks {
		if block.Name == name {
			return true
		}
	}
	return false
}

func containsSubmitName(blocks []ast.SubmitBlock, name string) bool {
	for _, block := range blocks {
		if block.Name == name {
			return true
		}
	}
	return false
}

func mergeUniqueStrings(dst []string, src []string) []string {
	for _, value := range src {
		dst = appendUniqueString(dst, value)
	}
	return dst
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
