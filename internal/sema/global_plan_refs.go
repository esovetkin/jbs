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
	var walk func(ast.Expr)
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
	walk = func(node ast.Expr) {
		if node == nil {
			return
		}
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
		case ast.MemberExpr:
			walk(n.Base)
		case ast.ListExpr:
			for _, item := range n.Items {
				walk(item)
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				walk(item)
			}
		case ast.DictExpr:
			for _, entry := range n.Entries {
				walk(entry.Key)
				walk(entry.Value)
			}
		case ast.CallExpr:
			walk(n.Callee)
			if isDeleteCallExpr(n) {
				return
			}
			for _, arg := range n.Args {
				walk(arg.Expr)
			}
		case ast.FunctionExpr:
			for _, param := range n.Params {
				walk(param.Default)
			}
			walkFuncBodyExprRefs(n.Body, walk)
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

func walkFuncBodyExprRefs(body []ast.FuncBodyStmt, walk func(ast.Expr)) {
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
			walkFuncBodyExprRefs(node.Then, walk)
			for _, branch := range node.Elifs {
				walk(branch.Cond)
				walkFuncBodyExprRefs(branch.Body, walk)
			}
			walkFuncBodyExprRefs(node.Else, walk)
		case ast.FuncForStmt:
			walk(node.Iterable)
			walkFuncBodyExprRefs(node.Body, walk)
		case ast.FuncWhileStmt:
			walk(node.Cond)
			walkFuncBodyExprRefs(node.Body, walk)
		}
	}
}
