package sema

import (
	"fmt"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
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
			out.WriteString("$jube_pat_int")
			sawInt = true
		case 'f':
			out.WriteString("$jube_pat_fp")
			sawFloat = true
		case 'w':
			out.WriteString("$jube_pat_wrd")
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

func patternLookupKey(group, name string) string {
	return group + "." + name
}

func compileAnalyseBlock(block ast.AnalyseBlock, res *Result, diags *diag.Diagnostics) *AnalyseSpec {
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
		for _, submit := range res.Submits {
			if submit.Name == block.StepName {
				stepKind = "submit"
				break
			}
		}
	}
	if stepKind == "" {
		diags.AddError(
			diag.CodeE410,
			fmt.Sprintf("unknown analyse target step '%s'", block.StepName),
			block.Span,
			"analyse must reference an existing do/submit block name",
		)
	}
	spec.StepKind = stepKind

	var stepWithItems []ast.WithItem
	for _, doBlock := range res.DoBlocks {
		if doBlock.Name == block.StepName {
			stepWithItems = doBlock.WithItems
			break
		}
	}
	if len(stepWithItems) == 0 {
		for _, submit := range res.Submits {
			if submit.Name == block.StepName {
				stepWithItems = submit.WithItems
				break
			}
		}
	}
	if plan := res.StepImportByName[block.StepName]; plan != nil {
		spec.StepVars = stepVisibleVariablesFromPlan(plan, res.ImportSourceByName)
	} else {
		spec.StepVars = stepVisibleVariables(stepWithItems, res.ImportSourceByName)
	}

	env := make(map[string]eval.Value, len(res.Globals.Values)+32)
	for k, v := range res.Globals.Values {
		env[k] = v
	}
	if plan := res.StepImportByName[block.StepName]; plan != nil {
		addStepValuesToEnvFromPlan(env, plan, res.ImportSourceByName)
	} else {
		addStepValuesToEnvFromWithItems(env, stepWithItems, res.ImportSourceByName)
	}
	analyseImports := resolveAnalyseWithImports(block.WithItems, res, diags)
	for visible, imported := range analyseImports {
		ns := res.LetByName[imported.Source]
		if ns == nil {
			continue
		}
		v, ok := ns.Vars[imported.SourceVar]
		if !ok {
			continue
		}
		env[visible] = v
	}

	seenAssignments := make(map[string]diag.Span, len(block.Assignments))
	assignmentVars := make(map[string]diag.Span, len(block.Assignments))
	for _, assign := range block.Assignments {
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
			warnModeExprInCollections(assign.Expr, diags)
			value := eval.EvalExpr(assign.Expr, env, diags)
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
		warnModeExprInCollections(assign.Expr, diags)
		before := len(diags.Items)
		value := eval.EvalExpr(assign.Expr, env, diags)
		if value.Kind != eval.KindString {
			if hasErrorCodeSince(diags, before, diag.CodeE100) {
				continue
			}
			diags.AddError(
				diag.CodeE412,
				fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string", assign.Name),
				assign.Span,
				"use a string expression such as an imported let variable or a quoted regex pattern",
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

		groupName := ""
		patternName := ""
		if ident, ok := assign.Expr.(ast.IdentExpr); ok {
			if imported, exists := analyseImports[ident.Name]; exists {
				groupName = imported.Source
				patternName = imported.SourceVar
			}
		}
		if groupName == "" {
			groupName = "_ja_" + sanitizeStepName(block.StepName) + "_" + sanitizeStepName(assign.Name)
			patternName = assign.Name
		}

		spec.Assignments = append(spec.Assignments, AnalyseAssignmentSpec{
			Name:    assign.Name,
			Group:   groupName,
			Pattern: patternName,
			File:    assign.File,
			Template: PatternTemplate{
				Group: groupName,
				Name:  patternName,
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

type analyseLetImport struct {
	Source    string
	SourceVar string
	Span      diag.Span
}

func resolveAnalyseWithImports(items []ast.WithItem, res *Result, diags *diag.Diagnostics) map[string]analyseLetImport {
	out := make(map[string]analyseLetImport)
	reported := make(map[string]struct{})
	resolveSource := func(name string) (*ImportSource, bool) {
		_, hasParam := res.ParamByName[name]
		_, hasLet := res.LetByName[name]
		if hasParam && hasLet {
			return nil, true
		}
		return res.ImportSourceByName[name], false
	}
	addImported := func(visible string, src *ImportSource, sourceVar string, span diag.Span) {
		if src == nil {
			return
		}
		if sourceVar == "" {
			sourceVar = visible
		}
		if src.Kind != SourceKindLet {
			diags.AddError(
				diag.CodeE420,
				fmt.Sprintf("analyse with-clause can only import from let namespaces; '%s' is not a let namespace", src.Name),
				span,
				"use `with <let_namespace>` or `with <variable> from <let_namespace>`",
			)
			return
		}
		ns := res.LetByName[src.Name]
		if ns == nil {
			return
		}
		v, ok := ns.Vars[sourceVar]
		if !ok {
			return
		}
		if v.Kind != eval.KindString {
			diags.AddError(
				diag.CodeE422,
				fmt.Sprintf("analyse with-clause variable '%s' from let '%s' must be a string", sourceVar, src.Name),
				span,
				"use string-valued let variables for analyse imports",
			)
			return
		}
		if prev, exists := out[visible]; exists {
			if prev.Source == src.Name && prev.SourceVar == sourceVar {
				return
			}
			a := prev.Source
			b := src.Name
			if a > b {
				a, b = b, a
			}
			key := visible + "|" + a + "|" + b
			if _, seen := reported[key]; seen {
				return
			}
			reported[key] = struct{}{}
			diags.AddError(
				diag.CodeE214,
				fmt.Sprintf("conflicting analyse import '%s' from let namespaces '%s' and '%s'", visible, prev.Source, src.Name),
				span,
				"import each analyse variable from only one let namespace",
				diag.RelatedSpan{Message: "first conflicting import", Span: prev.Span},
			)
			return
		}
		out[visible] = analyseLetImport{
			Source:    src.Name,
			SourceVar: sourceVar,
			Span:      span,
		}
	}

	for _, item := range items {
		if item.From == "" {
			src, ambiguous := resolveSource(item.Name)
			if ambiguous {
				diags.AddError(
					diag.CodeE218,
					fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
					item.Span,
					"disambiguate by renaming the param or let namespace",
				)
				continue
			}
			if src == nil {
				diags.AddError(
					diag.CodeE020,
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"import from an existing let namespace",
				)
				continue
			}
			if src.Kind != SourceKindLet {
				addImported("", src, "", item.Span)
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				addImported(name, src, name, item.Span)
			}
			continue
		}

		src, ambiguous := resolveSource(item.From)
		if ambiguous {
			diags.AddError(
				diag.CodeE218,
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.From),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if src == nil {
			diags.AddError(
				diag.CodeE020,
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"import from an existing let namespace",
			)
			continue
		}
		if src.Kind != SourceKindLet {
			diags.AddError(
				diag.CodeE420,
				fmt.Sprintf("analyse with-clause can only import from let namespaces; '%s' is not a let namespace", src.Name),
				item.Span,
				"use `with <let_namespace>` or `with <variable> from <let_namespace>`",
			)
			continue
		}
		if _, ok := src.Vars[item.Name]; ok {
			addImported(item.Name, src, item.Name, item.Span)
			continue
		}
		fallback, fallbackAmbiguous := resolveSource(item.Name)
		if fallbackAmbiguous {
			diags.AddError(
				diag.CodeE218,
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if fallback != nil {
			if fallback.Kind != SourceKindLet {
				diags.AddError(
					diag.CodeE420,
					fmt.Sprintf("analyse with-clause can only import from let namespaces; '%s' is not a let namespace", fallback.Name),
					item.Span,
					"use `with <let_namespace>` or `with <variable> from <let_namespace>`",
				)
				continue
			}
			for _, name := range planutil.SourceVarNames(fallback.Order, fallback.Vars) {
				addImported(name, fallback, name, item.Span)
			}
			continue
		}
		diags.AddError(
			diag.CodeE021,
			fmt.Sprintf("unknown variable '%s' in source '%s'", item.Name, item.From),
			item.Span,
			"import a variable that exists in the selected let namespace",
		)
	}

	return out
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
