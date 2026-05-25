package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/shellref"
)

func collectExprStringRefs(expr ast.Expr) []varRef {
	return collectExprStringRefsWith(expr, collectShellStringRefs)
}

func collectShellStringRefs(text string, base diag.Position, file string) []varRef {
	return shellRefsToVarRefs(shellref.Collect(text, base, file))
}

func collectExprIdentRefs(expr ast.Expr) []varRef {
	if expr == nil {
		return nil
	}
	out := make([]varRef, 0)
	var callbacks ast.WalkCallbacks
	callbacks.Expr = func(node ast.Expr) ast.WalkAction {
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

func collectExprFreeIdentRefs(expr ast.Expr) []varRef {
	if expr == nil {
		return nil
	}
	out := make([]varRef, 0)
	var walkExprBound func(ast.Expr, map[string]struct{})
	var walkBodyBound func([]ast.FuncBodyStmt, map[string]struct{})

	callbacksForBound := func(bound map[string]struct{}) ast.WalkCallbacks {
		var callbacks ast.WalkCallbacks
		callbacks.Expr = func(node ast.Expr) ast.WalkAction {
			switch n := node.(type) {
			case ast.IdentExpr:
				if n.Name != "" && !nameSetContains(bound, n.Name) {
					out = append(out, varRef{
						Name: n.Name,
						Span: n.Span,
					})
				}
			case ast.QualifiedIdentExpr:
				if n.Namespace != "" && !nameSetContains(bound, n.Namespace) {
					out = append(out, varRef{
						Name: n.Namespace,
						Span: n.Span,
					})
				}
			case ast.CallExpr:
				if isDeleteCallExpr(n) && deleteCallHasOnlyBareTargets(n) {
					walkExprBound(n.Callee, bound)
					return ast.WalkSkipChildren
				}
			case ast.FunctionExpr:
				nextBound := cloneNameSet(bound)
				for _, param := range n.Params {
					walkExprBound(param.Default, nextBound)
					if param.Name != "" {
						nextBound[param.Name] = struct{}{}
					}
				}
				collectFuncBodyLocalNames(n.Body, nextBound)
				walkBodyBound(n.Body, nextBound)
				return ast.WalkSkipChildren
			}
			return ast.WalkContinue
		}
		callbacks.FuncBodyStmt = func(stmt ast.FuncBodyStmt) ast.WalkAction {
			node, ok := stmt.(ast.FuncForStmt)
			if !ok {
				return ast.WalkContinue
			}
			walkExprBound(node.Iterable, bound)
			nextBound := cloneNameSet(bound)
			if node.Target != "" {
				nextBound[node.Target] = struct{}{}
			}
			walkBodyBound(node.Body, nextBound)
			return ast.WalkSkipChildren
		}
		return callbacks
	}

	walkExprBound = func(expr ast.Expr, bound map[string]struct{}) {
		ast.WalkExpr(expr, callbacksForBound(bound))
	}
	walkBodyBound = func(body []ast.FuncBodyStmt, bound map[string]struct{}) {
		ast.WalkFuncBody(body, callbacksForBound(bound))
	}
	walkExprBound(expr, nil)
	return out
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
	ast.WalkExpr(expr, ast.WalkCallbacks{
		Expr: func(node ast.Expr) ast.WalkAction {
			n, ok := node.(ast.StringExpr)
			if !ok {
				return ast.WalkContinue
			}
			base := n.Span.Start
			base.Offset++
			base.Column++
			out = append(out, collect(n.Value, base, n.Span.File)...)
			return ast.WalkContinue
		},
	})
	return out
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
		case eval.KindDict:
			if v.D == nil {
				return
			}
			for _, key := range v.D.Order {
				walk(eval.ValueFromDictKey(key))
				if value, ok := v.D.Entries[key]; ok {
					walk(value)
				}
			}
		}
	}
	walk(value)
	return out
}
