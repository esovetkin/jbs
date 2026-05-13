package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
)

func globalExprReadNames(expr ast.Expr) []string {
	refs := globalExprReadRefs(expr)
	out := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs)*2)
	for _, ref := range refs {
		if ref.Name != "" {
			if _, ok := seen[ref.Name]; !ok {
				seen[ref.Name] = struct{}{}
				out = append(out, ref.Name)
			}
		}
		if ref.SeedAlt != "" {
			if _, ok := seen[ref.SeedAlt]; !ok {
				seen[ref.SeedAlt] = struct{}{}
				out = append(out, ref.SeedAlt)
			}
		}
	}
	return out
}

func globalExprReadRefs(expr ast.Expr) []globalReadRef {
	out := make([]globalReadRef, 0)
	seen := make(map[globalReadRef]struct{})
	appendRef := func(ref globalReadRef) {
		if ref.Name == "" && ref.SeedAlt == "" {
			return
		}
		if _, ok := seen[ref]; ok {
			return
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	var callbacks ast.WalkCallbacks
	callbacks.Expr = func(node ast.Expr) ast.WalkAction {
		switch n := node.(type) {
		case ast.IdentExpr:
			appendRef(globalReadRef{Name: n.Name})
		case ast.QualifiedIdentExpr:
			if n.Namespace != "" {
				seedAlt := ""
				if n.Name != "" {
					seedAlt = n.Namespace + "." + n.Name
				}
				appendRef(globalReadRef{Name: n.Namespace, SeedAlt: seedAlt})
			}
		case ast.CallExpr:
			if isDeleteCallExpr(n) && deleteCallHasOnlyBareTargets(n) {
				ast.WalkExpr(n.Callee, callbacks)
				return ast.WalkSkipChildren
			}
		}
		return ast.WalkContinue
	}
	ast.WalkExpr(expr, callbacks)
	return out
}
