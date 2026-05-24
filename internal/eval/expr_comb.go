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
	case ast.RangeExpr:
		return binaryNeedsRelaxedCombEval(e.Start) ||
			binaryNeedsRelaxedCombEval(e.Stop) ||
			binaryNeedsRelaxedCombEval(e.Step)
	case ast.CompareExpr:
		return binaryNeedsRelaxedCombEval(e.Left) || binaryNeedsRelaxedCombEval(e.Right)
	case ast.ConditionalExpr:
		return binaryNeedsRelaxedCombEval(e.Then) || binaryNeedsRelaxedCombEval(e.Cond) || binaryNeedsRelaxedCombEval(e.Else)
	default:
		return false
	}
}

func evalRelaxedCombBinary(expr ast.BinaryExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	left, okLeft := evalTableAlgebraOperand(expr.Left, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	right, okRight := evalTableAlgebraOperand(expr.Right, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	if !okLeft || !okRight {
		return Null()
	}
	opNode := ast.CombBinary{Op: expr.Op, OpSpan: expr.Span, Span: expr.Span}
	if expr.Op == "+" {
		return combValueFromOrderedRows(rowWiseMergeOrderedRows(left, right, opNode, diags))
	}
	return combValueFromOrderedRows(productOrderedRows(left, right, opNode, diags))
}

func evalTableAlgebraOperand(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) (orderedRows, bool) {
	if expr == nil {
		return orderedRows{}, false
	}
	if alias, ok := expr.(ast.AliasExpr); ok {
		return evalAliasedTableAlgebraOperand(alias, env, diags, opts, ctx)
	}
	value := evalExprWithCtx(expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return orderedRows{}, false
	}
	return tableAlgebraRowsFromUnaliasedValue(expr, value, diags)
}

func evalAliasedTableAlgebraOperand(alias ast.AliasExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) (orderedRows, bool) {
	if alias.Alias == "" {
		diags.AddError(diag.CodeE106, "table operand alias cannot be empty", alias.Span, "use syntax: expression as name")
		return orderedRows{}, false
	}
	value := evalExprWithCtx(alias.Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return orderedRows{}, false
	}
	if IsComb(value) {
		diags.AddError(diag.CodeE106, "alias cannot be applied to a table-valued expression", alias.Span, "apply alias only to non-table operands")
		return orderedRows{}, false
	}
	return orderedRowsFromNamedValue(alias.Alias, value, alias.Span), true
}

func combRowsFromNamedValue(name string, value Value, span diag.Span) []Row {
	if IsComb(value) {
		return cloneRows(value.C.Rows)
	}
	series := ToSeries(value)
	rows := make([]Row, 0, len(series))
	source := NewProjectionSource()
	for i, v := range series {
		rows = append(rows, Row{
			Values: map[string]Cell{
				name: {
					Value:      v,
					Origin:     span,
					Projection: ProjectionKey{Source: source, Index: i},
				},
			},
		})
	}
	return rows
}

type orderedRows struct {
	Order []string
	Rows  []Row
}

func orderedRowsFromNamedValue(name string, value Value, span diag.Span) orderedRows {
	rows := combRowsFromNamedValue(name, value, span)
	if IsComb(value) {
		return orderedRowsFromTableValue(value)
	}
	return orderedRows{Order: []string{name}, Rows: rows}
}

func tableAlgebraRowsFromUnaliasedValue(expr ast.Expr, value Value, diags *diag.Diagnostics) (orderedRows, bool) {
	if expr == nil {
		if IsComb(value) {
			return orderedRowsFromTableValue(value), true
		}
		addAnonymousTableAlgebraOperandDiag(diag.Span{}, diags)
		return orderedRows{}, false
	}
	if IsComb(value) {
		return orderedRowsFromTableValue(value), true
	}
	if ident, ok := expr.(ast.IdentExpr); ok {
		return orderedRowsFromNamedValue(ident.Name, value, ident.Span), true
	}
	addAnonymousTableAlgebraOperandDiag(expr.GetSpan(), diags)
	return orderedRows{}, false
}

func tableAlgebraRowsFromEvaluatedOperand(expr ast.Expr, value Value, diags *diag.Diagnostics) (orderedRows, bool) {
	if IsComb(value) {
		return orderedRowsFromTableValue(value), true
	}
	if ident, ok := expr.(ast.IdentExpr); ok {
		return orderedRowsFromNamedValue(ident.Name, value, ident.Span), true
	}
	addAnonymousTableAlgebraOperandDiag(expr.GetSpan(), diags)
	return orderedRows{}, false
}

func orderedRowsFromTableValue(value Value) orderedRows {
	return orderedRows{Order: CombNames(value), Rows: cloneRows(value.C.Rows)}
}

func addAnonymousTableAlgebraOperandDiag(at diag.Span, diags *diag.Diagnostics) {
	diags.AddError(
		diag.CodeE106,
		"anonymous non-table operand in table algebra requires an alias",
		at,
		"write `(<expr> as name)` or `table(name = <expr>)`",
	)
}

func rowWiseMergeOrderedRows(left, right orderedRows, op ast.CombBinary, diags *diag.Diagnostics) orderedRows {
	return orderedRows{
		Order: mergeRowOrders(left.Order, right.Order),
		Rows:  rowWiseMergeRows(left.Rows, right.Rows, op, diags),
	}
}

func productOrderedRows(left, right orderedRows, op ast.CombBinary, diags *diag.Diagnostics) orderedRows {
	return orderedRows{
		Order: mergeRowOrders(left.Order, right.Order),
		Rows:  productRows(left.Rows, right.Rows, op, diags),
	}
}

func mergeRowOrders(left, right []string) []string {
	out := make([]string, 0, len(left)+len(right))
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, name := range left {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range right {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func combValueFromOrderedRows(rows orderedRows) Value {
	order := rows.Order
	if len(order) == 0 {
		order = RowVariableNames(rows.Rows)
	}
	return tableValueFromOrderedRows(order, rows.Rows)
}
