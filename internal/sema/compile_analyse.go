// compile `analyse` blocks into semantic `AnalyseSpec`
//
// validate target step existence/kind, resolve step-visible/imported
// symbols, evaluate helper assignments, validate extraction
// expressions/files/placeholders, build pattern templates, check result-tuple columns
package sema

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/patternutil"
)

func normalizePatternRegex(input string) (string, map[string]string, bool) {
	normalized, ok := patternutil.NormalizePercentPattern(input)
	if !ok {
		return "", nil, false
	}
	captureTypes := make(map[string]string, len(normalized.CaptureTypesByName))
	for name, kind := range normalized.CaptureTypesByName {
		captureTypes[name] = string(kind)
	}
	return normalized.Regex, captureTypes, true
}

func compileAnalyseBlock(block ast.AnalyseBlock, res *Result, opts AnalyzeOptions, diags *diag.Diagnostics) *AnalyseSpec {
	snap := snapshotForAnalyseBlock(res, block)
	analyseBindings := snapshotBindings(res, snap)
	analyseGlobals := snapshotGlobals(res, snap)
	analyseNamespaces := snapshotNamespaces(res, snap)
	spec := &AnalyseSpec{
		Name:        block.StepName,
		Block:       block,
		StepVars:    make(map[string]diag.Span),
		Assignments: make([]AnalyseAssignmentSpec, 0, len(block.Assignments)),
		Columns:     make([]AnalyseColumnSpec, 0, len(block.Columns)),
		Span:        block.Span,
	}

	stepKind := ""
	for _, doBlock := range res.DoBlocks {
		if doBlock.Name == block.StepName {
			stepKind = "do"
			break
		}
	}
	if stepKind == "" {
		diags.AddError(
			diag.CodeE410,
			fmt.Sprintf("unknown analyse target step '%s'", block.StepName),
			block.Span,
			"analyse must reference an existing do block name",
		)
	}
	spec.StepKind = stepKind

	plan := res.StepScopeByName[block.StepName]
	spec.StepVars = visibleSpansFromStepPlan(plan, res.BindingsByKey, res.BindingsByName)

	env := make(map[string]eval.Value, len(analyseGlobals)+32)
	for k, v := range analyseGlobals {
		env[k] = eval.CloneValue(v)
	}
	addEnvFromStepPlan(env, plan, res.BindingsByKey, res.BindingsByName)
	analyseImports := resolveAnalyseWithImports(block.WithItems, analyseBindings, analyseGlobals, analyseNamespaces, diags)
	for visible, imported := range analyseImports {
		binding := analyseBindings[imported.Source]
		if binding == nil {
			continue
		}
		v, ok := binding.Vars[imported.SourceVar]
		if !ok {
			continue
		}
		env[visible] = seriesAsValue(v)
	}

	seenAssignments := make(map[string]diag.Span, len(block.Assignments))
	assignmentVars := make(map[string]diag.Span, len(block.Assignments))
	for _, assign := range block.Assignments {
		effectiveExpr := assignmentExpr(assign.Name, assign.Op, assign.Expr, assign.Span)
		if prev, exists := seenAssignments[assign.Name]; exists {
			diags.AddError(
				diag.CodeE414,
				fmt.Sprintf("duplicate analyse variable '%s'", assign.Name),
				assign.Span,
				"use unique variable names in analyse assignments",
				diag.RelatedSpan{Message: "first assignment", Span: prev},
			)
			continue
		}
		seenAssignments[assign.Name] = assign.Span

		fileTarget := assign.EffectiveFileTarget()
		if !fileTarget.IsSet() {
			if existing, ok := spec.StepVars[assign.Name]; ok {
				diags.AddWarning(
					diag.CodeW320,
					fmt.Sprintf("analyse helper variable '%s' shadows step-visible variable", assign.Name),
					assign.Span,
					"use a distinct helper variable name to avoid ambiguity",
					diag.RelatedSpan{Message: "step variable", Span: existing},
				)
			}
			value := eval.EvalExprWithOptions(effectiveExpr, env, diags, analyseEvalOptions(res, assign, visibleNamesFromEnv(env), analyseNamespaces, opts))
			if value.Kind == eval.KindFunction {
				diags.AddError(
					diag.CodeE412,
					fmt.Sprintf("analyse helper '%s' must evaluate to data, not function", assign.Name),
					assign.Span,
					"use scalar/string/list data in analyse helpers, not function-valued globals",
				)
				continue
			}
			if hasNestedList(value) {
				diags.AddError(
					diag.CodeE305,
					fmt.Sprintf("nested tuple/list value is not allowed for analyse helper '%s'", assign.Name),
					assign.Span,
					"use flat tuple/list values only",
				)
			}
			env[assign.Name] = value
			continue
		}

		if existing, ok := spec.StepVars[assign.Name]; ok {
			diags.AddError(
				diag.CodeE413,
				fmt.Sprintf("analyse extraction variable '%s' collides with step-visible variable", assign.Name),
				assign.Span,
				"use a distinct extraction variable name in analyse",
				diag.RelatedSpan{Message: "step variable", Span: existing},
			)
			continue
		}
		compiled, _, ok := compileAnalyseExtraction(assign.Name, assign.Name, fileTarget, effectiveExpr, assign.Span, env, res, visibleNamesFromEnv(env), analyseNamespaces, opts, diags)
		if !ok {
			continue
		}
		spec.Assignments = append(spec.Assignments, compiled)
		assignmentVars[assign.Name] = assign.Span
	}

	inlineIndex := 0
	for _, col := range block.Columns {
		kind := col.Kind
		if kind == "" {
			kind = ast.AnalyseColumnNamed
		}
		if kind == ast.AnalyseColumnInlinePattern {
			source := nextInlineAnalyseSource(&inlineIndex, seenAssignments)
			compiled, patternText, ok := compileAnalyseExtraction(source, "inline pattern", col.EffectiveFileTarget(), col.Expr, col.Span, env, res, visibleNamesFromEnv(env), analyseNamespaces, opts, diags)
			if !ok {
				continue
			}
			compiled.DisplayName = patternText
			spec.Assignments = append(spec.Assignments, compiled)
			title := col.Title
			if title == "" {
				title = patternText
			}
			spec.Columns = append(spec.Columns, AnalyseColumnSpec{
				Name:   title,
				Title:  title,
				Source: source,
				Span:   col.Span,
			})
			continue
		}
		if _, ok := spec.StepVars[col.Name]; ok {
			spec.Columns = append(spec.Columns, AnalyseColumnSpec{
				Name:   col.Name,
				Title:  col.Title,
				Source: col.Name,
				Span:   col.Span,
			})
			continue
		}
		if _, ok := assignmentVars[col.Name]; ok {
			spec.Columns = append(spec.Columns, AnalyseColumnSpec{
				Name:   col.Name,
				Title:  col.Title,
				Source: col.Name,
				Span:   col.Span,
			})
			continue
		}
		diags.AddError(
			diag.CodeE415,
			fmt.Sprintf("unknown symbol '%s' in analyse result tuple", col.Name),
			col.Span,
			"use a step-visible variable or an analyse extraction alias",
		)
	}

	return spec
}

func compileAnalyseExtraction(name, diagnosticName string, target ast.AnalyseFileTarget, expr ast.Expr, span diag.Span, env map[string]eval.Value, res *Result, visible []string, namespaces map[string]*Namespace, opts AnalyzeOptions, diags *diag.Diagnostics) (AnalyseAssignmentSpec, string, bool) {
	if diagnosticName == "" {
		diagnosticName = name
	}
	before := len(diags.Items)
	value := eval.EvalExprWithOptions(expr, env, diags, analyseEvalOptionsForSpan(res, span, visible, namespaces, opts))
	if value.Kind != eval.KindString {
		if hasErrorCodeSince(diags, before, diag.CodeE100) {
			return AnalyseAssignmentSpec{}, "", false
		}
		if value.Kind == eval.KindFunction {
			diags.AddError(
				diag.CodeE412,
				fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string data, not function", diagnosticName),
				span,
				"use a string-valued expression, not a function-valued global",
			)
			return AnalyseAssignmentSpec{}, "", false
		}
		diags.AddError(
			diag.CodeE412,
			fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string", diagnosticName),
			span,
			"use a string expression such as an imported global or a quoted regex pattern",
		)
		return AnalyseAssignmentSpec{}, "", false
	}
	regex, captureTypes, ok := normalizePatternRegex(value.S)
	if !ok {
		diags.AddError(
			diag.CodeE402,
			fmt.Sprintf("invalid placeholder in analyse extraction expression for '%s'", diagnosticName),
			span,
			"supported placeholders are %d, %f, %w and %% for a literal percent",
		)
		return AnalyseAssignmentSpec{}, "", false
	}

	return AnalyseAssignmentSpec{
		Name:       name,
		File:       target.Value,
		FileTarget: analyseFileTargetSpec(target),
		Template: PatternTemplate{
			Regex:              regex,
			CaptureTypesByName: captureTypes,
			Span:               span,
		},
		Span: span,
	}, value.S, true
}

func analyseFileTargetSpec(target ast.AnalyseFileTarget) AnalyseFileTargetSpec {
	switch target.Kind {
	case ast.AnalyseFileRegex:
		return AnalyseFileTargetSpec{Kind: AnalyseFileRegex, Value: target.Value, Span: target.Span}
	case ast.AnalyseFileExact:
		return AnalyseFileTargetSpec{Kind: AnalyseFileExact, Value: target.Value, Span: target.Span}
	default:
		return AnalyseFileTargetSpec{}
	}
}

func nextInlineAnalyseSource(index *int, used map[string]diag.Span) string {
	for {
		name := fmt.Sprintf("__analyse_inline_%d", *index)
		*index = *index + 1
		if _, exists := used[name]; exists {
			continue
		}
		used[name] = diag.Span{}
		return name
	}
}

func analyseEvalOptions(res *Result, assign ast.AnalyseAssign, visible []string, namespaces map[string]*Namespace, opts AnalyzeOptions) eval.ExprOptions {
	return analyseEvalOptionsForSpan(res, assign.Span, visible, namespaces, opts)
}

func analyseEvalOptionsForSpan(res *Result, span diag.Span, visible []string, namespaces map[string]*Namespace, opts AnalyzeOptions) eval.ExprOptions {
	return eval.ExprOptions{
		Context:     eval.EvalCtxAnalyseAssign,
		Names:       scopeNameCatalog(visible, namespaces),
		Files:       fileAccessForSpan(res.BaseDirByFile, span),
		ShellRunner: opts.ShellRunner,
		Environ:     opts.Environ,
	}
}

type analyseBindingImport struct {
	Source    string
	SourceVar string
	Span      diag.Span
}

type analyseImportOptions struct {
	EmitDiagnostics bool
}

func resolveAnalyseImportsCanonical(items []ast.WithItem, bindings map[string]*GlobalBinding, globals map[string]eval.Value, namespaces map[string]*Namespace, diags *diag.Diagnostics, opts analyseImportOptions) map[string]analyseBindingImport {
	resolver := BindingResolver{
		Bindings:   bindings,
		Globals:    globals,
		Namespaces: namespaces,
	}
	out, issues := resolver.ResolveAnalyseWithItems(items, diags)
	if opts.EmitDiagnostics && diags != nil {
		emitWithIssues(diags, analyseWithDiagPolicy(), issues)
	}
	return out
}

func resolveAnalyseWithImports(items []ast.WithItem, bindings map[string]*GlobalBinding, globals map[string]eval.Value, namespaces map[string]*Namespace, diags *diag.Diagnostics) map[string]analyseBindingImport {
	return resolveAnalyseImportsCanonical(items, bindings, globals, namespaces, diags, analyseImportOptions{
		EmitDiagnostics: true,
	})
}

func hasErrorCodeSince(diags *diag.Diagnostics, start int, code diag.Code) bool {
	if diags == nil {
		return false
	}
	if start < 0 {
		start = 0
	}
	if start >= len(diags.Items) {
		return false
	}
	for i := start; i < len(diags.Items); i++ {
		item := diags.Items[i]
		if item.Severity != diag.SeverityError {
			continue
		}
		if item.Code == string(code) {
			return true
		}
	}
	return false
}
