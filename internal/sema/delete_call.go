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
