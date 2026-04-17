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
		Program:         prog,
		Globals:         GlobalState{Values: map[string]eval.Value{}, Modes: map[string]string{}, Spans: map[string]diag.Span{}},
		GlobalVarByName: make(map[string]*GlobalVar),
		GlobalVarOrder:  make([]string, 0),
		Bindings:        make([]*GlobalBinding, 0),
		BindingsByName:  make(map[string]*GlobalBinding),
		Namespaces:      make(map[string]*Namespace),
		DoBlocks:        make([]ast.DoBlock, 0),
		Submits:         make([]ast.SubmitBlock, 0),
		StepOrder:       make([]string, 0),
		SubmitByName:    make(map[string]*SubmitSpec),
		StepScopeByName: make(map[string]*StepScopePlan),
		Analyse:         make([]*AnalyseSpec, 0),
	}

	var scope *moduleScope
	if loadRes == nil {
		exec := execGlobalPlan(buildGlobalPlan(prog), globals, globals, diags)
		scope = emptyModuleScope()
		scope.Program = prog
		scope.Globals = GlobalState{
			Values: maps.Clone(exec.ScalarGlobals.Values),
			Modes:  maps.Clone(exec.ScalarGlobals.Modes),
			Spans:  maps.Clone(exec.ScalarGlobals.Spans),
		}
		scope.GlobalVarByName, scope.GlobalVarOrder = globalVarsFromExec(exec)
		for _, name := range scope.GlobalVarOrder {
			gv := scope.GlobalVarByName[name]
			if gv == nil {
				continue
			}
			binding := bindingFromGlobalVar(name, gv)
			if binding == nil {
				continue
			}
			scope.LocalBindings = append(scope.LocalBindings, binding)
			scope.LocalBindingsByName[name] = binding
			scope.Bindings = append(scope.Bindings, binding)
			scope.BindingsByName[name] = binding
			scope.Env[name] = binding.Value
		}
		for _, stmt := range prog.Stmts {
			switch n := stmt.(type) {
			case ast.DoBlock:
				scope.DoBlocks = append(scope.DoBlocks, n)
				scope.StepOrder = append(scope.StepOrder, n.Name)
			case ast.SubmitBlock:
				scope.Submits = append(scope.Submits, n)
				scope.StepOrder = append(scope.StepOrder, n.Name)
			case ast.AnalyseBlock:
				scope.AnalyseBlocks = append(scope.AnalyseBlocks, n)
			}
		}
		mergeBindingValues(scope.Globals.Values, scope.BindingsByName)
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
		Modes:  maps.Clone(scope.Globals.Modes),
		Spans:  maps.Clone(scope.Globals.Spans),
	}
	res.GlobalVarByName, res.GlobalVarOrder = cloneGlobalVars(scope.GlobalVarByName, scope.GlobalVarOrder)
	for _, binding := range scope.Bindings {
		next := cloneBinding(binding)
		res.Bindings = append(res.Bindings, next)
		res.BindingsByName[next.Name] = next
	}
	res.DoBlocks = append([]ast.DoBlock(nil), scope.DoBlocks...)
	res.Submits = append([]ast.SubmitBlock(nil), scope.Submits...)
	res.StepOrder = append([]string(nil), scope.StepOrder...)
	for name, ns := range scope.Namespaces {
		res.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	mergeBindingValues(res.Globals.Values, res.BindingsByName)

	validateSteps(res, diags)
	validateUseClauses(res, diags)
	buildStepScopePlans(res, diags)
	for _, submit := range res.Submits {
		effective := map[string]VisibleBinding{}
		if plan := res.StepScopeByName[submit.Name]; plan != nil {
			effective = plan.Effective
		}
		res.SubmitByName[submit.Name] = compileSubmitBlock(submit, res.BindingsByName, res.Globals.Values, effective, res.Namespaces, diags)
	}
	validateStepVarReferences(res, diags)
	for _, block := range scope.AnalyseBlocks {
		spec := compileAnalyseBlock(block, res, diags)
		res.Analyse = append(res.Analyse, spec)
	}
	return res
}
