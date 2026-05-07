// compile `analyse` blocks into semantic `AnalyseSpec`
//
// validate target step existence/kind, resolve step-visible/imported
// symbols, evaluate helper assignments, validate extraction
// expressions/files/placeholders, build pattern templates, check result-tuple columns
package sema

import (
	"fmt"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

const (
	runtimeIntPattern   = `([-+]?[0-9]+)`
	runtimeFloatPattern = `([-+]?(?:(?:[0-9]+(?:\.[0-9]*)?)|(?:\.[0-9]+))(?:[eE][-+]?[0-9]+)?)`
	runtimeWordPattern  = `([A-Za-z0-9_]+)`
)

func normalizePatternRegex(input string) (string, string, bool) {
	var out strings.Builder
	sawInt := false
	sawFloat := false
	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '%' {
			out.WriteRune(r)
			continue
		}
		if i+1 >= len(runes) {
			out.WriteRune('%')
			continue
		}
		next := runes[i+1]
		switch next {
		case '%':
			out.WriteRune('%')
		case 'd':
			out.WriteString(runtimeIntPattern)
			sawInt = true
		case 'f':
			out.WriteString(runtimeFloatPattern)
			sawFloat = true
		case 'w':
			out.WriteString(runtimeWordPattern)
		default:
			return "", "", false
		}
		i++
	}
	t := "string"
	if sawFloat {
		t = "float"
	} else if sawInt {
		t = "int"
	}
	return out.String(), t, true
}

func compileAnalyseBlock(block ast.AnalyseBlock, res *Result, diags *diag.Diagnostics) *AnalyseSpec {
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
	spec.StepVars = visibleSpansFromStepPlan(plan, res.BindingsByName)

	env := make(map[string]eval.Value, len(analyseGlobals)+32)
	for k, v := range analyseGlobals {
		env[k] = v
	}
	addEnvFromStepPlan(env, plan, res.BindingsByName)
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

		if assign.File == "" {
			if existing, ok := spec.StepVars[assign.Name]; ok {
				diags.AddWarning(
					diag.CodeW320,
					fmt.Sprintf("analyse helper variable '%s' shadows step-visible variable", assign.Name),
					assign.Span,
					"use a distinct helper variable name to avoid ambiguity",
					diag.RelatedSpan{Message: "step variable", Span: existing},
				)
			}
			value := eval.EvalExprWithOptions(effectiveExpr, env, diags, eval.ExprOptions{
				Context: eval.EvalCtxAnalyseAssign,
				Names:   scopeNameCatalog(visibleNamesFromEnv(env), analyseNamespaces),
				Files:   fileAccessForSpan(res.BaseDirByFile, assign.Span),
			})
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
		before := len(diags.Items)
		value := eval.EvalExprWithOptions(effectiveExpr, env, diags, eval.ExprOptions{
			Context: eval.EvalCtxAnalyseAssign,
			Names:   scopeNameCatalog(visibleNamesFromEnv(env), analyseNamespaces),
			Files:   fileAccessForSpan(res.BaseDirByFile, assign.Span),
		})
		if value.Kind != eval.KindString {
			if hasErrorCodeSince(diags, before, diag.CodeE100) {
				continue
			}
			if value.Kind == eval.KindFunction {
				diags.AddError(
					diag.CodeE412,
					fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string data, not function", assign.Name),
					assign.Span,
					"use a string-valued expression, not a function-valued global",
				)
				continue
			}
			diags.AddError(
				diag.CodeE412,
				fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string", assign.Name),
				assign.Span,
				"use a string expression such as an imported global or a quoted regex pattern",
			)
			continue
		}
		regex, inferredType, ok := normalizePatternRegex(value.S)
		if !ok {
			diags.AddError(
				diag.CodeE402,
				fmt.Sprintf("invalid placeholder in analyse extraction expression for '%s'", assign.Name),
				assign.Span,
				"supported placeholders are %d, %f, %w and %% for a literal percent",
			)
			continue
		}

		spec.Assignments = append(spec.Assignments, AnalyseAssignmentSpec{
			Name: assign.Name,
			File: assign.File,
			Template: PatternTemplate{
				Regex: regex,
				Type:  inferredType,
				Span:  assign.Span,
			},
			Span: assign.Span,
		})
		assignmentVars[assign.Name] = assign.Span
	}

	for _, col := range block.Columns {
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

type analyseBindingImport struct {
	Source    string
	SourceVar string
	Span      diag.Span
}

type analyseImportOptions struct {
	EmitDiagnostics bool
}

func resolveAnalyseImportsCanonical(items []ast.WithItem, bindings map[string]*GlobalBinding, globals map[string]eval.Value, namespaces map[string]*Namespace, diags *diag.Diagnostics, opts analyseImportOptions) map[string]analyseBindingImport {
	out := make(map[string]analyseBindingImport)
	resolver := BindingResolver{
		Bindings:   bindings,
		Globals:    globals,
		Namespaces: namespaces,
	}
	expanded, issues := resolver.ExpandWithItems(items, ResolveOptions{Context: ImportIntoAnalyse})
	if opts.EmitDiagnostics && diags != nil {
		emitWithIssues(diags, analyseWithDiagPolicy(), issues)
	}

	tracker := newImportConflictTracker()
	for _, item := range expanded {
		binding := bindings[item.Source]
		if binding == nil {
			continue
		}
		for _, v := range item.Vars {
			values, ok := binding.Vars[v.SourceVar]
			if !ok {
				continue
			}
			value := eval.Null()
			if len(values) > 0 {
				value = values[0]
			}
			if value.Kind != eval.KindString {
				if opts.EmitDiagnostics && diags != nil {
					diags.AddError(
						diag.CodeE422,
						fmt.Sprintf("analyse with-clause variable '%s' from global '%s' must be a string", v.SourceVar, item.Source),
						item.Span,
						"use string-valued globals for analyse imports",
					)
				}
				continue
			}
			prev, conflict, first := tracker.Add(v.Visible, item.Source, item.Span)
			if conflict {
				if opts.EmitDiagnostics && diags != nil && first {
					diags.AddError(
						diag.CodeE214,
						fmt.Sprintf("conflicting analyse import '%s' from globals '%s' and '%s'", v.Visible, prev.Source, item.Source),
						item.Span,
						"import each analyse variable from only one global binding",
						diag.RelatedSpan{Message: "first conflicting import", Span: prev.Span},
					)
				}
				continue
			}
			out[v.Visible] = analyseBindingImport{
				Source:    item.Source,
				SourceVar: v.SourceVar,
				Span:      item.Span,
			}
		}
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
