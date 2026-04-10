package sema

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func compileParamBlock(block ast.ParamBlock, known map[string]*Paramset, globals map[string]eval.Value, lets map[string]*LetNamespace, diags *diag.Diagnostics) *Paramset {
	env := make(map[string]eval.Value, len(globals)+16)
	origins := make(map[string]diag.Span, len(globals)+16)
	modes := make(map[string]string, 16)
	localAssigns := make(map[string]localAssignMeta, len(block.Assignments))
	localAssignOrder := make([]string, 0, len(block.Assignments))
	localAssignSeen := make(map[string]bool, len(block.Assignments))
	for k, v := range globals {
		env[k] = v
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

	importSources := make(map[string]*ImportSource, len(known)+len(lets))
	for name, ps := range known {
		importSources[name] = importSourceFromParam(ps)
	}
	for name, ls := range lets {
		importSources[name] = importSourceFromLet(ls)
	}
	resolver := WithResolver{
		Params:  known,
		Lets:    lets,
		Sources: importSources,
	}
	expanded, issues := resolver.ExpandWithItems(block.WithItems, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	})
	emitWithIssues(diags, paramWithDiagPolicy(), issues)
	for _, item := range expanded {
		switch item.Kind {
		case SourceKindParam:
			src := known[item.Source]
			for _, v := range item.Vars {
				importParamVar(v.Visible, v.SourceVar, src)
			}
		case SourceKindLet:
			src := lets[item.Source]
			for _, v := range item.Vars {
				importLetVar(v.Visible, v.SourceVar, src)
			}
		}
	}

	for _, asn := range block.Assignments {
		effectiveExpr := assignmentExpr(asn.Name, asn.Op, asn.Expr, asn.Span)
		warnModeExprInCollections(effectiveExpr, diags)
		localAssigns[asn.Name] = localAssignMeta{
			Expr: asn.Expr,
			Span: asn.Span,
		}
		if !localAssignSeen[asn.Name] {
			localAssignSeen[asn.Name] = true
			localAssignOrder = append(localAssignOrder, asn.Name)
		}
		mode, inner, isModeExpr := unwrapModeExpr(effectiveExpr)
		expr := effectiveExpr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExprWithOptions(expr, env, diags, eval.ExprOptions{
			ParamAssignmentTupleArithmetic: true,
		})
		if isModeExpr {
			value = coerceModeValue(mode, value, asn.Span, diags)
			modes[asn.Name] = mode
		} else {
			delete(modes, asn.Name)
		}
		if hasNestedList(value) {
			diags.AddError(
				diag.CodeE305,
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

	order := combIdentOrder(block.Final)
	warnUnusedParamLocals(localAssigns, localAssignOrder, order, diags)

	rows := eval.EvalCombination(block.Final, series, origins, diags)
	if rows == nil {
		rows = make([]eval.Row, 0)
	}

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
					diag.CodeW301,
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
		case ast.ConvertExpr:
			walk(n.Expr, inCollection)
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
	case eval.KindList, eval.KindTuple:
		items := make([]eval.Value, len(value.L))
		for i, it := range value.L {
			if it.Kind != eval.KindString {
				diags.AddError(
					diag.CodeE215,
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
			diag.CodeE215,
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
