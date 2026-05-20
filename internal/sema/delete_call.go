package sema

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"

func isDeleteCallExpr(expr ast.Expr) bool {
	call, ok := expr.(ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Callee.(ast.IdentExpr)
	return ok && ident.Name == "delete"
}

func isQuietTopLevelCallExpr(expr ast.Expr) bool {
	call, ok := expr.(ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Callee.(ast.IdentExpr)
	if !ok {
		return false
	}
	return ident.Name == "delete" || ident.Name == "setseed"
}

func deleteCallHasOnlyBareTargets(call ast.CallExpr) bool {
	for _, arg := range call.Args {
		if arg.EffectiveKind() != ast.CallArgPositional {
			return false
		}
		if _, ok := arg.Expr.(ast.IdentExpr); !ok {
			return false
		}
	}
	return true
}
