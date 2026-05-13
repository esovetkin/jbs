package sema

import (
	"maps"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

type AnalyzeOptions struct {
	CollectPrints bool
	ShellRunner   eval.ShellRunner
	ShellMode     eval.ShellMode
	Environ       func() []string
}

func Analyze(prog ast.Program, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	return AnalyzeWithOptions(prog, globals, AnalyzeOptions{}, diags)
}

func AnalyzeWithOptions(prog ast.Program, globals map[string]eval.Value, opts AnalyzeOptions, diags *diag.Diagnostics) *Result {
	return analyzeProgram(prog, globals, nil, opts, diags)
}

func AnalyzeWithImports(loadRes *imports.LoadResult, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	return AnalyzeWithImportsOptions(loadRes, globals, AnalyzeOptions{}, diags)
}

func AnalyzeWithImportsOptions(loadRes *imports.LoadResult, globals map[string]eval.Value, opts AnalyzeOptions, diags *diag.Diagnostics) *Result {
	if loadRes == nil {
		return AnalyzeWithOptions(ast.Program{}, globals, opts, diags)
	}
	entryInfo := loadRes.Modules[loadRes.Entry.ID]
	prog := ast.Program{}
	if entryInfo != nil {
		prog = entryInfo.Program
	}
	return analyzeProgram(prog, globals, loadRes, opts, diags)
}

func analyzeProgram(prog ast.Program, globals map[string]eval.Value, loadRes *imports.LoadResult, opts AnalyzeOptions, diags *diag.Diagnostics) *Result {
	res := &Result{
		Program:               prog,
		BaseDirByFile:         make(map[string]string),
		Globals:               GlobalState{Values: map[string]eval.Value{}, Spans: map[string]diag.Span{}},
		GlobalVarByName:       make(map[string]*GlobalVar),
		GlobalVarOrder:        make([]string, 0),
		TopLevelExprs:         make([]TopLevelExprResult, 0),
		PrintEvents:           make([]PrintEvent, 0),
		Bindings:              make([]*GlobalBinding, 0),
		BindingsByName:        make(map[string]*GlobalBinding),
		BindingsByKey:         make(map[BindingVersionKey]*GlobalBinding),
		ScopeSnapshotsByIndex: make(map[int]*ScopeSnapshot),
		ScopeSnapshotsByBlock: make(map[string]*ScopeSnapshot),
		Namespaces:            make(map[string]*Namespace),
		DoBlocks:              make([]ast.DoBlock, 0),
		StepOrder:             make([]string, 0),
		StepScopeByName:       make(map[string]*StepScopePlan),
		Analyse:               make([]*AnalyseSpec, 0),
	}

	var scope *moduleScope
	if loadRes == nil {
		exec := execGlobalPlanWithOptions(
			buildGlobalPlan(prog, globals, baseDirForProgramFile(prog.File)),
			globals,
			globals,
			globalExecOptions(opts),
			diags,
		)
		scope = emptyModuleScope()
		scope.Program = prog
		if baseDir := baseDirForProgramFile(prog.File); baseDir != "" {
			scope.BaseDirByFile[prog.File] = baseDir
		}
		scope.Globals = GlobalState{
			Values: cloneValueMap(exec.ScalarGlobals.Values),
			Spans:  maps.Clone(exec.ScalarGlobals.Spans),
		}
		scope.GlobalVarByName, scope.GlobalVarOrder = globalVarsFromExec(exec)
		scope.TopLevelExprs = cloneTopLevelExprResults(exec.TopLevelExprs)
		scope.PrintEvents = clonePrintEvents(exec.PrintEvents)
		for _, name := range scope.GlobalVarOrder {
			gv := scope.GlobalVarByName[name]
			registerModuleExport(scope, name, gv, !isBuiltinGlobalName(name))
			if !isBuiltinGlobalName(name) {
				registerModuleBinding(scope, bindingFromGlobalVar(name, gv), true)
			}
		}
		registerSnapshotBindings(scope, exec.SnapshotBindings)
		scope.ScopeSnapshotsByIndex = cloneScopeSnapshotsByIndex(exec.ScopeSnapshotsByIndex)
		scope.ScopeSnapshotsByBlock = cloneScopeSnapshotsByBlock(exec.ScopeSnapshotsByBlock)
		for _, stmt := range prog.Stmts {
			switch n := stmt.(type) {
			case ast.DoBlock:
				scope.DoBlocks = append(scope.DoBlocks, n)
				scope.StepOrder = append(scope.StepOrder, n.Name)
			case ast.AnalyseBlock:
				scope.AnalyseBlocks = append(scope.AnalyseBlocks, n)
			}
		}
		materializeModuleFunctionExports(scope)
		mergeGlobalVarsIntoState(&scope.Globals, scope.ExportsByName)
	} else {
		scope = buildEntryModuleScope(loadRes, globals, opts, diags)
		if scope != nil {
			res.Program = scope.Program
		}
	}
	if scope == nil {
		scope = emptyModuleScope()
	}

	res.Globals = GlobalState{
		Values: cloneValueMap(scope.Globals.Values),
		Spans:  maps.Clone(scope.Globals.Spans),
	}
	res.BaseDirByFile = maps.Clone(scope.BaseDirByFile)
	res.GlobalVarByName, res.GlobalVarOrder = cloneGlobalVars(scope.GlobalVarByName, scope.GlobalVarOrder)
	res.TopLevelExprs = cloneTopLevelExprResults(scope.TopLevelExprs)
	res.PrintEvents = clonePrintEvents(scope.PrintEvents)
	for _, binding := range scope.Bindings {
		next := cloneBinding(binding)
		res.Bindings = append(res.Bindings, next)
		if _, exists := res.BindingsByName[next.Name]; !exists || !next.SyntheticGlobal {
			res.BindingsByName[next.Name] = next
		}
		indexBindingByKey(res.BindingsByKey, next, next.Name)
	}
	res.ScopeSnapshotsByIndex = cloneScopeSnapshotsByIndex(scope.ScopeSnapshotsByIndex)
	res.ScopeSnapshotsByBlock = cloneScopeSnapshotsByBlock(scope.ScopeSnapshotsByBlock)
	res.DoBlocks = append([]ast.DoBlock(nil), scope.DoBlocks...)
	res.StepOrder = append([]string(nil), scope.StepOrder...)
	for name, ns := range scope.Namespaces {
		res.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Members:  append([]string(nil), ns.Members...),
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	mergeGlobalVarsIntoState(&res.Globals, scope.ExportsByName)

	validateSteps(res, diags)
	validateUseClauses(res, diags)
	buildStepScopePlans(res, diags)
	validateFileSubstitutions(res, diags)
	validateStepVarReferences(res, diags)
	for _, block := range scope.AnalyseBlocks {
		spec := compileAnalyseBlock(block, res, opts, diags)
		res.Analyse = append(res.Analyse, spec)
	}
	validateBenchmarksGlobal(res, diags)
	return res
}
