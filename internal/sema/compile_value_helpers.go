// share mode/series helper functions across active sema compilers
package sema

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

// warnModeExprInCollections reports mode() usage nested inside list/tuple expressions.
func warnModeExprInCollections(expr ast.Expr, diags *diag.Diagnostics) {
	var walk func(ast.Expr, bool)
	walk = func(node ast.Expr, inCollection bool) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case ast.ModeExpr:
			if inCollection {
				diags.AddWarning(
					diag.CodeW301,
					fmt.Sprintf("%s(...) used inside tuple/list expression", n.Mode),
					n.Span,
					"use shell()/python() as a standalone assignment value, then reference the variable",
				)
			}
			walk(n.Expr, inCollection)
		case ast.ListExpr:
			for _, item := range n.Items {
				walk(item, true)
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				walk(item, true)
			}
		case ast.ConvertExpr:
			walk(n.Expr, inCollection)
		case ast.CallExpr:
			for _, arg := range n.Args {
				walk(arg, inCollection)
			}
		case ast.AliasExpr:
			walk(n.Expr, inCollection)
		case ast.UnaryExpr:
			walk(n.Expr, inCollection)
		case ast.BinaryExpr:
			walk(n.Left, inCollection)
			walk(n.Right, inCollection)
		case ast.CompareExpr:
			walk(n.Left, inCollection)
			walk(n.Right, inCollection)
		case ast.ConditionalExpr:
			walk(n.Then, inCollection)
			walk(n.Cond, inCollection)
			walk(n.Else, inCollection)
		}
	}
	walk(expr, false)
}

// seriesAsValue converts a row series into scalar/list value representation.
func seriesAsValue(v []eval.Value) eval.Value {
	if len(v) == 0 {
		return eval.Null()
	}
	if len(v) == 1 {
		return v[0]
	}
	out := make([]eval.Value, len(v))
	copy(out, v)
	return eval.List(out)
}

// unwrapModeExpr extracts mode and inner expression from mode wrappers.
func unwrapModeExpr(expr ast.Expr) (string, ast.Expr, bool) {
	modeExpr, ok := expr.(ast.ModeExpr)
	if !ok {
		return "", nil, false
	}
	return modeExpr.Mode, modeExpr.Expr, true
}

// coerceModeValue coerces mode expression values into string/list-of-string.
func coerceModeValue(mode string, value eval.Value, at diag.Span, diags *diag.Diagnostics) eval.Value {
	switch value.Kind {
	case eval.KindString:
		return value
	case eval.KindList, eval.KindTuple:
		items := make([]eval.Value, len(value.L))
		for i, it := range value.L {
			if it.Kind != eval.KindString {
				diags.AddError(
					diag.CodeE215,
					fmt.Sprintf("%s(...) requires string values", mode),
					at,
					"pass a string expression to mode declarations",
				)
			}
			items[i] = eval.String(it.String())
		}
		return eval.List(items)
	default:
		diags.AddError(
			diag.CodeE215,
			fmt.Sprintf("%s(...) requires string values", mode),
			at,
			"pass a string expression to mode declarations",
		)
		return eval.String(value.String())
	}
}
