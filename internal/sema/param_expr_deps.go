// track local variable dependencies in `param` assignments
package sema

import (
	"fmt"
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
	case ast.CallExpr:
		for _, arg := range e.Args {
			collectExprLocalIdentDeps(arg, out)
		}
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
	warnUnusedParamContributors(assigns, order, nil, nil, seed, diags)
}

func warnUnusedParamContributors(assigns map[string]localAssignMeta, order []string, imported map[string]importedContribution, importedOrder []string, seed []string, diags *diag.Diagnostics) {
	if len(assigns) == 0 && len(imported) == 0 {
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
				continue
			}
			if _, ok := imported[dep]; ok {
				deps = append(deps, dep)
			}
		}
		slices.Sort(deps)
		depsByVar[name] = deps
	}

	reachable := make(map[string]bool, len(assigns)+len(imported))
	var markReachable func(string)
	markReachable = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true
		if _, ok := imported[name]; ok {
			return
		}
		for _, dep := range depsByVar[name] {
			markReachable(dep)
		}
	}

	for _, root := range seed {
		if root == "" {
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
	for _, name := range importedOrder {
		meta, ok := imported[name]
		if !ok {
			continue
		}
		if reachable[name] {
			continue
		}
		diags.AddWarning(
			diag.CodeW312,
			fmt.Sprintf("imported variable '%s' from source '%s' does not contribute to the final expression", name, meta.Source),
			meta.Span,
			"remove it from imports or reference it (directly or transitively) in the final combination expression",
		)
	}
}
