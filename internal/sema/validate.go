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
		Program:     prog,
		Globals:     resolvedGlobals,
		Paramsets:   make([]*Paramset, 0),
		ParamByName: make(map[string]*Paramset),
		DoBlocks:    make([]ast.DoBlock, 0),
		Submits:     make([]ast.SubmitBlock, 0),
	}

	paramSpans := make(map[string]diag.Span)
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
		}
	}

	validateSteps(res, diags)
	validateUseClauses(res, diags)
	return res
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
				fmt.Sprintf("global variable '%s' must be scalar; tuples/lists/dicts are not allowed", assign.Name),
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
	for _, item := range items {
		if item.From == "" {
			if _, ok := params[item.Name]; !ok {
				diags.AddError(
					"E020",
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"import an existing parameterset",
				)
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
			continue
		}
		if _, ok := params[item.Name]; ok {
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
