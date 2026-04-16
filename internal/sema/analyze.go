// the semantic-analysis orchestrator
//
// walk the parsed program and builds a `sema.Result` by resolving
// globals, collecting do/submit/analyse blocks, building import
// sources and step import plans, validating step/use semantics and
// references, compiling submit/analyse specs, and accumulating
// diagnostics across all phases
package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func Analyze(prog ast.Program, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	resolvedGlobals := resolveTopLevelGlobals(prog, globals, diags)
	res := &Result{
		Program:            prog,
		Globals:            resolvedGlobals,
		GlobalVarByName:    make(map[string]*GlobalVar),
		GlobalVarOrder:     make([]string, 0),
		LetNamespaces:      make([]*LetNamespace, 0),
		LetByName:          make(map[string]*LetNamespace),
		ImportSourceByName: make(map[string]*ImportSource),
		Paramsets:          make([]*Paramset, 0),
		ParamByName:        make(map[string]*Paramset),
		DoBlocks:           make([]ast.DoBlock, 0),
		Submits:            make([]ast.SubmitBlock, 0),
		SubmitByName:       make(map[string]*SubmitSpec),
		StepImportByName:   make(map[string]*StepImportPlan),
		Analyse:            make([]*AnalyseSpec, 0),
	}

	analyseBlocks := make([]ast.AnalyseBlock, 0)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			continue
		case ast.DoBlock:
			res.DoBlocks = append(res.DoBlocks, n)
		case ast.SubmitBlock:
			res.Submits = append(res.Submits, n)
		case ast.AnalyseBlock:
			analyseBlocks = append(analyseBlocks, n)
		}
	}

	compiledGlobals, order := compileUserGlobals(prog, resolvedGlobals.Values, diags)
	res.GlobalVarByName = compiledGlobals
	res.GlobalVarOrder = order
	addGlobalSources(res, compiledGlobals, order, diags)

	buildImportSources(res)
	validateSteps(res, diags)
	validateUseClauses(res, diags)
	buildStepImportPlans(res, diags)
	// From this point on, step-level semantic consumers should project
	// imports from StepImportPlan only, not by re-expanding raw with-items.
	for _, submit := range res.Submits {
		effective := map[string]VarOrigin{}
		if plan := res.StepImportByName[submit.Name]; plan != nil {
			effective = plan.Effective
		}
		res.SubmitByName[submit.Name] = compileSubmitBlock(submit, res.ImportSourceByName, resolvedGlobals.Values, effective, diags)
	}
	validateStepVarReferences(res, diags)
	for _, block := range analyseBlocks {
		spec := compileAnalyseBlock(block, res, diags)
		res.Analyse = append(res.Analyse, spec)
	}
	return res
}
