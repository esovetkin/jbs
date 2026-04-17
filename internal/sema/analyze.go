package sema

import (
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
	return analyzeProgram(loadRes.Program, globals, loadRes, diags)
}

func analyzeProgram(prog ast.Program, globals map[string]eval.Value, loadRes *imports.LoadResult, diags *diag.Diagnostics) *Result {
	namespaceUnit, namespaceEnv := buildEntryNamespaceUnit(loadRes, globals, diags)
	seedEnv := mergeValueEnv(globals, namespaceEnv)
	resolvedGlobals := resolveTopLevelGlobals(prog, seedEnv, diags)
	res := &Result{
		Program:         prog,
		Globals:         resolvedGlobals,
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

	integrateModuleUnit(res, namespaceUnit)
	analyseBlocks := make([]ast.AnalyseBlock, 0)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			continue
		case ast.DoBlock:
			res.DoBlocks = append(res.DoBlocks, n)
			res.StepOrder = append(res.StepOrder, n.Name)
		case ast.SubmitBlock:
			res.Submits = append(res.Submits, n)
			res.StepOrder = append(res.StepOrder, n.Name)
		case ast.AnalyseBlock:
			analyseBlocks = append(analyseBlocks, n)
		}
	}

	compiledGlobals, order := compileUserGlobals(prog, seedEnv, diags)
	res.GlobalVarByName = compiledGlobals
	res.GlobalVarOrder = order
	for _, name := range order {
		gv := compiledGlobals[name]
		if gv == nil {
			continue
		}
		binding := bindingFromGlobalVar(name, gv)
		if binding == nil {
			continue
		}
		res.Bindings = append(res.Bindings, binding)
		res.BindingsByName[name] = binding
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
		res.SubmitByName[submit.Name] = compileSubmitBlock(submit, res.BindingsByName, resolvedGlobals.Values, effective, res.Namespaces, diags)
	}
	validateStepVarReferences(res, diags)
	for _, block := range analyseBlocks {
		spec := compileAnalyseBlock(block, res, diags)
		res.Analyse = append(res.Analyse, spec)
	}
	return res
}
