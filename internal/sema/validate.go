package sema

import (
	"fmt"
	"sort"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func Analyze(prog ast.Program, globals map[string]eval.Value, diags *diag.Diagnostics) *Result {
	resolvedGlobals := resolveTopLevelGlobals(prog, globals, diags)
	res := &Result{
		Program:        prog,
		Globals:        resolvedGlobals,
		Paramsets:      make([]*Paramset, 0),
		ParamByName:    make(map[string]*Paramset),
		DoBlocks:       make([]ast.DoBlock, 0),
		Submits:        make([]ast.SubmitBlock, 0),
		SubmitByName:   make(map[string]*SubmitSpec),
		Patterns:       make([]*PatternGroup, 0),
		PatternByGroup: make(map[string]*PatternGroup),
		PatternByKey:   make(map[string]*PatternTemplate),
		Analyse:        make([]*AnalyseSpec, 0),
	}

	paramSpans := make(map[string]diag.Span)
	patternSpans := make(map[string]diag.Span)
	analyseBlocks := make([]ast.AnalyseBlock, 0)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			continue
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
			compiled := compileParamBlock(n, res.ParamByName, resolvedGlobals.Values, diags)
			res.Paramsets = append(res.Paramsets, compiled)
			res.ParamByName[n.Name] = compiled
		case ast.DoBlock:
			res.DoBlocks = append(res.DoBlocks, n)
		case ast.SubmitBlock:
			res.Submits = append(res.Submits, n)
			res.SubmitByName[n.Name] = compileSubmitBlock(n, res.ParamByName, resolvedGlobals.Values, diags)
		case ast.PatternsBlock:
			if prev, exists := patternSpans[n.Name]; exists {
				diags.AddError(
					"E400",
					fmt.Sprintf("duplicate patterns block name '%s'", n.Name),
					n.Span,
					"use a unique patterns block name",
					diag.RelatedSpan{Message: "first definition", Span: prev},
				)
				continue
			}
			patternSpans[n.Name] = n.Span
			group := compilePatternsBlock(n, diags)
			res.Patterns = append(res.Patterns, group)
			res.PatternByGroup[group.Name] = group
			for _, pat := range group.Patterns {
				p := pat
				res.PatternByKey[patternLookupKey(p.Group, p.Name)] = &p
			}
		case ast.AnalyseBlock:
			analyseBlocks = append(analyseBlocks, n)
		}
	}

	validateSteps(res, diags)
	validateUseClauses(res, diags)
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

func compilePatternsBlock(block ast.PatternsBlock, diags *diag.Diagnostics) *PatternGroup {
	group := &PatternGroup{
		Name:     block.Name,
		Patterns: make([]PatternTemplate, 0, len(block.Patterns)),
		Span:     block.Span,
	}
	seen := make(map[string]diag.Span, len(block.Patterns))
	for _, pat := range block.Patterns {
		if prev, exists := seen[pat.Name]; exists {
			diags.AddError(
				"E401",
				fmt.Sprintf("duplicate pattern name '%s' in patterns block '%s'", pat.Name, block.Name),
				pat.Span,
				"use unique pattern names within a patterns block",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		seen[pat.Name] = pat.Span
		regex, inferredType, ok := normalizePatternRegex(pat.Regex)
		if !ok {
			diags.AddError(
				"E402",
				fmt.Sprintf("invalid placeholder in pattern '%s' of patterns block '%s'", pat.Name, block.Name),
				pat.Span,
				"supported placeholders are %d, %f, %w and %% for a literal percent",
			)
			continue
		}
		group.Patterns = append(group.Patterns, PatternTemplate{
			Group: block.Name,
			Name:  pat.Name,
			Regex: regex,
			Type:  inferredType,
			Span:  pat.Span,
		})
	}
	return group
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
	withItems := make([]ast.WithItem, 0)
	for _, doBlock := range res.DoBlocks {
		if doBlock.Name == block.StepName {
			stepKind = "do"
			withItems = doBlock.WithItems
			break
		}
	}
	if stepKind == "" {
		for _, submit := range res.Submits {
			if submit.Name == block.StepName {
				stepKind = "submit"
				withItems = submit.WithItems
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
	spec.StepVars = stepVisibleVariables(withItems, res.ParamByName)

	seenAssignments := make(map[string]diag.Span)
	assignmentVars := make(map[string]diag.Span)
	for _, assign := range block.Assignments {
		if prev, exists := seenAssignments[assign.Name]; exists {
			diags.AddError(
				"E414",
				fmt.Sprintf("duplicate analyse variable '%s'", assign.Name),
				assign.Span,
				"use unique alias names in analyse assignments",
				diag.RelatedSpan{Message: "first assignment", Span: prev},
			)
			continue
		}
		seenAssignments[assign.Name] = assign.Span
		if existing, ok := spec.StepVars[assign.Name]; ok {
			diags.AddError(
				"E413",
				fmt.Sprintf("analyse variable '%s' collides with step-visible variable", assign.Name),
				assign.Span,
				"use a distinct alias name in analyse",
				diag.RelatedSpan{Message: "step variable", Span: existing},
			)
			continue
		}
		group, ok := res.PatternByGroup[assign.PatternGroup]
		if !ok {
			diags.AddError(
				"E411",
				fmt.Sprintf("unknown pattern group '%s' in analyse assignment", assign.PatternGroup),
				assign.Span,
				"define the patterns block before using it",
			)
			continue
		}
		template := res.PatternByKey[patternLookupKey(assign.PatternGroup, assign.PatternName)]
		if template == nil {
			diags.AddError(
				"E412",
				fmt.Sprintf("unknown pattern '%s' in pattern group '%s'", assign.PatternName, assign.PatternGroup),
				assign.Span,
				"use an existing pattern name from the selected patterns block",
				diag.RelatedSpan{Message: "pattern group", Span: group.Span},
			)
			continue
		}
		spec.Assignments = append(spec.Assignments, AnalyseAssignmentSpec{
			Name:     assign.Name,
			Group:    assign.PatternGroup,
			Pattern:  assign.PatternName,
			File:     assign.File,
			Template: *template,
			Span:     assign.Span,
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
			"use a step-visible variable or an analyse assignment alias",
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

func compileSubmitBlock(block ast.SubmitBlock, known map[string]*Paramset, globals map[string]eval.Value, diags *diag.Diagnostics) *SubmitSpec {
	env := make(map[string]eval.Value, len(globals)+16)
	for k, v := range globals {
		env[k] = v
	}

	for _, item := range block.WithItems {
		if item.From == "" {
			src, ok := known[item.Name]
			if !ok {
				continue
			}
			for _, name := range src.Order {
				env[name] = seriesAsValue(src.Vars[name])
			}
			continue
		}

		src, ok := known[item.From]
		if !ok {
			continue
		}
		vals, ok := src.Vars[item.Name]
		if ok {
			env[item.Name] = seriesAsValue(vals)
			continue
		}

		if fallback, ok := known[item.Name]; ok {
			for _, name := range fallback.Order {
				env[name] = seriesAsValue(fallback.Vars[name])
			}
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

func compileParamBlock(block ast.ParamBlock, known map[string]*Paramset, globals map[string]eval.Value, diags *diag.Diagnostics) *Paramset {
	env := make(map[string]eval.Value, len(globals)+16)
	origins := make(map[string]diag.Span, len(globals)+16)
	modes := make(map[string]string, 16)
	for k, v := range globals {
		env[k] = v
	}

	for _, item := range block.WithItems {
		if item.From == "" {
			src, ok := known[item.Name]
			if !ok {
				diags.AddError(
					"E020",
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"define/import the parameterset before using it",
				)
				continue
			}
			for _, name := range src.Order {
				env[name] = seriesAsValue(src.Vars[name])
				if origin, ok := src.Origins[name]; ok {
					origins[name] = origin
				}
				if mode, ok := src.Modes[name]; ok {
					modes[name] = mode
				}
			}
			continue
		}

		src, ok := known[item.From]
		if !ok {
			diags.AddError(
				"E020",
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"define/import the parameterset before using it",
			)
			continue
		}
		vals, ok := src.Vars[item.Name]
		if ok {
			env[item.Name] = seriesAsValue(vals)
			if origin, ok := src.Origins[item.Name]; ok {
				origins[item.Name] = origin
			}
			if mode, ok := src.Modes[item.Name]; ok {
				modes[item.Name] = mode
			}
			continue
		}

		// Mixed form support:
		// with x from p1, p2
		// If "p2" is not a variable in p1 but is an existing parameterset,
		// interpret it as importing the whole parameterset p2.
		if fallback, ok := known[item.Name]; ok {
			for _, name := range fallback.Order {
				env[name] = seriesAsValue(fallback.Vars[name])
				if origin, exists := fallback.Origins[name]; exists {
					origins[name] = origin
				}
				if mode, exists := fallback.Modes[name]; exists {
					modes[name] = mode
				}
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
