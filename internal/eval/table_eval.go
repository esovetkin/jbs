package eval

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func evalTableCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) == 0 {
		diags.AddError(diag.CodeE106, "table() expects at least one named column argument", at, "use syntax such as table(id = ids, label = labels)")
		return Null()
	}

	order := make([]string, 0, len(rawArgs))
	seriesByName := make(map[string][]Value, len(rawArgs))
	spansByName := make(map[string]diag.Span, len(rawArgs))
	expectedLen := -1

	for _, arg := range rawArgs {
		if arg.Name == "" {
			diags.AddError(diag.CodeE106, "table() requires named arguments only", arg.Span, "use syntax such as table(id = ids, label = labels)")
			return Null()
		}
		if _, exists := seriesByName[arg.Name]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() duplicate column name '%s'", arg.Name), arg.Span, "use each column name at most once")
			return Null()
		}
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		if IsComb(value) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() column '%s' cannot be a table value", arg.Name), arg.Span, "pass a scalar, tuple, or list column value")
			return Null()
		}
		series := ToSeries(value)
		if expectedLen < 0 {
			expectedLen = len(series)
		} else if len(series) != expectedLen {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("table() column '%s' has length %d; expected %d", arg.Name, len(series), expectedLen),
				arg.Span,
				"use equal-length column values; table() does not broadcast columns",
			)
			return Null()
		}
		order = append(order, arg.Name)
		seriesByName[arg.Name] = series
		spansByName[arg.Name] = arg.Span
	}

	if expectedLen < 0 {
		expectedLen = 0
	}
	rows := make([]Row, 0, expectedLen)
	for i := 0; i < expectedLen; i++ {
		values := make(map[string]Cell, len(order))
		for _, name := range order {
			origin := spansByName[name]
			if origin.IsZero() {
				origin = at
			}
			values[name] = Cell{
				Value:  seriesByName[name][i],
				Origin: origin,
			}
		}
		rows = append(rows, Row{Values: values})
	}
	return tableValueFromOrderedRows(order, rows)
}

func evalZipCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	tables, ok := evalPositionalTableArgs("zip", rawArgs, env, at, diags, opts, ctx)
	if !ok {
		return Null()
	}
	if len(tables) == 1 {
		return cloneTableValue(tables[0])
	}

	refCount := CombRowCount(tables[0])
	for i := 1; i < len(tables); i++ {
		if got := CombRowCount(tables[i]); got != refCount {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("zip() row count mismatch: argument 1 has %d rows, argument %d has %d", refCount, i+1, got),
				rawArgs[i].Span,
				"use equal-length tables in zip(); zip() does not broadcast rows",
			)
			return Null()
		}
	}

	order, ok := combineTableOrders("zip", rawArgs, tables, diags)
	if !ok {
		return Null()
	}

	rows := make([]Row, 0, refCount)
	for rowIdx := 0; rowIdx < refCount; rowIdx++ {
		merged := Row{Values: map[string]Cell{}}
		for _, table := range tables {
			next, mergedOK := mergeRows(merged, table.C.Rows[rowIdx], at, diags)
			if !mergedOK {
				return Null()
			}
			merged = next
		}
		rows = append(rows, merged)
	}
	return tableValueFromOrderedRows(order, rows)
}

func evalProductCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	tables, ok := evalPositionalTableArgs("product", rawArgs, env, at, diags, opts, ctx)
	if !ok {
		return Null()
	}
	if len(tables) == 1 {
		return cloneTableValue(tables[0])
	}

	order, ok := combineTableOrders("product", rawArgs, tables, diags)
	if !ok {
		return Null()
	}

	rows := cloneRows(tables[0].C.Rows)
	opNode := ast.CombBinary{Op: "*", OpSpan: at, Span: at}
	for _, table := range tables[1:] {
		rows = productRows(rows, table.C.Rows, opNode, diags)
		if diags.HasErrors() {
			return Null()
		}
	}
	return tableValueFromOrderedRows(order, rows)
}

func evalSelectCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) < 2 {
		diags.AddError(diag.CodeE106, "select() expects a table and one or more column selectors", at, "use syntax such as select(grid, id, replica)")
		return Null()
	}
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "select() does not accept named arguments", arg.Span, "pass the table first, then one or more identifier selectors")
			return Null()
		}
	}

	tableValue := evalExprWithCtx(rawArgs[0].Expr, env, diags, opts, ctx)
	if !IsComb(tableValue) {
		diags.AddError(diag.CodeE106, "select() first argument must be a table value", rawArgs[0].Span, "pass a table built by table(), zip(), product(), select(), read_csv(), or legacy comb()")
		return Null()
	}

	selectors := make([]string, 0, len(rawArgs)-1)
	for _, arg := range rawArgs[1:] {
		name, ok := selectorName(arg.Expr)
		if !ok {
			diags.AddError(diag.CodeE106, "select() selectors must be identifiers", arg.Span, "use syntax such as select(grid, id, replica)")
			return Null()
		}
		selectors = append(selectors, name)
	}

	projected, ok := CombProject(tableValue, selectors)
	if !ok {
		diags.AddError(diag.CodeE106, "select() selectors must name existing table columns", at, "select existing columns only")
		return Null()
	}
	return projected
}

func evalPositionalTableArgs(name string, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) ([]Value, bool) {
	if len(rawArgs) == 0 {
		diags.AddError(diag.CodeE106, name+"() expects at least one table argument", at, "pass one or more table values")
		return nil, false
	}
	values := make([]Value, 0, len(rawArgs))
	for i, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, name+"() does not accept named arguments", arg.Span, "pass positional table arguments only")
			return nil, false
		}
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		if !IsComb(value) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() argument %d must be a table value", name, i+1), arg.Span, "pass table values only")
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func combineTableOrders(name string, rawArgs []ast.CallArg, tables []Value, diags *diag.Diagnostics) ([]string, bool) {
	order := make([]string, 0)
	seen := make(map[string]struct{})
	for i, table := range tables {
		for _, col := range CombNames(table) {
			if _, exists := seen[col]; exists {
				diags.AddError(diag.CodeE106, fmt.Sprintf("%s() duplicate column name '%s'", name, col), rawArgs[i].Span, "rename columns so every table column name is unique")
				return nil, false
			}
			seen[col] = struct{}{}
			order = append(order, col)
		}
	}
	return order, true
}

func selectorName(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case ast.IdentExpr:
		return e.Name, e.Name != ""
	case ast.QualifiedIdentExpr:
		if e.Namespace == "" || e.Name == "" {
			return "", false
		}
		return e.Namespace + "." + e.Name, true
	default:
		return "", false
	}
}

func tableValueFromOrderedRows(order []string, rows []Row) Value {
	return CombValue(&Comb{
		Order: append([]string(nil), order...),
		Rows:  cloneRows(rows),
	})
}

func cloneTableValue(value Value) Value {
	if !IsComb(value) {
		return Null()
	}
	return tableValueFromOrderedRows(value.C.Order, value.C.Rows)
}
