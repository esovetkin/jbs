package sema

import (
	"maps"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/imports"
)

func Analyze(prog ast.Program, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	return analyzeProgram(prog, globals, nil, diags)
}

func AnalyzeWithImports(loadRes *imports.LoadResult, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	if loadRes == nil {
		return Analyze(ast.Program{}, globals, diags)
	}
	entryInfo := loadRes.Modules[loadRes.Entry.ID]
	prog := ast.Program{}
	if entryInfo != nil {
		prog = entryInfo.Program
	}
	return analyzeProgram(prog, globals, loadRes, diags)
}

func analyzeProgram(prog ast.Program, globals map[string]eval.Value, loadRes *imports.LoadResult, diags *diag.Diagnostics) *Result {
	res := &Result{
		Program:               prog,
		BaseDirByFile:         make(map[string]string),
		Globals:               GlobalState{Values: map[string]eval.Value{}, Spans: map[string]diag.Span{}},
		GlobalVarByName:       make(map[string]*GlobalVar),
		GlobalVarOrder:        make([]string, 0),
		TopLevelExprs:         make([]TopLevelExprResult, 0),
		Bindings:              make([]*GlobalBinding, 0),
		BindingsByName:        make(map[string]*GlobalBinding),
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
		exec := execGlobalPlan(buildGlobalPlan(prog, globals, baseDirForProgramFile(prog.File), diags), globals, globals, diags)
		scope = emptyModuleScope()
		scope.Program = prog
		if baseDir := baseDirForProgramFile(prog.File); baseDir != "" {
			scope.BaseDirByFile[prog.File] = baseDir
		}
		scope.Globals = GlobalState{
			Values: maps.Clone(exec.ScalarGlobals.Values),
			Spans:  maps.Clone(exec.ScalarGlobals.Spans),
		}
		scope.GlobalVarByName, scope.GlobalVarOrder = globalVarsFromExec(exec)
		scope.TopLevelExprs = cloneTopLevelExprResults(exec.TopLevelExprs)
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
		scope = buildEntryModuleScope(loadRes, globals, diags)
		if scope != nil {
			res.Program = scope.Program
		}
	}
	if scope == nil {
		scope = emptyModuleScope()
	}

	res.Globals = GlobalState{
		Values: maps.Clone(scope.Globals.Values),
		Spans:  maps.Clone(scope.Globals.Spans),
	}
	res.BaseDirByFile = maps.Clone(scope.BaseDirByFile)
	res.GlobalVarByName, res.GlobalVarOrder = cloneGlobalVars(scope.GlobalVarByName, scope.GlobalVarOrder)
	res.TopLevelExprs = cloneTopLevelExprResults(scope.TopLevelExprs)
	for _, binding := range scope.Bindings {
		next := cloneBinding(binding)
		res.Bindings = append(res.Bindings, next)
		res.BindingsByName[next.Name] = next
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
	validateStepVarReferences(res, diags)
	for _, block := range scope.AnalyseBlocks {
		spec := compileAnalyseBlock(block, res, diags)
		res.Analyse = append(res.Analyse, spec)
	}
	return res
}
