package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func countDiagCode(diags *diag.Diagnostics, code string) int {
	if diags == nil {
		return 0
	}
	count := 0
	for _, item := range diags.Items {
		if item.Code == code {
			count++
		}
	}
	return count
}

func withIdentItem(name string, span diag.Span) ast.WithItem {
	return ast.WithItem{Expr: ast.IdentExpr{Name: name, Span: span}, Span: span}
}

func withIndexStringItem(source string, selectors []string, span diag.Span) ast.WithItem {
	items := make([]ast.Expr, 0, len(selectors))
	for _, selector := range selectors {
		items = append(items, ast.StringExpr{Value: selector, Span: span})
	}
	return ast.WithItem{
		Expr: ast.IndexExpr{
			Base:  ast.IdentExpr{Name: source, Span: span},
			Items: items,
			Span:  span,
		},
		Span: span,
	}
}

func tableValueFromVars(order []string, vars map[string][]eval.Value) eval.Value {
	rowCount := 0
	for _, values := range vars {
		if len(values) > rowCount {
			rowCount = len(values)
		}
	}
	rows := make([]eval.Row, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		cells := make(map[string]eval.Cell, len(order))
		for _, name := range order {
			values := vars[name]
			if i >= len(values) {
				continue
			}
			cells[name] = eval.Cell{Value: eval.CloneValue(values[i])}
		}
		rows = append(rows, eval.Row{Values: cells})
	}
	return eval.CombValue(&eval.Comb{Order: append([]string(nil), order...), Rows: rows})
}
