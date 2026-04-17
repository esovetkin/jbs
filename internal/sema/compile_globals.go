package sema

import (
	"fmt"
	"maps"
	"slices"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func compileUserGlobals(prog ast.Program, builtins map[string]eval.Value, diags *diag.Diagnostics) (map[string]*GlobalVar, []string) {
	knownBuiltins := make(map[string]struct{}, len(builtins))
	env := make(map[string]eval.Value, len(builtins)+16)
	for name, value := range builtins {
		knownBuiltins[name] = struct{}{}
		env[name] = value
	}

	out := make(map[string]*GlobalVar)
	order := make([]string, 0, 16)
	seenOrder := make(map[string]bool, 16)

	for _, stmt := range prog.Stmts {
		asn, ok := stmt.(ast.GlobalAssign)
		if !ok {
			continue
		}
		if _, builtin := knownBuiltins[asn.Name]; builtin {
			continue
		}

		effectiveExpr := assignmentExpr(asn.Name, asn.Op, asn.Expr, asn.Span)
		warnModeExprInCollections(effectiveExpr, diags)

		mode, inner, isModeExpr := unwrapModeExpr(effectiveExpr)
		expr := effectiveExpr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExprWithOptions(expr, env, diags, eval.ExprOptions{
			GlobalAssignmentTupleArithmetic: true,
			Context:                         eval.EvalCtxBindingAssign,
		})
		if isModeExpr {
			value = coerceModeValue(mode, value, asn.Span, diags)
		} else {
			mode = ""
		}

		if hasNestedList(value) {
			diags.AddError(
				diag.CodeE305,
				fmt.Sprintf("nested tuple/list value is not allowed for global variable '%s'", asn.Name),
				asn.Span,
				"use flat tuple/list values only",
			)
		}

		orderNames, vars := globalVarSeries(asn.Name, value)
		out[asn.Name] = &GlobalVar{
			Name:      asn.Name,
			Value:     value,
			Mode:      mode,
			Span:      asn.Span,
			Order:     orderNames,
			Vars:      vars,
			DependsOn: globalExprDependencies(effectiveExpr, asn.Name),
		}
		if !seenOrder[asn.Name] {
			seenOrder[asn.Name] = true
			order = append(order, asn.Name)
		}

		env[asn.Name] = value
	}

	return out, order
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
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if e.Name != "" {
			out[e.Name] = struct{}{}
		}
	case ast.QualifiedIdentExpr:
		if e.Namespace != "" {
			out[e.Namespace] = struct{}{}
		}
	case ast.ModeExpr:
		collectGlobalExprDeps(e.Expr, out)
	case ast.ListExpr:
		for _, it := range e.Items {
			collectGlobalExprDeps(it, out)
		}
	case ast.TupleExpr:
		for _, it := range e.Items {
			collectGlobalExprDeps(it, out)
		}
	case ast.ConvertExpr:
		collectGlobalExprDeps(e.Expr, out)
	case ast.CallExpr:
		for _, arg := range e.Args {
			collectGlobalExprDeps(arg, out)
		}
	case ast.AliasExpr:
		collectGlobalExprDeps(e.Expr, out)
	case ast.IndexExpr:
		collectGlobalExprDeps(e.Base, out)
	case ast.UnaryExpr:
		collectGlobalExprDeps(e.Expr, out)
	case ast.BinaryExpr:
		collectGlobalExprDeps(e.Left, out)
		collectGlobalExprDeps(e.Right, out)
	case ast.CompareExpr:
		collectGlobalExprDeps(e.Left, out)
		collectGlobalExprDeps(e.Right, out)
	case ast.ConditionalExpr:
		collectGlobalExprDeps(e.Then, out)
		collectGlobalExprDeps(e.Cond, out)
		collectGlobalExprDeps(e.Else, out)
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

func bindingFromGlobalVar(name string, gv *GlobalVar) *GlobalBinding {
	order := append([]string(nil), gv.Order...)
	if len(order) == 0 {
		order = []string{name}
	}

	vars := cloneSeriesMap(gv.Vars)
	baseVars := cloneSeriesMap(gv.Vars)
	origins := make(map[string]diag.Span, len(order))
	modes := make(map[string]string)
	for _, col := range order {
		origins[col] = gv.Span
	}
	if gv.Mode != "" && len(order) == 1 {
		modes[order[0]] = gv.Mode
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
		Modes:           modes,
		Order:           order,
		Span:            gv.Span,
		DependsOn:       append([]string(nil), gv.DependsOn...),
		SyntheticGlobal: true,
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
