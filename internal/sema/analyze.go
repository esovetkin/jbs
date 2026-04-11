// the semantic-analysis orchestrator
//
// walk the parsed program and builds a `sema.Result` by resolving
// globals, compiling let/param blocks, collecting do/submit/analyse
// blocks, building import sources and step import plans, validating
// step/use semantics and references, compiling submit/analyse specs,
// and accumulating diagnostics across all phases
package sema

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func Analyze(prog ast.Program, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	resolvedGlobals := resolveTopLevelGlobals(prog, globals, diags)
	res := &Result{
		Program:            prog,
		Globals:            resolvedGlobals,
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

	letSpans := make(map[string]diag.Span)
	paramSpans := make(map[string]diag.Span)
	analyseBlocks := make([]ast.AnalyseBlock, 0)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			continue
		case ast.LetBlock:
			if prev, exists := letSpans[n.Name]; exists {
				diags.AddError(
					diag.CodeE400,
					fmt.Sprintf("duplicate let block name '%s'", n.Name),
					n.Span,
					"use a unique let block name",
					diag.RelatedSpan{Message: "first definition", Span: prev},
				)
				continue
			}
			letSpans[n.Name] = n.Span
			compiled := compileLetBlock(n, resolvedGlobals.Values, diags)
			if compiled != nil {
				res.LetNamespaces = append(res.LetNamespaces, compiled)
				res.LetByName[compiled.Name] = compiled
			}
		case ast.ParamBlock:
			if prev, exists := paramSpans[n.Name]; exists {
				diags.AddError(
					diag.CodeE210,
					fmt.Sprintf("duplicate param block name '%s'", n.Name),
					n.Span,
					"use a unique param block name",
					diag.RelatedSpan{Message: "first definition", Span: prev},
				)
				continue
			}
			paramSpans[n.Name] = n.Span
			compiled := compileParamBlock(n, res.ParamByName, resolvedGlobals.Values, res.LetByName, diags)
			res.Paramsets = append(res.Paramsets, compiled)
			res.ParamByName[n.Name] = compiled
		case ast.DoBlock:
			res.DoBlocks = append(res.DoBlocks, n)
		case ast.SubmitBlock:
			res.Submits = append(res.Submits, n)
		case ast.AnalyseBlock:
			analyseBlocks = append(analyseBlocks, n)
		}
	}

	buildImportSources(res)
	validateSteps(res, diags)
	validateUseClauses(res, diags)
	buildStepImportPlans(res, diags)
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
