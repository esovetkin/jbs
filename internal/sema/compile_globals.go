package sema

import (
	"maps"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func globalExprDependencies(expr ast.Expr, self string) []string {
	if expr == nil {
		return nil
	}
	refs := make(map[string]struct{})
	collectGlobalExprDeps(expr, refs)
	delete(refs, self)
	if len(refs) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(refs))
}

func collectGlobalExprDeps(expr ast.Expr, out map[string]struct{}) {
	collectGlobalExprDepsBound(expr, out, nil)
}

func collectGlobalExprDepsBound(expr ast.Expr, out map[string]struct{}, bound map[string]struct{}) {
	if expr == nil {
		return
	}
	var walkExprBound func(ast.Expr, map[string]struct{})
	var walkBodyBound func([]ast.FuncBodyStmt, map[string]struct{})

	callbacksForBound := func(bound map[string]struct{}) ast.WalkCallbacks {
		var callbacks ast.WalkCallbacks
		callbacks.Expr = func(node ast.Expr) ast.WalkAction {
			switch n := node.(type) {
			case ast.IdentExpr:
				if n.Name != "" && !nameSetContains(bound, n.Name) {
					out[n.Name] = struct{}{}
				}
			case ast.QualifiedIdentExpr:
				if n.Namespace != "" && !nameSetContains(bound, n.Namespace) {
					out[n.Namespace] = struct{}{}
				}
			case ast.CallExpr:
				if isDeleteCallExpr(n) && deleteCallHasOnlyBareTargets(n) {
					walkExprBound(n.Callee, bound)
					return ast.WalkSkipChildren
				}
			case ast.FunctionExpr:
				nextBound := cloneNameSet(bound)
				for _, param := range n.Params {
					walkExprBound(param.Default, nextBound)
					if param.Name != "" {
						nextBound[param.Name] = struct{}{}
					}
				}
				collectFuncBodyLocalNames(n.Body, nextBound)
				walkBodyBound(n.Body, nextBound)
				return ast.WalkSkipChildren
			}
			return ast.WalkContinue
		}
		callbacks.FuncBodyStmt = func(stmt ast.FuncBodyStmt) ast.WalkAction {
			node, ok := stmt.(ast.FuncForStmt)
			if !ok {
				return ast.WalkContinue
			}
			walkExprBound(node.Iterable, bound)
			nextBound := cloneNameSet(bound)
			if node.Target != "" {
				nextBound[node.Target] = struct{}{}
			}
			walkBodyBound(node.Body, nextBound)
			return ast.WalkSkipChildren
		}
		return callbacks
	}

	walkExprBound = func(expr ast.Expr, bound map[string]struct{}) {
		ast.WalkExpr(expr, callbacksForBound(bound))
	}
	walkBodyBound = func(body []ast.FuncBodyStmt, bound map[string]struct{}) {
		ast.WalkFuncBody(body, callbacksForBound(bound))
	}
	walkExprBound(expr, bound)
}

func collectFuncBodyLocalNames(body []ast.FuncBodyStmt, out map[string]struct{}) {
	ast.WalkFuncBody(body, ast.WalkCallbacks{
		Expr: func(ast.Expr) ast.WalkAction {
			return ast.WalkSkipChildren
		},
		FuncBodyStmt: func(stmt ast.FuncBodyStmt) ast.WalkAction {
			switch node := stmt.(type) {
			case ast.LocalAssignStmt:
				if node.Name != "" {
					out[node.Name] = struct{}{}
				}
			case ast.FuncForStmt:
				if node.Target != "" {
					out[node.Target] = struct{}{}
				}
			}
			return ast.WalkContinue
		},
	})
}

func nameSetContains(names map[string]struct{}, name string) bool {
	_, ok := names[name]
	return ok
}

func cloneNameSet(names map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for name := range names {
		out[name] = struct{}{}
	}
	return out
}

func globalVarSeries(name string, value eval.Value) ([]string, map[string][]eval.Value) {
	if eval.IsComb(value) {
		order := eval.CombNames(value)
		vars := make(map[string][]eval.Value, len(order))
		for _, col := range order {
			colVals, ok := eval.CombColumn(value, col)
			if !ok {
				continue
			}
			vars[col] = slices.Clone(colVals)
		}
		return order, vars
	}
	return []string{name}, map[string][]eval.Value{
		name: eval.ToSeries(value),
	}
}

func globalVarFromImportedBinding(name string, binding *GlobalBinding, span diag.Span) *GlobalVar {
	if binding == nil {
		return nil
	}
	order, vars := globalVarSeries(name, binding.Value)
	return &GlobalVar{
		Name:          name,
		Value:         binding.Value,
		Span:          span,
		Order:         order,
		Vars:          vars,
		DependsOn:     append([]string(nil), binding.DependsOn...),
		DependsOnKeys: append([]BindingVersionKey(nil), binding.DependsOnKeys...),
		VersionID:     binding.VersionID,
	}
}

func globalVarFromImportedGlobal(name string, source *GlobalVar, span diag.Span) *GlobalVar {
	if source == nil {
		return nil
	}
	order, vars := globalVarSeries(name, source.Value)
	return &GlobalVar{
		Name:          name,
		Value:         source.Value,
		Span:          span,
		Order:         order,
		Vars:          vars,
		DependsOn:     append([]string(nil), source.DependsOn...),
		DependsOnKeys: append([]BindingVersionKey(nil), source.DependsOnKeys...),
		VersionID:     source.VersionID,
	}
}

func bindingFromGlobalVar(name string, gv *GlobalVar) *GlobalBinding {
	if gv == nil || gv.Value.Kind == eval.KindFunction {
		return nil
	}
	isTable := eval.IsComb(gv.Value)
	order := append([]string(nil), gv.Order...)
	if len(order) == 0 && !isTable {
		order = []string{name}
	}

	vars := cloneSeriesMap(gv.Vars)
	baseVars := cloneSeriesMap(gv.Vars)
	origins := make(map[string]diag.Span, len(order))
	for _, col := range order {
		origins[col] = gv.Span
	}

	rows := make([]eval.Row, 0)
	shape := BindingScalar
	if isTable {
		shape = BindingTable
		rows = cloneCombRows(gv.Value.C.Rows, gv.Span)
	} else {
		series := vars[gv.Name]
		for _, value := range series {
			rows = append(rows, eval.Row{
				Values: map[string]eval.Cell{
					name: {
						Value:  value,
						Origin: gv.Span,
					},
				},
			})
		}
	}

	return &GlobalBinding{
		Name:            name,
		Value:           gv.Value,
		Shape:           shape,
		Rows:            rows,
		Vars:            vars,
		BaseVars:        baseVars,
		Origins:         origins,
		Order:           order,
		Span:            gv.Span,
		DependsOn:       append([]string(nil), gv.DependsOn...),
		DependsOnKeys:   append([]BindingVersionKey(nil), gv.DependsOnKeys...),
		SyntheticGlobal: true,
		VersionID:       gv.VersionID,
	}
}

func mergeGlobalVarsIntoState(state *GlobalState, byName map[string]*GlobalVar) {
	if state == nil {
		return
	}
	if state.Values == nil {
		state.Values = make(map[string]eval.Value)
	}
	if state.Spans == nil {
		state.Spans = make(map[string]diag.Span)
	}
	for name, gv := range byName {
		if gv == nil || name == "" {
			continue
		}
		state.Values[name] = eval.CloneValue(gv.Value)
		state.Spans[name] = gv.Span
	}
}

func cloneCombRows(rows []eval.Row, fallback diag.Span) []eval.Row {
	out := make([]eval.Row, 0, len(rows))
	for _, row := range rows {
		values := make(map[string]eval.Cell, len(row.Values))
		for name, cell := range row.Values {
			if cell.Origin.IsZero() {
				cell.Origin = fallback
			}
			cell.Value = eval.CloneValue(cell.Value)
			values[name] = cell
		}
		out = append(out, eval.Row{Values: values})
	}
	return out
}
