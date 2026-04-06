package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
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

func IsSubmitKey(name string) bool {
	_, ok := allowedSubmitKeys[name]
	return ok
}

func SubmitKeys() []string {
	return slices.Sorted(maps.Keys(allowedSubmitKeys))
}

func compileLetBlock(block ast.LetBlock, globals map[string]eval.Value, lets map[string]*LetNamespace, diags *diag.Diagnostics) *LetNamespace {
	env := make(map[string]eval.Value, len(globals)+16)
	for k, v := range globals {
		env[k] = v
	}
	for _, ns := range lets {
		for name, value := range ns.Vars {
			env[name] = value
		}
	}

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
		if v.Kind == eval.KindList {
			diags.AddError(
				"E403",
				fmt.Sprintf("let variable '%s' must be scalar", asn.Name),
				asn.Span,
				"use string/int/float/bool or shell()/python() scalar values",
			)
			continue
		}
		out.Vars[asn.Name] = v
		out.Origins[asn.Name] = asn.Span
		env[asn.Name] = v
	}

	return out
}

func buildImportSources(res *Result) {
	res.ImportSourceByName = make(map[string]*ImportSource)
	for _, ps := range res.Paramsets {
		if ps == nil {
			continue
		}
		res.ImportSourceByName[ps.Name] = importSourceFromParam(ps)
	}
	for _, ls := range res.LetNamespaces {
		if ls == nil {
			continue
		}
		res.ImportSourceByName[ls.Name] = importSourceFromLet(ls)
	}
}

func importSourceFromParam(ps *Paramset) *ImportSource {
	return &ImportSource{
		Name:    ps.Name,
		Kind:    SourceKindParam,
		Vars:    cloneSeriesMap(ps.Vars),
		Origins: cloneSpanMap(ps.Origins),
		Modes:   cloneModeMap(ps.Modes),
		Order:   append([]string(nil), exposedVarNames(ps)...),
		Span:    ps.Block.Span,
	}
}

func importSourceFromLet(ns *LetNamespace) *ImportSource {
	vars := make(map[string][]eval.Value, len(ns.Vars))
	order := slices.Sorted(maps.Keys(ns.Vars))
	for _, name := range order {
		vars[name] = valueAsSeries(ns.Vars[name])
	}
	return &ImportSource{
		Name:    ns.Name,
		Kind:    SourceKindLet,
		Vars:    vars,
		Origins: cloneSpanMap(ns.Origins),
		Modes:   cloneModeMap(ns.Modes),
		Order:   order,
		Span:    ns.Span,
	}
}

func valueAsSeries(v eval.Value) []eval.Value {
	if v.Kind == eval.KindList {
		return slices.Clone(v.L)
	}
	return []eval.Value{v}
}

func cloneSeriesMap(src map[string][]eval.Value) map[string][]eval.Value {
	out := make(map[string][]eval.Value, len(src))
	for name, vals := range src {
		cp := make([]eval.Value, len(vals))
		copy(cp, vals)
		out[name] = cp
	}
	return out
}

func cloneSpanMap(src map[string]diag.Span) map[string]diag.Span {
	return maps.Clone(src)
}

func cloneModeMap(src map[string]string) map[string]string {
	return maps.Clone(src)
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
		before := len(diags.Items)
		value := eval.EvalExpr(assign.Expr, env, diags)
		if value.Kind != eval.KindString {
			if hasErrorCodeSince(diags, before, "E100") {
				continue
			}
			diags.AddError(
				"E412",
				fmt.Sprintf("analyse extraction expression for '%s' must evaluate to string", assign.Name),
				assign.Span,
				"use a string expression such as an imported let variable or a quoted regex pattern",
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
			"E415",
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
				"E420",
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
				"E422",
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
				"E214",
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
					"E022",
					fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
					item.Span,
					"disambiguate by renaming the param or let namespace",
				)
				continue
			}
			if src == nil {
				diags.AddError(
					"E020",
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
				"E022",
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.From),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if src == nil {
			diags.AddError(
				"E020",
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"import from an existing let namespace",
			)
			continue
		}
		if src.Kind != SourceKindLet {
			diags.AddError(
				"E420",
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
				"E022",
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if fallback != nil {
			if fallback.Kind != SourceKindLet {
				diags.AddError(
					"E420",
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
			"E021",
			fmt.Sprintf("unknown variable '%s' in source '%s'", item.Name, item.From),
			item.Span,
			"import a variable that exists in the selected let namespace",
		)
	}

	return out
}

func hasErrorCodeSince(diags *diag.Diagnostics, start int, code string) bool {
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
		if item.Code == code {
			return true
		}
	}
	return false
}

func stepVisibleVariables(items []ast.WithItem, sources map[string]*ImportSource) map[string]diag.Span {
	out := make(map[string]diag.Span)
	imports := resolveImportedVars(items, sources)
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		origin := origins[0]
		src := sources[origin.Paramset]
		if src == nil {
			out[name] = origin.Span
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if s, ok := src.Origins[sourceVar]; ok {
			out[name] = s
		} else {
			out[name] = origin.Span
		}
	}
	return out
}

func stepVisibleVariablesFromPlan(plan *StepImportPlan, sources map[string]*ImportSource) map[string]diag.Span {
	out := make(map[string]diag.Span, len(plan.Effective))
	for name, origin := range plan.Effective {
		if src := sources[origin.Paramset]; src != nil {
			sourceVar := origin.SourceVar
			if sourceVar == "" {
				sourceVar = name
			}
			if span, ok := src.Origins[sourceVar]; ok {
				out[name] = span
				continue
			}
		}
		out[name] = origin.Span
	}
	return out
}

func addStepValuesToEnvFromPlan(env map[string]eval.Value, plan *StepImportPlan, sources map[string]*ImportSource) {
	if plan == nil {
		return
	}
	for name, origin := range plan.Effective {
		src := sources[origin.Paramset]
		if src == nil {
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if vals, ok := src.Vars[sourceVar]; ok {
			env[name] = seriesAsValue(vals)
		}
	}
}

func addStepValuesToEnvFromWithItems(env map[string]eval.Value, items []ast.WithItem, sources map[string]*ImportSource) {
	imports := resolveImportedVars(items, sources)
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		origin := origins[0]
		src := sources[origin.Paramset]
		if src == nil {
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if vals, ok := src.Vars[sourceVar]; ok {
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

func compileSubmitBlock(block ast.SubmitBlock, sources map[string]*ImportSource, globals map[string]eval.Value, effective map[string]VarOrigin, diags *diag.Diagnostics) *SubmitSpec {
	env := make(map[string]eval.Value, len(globals)+16)
	for k, v := range globals {
		env[k] = v
	}

	for name, origin := range effective {
		src := sources[origin.Paramset]
		if src == nil {
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if vals, ok := src.Vars[sourceVar]; ok {
			env[name] = seriesAsValue(vals)
		}
	}

	spec := &SubmitSpec{
		Name:   block.Name,
		Values: make([]SubmitValue, 0, len(block.Fields)+len(block.UseNames)*4),
		Span:   block.Span,
	}
	resolved := make(map[string]SubmitValue, len(block.Fields)+len(block.UseNames)*4)
	order := make([]string, 0, len(block.Fields)+len(block.UseNames)*4)
	setValue := func(v SubmitValue) {
		if _, exists := resolved[v.Name]; !exists {
			order = append(order, v.Name)
		}
		resolved[v.Name] = v
	}
	type submitUseOrigin struct {
		useName string
		span    diag.Span
	}
	seenFromUse := make(map[string]submitUseOrigin, len(block.UseNames)*4)

	for _, useName := range block.UseNames {
		src := sources[useName]
		if src == nil {
			diags.AddError(
				"E078",
				fmt.Sprintf("unknown submit use namespace '%s'", useName),
				block.Span,
				"use an existing let namespace in submit header use clause",
			)
			continue
		}
		if src.Kind != SourceKindLet {
			diags.AddError(
				"E071",
				fmt.Sprintf("submit use source '%s' must be a let namespace", useName),
				block.Span,
				"use a let namespace in submit header use clause",
			)
			continue
		}
		for _, varName := range planutil.SourceVarNames(src.Order, src.Vars) {
			if _, ok := allowedSubmitKeys[varName]; !ok {
				origin := src.Origins[varName]
				if origin.IsZero() {
					origin = src.Span
				}
				diags.AddWarning(
					"W070",
					fmt.Sprintf("submit default '%s' from let '%s' is ignored (not a submit key)", varName, useName),
					origin,
					"keep only valid submit keys in submit defaults",
				)
				continue
			}
			if isRawSubmitKey(varName) {
				origin := src.Origins[varName]
				if origin.IsZero() {
					origin = src.Span
				}
				diags.AddWarning(
					"W071",
					fmt.Sprintf("submit default '%s' from let '%s' is ignored (raw-block key)", varName, useName),
					origin,
					"set raw-block submit keys directly in submit body",
				)
				continue
			}
			vals := src.Vars[varName]
			value := eval.Null()
			if len(vals) > 0 {
				value = vals[0]
			}
			span := src.Origins[varName]
			if span.IsZero() {
				span = src.Span
			}
			if prev, exists := seenFromUse[varName]; exists && prev.useName != useName {
				diags.AddWarning(
					"W072",
					fmt.Sprintf("submit default '%s' is defined in multiple use namespaces ('%s', '%s'); last wins ('%s')", varName, prev.useName, useName, useName),
					span,
					"merge defaults explicitly or keep one namespace per submit key",
					diag.RelatedSpan{Message: "first definition", Span: prev.span},
				)
			}
			seenFromUse[varName] = submitUseOrigin{
				useName: useName,
				span:    span,
			}
			setValue(SubmitValue{
				Name:  varName,
				Mode:  src.Modes[varName],
				Value: value,
				Span:  span,
			})
		}
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
			setValue(SubmitValue{
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
		setValue(SubmitValue{
			Name:  field.Name,
			Mode:  mode,
			Value: value,
			Span:  field.Span,
		})
	}
	if _, hasTasks := resolved["tasks"]; !hasTasks {
		if nodes, hasNodes := resolved["nodes"]; hasNodes {
			setValue(SubmitValue{
				Name:  "tasks",
				Mode:  nodes.Mode,
				Value: nodes.Value,
				Span:  nodes.Span,
			})
		} else {
			setValue(SubmitValue{
				Name:  "tasks",
				Value: eval.String("$nodes"),
				Span:  block.Span,
			})
		}
	}
	accountEmptyOrMissing, accountSpan := submitKeyMissingOrEmpty(resolved, "account", block.Span)
	if accountEmptyOrMissing {
		diags.AddWarning(
			"W073",
			"submit key 'account' is missing or empty",
			accountSpan,
			"set a non-empty account",
		)
	}
	queueEmptyOrMissing, queueSpan := submitKeyMissingOrEmpty(resolved, "queue", block.Span)
	if queueEmptyOrMissing {
		diags.AddWarning(
			"W073",
			"submit key 'queue' is missing or empty",
			queueSpan,
			"set a non-empty queue",
		)
	}
	executableEmptyOrMissing, executableSpan := submitKeyMissingOrEmpty(resolved, "executable", block.Span)
	argsExecEmptyOrMissing, argsExecSpan := submitKeyMissingOrEmpty(resolved, "args_exec", block.Span)
	if executableEmptyOrMissing && argsExecEmptyOrMissing {
		starterEmptyOrMissing, _ := submitKeyMissingOrEmpty(resolved, "starter", block.Span)
		if !starterEmptyOrMissing {
			primary := argsExecSpan
			if primary.IsZero() {
				primary = executableSpan
			}
			related := []diag.RelatedSpan{}
			if !executableSpan.IsZero() && executableSpan != primary {
				related = append(related, diag.RelatedSpan{
					Message: "executable is missing or empty",
					Span:    executableSpan,
				})
			}
			if !argsExecSpan.IsZero() && argsExecSpan != primary {
				related = append(related, diag.RelatedSpan{
					Message: "args_exec is missing or empty",
					Span:    argsExecSpan,
				})
			}
			diags.AddWarning(
				"W074",
				"submit keys 'executable' and 'args_exec' are both missing or empty",
				primary,
				"set at least one of executable or args_exec to a non-empty value",
				related...,
			)
		}
	}
	for _, name := range order {
		spec.Values = append(spec.Values, resolved[name])
	}
	return spec
}

func submitValueHasEmptyString(v SubmitValue) bool {
	if v.IsRaw {
		return false
	}
	return evalValueHasEmptyString(v.Value)
}

func submitKeyMissingOrEmpty(resolved map[string]SubmitValue, key string, fallback diag.Span) (bool, diag.Span) {
	v, ok := resolved[key]
	if !ok {
		return true, fallback
	}
	return submitValueHasEmptyString(v), v.Span
}

func evalValueHasEmptyString(v eval.Value) bool {
	switch v.Kind {
	case eval.KindString:
		return v.S == ""
	case eval.KindList:
		if len(v.L) == 0 {
			return true
		}
		for _, item := range v.L {
			if item.Kind != eval.KindString || item.S != "" {
				return false
			}
		}
		return true
	}
	return false
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

	resolveSource := func(name string) (*Paramset, *LetNamespace, bool) {
		ps := known[name]
		ls := lets[name]
		if ps != nil && ls != nil {
			return nil, nil, true
		}
		return ps, ls, false
	}
	letVarNames := func(ns *LetNamespace) []string {
		return slices.Sorted(maps.Keys(ns.Vars))
	}
	type importedOwner struct {
		Source string
	}
	importedOwners := make(map[string]importedOwner)
	canImport := func(visible, source string) bool {
		if prev, exists := importedOwners[visible]; exists {
			return prev.Source == source
		}
		importedOwners[visible] = importedOwner{Source: source}
		return true
	}
	importParamVar := func(visible, sourceVar string, src *Paramset) {
		if src == nil {
			return
		}
		vals, ok := src.Vars[sourceVar]
		if !ok {
			return
		}
		if !canImport(visible, src.Name) {
			return
		}
		env[visible] = seriesAsValue(vals)
		if origin, ok := src.Origins[sourceVar]; ok {
			origins[visible] = origin
		}
		if mode, ok := src.Modes[sourceVar]; ok {
			modes[visible] = mode
		}
	}
	importLetVar := func(visible, sourceVar string, src *LetNamespace) {
		if src == nil {
			return
		}
		v, ok := src.Vars[sourceVar]
		if !ok {
			return
		}
		if !canImport(visible, src.Name) {
			return
		}
		env[visible] = v
		if origin, ok := src.Origins[sourceVar]; ok {
			origins[visible] = origin
		}
		if mode, ok := src.Modes[sourceVar]; ok {
			modes[visible] = mode
		}
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
					importLetVar(name, name, srcLet)
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
				importParamVar(name, name, srcParam)
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
			if _, ok := srcParam.Vars[item.Name]; ok {
				importParamVar(item.Name, item.Name, srcParam)
				continue
			}
		}
		if srcLet != nil {
			if _, ok := srcLet.Vars[item.Name]; ok {
				importLetVar(item.Name, item.Name, srcLet)
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
				importParamVar(name, name, fallbackParam)
			}
			continue
		} else if fallbackLet != nil {
			for _, name := range letVarNames(fallbackLet) {
				importLetVar(name, name, fallbackLet)
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
		validateStepHeaderOptions("do", b.Name, b.MaxAsync, b.Iterations, b.Span, diags)
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
		validateStepHeaderOptions("submit", b.Name, b.MaxAsync, b.Iterations, b.Span, diags)
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

	for _, name := range slices.Sorted(maps.Keys(edges)) {
		if state[name] == 0 {
			visit(name)
		}
	}
}

func validateStepHeaderOptions(kind, stepName string, maxAsync *int, iterations *int, at diag.Span, diags *diag.Diagnostics) {
	if maxAsync != nil && *maxAsync < 0 {
		diags.AddError(
			"E216",
			fmt.Sprintf("%s step '%s' has invalid max_async=%d (expected >= 0)", kind, stepName, *maxAsync),
			at,
			"set max_async to an integer value >= 0",
		)
	}
	if iterations != nil && *iterations < 1 {
		diags.AddError(
			"E217",
			fmt.Sprintf("%s step '%s' has invalid iterations=%d (expected >= 1)", kind, stepName, *iterations),
			at,
			"set iterations to an integer value >= 1",
		)
	}
}

func validateUseClauses(res *Result, diags *diag.Diagnostics) {
	for _, ps := range res.Paramsets {
		validateWithItems(ps.Block.WithItems, res.ParamByName, res.LetByName, res.ImportSourceByName, diags)
	}
	for _, block := range res.DoBlocks {
		validateWithItems(block.WithItems, res.ParamByName, res.LetByName, res.ImportSourceByName, diags)
	}
	for _, block := range res.Submits {
		validateWithItems(block.WithItems, res.ParamByName, res.LetByName, res.ImportSourceByName, diags)
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
	Kind   SourceKind
	Vars   []expandedVar
	Full   bool
	Span   diag.Span
}

type expandedVar struct {
	Visible   string
	SourceVar string
}

type stepConflictReporter func(name string, left VarOrigin, right VarOrigin, at diag.Span, relation string)

func buildStepImportPlans(res *Result, diags *diag.Diagnostics) {
	defs, order := collectStepDefinitions(res)
	plans := make(map[string]*StepImportPlan, len(defs))
	for _, stepName := range planutil.TopoStepOrder(stepDefinitionDeps(defs), order) {
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

		explicitDelta := make([]PlannedImport, 0)
		selected := make(map[string]VarOrigin)
		for _, item := range def.WithItems {
			expanded, ok := expandWithItem(item, res.ParamByName, res.LetByName, res.ImportSourceByName)
			if !ok {
				continue
			}
			kept := make([]expandedVar, 0, len(expanded.Vars))
			sourceObj := res.ImportSourceByName[expanded.Source]
			for _, v := range expanded.Vars {
				name := v.Visible
				originSpan := item.Span
				if sourceObj != nil {
					sourceVar := v.SourceVar
					if sourceVar == "" {
						sourceVar = name
					}
					if origin, ok := sourceObj.Origins[sourceVar]; ok && !origin.IsZero() {
						originSpan = origin
					}
				}
				current := VarOrigin{
					Name:      name,
					SourceVar: v.SourceVar,
					Paramset:  expanded.Source,
					Kind:      expanded.Kind,
					Span:      originSpan,
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
				kept = append(kept, v)
			}
			if len(kept) == 0 {
				continue
			}
			if expanded.Full && len(kept) == len(expanded.Vars) {
				explicitDelta = append(explicitDelta, PlannedImport{
					Source: expanded.Source,
					Kind:   expanded.Kind,
					Full:   true,
					Span:   item.Span,
				})
				continue
			}
			for _, keptVar := range kept {
				explicitDelta = append(explicitDelta, PlannedImport{
					Source:    expanded.Source,
					Kind:      expanded.Kind,
					Visible:   keptVar.Visible,
					SourceVar: keptVar.SourceVar,
					Span:      item.Span,
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

func stepDefinitionDeps(defs map[string]stepDefinition) map[string][]string {
	out := make(map[string][]string, len(defs))
	for name, def := range defs {
		out[name] = append([]string(nil), def.After...)
	}
	return out
}

func expandWithItem(
	item ast.WithItem,
	params map[string]*Paramset,
	lets map[string]*LetNamespace,
	sources map[string]*ImportSource,
) (expandedWithImport, bool) {
	resolveSource := func(name string) (*ImportSource, bool) {
		_, hasParam := params[name]
		_, hasLet := lets[name]
		if hasParam && hasLet {
			return nil, true
		}
		src := sources[name]
		if src == nil {
			return nil, false
		}
		return src, false
	}
	if item.From == "" {
		src, ambiguous := resolveSource(item.Name)
		if ambiguous || src == nil {
			return expandedWithImport{}, false
		}
		vars := make([]expandedVar, 0, len(src.Order))
		for _, name := range src.Order {
			vars = append(vars, expandedVar{Visible: name, SourceVar: name})
		}
		return expandedWithImport{
			Source: src.Name,
			Kind:   src.Kind,
			Vars:   vars,
			Full:   true,
			Span:   item.Span,
		}, true
	}

	src, ambiguous := resolveSource(item.From)
	if ambiguous || src == nil {
		return expandedWithImport{}, false
	}
	if _, ok := src.Vars[item.Name]; ok {
		return expandedWithImport{
			Source: src.Name,
			Kind:   src.Kind,
			Vars:   []expandedVar{{Visible: item.Name, SourceVar: item.Name}},
			Full:   false,
			Span:   item.Span,
		}, true
	}
	fallback, fallbackAmbiguous := resolveSource(item.Name)
	if fallbackAmbiguous || fallback == nil {
		return expandedWithImport{}, false
	}
	vars := make([]expandedVar, 0, len(fallback.Order))
	for _, name := range fallback.Order {
		vars = append(vars, expandedVar{Visible: name, SourceVar: name})
	}
	return expandedWithImport{
		Source: fallback.Name,
		Kind:   fallback.Kind,
		Vars:   vars,
		Full:   true,
		Span:   item.Span,
	}, true
}

type importedVar struct {
	Name      string
	SourceVar string
	Paramset  string
	Kind      SourceKind
	Span      diag.Span
}

type varRef struct {
	Name string
	Span diag.Span
}

func validateStepVarReferences(res *Result, diags *diag.Diagnostics) {
	type sourceCandidate struct {
		Source    string
		SourceVar string
	}
	exposedBySource := make(map[string]map[string]diag.Span)
	candidatesByVar := make(map[string][]sourceCandidate)
	used := make(map[string]map[string]bool)

	for _, sourceName := range slices.Sorted(maps.Keys(res.ImportSourceByName)) {
		src := res.ImportSourceByName[sourceName]
		if src == nil {
			continue
		}
		varNames := planutil.SourceVarNames(src.Order, src.Vars)
		if len(varNames) == 0 {
			continue
		}
		if _, ok := exposedBySource[sourceName]; !ok {
			exposedBySource[sourceName] = make(map[string]diag.Span)
		}
		for _, name := range varNames {
			origin := src.Origins[name]
			if origin.IsZero() {
				origin = src.Span
			}
			exposedBySource[sourceName][name] = origin
			candidatesByVar[name] = append(candidatesByVar[name], sourceCandidate{
				Source:    sourceName,
				SourceVar: name,
			})
		}
	}

	markUsedExact := func(sourceName, sourceVar string) {
		if _, ok := used[sourceName]; !ok {
			used[sourceName] = make(map[string]bool)
		}
		used[sourceName][sourceVar] = true
	}

	markUsedByImports := func(imports []importedVar) {
		for _, imp := range imports {
			sourceVar := imp.SourceVar
			if sourceVar == "" {
				sourceVar = imp.Name
			}
			markUsedExact(imp.Paramset, sourceVar)
		}
	}

	markUsedCandidates := func(candidates []sourceCandidate) {
		for _, cand := range candidates {
			markUsedExact(cand.Source, cand.SourceVar)
		}
	}

	warnMissing := func(stepName string, ref varRef, candidates []sourceCandidate) {
		if len(candidates) == 0 {
			return
		}
		originSpan := diag.Span{}
		source := candidates[0].Source
		sourceVar := candidates[0].SourceVar
		if byVar, ok := exposedBySource[source]; ok {
			originSpan = byVar[sourceVar]
		}
		related := []diag.RelatedSpan{}
		if !originSpan.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", source),
				Span:    originSpan,
			})
		}
		diags.AddWarning(
			"W311",
			fmt.Sprintf("variable '%s' is referenced in step '%s' but not imported via with-clause", ref.Name, stepName),
			ref.Span,
			"add `with <source>` or `with <variable> from <source>`",
			related...,
		)
	}

	processStep := func(stepName string, withItems []ast.WithItem, refs []varRef) {
		imports := resolveImportedVars(withItems, res.ImportSourceByName)
		if plan := res.StepImportByName[stepName]; plan != nil {
			imports = resolveImportedVarsFromPlan(plan)
		}
		warned := make(map[string]struct{})
		for _, ref := range refs {
			candidates := candidatesByVar[ref.Name]
			if len(candidates) == 0 {
				continue
			}
			origins := imports[ref.Name]
			if len(origins) > 0 {
				markUsedByImports(origins)
				continue
			}
			markUsedCandidates(candidates)
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
		for _, useName := range block.UseNames {
			src := res.ImportSourceByName[useName]
			if src == nil || src.Kind != SourceKindLet {
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				if _, ok := allowedSubmitKeys[name]; !ok {
					continue
				}
				if isRawSubmitKey(name) {
					continue
				}
				markUsedExact(useName, name)
			}
		}
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
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.AnalyseBlock)
		if !ok {
			continue
		}
		imports := resolveImportedVars(block.WithItems, res.ImportSourceByName)
		for _, origins := range imports {
			for _, origin := range origins {
				if origin.Kind != SourceKindLet {
					continue
				}
				sourceVar := origin.SourceVar
				if sourceVar == "" {
					sourceVar = origin.Name
				}
				markUsedExact(origin.Paramset, sourceVar)
			}
		}
	}

	for sourceName, byVar := range exposedBySource {
		src := res.ImportSourceByName[sourceName]
		for varName, origin := range byVar {
			if used[sourceName][varName] {
				continue
			}
			message := fmt.Sprintf("exposed variable '%s' from param '%s' is never used in any do/submit block", varName, sourceName)
			hint := fmt.Sprintf("remove it from the final expression or reference it with $%s/${%s} in a step", varName, varName)
			if src != nil && src.Kind == SourceKindLet {
				message = fmt.Sprintf("exposed variable '%s' from let '%s' is never used in any do/submit/analyse block", varName, sourceName)
				hint = fmt.Sprintf("remove it from the let block or reference it with %s via with-imports", varName)
			}
			diags.AddWarning(
				"W310",
				message,
				origin,
				hint,
			)
		}
	}
}

func exposedVarNames(ps *Paramset) []string {
	return planutil.SourceVarNames(ps.Order, ps.Vars)
}

func resolveImportedVars(items []ast.WithItem, sources map[string]*ImportSource) map[string][]importedVar {
	out := make(map[string][]importedVar)
	seen := make(map[string]struct{})
	add := func(name, sourceVar, source string, kind SourceKind, span diag.Span) {
		key := source + "::" + sourceVar + "::" + name
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out[name] = append(out[name], importedVar{
			Name:      name,
			SourceVar: sourceVar,
			Paramset:  source,
			Kind:      kind,
			Span:      span,
		})
	}

	for _, item := range items {
		if item.From == "" {
			src := sources[item.Name]
			if src == nil {
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				add(name, name, src.Name, src.Kind, item.Span)
			}
			continue
		}

		src := sources[item.From]
		if src == nil {
			continue
		}
		if _, ok := src.Vars[item.Name]; ok {
			add(item.Name, item.Name, src.Name, src.Kind, item.Span)
			continue
		}
		if fallback := sources[item.Name]; fallback != nil {
			for _, name := range planutil.SourceVarNames(fallback.Order, fallback.Vars) {
				add(name, name, fallback.Name, fallback.Kind, item.Span)
			}
		}
	}
	return out
}

func resolveImportedVarsFromPlan(plan *StepImportPlan) map[string][]importedVar {
	out := make(map[string][]importedVar, len(plan.Effective))
	for name, origin := range plan.Effective {
		out[name] = append(out[name], importedVar{
			Name:      name,
			SourceVar: origin.SourceVar,
			Paramset:  origin.Paramset,
			Kind:      origin.Kind,
			Span:      origin.Span,
		})
	}
	return out
}

type shellScanState uint8

const (
	shellScanCode shellScanState = iota
	shellScanSingleQuote
	shellScanDoubleQuote
	shellScanComment
)

// collectShellLikeRefs scans shell-like text to detect unqualified variable
// references for W310/W311 usage accounting. This scanner is intentionally
// lightweight and context-aware (comments/quotes), not a full shell parser.
func collectShellLikeRefs(text string, base diag.Position, file string) []varRef {
	runes := []rune(text)
	refs := make([]varRef, 0)
	line := base.Line
	col := base.Column
	off := base.Offset
	i := 0
	state := shellScanCode

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
	appendRef := func(name string, start diag.Position) {
		end := diag.NewPos(off, line, col)
		refs = append(refs, varRef{
			Name: name,
			Span: diag.NewSpan(file, start, end),
		})
	}
	parseExpansion := func(start diag.Position) {
		if i+1 < len(runes) && runes[i+1] == '{' {
			name, end, ok := parseBracedVarRef(runes, i+2)
			if ok {
				advanceN(end + 1)
				appendRef(name, start)
				return
			}
			advance()
			return
		}
		if end, ok := parseBareVarName(runes, i+1); ok {
			name := string(runes[i+1 : end])
			advanceN(end)
			appendRef(name, start)
			return
		}
		advance()
	}

	for i < len(runes) {
		switch state {
		case shellScanCode:
			curr := runes[i]
			if curr == '\'' {
				advance()
				state = shellScanSingleQuote
				continue
			}
			if curr == '"' {
				advance()
				state = shellScanDoubleQuote
				continue
			}
			if curr == '#' && isCommentStart(runes, i) {
				advance()
				state = shellScanComment
				continue
			}
			if curr == '$' && !isEscapedDollar(runes, i) {
				start := diag.NewPos(off, line, col)
				parseExpansion(start)
				continue
			}
			advance()
		case shellScanSingleQuote:
			if runes[i] == '\'' {
				advance()
				state = shellScanCode
				continue
			}
			advance()
		case shellScanDoubleQuote:
			curr := runes[i]
			if curr == '\\' {
				advance()
				if i < len(runes) {
					advance()
				}
				continue
			}
			if curr == '"' {
				advance()
				state = shellScanCode
				continue
			}
			if curr == '$' && !isEscapedDollar(runes, i) {
				start := diag.NewPos(off, line, col)
				parseExpansion(start)
				continue
			}
			advance()
		case shellScanComment:
			if runes[i] == '\n' {
				advance()
				state = shellScanCode
				continue
			}
			advance()
		default:
			advance()
			continue
		}
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

func parseBareVarName(runes []rune, start int) (int, bool) {
	j := start
	if j >= len(runes) || !isIdentStart(runes[j]) {
		return 0, false
	}
	j++
	for j < len(runes) && isIdentPart(runes[j]) {
		j++
	}
	return j, true
}

func parseBracedVarRef(runes []rune, start int) (string, int, bool) {
	j := start
	if j >= len(runes) {
		return "", 0, false
	}
	if runes[j] == '#' || runes[j] == '!' {
		j++
	}
	nameStart := j
	nameEnd, ok := parseBareVarName(runes, j)
	if !ok {
		return "", 0, false
	}
	name := string(runes[nameStart:nameEnd])
	j = nameEnd
	depth := 1
	for j < len(runes) {
		switch runes[j] {
		case '\\':
			j += 2
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return name, j, true
			}
		}
		j++
	}
	return "", 0, false
}

func isCommentStart(runes []rune, idx int) bool {
	if idx < 0 || idx >= len(runes) || runes[idx] != '#' {
		return false
	}
	if idx == 0 {
		return true
	}
	return isShellCommentBoundary(runes[idx-1])
}

func isShellCommentBoundary(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	switch r {
	case ';', '|', '&', '(', ')', '{', '}':
		return true
	default:
		return false
	}
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

func validateWithItems(
	items []ast.WithItem,
	params map[string]*Paramset,
	lets map[string]*LetNamespace,
	sources map[string]*ImportSource,
	diags *diag.Diagnostics,
) {
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
				fmt.Sprintf("conflicting variable '%s' imported from sources '%s' and '%s'", name, prev.source, source),
				span,
				"import each variable name from only one source",
				diag.RelatedSpan{Message: "first conflicting import", Span: prev.span},
			)
			return
		}
		seen[name] = importOrigin{source: source, span: span}
	}

	resolveSource := func(name string) (*ImportSource, bool) {
		_, hasParam := params[name]
		_, hasLet := lets[name]
		if hasParam && hasLet {
			return nil, true
		}
		return sources[name], false
	}

	for _, item := range items {
		if item.From == "" {
			src, ambiguous := resolveSource(item.Name)
			if ambiguous {
				diags.AddError(
					"E022",
					fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
					item.Span,
					"disambiguate by renaming the param or let namespace",
				)
				continue
			}
			if src == nil {
				diags.AddError(
					"E020",
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"import an existing parameterset or let namespace",
				)
			} else {
				for _, varName := range planutil.SourceVarNames(src.Order, src.Vars) {
					addImported(varName, src.Name, item.Span)
				}
			}
			continue
		}

		src, ambiguous := resolveSource(item.From)
		if ambiguous {
			diags.AddError(
				"E022",
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.From),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if src == nil {
			diags.AddError(
				"E020",
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"import from an existing parameterset or let namespace",
			)
			continue
		}

		if _, ok := src.Vars[item.Name]; ok {
			addImported(item.Name, src.Name, item.Span)
			continue
		}
		fallback, fallbackAmbiguous := resolveSource(item.Name)
		if fallbackAmbiguous {
			diags.AddError(
				"E022",
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if fallback != nil {
			for _, varName := range planutil.SourceVarNames(fallback.Order, fallback.Vars) {
				addImported(varName, fallback.Name, item.Span)
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
}
