// track local variable dependencies in `param` assignments
package sema

import (
	"slices"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

type localAssignMeta struct {
	Expr ast.Expr
	Span diag.Span
}

func collectExprLocalIdentDeps(expr ast.Expr, out map[string]struct{}) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if e.Name != "" {
			out[e.Name] = struct{}{}
		}
	case ast.QualifiedIdentExpr:
		return
	case ast.ModeExpr:
		collectExprLocalIdentDeps(e.Expr, out)
	case ast.ListExpr:
		for _, it := range e.Items {
			collectExprLocalIdentDeps(it, out)
		}
	case ast.TupleExpr:
		for _, it := range e.Items {
			collectExprLocalIdentDeps(it, out)
		}
	case ast.ConvertExpr:
		collectExprLocalIdentDeps(e.Expr, out)
	case ast.UnaryExpr:
		collectExprLocalIdentDeps(e.Expr, out)
	case ast.BinaryExpr:
		collectExprLocalIdentDeps(e.Left, out)
		collectExprLocalIdentDeps(e.Right, out)
	case ast.CompareExpr:
		collectExprLocalIdentDeps(e.Left, out)
		collectExprLocalIdentDeps(e.Right, out)
	case ast.ConditionalExpr:
		collectExprLocalIdentDeps(e.Then, out)
		collectExprLocalIdentDeps(e.Cond, out)
		collectExprLocalIdentDeps(e.Else, out)
	}
}

func warnUnusedParamLocals(assigns map[string]localAssignMeta, order []string, seed []string, diags *diag.Diagnostics) {
	if len(assigns) == 0 || len(seed) == 0 {
		return
	}

	depsByVar := make(map[string][]string, len(assigns))
	for name, meta := range assigns {
		depsSet := make(map[string]struct{}, 4)
		collectExprLocalIdentDeps(meta.Expr, depsSet)
		deps := make([]string, 0, len(depsSet))
		for dep := range depsSet {
			if dep == name {
				continue
			}
			if _, ok := assigns[dep]; ok {
				deps = append(deps, dep)
			}
		}
		slices.Sort(deps)
		depsByVar[name] = deps
	}

	reachable := make(map[string]bool, len(assigns))
	var markReachable func(string)
	markReachable = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true
		for _, dep := range depsByVar[name] {
			markReachable(dep)
		}
	}

	for _, root := range seed {
		if _, ok := assigns[root]; !ok {
			continue
		}
		markReachable(root)
	}

	for _, name := range order {
		meta, ok := assigns[name]
		if !ok {
			continue
		}
		if reachable[name] {
			continue
		}
		diags.AddWarning(
			diag.CodeW312,
			"param variable '"+name+"' is declared but does not contribute to the final expression",
			meta.Span,
			"remove it, or reference it (directly or transitively) in the final combination expression",
		)
	}
}
