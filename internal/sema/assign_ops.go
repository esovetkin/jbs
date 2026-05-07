// rewrite `name <op>= rhs` into `name = name <op> rhs`
package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func mapAssignOpToBinary(op ast.AssignOp) (string, bool) {
	switch op {
	case ast.AssignPlusEq:
		return "+", true
	case ast.AssignMinusEq:
		return "-", true
	case ast.AssignStarEq:
		return "*", true
	case ast.AssignSlashEq:
		return "/", true
	case ast.AssignPctEq:
		return "%", true
	default:
		return "", false
	}
}

func assignmentExpr(name string, op ast.AssignOp, rhs ast.Expr, lhsSpan diag.Span) ast.Expr {
	if rhs == nil || op == ast.AssignEq {
		return rhs
	}
	bop, ok := mapAssignOpToBinary(op)
	if !ok {
		return rhs
	}
	return ast.BinaryExpr{
		Left:  ast.IdentExpr{Name: name, Span: lhsSpan},
		Op:    bop,
		Right: rhs,
		Span:  diag.Merge(lhsSpan, rhs.GetSpan()),
	}
}
