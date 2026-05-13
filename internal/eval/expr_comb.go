package eval

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func binaryNeedsRelaxedCombEval(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case ast.AliasExpr:
		return true
	case ast.MemberExpr:
		return binaryNeedsRelaxedCombEval(e.Base)
	case ast.BinaryExpr:
		return binaryNeedsRelaxedCombEval(e.Left) || binaryNeedsRelaxedCombEval(e.Right)
	case ast.UnaryExpr:
		return binaryNeedsRelaxedCombEval(e.Expr)
	case ast.CallExpr:
		if binaryNeedsRelaxedCombEval(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if binaryNeedsRelaxedCombEval(arg.Expr) {
				return true
			}
		}
		return false
	case ast.IndexExpr:
		if binaryNeedsRelaxedCombEval(e.Base) {
			return true
		}
		for _, item := range e.Items {
			if binaryNeedsRelaxedCombEval(item) {
				return true
			}
		}
		return false
	case ast.ListExpr:
		for _, item := range e.Items {
			if binaryNeedsRelaxedCombEval(item) {
				return true
			}
		}
		return false
	case ast.TupleExpr:
		for _, item := range e.Items {
			if binaryNeedsRelaxedCombEval(item) {
				return true
			}
		}
		return false
	case ast.DictExpr:
		for _, entry := range e.Entries {
			if binaryNeedsRelaxedCombEval(entry.Key) || binaryNeedsRelaxedCombEval(entry.Value) {
				return true
			}
		}
		return false
	case ast.CompareExpr:
		return binaryNeedsRelaxedCombEval(e.Left) || binaryNeedsRelaxedCombEval(e.Right)
	case ast.ConditionalExpr:
		return binaryNeedsRelaxedCombEval(e.Then) || binaryNeedsRelaxedCombEval(e.Cond) || binaryNeedsRelaxedCombEval(e.Else)
	default:
		return false
	}
}

func evalRelaxedCombBinary(expr ast.BinaryExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	left, okLeft := evalRelaxedCombOperand(expr.Left, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	right, okRight := evalRelaxedCombOperand(expr.Right, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	if !okLeft || !okRight {
		return Null()
	}
	opNode := ast.CombBinary{Op: expr.Op, OpSpan: expr.Span, Span: expr.Span}
	if expr.Op == "+" {
		return combValueFromRows(rowWiseMergeRows(left, right, opNode, diags))
	}
	return combValueFromRows(productRows(left, right, opNode, diags))
}

func evalRelaxedCombOperand(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) ([]Row, bool) {
	if expr == nil {
		return nil, false
	}
	if alias, ok := expr.(ast.AliasExpr); ok {
		if alias.Alias == "" {
			diags.AddError(diag.CodeE106, "table operand alias cannot be empty", alias.Span, "use syntax: expression as name")
			return nil, false
		}
		value := evalExprWithCtx(alias.Expr, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return nil, false
		}
		if IsComb(value) {
			diags.AddError(diag.CodeE106, "alias cannot be applied to a table-valued expression", alias.Span, "apply alias only to non-table operands")
			return nil, false
		}
		return combRowsFromNamedValue(alias.Alias, value, alias.Span), true
	}
	value := evalExprWithCtx(expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return nil, false
	}
	return combRowsFromBinaryOperand(expr, value, env, diags, opts, ctx), true
}

func combRowsFromNamedValue(name string, value Value, span diag.Span) []Row {
	if IsComb(value) {
		return cloneRows(value.C.Rows)
	}
	series := ToSeries(value)
	rows := make([]Row, 0, len(series))
	for _, v := range series {
		rows = append(rows, Row{
			Values: map[string]Cell{
				name: {
					Value:  v,
					Origin: span,
				},
			},
		})
	}
	return rows
}

func combRowsFromBinaryOperand(expr ast.Expr, value Value, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) []Row {
	if expr == nil {
		return combRowsFromValue(value, diag.Span{})
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		return combRowsFromNamedValue(e.Name, value, e.Span)
	case ast.QualifiedIdentExpr:
		return combRowsFromNamedValue(e.Namespace+"."+e.Name, value, e.Span)
	case ast.AliasExpr:
		rows, _ := evalRelaxedCombOperand(e, env, diags, opts, ctx)
		return rows
	default:
		return combRowsFromValue(value, expr.GetSpan())
	}
}

func combRowsFromValue(value Value, _ diag.Span) []Row {
	if IsComb(value) {
		return cloneRows(value.C.Rows)
	}
	series := ToSeries(value)
	rows := make([]Row, 0, len(series))
	for range series {
		rows = append(rows, Row{Values: map[string]Cell{}})
	}
	return rows
}

func combValueFromRows(rows []Row) Value {
	if rows == nil {
		rows = make([]Row, 0)
	}
	return CombValue(&Comb{
		Order: RowVariableNames(rows),
		Rows:  cloneRows(rows),
	})
}
