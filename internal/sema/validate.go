package sema

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func Analyze(prog ast.Program, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	resolvedGlobals := resolveTopLevelGlobals(prog, globals, diags)
	res := &Result{
		Program:          prog,
		Globals:          resolvedGlobals,
		LetNamespaces:    make([]*LetNamespace, 0),
		LetByName:        make(map[string]*LetNamespace),
		Paramsets:        make([]*Paramset, 0),
		ParamByName:      make(map[string]*Paramset),
		DoBlocks:         make([]ast.DoBlock, 0),
		Submits:          make([]ast.SubmitBlock, 0),
		SubmitByName:     make(map[string]*SubmitSpec),
		StepImportByName: make(map[string]*StepImportPlan),
		Analyse:          make([]*AnalyseSpec, 0),
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
					"E400",
					fmt.Sprintf("duplicate let block name '%s'", n.Name),
					n.Span,
					"use a unique let block name",
					diag.RelatedSpan{Message: "first definition", Span: prev},
				)
				continue
			}
			letSpans[n.Name] = n.Span
			compiled := compileLetBlock(n, resolvedGlobals.Values, res.LetByName, diags)
			if compiled != nil {
				res.LetNamespaces = append(res.LetNamespaces, compiled)
				res.LetByName[compiled.Name] = compiled
			}
		case ast.ParamBlock:
			if prev, exists := paramSpans[n.Name]; exists {
				diags.AddError(
					"E210",
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

	validateSteps(res, diags)
	validateUseClauses(res, diags)
	buildStepImportPlans(res, diags)
	for _, submit := range res.Submits {
		effective := map[string]VarOrigin{}
		if plan := res.StepImportByName[submit.Name]; plan != nil {
			effective = plan.Effective
		}
		res.SubmitByName[submit.Name] = compileSubmitBlock(submit, res.ParamByName, resolvedGlobals.Values, res.LetByName, effective, diags)
	}
	validateStepVarReferences(res, diags)
	for _, block := range analyseBlocks {
		spec := compileAnalyseBlock(block, res, diags)
		res.Analyse = append(res.Analyse, spec)
	}
	return res
}

var allowedSubmitKeys = map[string]struct{}{
	"account":        {},
	"args_exec":      {},
	"args_starter":   {},
	"executable":     {},
	"gres":           {},
	"mail":           {},
	"measurement":    {},
	"nodes":          {},
	"notification":   {},
	"outlogfile":     {},
	"outerrfile":     {},
	"queue":          {},
	"starter":        {},
	"tasks":          {},
	"threadspertask": {},
	"timelimit":      {},
	"preprocess":     {},
	"postprocess":    {},
}

func compileLetBlock(block ast.LetBlock, globals map[string]eval.Value, lets map[string]*LetNamespace, diags *diag.Diagnostics) *LetNamespace {
	env := make(map[string]eval.Value, len(globals)+16)
	for k, v := range globals {
		env[k] = v
	}
	addQualifiedLetValuesToEnv(env, lets)

	out := &LetNamespace{
		Name:    block.Name,
		Vars:    make(map[string]eval.Value, len(block.Assignments)),
		Modes:   make(map[string]string, len(block.Assignments)),
		Origins: make(map[string]diag.Span, len(block.Assignments)),
		Span:    block.Span,
	}

	seen := make(map[string]diag.Span, len(block.Assignments))
	for _, asn := range block.Assignments {
		if prev, exists := seen[asn.Name]; exists {
			diags.AddError(
				"E401",
				fmt.Sprintf("duplicate variable '%s' in let block '%s'", asn.Name, block.Name),
				asn.Span,
				"use unique variable names within a let block",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		seen[asn.Name] = asn.Span
		warnModeExprInCollections(asn.Expr, diags)
		mode, inner, isModeExpr := unwrapModeExpr(asn.Expr)
		expr := asn.Expr
		if isModeExpr {
			expr = inner
		}
		v := eval.EvalExpr(expr, env, diags)
		if isModeExpr {
			v = coerceModeValue(mode, v, asn.Span, diags)
			out.Modes[asn.Name] = mode
		} else {
			delete(out.Modes, asn.Name)
		}
		if hasNestedList(v) {
			diags.AddError(
				"E305",
				fmt.Sprintf("nested tuple/list value is not allowed for let variable '%s'", asn.Name),
				asn.Span,
				"use flat tuple/list values only",
			)
		}
		out.Vars[asn.Name] = v
		out.Origins[asn.Name] = asn.Span
		env[asn.Name] = v
		env[block.Name+"."+asn.Name] = v
	}

	return out
}

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
			"E410",
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
		spec.StepVars = stepVisibleVariablesFromPlan(plan, res.ParamByName)
	} else {
		spec.StepVars = stepVisibleVariables(stepWithItems, res.ParamByName)
	}

	env := make(map[string]eval.Value, len(res.Globals.Values)+32)
	for k, v := range res.Globals.Values {
		env[k] = v
	}
	addQualifiedLetValuesToEnv(env, res.LetByName)
	if plan := res.StepImportByName[block.StepName]; plan != nil {
		addStepValuesToEnvFromPlan(env, plan, res.ParamByName)
	} else {
		addStepValuesToEnvFromWithItems(env, stepWithItems, res.ParamByName)
	}

	seenAssignments := make(map[string]diag.Span, len(block.Assignments))
	assignmentVars := make(map[string]diag.Span, len(block.Assignments))
	for _, assign := range block.Assignments {
		if prev, exists := seenAssignments[assign.Name]; exists {
			diags.AddError(
				"E414",
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
					"W320",
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
					"E305",
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
				"E413",
				fmt.Sprintf("analyse extraction variable '%s' collides with step-visible variable", assign.Name),
				assign.Span,
				"use a distinct extraction variable name in analyse",
				diag.RelatedSpan{Message: "step variable", Span: existing},
			)
			continue
		}
		warnModeExprInCollections(assign.Expr, diags)
		value := eval.EvalExpr(assign.Expr, env, diags)
		if value.Kind != eval.KindString {
			diags.AddError(
				"E412",
				fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string", assign.Name),
				assign.Span,
				"use a string expression such as let_namespace.variable or a quoted regex pattern",
			)
			continue
		}
		regex, inferredType, ok := normalizePatternRegex(value.S)
		if !ok {
			diags.AddError(
				"E402",
				fmt.Sprintf("invalid placeholder in analyse extraction expression for '%s'", assign.Name),
				assign.Span,
				"supported placeholders are %d, %f, %w and %% for a literal percent",
			)
			continue
		}

		groupName := ""
		patternName := ""
		if q, ok := assign.Expr.(ast.QualifiedIdentExpr); ok {
			if ns, exists := res.LetByName[q.Namespace]; exists {
				if _, exists := ns.Vars[q.Name]; exists {
					groupName = q.Namespace
					patternName = q.Name
				}
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
			"E415",
			fmt.Sprintf("unknown symbol '%s' in analyse result tuple", col.Name),
			col.Span,
			"use a step-visible variable or an analyse extraction alias",
		)
	}

	return spec
}

func stepVisibleVariables(items []ast.WithItem, params map[string]*Paramset) map[string]diag.Span {
	out := make(map[string]diag.Span)
	for _, item := range items {
		if item.From == "" {
			ps := params[item.Name]
			if ps == nil {
				continue
			}
			for _, name := range ps.Order {
				if _, exists := out[name]; exists {
					continue
				}
				if origin, ok := ps.Origins[name]; ok {
					out[name] = origin
				} else {
					out[name] = item.Span
				}
			}
			continue
		}

		ps := params[item.From]
		if ps == nil {
			continue
		}
		if _, ok := ps.Vars[item.Name]; ok {
			if _, exists := out[item.Name]; !exists {
				if origin, ok := ps.Origins[item.Name]; ok {
					out[item.Name] = origin
				} else {
					out[item.Name] = item.Span
				}
			}
			continue
		}
		fallback := params[item.Name]
		if fallback == nil {
			continue
		}
		for _, name := range fallback.Order {
			if _, exists := out[name]; exists {
				continue
			}
			if origin, ok := fallback.Origins[name]; ok {
				out[name] = origin
			} else {
				out[name] = item.Span
			}
		}
	}
	return out
}

func stepVisibleVariablesFromPlan(plan *StepImportPlan, params map[string]*Paramset) map[string]diag.Span {
	out := make(map[string]diag.Span, len(plan.Effective))
	for name, origin := range plan.Effective {
		if ps := params[origin.Paramset]; ps != nil {
			if span, ok := ps.Origins[name]; ok {
				out[name] = span
				continue
			}
		}
		out[name] = origin.Span
	}
	return out
}

func addQualifiedLetValuesToEnv(env map[string]eval.Value, lets map[string]*LetNamespace) {
	if len(lets) == 0 {
		return
	}
	namespaces := make([]string, 0, len(lets))
	for name := range lets {
		namespaces = append(namespaces, name)
	}
	sort.Strings(namespaces)
	for _, nsName := range namespaces {
		ns := lets[nsName]
		if ns == nil {
			continue
		}
		varNames := make([]string, 0, len(ns.Vars))
		for name := range ns.Vars {
			varNames = append(varNames, name)
		}
		sort.Strings(varNames)
		for _, name := range varNames {
			env[nsName+"."+name] = ns.Vars[name]
		}
	}
}

func addStepValuesToEnvFromPlan(env map[string]eval.Value, plan *StepImportPlan, params map[string]*Paramset) {
	if plan == nil {
		return
	}
	for name, origin := range plan.Effective {
		ps := params[origin.Paramset]
		if ps == nil {
			continue
		}
		if vals, ok := ps.Vars[name]; ok {
			env[name] = seriesAsValue(vals)
		}
	}
}

func addStepValuesToEnvFromWithItems(env map[string]eval.Value, items []ast.WithItem, params map[string]*Paramset) {
	imports := resolveImportedVars(items, params)
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		ps := params[origins[0].Paramset]
		if ps == nil {
			continue
		}
		if vals, ok := ps.Vars[name]; ok {
			env[name] = seriesAsValue(vals)
		}
	}
}

func resolveTopLevelGlobals(prog ast.Program, defaults map[string]eval.Value, diags *diag.Diagnostics) GlobalState {
	values := make(map[string]eval.Value, len(defaults))
	for k, v := range defaults {
		values[k] = v
	}
	modes := make(map[string]string)
	spans := make(map[string]diag.Span)
	known := make(map[string]struct{}, len(defaults))
	for name := range defaults {
		known[name] = struct{}{}
	}

	for _, stmt := range prog.Stmts {
		assign, ok := stmt.(ast.GlobalAssign)
		if !ok {
			continue
		}
		if _, ok := known[assign.Name]; !ok {
			diags.AddError(
				"E300",
				fmt.Sprintf("unknown global variable '%s'", assign.Name),
				assign.Span,
				"use `jbs help globals` to list supported globals",
			)
			continue
		}
		warnModeExprInCollections(assign.Expr, diags)
		if prev, exists := spans[assign.Name]; exists {
			diags.AddWarning(
				"W300",
				fmt.Sprintf("global variable '%s' reassigned; last value wins", assign.Name),
				assign.Span,
				"remove duplicate assignments to avoid ambiguity",
				diag.RelatedSpan{Message: "previous assignment", Span: prev},
			)
		}
		if assign.Name == "jbs_name" || assign.Name == "jbs_outpath" {
			if _, isMode := assign.Expr.(ast.ModeExpr); isMode {
				diags.AddError(
					"E303",
					fmt.Sprintf("%s must be a simple string, not shell()/python()", assign.Name),
					assign.Span,
					"assign a plain string literal",
				)
				continue
			}
			if _, ok := assign.Expr.(ast.StringExpr); !ok {
				code := "E301"
				if assign.Name == "jbs_outpath" {
					code = "E302"
				}
				diags.AddError(
					code,
					fmt.Sprintf("%s must be a simple string literal", assign.Name),
					assign.Span,
					"assign a plain quoted string",
				)
				continue
			}
			v := eval.EvalExpr(assign.Expr, values, diags)
			if v.Kind != eval.KindString {
				code := "E301"
				if assign.Name == "jbs_outpath" {
					code = "E302"
				}
				diags.AddError(
					code,
					fmt.Sprintf("%s must be a simple string literal", assign.Name),
					assign.Span,
					"assign a plain quoted string",
				)
				continue
			}
			values[assign.Name] = v
			delete(modes, assign.Name)
			spans[assign.Name] = assign.Span
			continue
		}

		mode, inner, isModeExpr := unwrapModeExpr(assign.Expr)
		expr := assign.Expr
		if isModeExpr {
			expr = inner
		}
		v := eval.EvalExpr(expr, values, diags)
		if isModeExpr {
			v = coerceModeValue(mode, v, assign.Span, diags)
			modes[assign.Name] = mode
		} else {
			delete(modes, assign.Name)
		}
		if !isScalarGlobalValue(v) {
			diags.AddError(
				"E304",
				fmt.Sprintf("global variable '%s' must be scalar; tuples/lists are not allowed", assign.Name),
				assign.Span,
				"use string/int/float/bool or shell()/python() scalar values",
			)
			continue
		}
		values[assign.Name] = v
		spans[assign.Name] = assign.Span
	}

	return GlobalState{
		Values: values,
		Modes:  modes,
		Spans:  spans,
	}
}

func isScalarGlobalValue(v eval.Value) bool {
	switch v.Kind {
	case eval.KindString, eval.KindInt, eval.KindFloat, eval.KindBool, eval.KindNull:
		return true
	default:
		return false
	}
}

func hasNestedList(v eval.Value) bool {
	if v.Kind != eval.KindList {
		return false
	}
	for _, item := range v.L {
		if item.Kind == eval.KindList {
			return true
		}
		if hasNestedList(item) {
			return true
		}
	}
	return false
}

func compileSubmitBlock(block ast.SubmitBlock, known map[string]*Paramset, globals map[string]eval.Value, lets map[string]*LetNamespace, effective map[string]VarOrigin, diags *diag.Diagnostics) *SubmitSpec {
	env := make(map[string]eval.Value, len(globals)+16)
	for k, v := range globals {
		env[k] = v
	}
	addQualifiedLetValuesToEnv(env, lets)

	for name, origin := range effective {
		src := known[origin.Paramset]
		if src == nil {
			continue
		}
		if vals, ok := src.Vars[name]; ok {
			env[name] = seriesAsValue(vals)
		}
	}

	spec := &SubmitSpec{
		Name:   block.Name,
		Values: make([]SubmitValue, 0, len(block.Fields)),
		Span:   block.Span,
	}
	seen := make(map[string]diag.Span)
	for _, field := range block.Fields {
		if _, ok := allowedSubmitKeys[field.Name]; !ok {
			diags.AddError(
				"E072",
				fmt.Sprintf("unknown submit key '%s'", field.Name),
				field.Span,
				"use one of the allowed submit keys",
			)
			continue
		}
		if prev, exists := seen[field.Name]; exists {
			diags.AddError(
				"E075",
				fmt.Sprintf("duplicate submit key '%s'", field.Name),
				field.Span,
				"set each submit key at most once",
				diag.RelatedSpan{Message: "first assignment", Span: prev},
			)
			continue
		}
		seen[field.Name] = field.Span

		if field.IsRaw {
			if !isRawSubmitKey(field.Name) {
				diags.AddError(
					"E074",
					fmt.Sprintf("submit key '%s' does not accept raw blocks", field.Name),
					field.Span,
					"use an expression value for this key",
				)
				continue
			}
			spec.Values = append(spec.Values, SubmitValue{
				Name:  field.Name,
				Raw:   field.Raw,
				IsRaw: true,
				Span:  field.Span,
			})
			continue
		}

		if isRawSubmitKey(field.Name) {
			diags.AddError(
				"E073",
				fmt.Sprintf("submit key '%s' must use a raw block", field.Name),
				field.Span,
				fmt.Sprintf("use syntax: %s = { ... }", field.Name),
			)
			continue
		}
		if field.Expr == nil {
			diags.AddError(
				"E076",
				fmt.Sprintf("submit key '%s' is missing a value expression", field.Name),
				field.Span,
				"use syntax: key = expression",
			)
			continue
		}
		warnModeExprInCollections(field.Expr, diags)

		mode, inner, isModeExpr := unwrapModeExpr(field.Expr)
		expr := field.Expr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExpr(expr, env, diags)
		if isModeExpr {
			value = coerceModeValue(mode, value, field.Span, diags)
		}
		if hasNestedList(value) {
			diags.AddError(
				"E305",
				fmt.Sprintf("nested tuple/list value is not allowed for submit key '%s'", field.Name),
				field.Span,
				"use flat tuple/list values only",
			)
		}
		spec.Values = append(spec.Values, SubmitValue{
			Name:  field.Name,
			Mode:  mode,
			Value: value,
			Span:  field.Span,
		})
	}
	return spec
}

func isRawSubmitKey(name string) bool {
	return name == "preprocess" || name == "postprocess"
}

func compileParamBlock(block ast.ParamBlock, known map[string]*Paramset, globals map[string]eval.Value, lets map[string]*LetNamespace, diags *diag.Diagnostics) *Paramset {
	env := make(map[string]eval.Value, len(globals)+16)
	origins := make(map[string]diag.Span, len(globals)+16)
	modes := make(map[string]string, 16)
	for k, v := range globals {
		env[k] = v
	}
	addQualifiedLetValuesToEnv(env, lets)

	resolveSource := func(name string) (*Paramset, *LetNamespace, bool) {
		ps := known[name]
		ls := lets[name]
		if ps != nil && ls != nil {
			return nil, nil, true
		}
		return ps, ls, false
	}
	letVarNames := func(ns *LetNamespace) []string {
		names := make([]string, 0, len(ns.Vars))
		for name := range ns.Vars {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}

	for _, item := range block.WithItems {
		if item.From == "" {
			srcParam, srcLet, ambiguous := resolveSource(item.Name)
			if ambiguous {
				diags.AddError(
					"E022",
					fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
					item.Span,
					"disambiguate by renaming the param or let namespace",
				)
				continue
			}
			if srcLet != nil {
				for _, name := range letVarNames(srcLet) {
					env[name] = srcLet.Vars[name]
					if origin, ok := srcLet.Origins[name]; ok {
						origins[name] = origin
					}
					if mode, ok := srcLet.Modes[name]; ok {
						modes[name] = mode
					}
				}
				continue
			}
			if srcParam == nil {
				diags.AddError(
					"E020",
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"define/import the parameterset or let namespace before using it",
				)
				continue
			}
			for _, name := range srcParam.Order {
				env[name] = seriesAsValue(srcParam.Vars[name])
				if origin, ok := srcParam.Origins[name]; ok {
					origins[name] = origin
				}
				if mode, ok := srcParam.Modes[name]; ok {
					modes[name] = mode
				}
			}
			continue
		}

		srcParam, srcLet, ambiguous := resolveSource(item.From)
		if ambiguous {
			diags.AddError(
				"E022",
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.From),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if srcParam == nil && srcLet == nil {
			diags.AddError(
				"E020",
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"define/import the parameterset or let namespace before using it",
			)
			continue
		}
		if srcParam != nil {
			vals, ok := srcParam.Vars[item.Name]
			if ok {
				env[item.Name] = seriesAsValue(vals)
				if origin, ok := srcParam.Origins[item.Name]; ok {
					origins[item.Name] = origin
				}
				if mode, ok := srcParam.Modes[item.Name]; ok {
					modes[item.Name] = mode
				}
				continue
			}
		}
		if srcLet != nil {
			if v, ok := srcLet.Vars[item.Name]; ok {
				env[item.Name] = v
				if origin, ok := srcLet.Origins[item.Name]; ok {
					origins[item.Name] = origin
				}
				if mode, ok := srcLet.Modes[item.Name]; ok {
					modes[item.Name] = mode
				}
				continue
			}
		}

		// Mixed form support:
		// with x from p1, p2
		// If "p2" is not a variable in p1 but is an existing parameterset,
		// interpret it as importing the whole parameterset p2.
		if fallbackParam, fallbackLet, fallbackAmbiguous := resolveSource(item.Name); fallbackAmbiguous {
			diags.AddError(
				"E022",
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		} else if fallbackParam != nil {
			for _, name := range fallbackParam.Order {
				env[name] = seriesAsValue(fallbackParam.Vars[name])
				if origin, exists := fallbackParam.Origins[name]; exists {
					origins[name] = origin
				}
				if mode, exists := fallbackParam.Modes[name]; exists {
					modes[name] = mode
				}
			}
			continue
		} else if fallbackLet != nil {
			for _, name := range letVarNames(fallbackLet) {
				env[name] = fallbackLet.Vars[name]
				if origin, exists := fallbackLet.Origins[name]; exists {
					origins[name] = origin
				}
				if mode, exists := fallbackLet.Modes[name]; exists {
					modes[name] = mode
				}
			}
			continue
		}

		diags.AddError(
			"E021",
			fmt.Sprintf("unknown variable '%s' in source '%s'", item.Name, item.From),
			item.Span,
			"import a variable that exists in the selected source",
		)
	}

	for _, asn := range block.Assignments {
		warnModeExprInCollections(asn.Expr, diags)
		mode, inner, isModeExpr := unwrapModeExpr(asn.Expr)
		expr := asn.Expr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExpr(expr, env, diags)
		if isModeExpr {
			value = coerceModeValue(mode, value, asn.Span, diags)
			modes[asn.Name] = mode
		} else {
			delete(modes, asn.Name)
		}
		if hasNestedList(value) {
			diags.AddError(
				"E305",
				fmt.Sprintf("nested tuple/list value is not allowed for param variable '%s'", asn.Name),
				asn.Span,
				"use flat tuple/list values only",
			)
		}
		env[asn.Name] = value
		origins[asn.Name] = asn.Span
	}

	series := make(map[string][]eval.Value, len(env))
	for name, value := range env {
		series[name] = eval.ToSeries(value)
	}

	if block.Final == nil {
		return &Paramset{
			Name:    block.Name,
			Block:   block,
			Rows:    nil,
			Vars:    map[string][]eval.Value{},
			Origins: map[string]diag.Span{},
			Modes:   map[string]string{},
			Order:   nil,
			HasPlus: false,
		}
	}

	rows := eval.EvalCombination(block.Final, series, origins, diags)
	if rows == nil {
		rows = make([]eval.Row, 0)
	}

	order := combIdentOrder(block.Final)
	vars := make(map[string][]eval.Value, len(order))
	varOrigins := make(map[string]diag.Span, len(order))

	for _, name := range order {
		values := make([]eval.Value, 0, len(rows))
		for _, row := range rows {
			cell, ok := row.Values[name]
			if !ok {
				continue
			}
			values = append(values, cell.Value)
			if _, exists := varOrigins[name]; !exists && !cell.Origin.IsZero() {
				varOrigins[name] = cell.Origin
			}
		}
		if len(values) == 0 {
			if s, ok := series[name]; ok {
				values = append(values, s...)
			}
		}
		vars[name] = values
		if _, exists := varOrigins[name]; !exists {
			if o, ok := origins[name]; ok {
				varOrigins[name] = o
			}
		}
	}

	return &Paramset{
		Name:    block.Name,
		Block:   block,
		Rows:    rows,
		Vars:    vars,
		Origins: varOrigins,
		Modes:   modes,
		Order:   order,
		HasPlus: combHasOp(block.Final, "+"),
	}
}

func warnModeExprInCollections(expr ast.Expr, diags *diag.Diagnostics) {
	var walk func(ast.Expr, bool)
	walk = func(node ast.Expr, inCollection bool) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.ModeExpr:
			if inCollection {
				diags.AddWarning(
					"W301",
					fmt.Sprintf("%s(...) used inside tuple/list expression", n.Mode),
					n.Span,
					"use shell()/python() as a standalone assignment value, then reference the variable",
				)
			}
			walk(n.Expr, inCollection)
		case ast.ListExpr:
			for _, item := range n.Items {
				walk(item, true)
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				walk(item, true)
			}
		case ast.UnaryExpr:
			walk(n.Expr, inCollection)
		case ast.BinaryExpr:
			walk(n.Left, inCollection)
			walk(n.Right, inCollection)
		case ast.CompareExpr:
			walk(n.Left, inCollection)
			walk(n.Right, inCollection)
		case ast.ConditionalExpr:
			walk(n.Then, inCollection)
			walk(n.Cond, inCollection)
			walk(n.Else, inCollection)
		}
	}
	walk(expr, false)
}

func seriesAsValue(v []eval.Value) eval.Value {
	if len(v) == 0 {
		return eval.Null()
	}
	if len(v) == 1 {
		return v[0]
	}
	out := make([]eval.Value, len(v))
	copy(out, v)
	return eval.List(out)
}

func unwrapModeExpr(expr ast.Expr) (string, ast.Expr, bool) {
	modeExpr, ok := expr.(ast.ModeExpr)
	if !ok {
		return "", nil, false
	}
	return modeExpr.Mode, modeExpr.Expr, true
}

func coerceModeValue(mode string, value eval.Value, at diag.Span, diags *diag.Diagnostics) eval.Value {
	switch value.Kind {
	case eval.KindString:
		return value
	case eval.KindList:
		items := make([]eval.Value, len(value.L))
		for i, it := range value.L {
			if it.Kind != eval.KindString {
				diags.AddError(
					"E215",
					fmt.Sprintf("%s(...) requires string values", mode),
					at,
					"pass a string expression to mode declarations",
				)
			}
			items[i] = eval.String(it.String())
		}
		return eval.List(items)
	default:
		diags.AddError(
			"E215",
			fmt.Sprintf("%s(...) requires string values", mode),
			at,
			"pass a string expression to mode declarations",
		)
		return eval.String(value.String())
	}
}

func combHasOp(expr ast.CombExpr, op string) bool {
	switch e := expr.(type) {
	case ast.CombBinary:
		if e.Op == op {
			return true
		}
		return combHasOp(e.Left, op) || combHasOp(e.Right, op)
	default:
		return false
	}
}

func combIdentOrder(expr ast.CombExpr) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	var walk func(ast.CombExpr)
	walk = func(node ast.CombExpr) {
		switch n := node.(type) {
		case ast.CombIdent:
			if n.Name == "" {
				return
			}
			if _, ok := seen[n.Name]; ok {
				return
			}
			seen[n.Name] = struct{}{}
			out = append(out, n.Name)
		case ast.CombBinary:
			walk(n.Left)
			walk(n.Right)
		}
	}
	walk(expr)
	return out
}

func validateSteps(res *Result, diags *diag.Diagnostics) {
	nameToSpan := make(map[string]diag.Span)
	edges := make(map[string][]string)

	for _, b := range res.DoBlocks {
		if prev, exists := nameToSpan[b.Name]; exists {
			diags.AddError(
				"E211",
				fmt.Sprintf("duplicate step name '%s'", b.Name),
				b.Span,
				"use unique names for do/submit blocks",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		nameToSpan[b.Name] = b.Span
		edges[b.Name] = append([]string(nil), b.After...)
	}
	for _, b := range res.Submits {
		if prev, exists := nameToSpan[b.Name]; exists {
			diags.AddError(
				"E211",
				fmt.Sprintf("duplicate step name '%s'", b.Name),
				b.Span,
				"use unique names for do/submit blocks",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		nameToSpan[b.Name] = b.Span
		edges[b.Name] = append([]string(nil), b.After...)
	}

	for step, deps := range edges {
		for _, dep := range deps {
			if _, ok := nameToSpan[dep]; !ok {
				diags.AddError(
					"E212",
					fmt.Sprintf("unknown dependency '%s' for step '%s'", dep, step),
					nameToSpan[step],
					"depend only on existing do/submit block names",
				)
			}
		}
	}

	state := make(map[string]int)
	stack := make([]string, 0)
	var visit func(string)
	visit = func(node string) {
		state[node] = 1
		stack = append(stack, node)
		for _, dep := range edges[node] {
			if _, ok := edges[dep]; !ok {
				continue
			}
			if state[dep] == 0 {
				visit(dep)
				continue
			}
			if state[dep] == 1 {
				cycle := append(stack, dep)
				diags.AddError(
					"E213",
					fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")),
					nameToSpan[node],
					"remove cyclic step dependencies",
					diag.RelatedSpan{Message: "cycle reference", Span: nameToSpan[dep]},
				)
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
	}

	names := make([]string, 0, len(edges))
	for name := range edges {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if state[name] == 0 {
			visit(name)
		}
	}
}

func validateUseClauses(res *Result, diags *diag.Diagnostics) {
	for _, block := range res.DoBlocks {
		validateWithItems(block.WithItems, res.ParamByName, diags)
	}
	for _, block := range res.Submits {
		validateWithItems(block.WithItems, res.ParamByName, diags)
	}
}

type stepDefinition struct {
	Name      string
	After     []string
	WithItems []ast.WithItem
	Span      diag.Span
}

type expandedWithImport struct {
	Source string
	Vars   []string
	Full   bool
	Span   diag.Span
}

func buildStepImportPlans(res *Result, diags *diag.Diagnostics) {
	defs, order := collectStepDefinitions(res)
	plans := make(map[string]*StepImportPlan, len(defs))
	for _, stepName := range topoStepOrder(defs, order) {
		def, ok := defs[stepName]
		if !ok {
			continue
		}
		reported := make(map[string]struct{})
		reportConflict := func(name string, left VarOrigin, right VarOrigin, at diag.Span, relation string) {
			a := left.Paramset
			b := right.Paramset
			if a == b {
				return
			}
			if a > b {
				a, b = b, a
			}
			key := name + "|" + a + "|" + b + "|" + relation
			if _, exists := reported[key]; exists {
				return
			}
			reported[key] = struct{}{}
			diags.AddError(
				"E214",
				fmt.Sprintf(
					"conflicting variable '%s' for step '%s' from parametersets '%s' and '%s'",
					name,
					stepName,
					left.Paramset,
					right.Paramset,
				),
				at,
				"import each variable name from only one source parameterset",
				diag.RelatedSpan{Message: "first conflicting source", Span: left.Span},
				diag.RelatedSpan{Message: "second conflicting source", Span: right.Span},
			)
		}

		inherited := make(map[string]VarOrigin)
		inheritedSteps := make([]string, 0, len(def.After))
		seenStep := make(map[string]struct{}, len(def.After))
		for _, dep := range def.After {
			if _, exists := seenStep[dep]; !exists {
				seenStep[dep] = struct{}{}
				inheritedSteps = append(inheritedSteps, dep)
			}
			depPlan := plans[dep]
			if depPlan == nil {
				continue
			}
			for name, origin := range depPlan.Effective {
				if prev, exists := inherited[name]; exists {
					if prev.Paramset != origin.Paramset {
						reportConflict(name, prev, origin, def.Span, "inherited")
					}
					continue
				}
				inherited[name] = origin
			}
		}

		explicitDelta := make([]ast.WithItem, 0)
		selected := make(map[string]VarOrigin)
		for _, item := range def.WithItems {
			expanded, ok := expandWithItem(item, res.ParamByName)
			if !ok {
				continue
			}
			kept := make([]string, 0, len(expanded.Vars))
			sourceParam := res.ParamByName[expanded.Source]
			for _, name := range expanded.Vars {
				originSpan := item.Span
				if sourceParam != nil {
					if origin, ok := sourceParam.Origins[name]; ok && !origin.IsZero() {
						originSpan = origin
					}
				}
				current := VarOrigin{
					Name:     name,
					Paramset: expanded.Source,
					Span:     originSpan,
				}
				if prev, exists := inherited[name]; exists {
					if prev.Paramset != current.Paramset {
						reportConflict(name, prev, current, item.Span, "explicit_vs_inherited")
					}
					continue
				}
				if prev, exists := selected[name]; exists {
					if prev.Paramset != current.Paramset {
						// Explicit-with conflicts are already diagnosed by validateWithItems.
						continue
					}
					continue
				}
				selected[name] = current
				kept = append(kept, name)
			}
			if len(kept) == 0 {
				continue
			}
			if expanded.Full && len(kept) == len(expanded.Vars) {
				explicitDelta = append(explicitDelta, ast.WithItem{
					Name: expanded.Source,
					Span: item.Span,
				})
				continue
			}
			for _, name := range kept {
				explicitDelta = append(explicitDelta, ast.WithItem{
					Name: name,
					From: expanded.Source,
					Span: item.Span,
				})
			}
		}

		effective := make(map[string]VarOrigin, len(inherited)+len(selected))
		for name, origin := range inherited {
			effective[name] = origin
		}
		for name, origin := range selected {
			if prev, exists := effective[name]; exists {
				if prev.Paramset != origin.Paramset {
					reportConflict(name, prev, origin, def.Span, "effective")
					continue
				}
			}
			effective[name] = origin
		}

		plans[stepName] = &StepImportPlan{
			StepName:       stepName,
			Inherited:      inherited,
			ExplicitDelta:  explicitDelta,
			Effective:      effective,
			InheritedSteps: inheritedSteps,
		}
	}
	res.StepImportByName = plans
}

func collectStepDefinitions(res *Result) (map[string]stepDefinition, []string) {
	defs := make(map[string]stepDefinition)
	order := make([]string, 0)
	for _, stmt := range res.Program.Stmts {
		switch node := stmt.(type) {
		case ast.DoBlock:
			if _, exists := defs[node.Name]; exists {
				continue
			}
			defs[node.Name] = stepDefinition{
				Name:      node.Name,
				After:     append([]string(nil), node.After...),
				WithItems: append([]ast.WithItem(nil), node.WithItems...),
				Span:      node.Span,
			}
			order = append(order, node.Name)
		case ast.SubmitBlock:
			if _, exists := defs[node.Name]; exists {
				continue
			}
			defs[node.Name] = stepDefinition{
				Name:      node.Name,
				After:     append([]string(nil), node.After...),
				WithItems: append([]ast.WithItem(nil), node.WithItems...),
				Span:      node.Span,
			}
			order = append(order, node.Name)
		}
	}
	return defs, order
}

func topoStepOrder(defs map[string]stepDefinition, preferred []string) []string {
	state := make(map[string]int, len(defs))
	order := make([]string, 0, len(defs))
	var visit func(string)
	visit = func(name string) {
		if state[name] == 2 {
			return
		}
		if state[name] == 1 {
			return
		}
		def, ok := defs[name]
		if !ok {
			return
		}
		state[name] = 1
		for _, dep := range def.After {
			if _, exists := defs[dep]; exists {
				visit(dep)
			}
		}
		state[name] = 2
		order = append(order, name)
	}
	for _, name := range preferred {
		visit(name)
	}
	extra := make([]string, 0, len(defs))
	for name := range defs {
		extra = append(extra, name)
	}
	sort.Strings(extra)
	for _, name := range extra {
		visit(name)
	}
	return order
}

func expandWithItem(item ast.WithItem, params map[string]*Paramset) (expandedWithImport, bool) {
	if item.From == "" {
		ps := params[item.Name]
		if ps == nil {
			return expandedWithImport{}, false
		}
		return expandedWithImport{
			Source: ps.Name,
			Vars:   exposedVarNames(ps),
			Full:   true,
			Span:   item.Span,
		}, true
	}

	src := params[item.From]
	if src == nil {
		return expandedWithImport{}, false
	}
	if _, ok := src.Vars[item.Name]; ok {
		return expandedWithImport{
			Source: src.Name,
			Vars:   []string{item.Name},
			Full:   false,
			Span:   item.Span,
		}, true
	}
	fallback := params[item.Name]
	if fallback == nil {
		return expandedWithImport{}, false
	}
	return expandedWithImport{
		Source: fallback.Name,
		Vars:   exposedVarNames(fallback),
		Full:   true,
		Span:   item.Span,
	}, true
}

type importedVar struct {
	Name     string
	Paramset string
	Span     diag.Span
}

type varRef struct {
	Name string
	Span diag.Span
}

func validateStepVarReferences(res *Result, diags *diag.Diagnostics) {
	exposedByParam := make(map[string]map[string]diag.Span)
	paramsetsByVar := make(map[string][]string)
	used := make(map[string]map[string]bool)

	for _, ps := range res.Paramsets {
		varNames := exposedVarNames(ps)
		if len(varNames) == 0 {
			continue
		}
		if _, ok := exposedByParam[ps.Name]; !ok {
			exposedByParam[ps.Name] = make(map[string]diag.Span)
		}
		for _, name := range varNames {
			origin := ps.Origins[name]
			if origin.IsZero() {
				origin = ps.Block.Span
			}
			exposedByParam[ps.Name][name] = origin
			if !containsString(paramsetsByVar[name], ps.Name) {
				paramsetsByVar[name] = append(paramsetsByVar[name], ps.Name)
			}
		}
	}

	markUsedExact := func(psName, name string) {
		if _, ok := used[psName]; !ok {
			used[psName] = make(map[string]bool)
		}
		used[psName][name] = true
	}

	markUsedByImports := func(name string, imports []importedVar) {
		for _, imp := range imports {
			markUsedExact(imp.Paramset, name)
		}
	}

	markUsedCandidates := func(name string, candidates []string) {
		for _, psName := range candidates {
			markUsedExact(psName, name)
		}
	}

	warnMissing := func(stepName string, ref varRef, candidates []string) {
		if len(candidates) == 0 {
			return
		}
		originSpan := diag.Span{}
		source := candidates[0]
		if byVar, ok := exposedByParam[source]; ok {
			originSpan = byVar[ref.Name]
		}
		related := []diag.RelatedSpan{}
		if !originSpan.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("parameter source '%s'", source),
				Span:    originSpan,
			})
		}
		diags.AddWarning(
			"W311",
			fmt.Sprintf("variable '%s' is referenced in step '%s' but not imported via with-clause", ref.Name, stepName),
			ref.Span,
			"add `with <paramset>` or `with <variable> from <paramset>`",
			related...,
		)
	}

	processStep := func(stepName string, withItems []ast.WithItem, refs []varRef) {
		imports := resolveImportedVars(withItems, res.ParamByName)
		if plan := res.StepImportByName[stepName]; plan != nil {
			imports = resolveImportedVarsFromPlan(plan)
		}
		warned := make(map[string]struct{})
		for _, ref := range refs {
			candidates := paramsetsByVar[ref.Name]
			if len(candidates) == 0 {
				continue
			}
			origins := imports[ref.Name]
			if len(origins) > 0 {
				markUsedByImports(ref.Name, origins)
				continue
			}
			markUsedCandidates(ref.Name, candidates)
			key := stepName + "::" + ref.Name
			if _, exists := warned[key]; exists {
				continue
			}
			warned[key] = struct{}{}
			warnMissing(stepName, ref, candidates)
		}
	}

	for _, block := range res.DoBlocks {
		base := block.BodyStart
		if base.Line == 0 {
			base = block.Span.Start
		}
		refs := collectShellLikeRefs(block.Body, base, block.Span.File)
		processStep(block.Name, block.WithItems, refs)
	}
	for _, block := range res.Submits {
		refs := make([]varRef, 0)
		for _, field := range block.Fields {
			if field.IsRaw {
				base := field.RawStart
				if base.Line == 0 {
					base = field.Span.Start
				}
				refs = append(refs, collectShellLikeRefs(field.Raw, base, field.Span.File)...)
				continue
			}
			refs = append(refs, collectExprStringRefs(field.Expr)...)
		}
		processStep(block.Name, block.WithItems, refs)
	}

	for psName, byVar := range exposedByParam {
		for varName, origin := range byVar {
			if used[psName][varName] {
				continue
			}
			diags.AddWarning(
				"W310",
				fmt.Sprintf("exposed variable '%s' from param '%s' is never used in any do/submit block", varName, psName),
				origin,
				fmt.Sprintf("remove it from the final expression or reference it with $%s/${%s} in a step", varName, varName),
			)
		}
	}
}

func exposedVarNames(ps *Paramset) []string {
	if len(ps.Order) > 0 {
		names := make([]string, len(ps.Order))
		copy(names, ps.Order)
		return names
	}
	names := make([]string, 0, len(ps.Vars))
	for name := range ps.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func resolveImportedVars(items []ast.WithItem, params map[string]*Paramset) map[string][]importedVar {
	out := make(map[string][]importedVar)
	seen := make(map[string]struct{})
	add := func(name, paramset string, span diag.Span) {
		key := paramset + "::" + name
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out[name] = append(out[name], importedVar{
			Name:     name,
			Paramset: paramset,
			Span:     span,
		})
	}

	for _, item := range items {
		if item.From == "" {
			ps, ok := params[item.Name]
			if !ok {
				continue
			}
			for _, name := range exposedVarNames(ps) {
				add(name, ps.Name, item.Span)
			}
			continue
		}

		src, ok := params[item.From]
		if !ok {
			continue
		}
		if _, ok := src.Vars[item.Name]; ok {
			add(item.Name, src.Name, item.Span)
			continue
		}
		if fallback, ok := params[item.Name]; ok {
			for _, name := range exposedVarNames(fallback) {
				add(name, fallback.Name, item.Span)
			}
		}
	}
	return out
}

func resolveImportedVarsFromPlan(plan *StepImportPlan) map[string][]importedVar {
	out := make(map[string][]importedVar, len(plan.Effective))
	for name, origin := range plan.Effective {
		out[name] = append(out[name], importedVar{
			Name:     name,
			Paramset: origin.Paramset,
			Span:     origin.Span,
		})
	}
	return out
}

func collectShellLikeRefs(text string, base diag.Position, file string) []varRef {
	runes := []rune(text)
	refs := make([]varRef, 0)
	line := base.Line
	col := base.Column
	off := base.Offset
	i := 0

	advance := func() {
		if i >= len(runes) {
			return
		}
		r := runes[i]
		i++
		off++
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	advanceN := func(target int) {
		for i < target {
			advance()
		}
	}

	for i < len(runes) {
		if runes[i] != '$' || isEscapedDollar(runes, i) {
			advance()
			continue
		}
		start := diag.NewPos(off, line, col)
		if i+1 < len(runes) && runes[i+1] == '{' {
			j, ok := parseVarPath(runes, i+2)
			if ok && j < len(runes) && runes[j] == '}' {
				name := string(runes[i+2 : j])
				advanceN(j + 1)
				end := diag.NewPos(off, line, col)
				refs = append(refs, varRef{
					Name: name,
					Span: diag.NewSpan(file, start, end),
				})
				continue
			}
			advance()
			continue
		}
		if j, ok := parseVarPath(runes, i+1); ok {
			name := string(runes[i+1 : j])
			advanceN(j)
			end := diag.NewPos(off, line, col)
			refs = append(refs, varRef{
				Name: name,
				Span: diag.NewSpan(file, start, end),
			})
			continue
		}
		advance()
	}
	return refs
}

func collectExprStringRefs(expr ast.Expr) []varRef {
	if expr == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.StringExpr:
			base := n.Span.Start
			base.Offset++
			base.Column++
			out = append(out, collectShellLikeRefs(n.Value, base, n.Span.File)...)
		case ast.ListExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.UnaryExpr:
			walk(n.Expr)
		case ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.CompareExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.ConditionalExpr:
			walk(n.Then)
			walk(n.Cond)
			walk(n.Else)
		case ast.ModeExpr:
			walk(n.Expr)
		}
	}
	walk(expr)
	return out
}

func isEscapedDollar(runes []rune, idx int) bool {
	count := 0
	for i := idx - 1; i >= 0; i-- {
		if runes[i] != '\\' {
			break
		}
		count++
	}
	return count%2 == 1
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsDigit(r) || isIdentStart(r)
}

func parseVarPath(runes []rune, start int) (int, bool) {
	j := start
	if j >= len(runes) || !isIdentStart(runes[j]) {
		return 0, false
	}
	for {
		j++
		for j < len(runes) && isIdentPart(runes[j]) {
			j++
		}
		if j < len(runes) && runes[j] == '.' && j+1 < len(runes) && isIdentStart(runes[j+1]) {
			j++
			continue
		}
		break
	}
	return j, true
}

func sanitizeStepName(input string) string {
	if input == "" {
		return "x"
	}
	var b strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	out := b.String()
	if out == "" {
		return "x"
	}
	return out
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func validateWithItems(items []ast.WithItem, params map[string]*Paramset, diags *diag.Diagnostics) {
	type importOrigin struct {
		source string
		span   diag.Span
	}
	seen := make(map[string]importOrigin)
	reported := make(map[string]struct{})

	addImported := func(name string, source string, span diag.Span) {
		if prev, ok := seen[name]; ok {
			if prev.source == source {
				return
			}
			left := prev.source
			right := source
			if left > right {
				left, right = right, left
			}
			key := name + "|" + left + "|" + right
			if _, exists := reported[key]; exists {
				return
			}
			reported[key] = struct{}{}
			diags.AddError(
				"E214",
				fmt.Sprintf("conflicting variable '%s' imported from parametersets '%s' and '%s'", name, prev.source, source),
				span,
				"import each variable name from only one source parameterset",
				diag.RelatedSpan{Message: "first conflicting import", Span: prev.span},
			)
			return
		}
		seen[name] = importOrigin{source: source, span: span}
	}

	paramVarNames := func(ps *Paramset) []string {
		if len(ps.Order) > 0 {
			out := make([]string, len(ps.Order))
			copy(out, ps.Order)
			return out
		}
		names := make([]string, 0, len(ps.Vars))
		for name := range ps.Vars {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}

	for _, item := range items {
		if item.From == "" {
			ps, ok := params[item.Name]
			if !ok {
				diags.AddError(
					"E020",
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"import an existing parameterset",
				)
			} else {
				for _, varName := range paramVarNames(ps) {
					addImported(varName, ps.Name, item.Span)
				}
			}
			continue
		}

		src, ok := params[item.From]
		if !ok {
			diags.AddError(
				"E020",
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"import from an existing parameterset",
			)
			continue
		}

		if _, ok := src.Vars[item.Name]; ok {
			addImported(item.Name, src.Name, item.Span)
			continue
		}
		if fallback, ok := params[item.Name]; ok {
			for _, varName := range paramVarNames(fallback) {
				addImported(varName, fallback.Name, item.Span)
			}
			continue
		}
		diags.AddError(
			"E021",
			fmt.Sprintf("unknown variable '%s' in parameterset '%s'", item.Name, item.From),
			item.Span,
			"import a variable that exists in the source parameterset",
		)
	}
}
