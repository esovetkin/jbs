package sema

import (
	"maps"
	"slices"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func compileUserGlobals(prog ast.Program, builtins map[string]eval.Value, diags *diag.Diagnostics) (map[string]*GlobalVar, []string) {
	exec := execGlobalPlan(buildGlobalPlan(prog, builtins, baseDirForProgramFile(prog.File), diags), builtins, builtins, diags)
	return globalVarsFromExec(exec)
}

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
	isBound := func(name string) bool {
		_, ok := bound[name]
		return ok
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if e.Name != "" && !isBound(e.Name) {
			out[e.Name] = struct{}{}
		}
	case ast.QualifiedIdentExpr:
		if e.Namespace != "" && !isBound(e.Namespace) {
			out[e.Namespace] = struct{}{}
		}
	case ast.MemberExpr:
		collectGlobalExprDepsBound(e.Base, out, bound)
	case ast.ListExpr:
		for _, it := range e.Items {
			collectGlobalExprDepsBound(it, out, bound)
		}
	case ast.TupleExpr:
		for _, it := range e.Items {
			collectGlobalExprDepsBound(it, out, bound)
		}
	case ast.CallExpr:
		collectGlobalExprDepsBound(e.Callee, out, bound)
		for _, arg := range e.Args {
			collectGlobalExprDepsBound(arg.Expr, out, bound)
		}
	case ast.FunctionExpr:
		nextBound := make(map[string]struct{}, len(bound)+len(e.Params))
		for name := range bound {
			nextBound[name] = struct{}{}
		}
		for _, param := range e.Params {
			collectGlobalExprDepsBound(param.Default, out, nextBound)
			if param.Name != "" {
				nextBound[param.Name] = struct{}{}
			}
		}
		collectFuncBodyLocalNames(e.Body, nextBound)
		collectFuncBodyGlobalExprDeps(e.Body, out, nextBound)
	case ast.AliasExpr:
		collectGlobalExprDepsBound(e.Expr, out, bound)
	case ast.IndexExpr:
		collectGlobalExprDepsBound(e.Base, out, bound)
	case ast.UnaryExpr:
		collectGlobalExprDepsBound(e.Expr, out, bound)
	case ast.BinaryExpr:
		collectGlobalExprDepsBound(e.Left, out, bound)
		collectGlobalExprDepsBound(e.Right, out, bound)
	case ast.CompareExpr:
		collectGlobalExprDepsBound(e.Left, out, bound)
		collectGlobalExprDepsBound(e.Right, out, bound)
	case ast.ConditionalExpr:
		collectGlobalExprDepsBound(e.Then, out, bound)
		collectGlobalExprDepsBound(e.Cond, out, bound)
		collectGlobalExprDepsBound(e.Else, out, bound)
	}
}

func collectFuncBodyLocalNames(body []ast.FuncBodyStmt, out map[string]struct{}) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			if node.Name != "" {
				out[node.Name] = struct{}{}
			}
		case ast.FuncIfStmt:
			collectFuncBodyLocalNames(node.Then, out)
			collectFuncBodyLocalNames(node.Else, out)
		case ast.FuncForStmt:
			if node.Target != "" {
				out[node.Target] = struct{}{}
			}
			collectFuncBodyLocalNames(node.Body, out)
		case ast.FuncWhileStmt:
			collectFuncBodyLocalNames(node.Body, out)
		}
	}
}

func collectFuncBodyGlobalExprDeps(body []ast.FuncBodyStmt, out map[string]struct{}, bound map[string]struct{}) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			collectGlobalExprDepsBound(node.Expr, out, bound)
		case ast.ReturnStmt:
			collectGlobalExprDepsBound(node.Expr, out, bound)
		case ast.ExprStmt:
			collectGlobalExprDepsBound(node.Expr, out, bound)
		case ast.FuncIfStmt:
			collectGlobalExprDepsBound(node.Cond, out, bound)
			collectFuncBodyGlobalExprDeps(node.Then, out, bound)
			collectFuncBodyGlobalExprDeps(node.Else, out, bound)
		case ast.FuncForStmt:
			collectGlobalExprDepsBound(node.Iterable, out, bound)
			nextBound := make(map[string]struct{}, len(bound)+1)
			for name := range bound {
				nextBound[name] = struct{}{}
			}
			if node.Target != "" {
				nextBound[node.Target] = struct{}{}
			}
			collectFuncBodyGlobalExprDeps(node.Body, out, nextBound)
		case ast.FuncWhileStmt:
			collectGlobalExprDepsBound(node.Cond, out, bound)
			collectFuncBodyGlobalExprDeps(node.Body, out, bound)
		}
	}
}

func globalVarSeries(name string, value eval.Value) ([]string, map[string][]eval.Value) {
	if eval.IsComb(value) {
		order := append([]string(nil), value.C.Order...)
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
	order := append([]string(nil), gv.Order...)
	if len(order) == 0 {
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
	if eval.IsComb(gv.Value) {
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
		state.Values[name] = gv.Value
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
			values[name] = cell
		}
		out = append(out, eval.Row{Values: values})
	}
	return out
}
