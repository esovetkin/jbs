package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func collectExprStringRefs(expr ast.Expr) []varRef {
	return collectExprStringRefsWith(expr, collectShellLikeRefs)
}

func collectExprIdentRefs(expr ast.Expr) []varRef {
	if expr == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.IdentExpr:
			out = append(out, varRef{
				Name: n.Name,
				Span: n.Span,
			})
		case ast.QualifiedIdentExpr:
			if n.Namespace != "" {
				out = append(out, varRef{
					Name: n.Namespace,
					Span: n.Span,
				})
			}
		case ast.MemberExpr:
			walk(n.Base)
		case ast.ListExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			walkFuncBodyIdentRefs(n.Body, walk)
		case ast.AliasExpr:
			walk(n.Expr)
		case ast.IndexExpr:
			walk(n.Base)
			for _, item := range n.Items {
				walk(item)
			}
		case ast.UnaryExpr:
			walk(n.Expr)
		case ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.CompareExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.ConditionalExpr:
			walk(n.Then)
			walk(n.Cond)
			walk(n.Else)
		}
	}
	walk(expr)
	return out
}

func walkFuncBodyIdentRefs(body []ast.FuncBodyStmt, walk func(ast.Expr)) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			walk(node.Expr)
		case ast.ReturnStmt:
			walk(node.Expr)
		case ast.ExprStmt:
			walk(node.Expr)
		case ast.FuncIfStmt:
			walk(node.Cond)
			walkFuncBodyIdentRefs(node.Then, walk)
			for _, branch := range node.Elifs {
				walk(branch.Cond)
				walkFuncBodyIdentRefs(branch.Body, walk)
			}
			walkFuncBodyIdentRefs(node.Else, walk)
		case ast.FuncForStmt:
			walk(node.Iterable)
			walkFuncBodyIdentRefs(node.Body, walk)
		case ast.FuncWhileStmt:
			walk(node.Cond)
			walkFuncBodyIdentRefs(node.Body, walk)
		}
	}
}

type stringRefCollector func(text string, base diag.Position, file string) []varRef

func collectExprStringRefsWith(expr ast.Expr, collect stringRefCollector) []varRef {
	if expr == nil {
		return nil
	}
	if collect == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.StringExpr:
			base := n.Span.Start
			base.Offset++
			base.Column++
			out = append(out, collect(n.Value, base, n.Span.File)...)
		case ast.ListExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.TupleExpr:
			for _, it := range n.Items {
				walk(it)
			}
		case ast.CallExpr:
			walk(n.Callee)
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			walkFuncBodyStringRefs(n.Body, walk)
		case ast.AliasExpr:
			walk(n.Expr)
		case ast.IndexExpr:
			walk(n.Base)
			for _, item := range n.Items {
				walk(item)
			}
		case ast.UnaryExpr:
			walk(n.Expr)
		case ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.CompareExpr:
			walk(n.Left)
			walk(n.Right)
		case ast.ConditionalExpr:
			walk(n.Then)
			walk(n.Cond)
			walk(n.Else)
		}
	}
	walk(expr)
	return out
}

func walkFuncBodyStringRefs(body []ast.FuncBodyStmt, walk func(ast.Expr)) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			walk(node.Expr)
		case ast.ReturnStmt:
			walk(node.Expr)
		case ast.ExprStmt:
			walk(node.Expr)
		case ast.FuncIfStmt:
			walk(node.Cond)
			walkFuncBodyStringRefs(node.Then, walk)
			for _, branch := range node.Elifs {
				walk(branch.Cond)
				walkFuncBodyStringRefs(branch.Body, walk)
			}
			walkFuncBodyStringRefs(node.Else, walk)
		case ast.FuncForStmt:
			walk(node.Iterable)
			walkFuncBodyStringRefs(node.Body, walk)
		case ast.FuncWhileStmt:
			walk(node.Cond)
			walkFuncBodyStringRefs(node.Body, walk)
		}
	}
}

func collectEvalStringRefsWith(value eval.Value, span diag.Span, collect stringRefCollector) []varRef {
	if collect == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walk func(eval.Value)
	walk = func(v eval.Value) {
		switch v.Kind {
		case eval.KindString:
			base := span.Start
			if base.Line == 0 {
				base = diag.NewPos(0, 1, 1)
			}
			out = append(out, collect(v.S, base, span.File)...)
		case eval.KindList, eval.KindTuple:
			for _, item := range v.L {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}
