package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/shellvar"
)

type tableColumnInput struct {
	Name   string
	Values []Value
	Span   diag.Span
}

func evalTableValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) == 0 {
		diags.AddError(diag.CodeE106, "table() expects named column arguments, one dictionary argument, or one list of dictionaries", at, "use table(id = ids), table(dict_value), or table(rows(table_value))")
		return Null()
	}
	if len(args) == 1 && args[0].Name == "" {
		return evalTableFromPositionalValueArg(args[0], at, diags)
	}
	if hasPositionalValueArg(args) {
		diags.AddError(diag.CodeE106, "table() positional argument must be a dictionary or list of dictionaries", firstPositionalValueSpan(args), "use table(dict_value), table(rows(table_value)), or named columns such as table(id = ids)")
		return Null()
	}
	return evalNamedTableValueColumns(args, at, diags)
}

func evalNamedTableValueColumns(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	seen := make(map[string]struct{}, len(args))
	columns := make([]tableColumnInput, 0, len(args))
	for _, arg := range args {
		if _, exists := seen[arg.Name]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() duplicate column name '%s'", arg.Name), arg.Span, "use each column name at most once")
			return Null()
		}
		if !isValidCombColumnName(arg.Name) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() invalid column name '%s'", arg.Name), arg.Span, "use shell variable names such as x, system_name, or _tmp")
			return Null()
		}
		seen[arg.Name] = struct{}{}
		if IsComb(arg.Value) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() column '%s' cannot be a table value", arg.Name), arg.Span, "pass a scalar, tuple, or list column value")
			return Null()
		}
		columns = append(columns, tableColumnInput{
			Name:   arg.Name,
			Values: ToSeries(arg.Value),
			Span:   arg.Span,
		})
	}
	return buildTableFromColumns(columns, at, diags)
}

func evalTableFromPositionalValueArg(arg CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	return tableFromPositionalValue(arg.Value, nonZeroSpan(arg.Span, at), diags)
}

func TableFromDictValue(value Value, at diag.Span, diags *diag.Diagnostics) (Value, bool) {
	if value.Kind != KindDict || value.D == nil {
		diags.AddError(diag.CodeE106, "table() positional argument must be a dictionary", at, "use table(dict_value) or named columns such as table(id = ids)")
		return Null(), false
	}
	before := len(diags.Items)
	out := tableFromDict(value, at, diags)
	for _, item := range diags.Items[before:] {
		if item.Severity == diag.SeverityError {
			return out, false
		}
	}
	return out, true
}

func tableFromPositionalValue(value Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch {
	case value.Kind == KindDict && value.D != nil:
		return tableFromDict(value, at, diags)
	case value.Kind == KindList:
		return tableFromRowDictList(value, at, diags)
	default:
		diags.AddError(diag.CodeE106, "table() positional argument must be a dictionary or list of dictionaries", at, "use table(dict_value), table(rows(table_value)), or named columns such as table(id = ids)")
		return Null()
	}
}

func tableFromDict(value Value, at diag.Span, diags *diag.Diagnostics) Value {
	if value.D == nil || len(value.D.Order) == 0 {
		return CombValue(&Comb{})
	}
	seen := make(map[string]struct{}, len(value.D.Order))
	columns := make([]tableColumnInput, 0, len(value.D.Order))
	for _, key := range value.D.Order {
		colValue, ok := value.D.Entries[key]
		if !ok {
			continue
		}
		name, ok := tableColumnNameFromDictKey(key, at, diags)
		if !ok {
			return Null()
		}
		if _, exists := seen[name]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() duplicate column name '%s'", name), at, "use each column name at most once")
			return Null()
		}
		if IsComb(colValue) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() column '%s' cannot be a table value", name), at, "pass a scalar, tuple, or list column value")
			return Null()
		}
		seen[name] = struct{}{}
		columns = append(columns, tableColumnInput{
			Name:   name,
			Values: ToSeries(colValue),
			Span:   at,
		})
	}
	return buildTableFromColumns(columns, at, diags)
}

func tableColumnNameFromDictKey(key DictKey, at diag.Span, diags *diag.Diagnostics) (string, bool) {
	if key.Kind != DictKeyString {
		diags.AddError(diag.CodeE106, "table() dictionary keys must be strings", at, "use string keys that are valid shell variable names")
		return "", false
	}
	if !isValidCombColumnName(key.S) {
		diags.AddError(diag.CodeE106, fmt.Sprintf("table() invalid dictionary key '%s'", key.S), at, "use shell variable names such as x, system_name, or _tmp")
		return "", false
	}
	return key.S, true
}

func tableFromRowDictList(value Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(value.L) == 0 {
		return CombValue(&Comb{
			Order: append([]string(nil), value.RowSchema...),
			Rows:  nil,
		})
	}

	order, expected, ok := rowDictSchema(value.L[0], 0, at, diags)
	if !ok {
		return Null()
	}

	rows := make([]Row, 0, len(value.L))
	for rowIndex, rowValue := range value.L {
		row, ok := tableRowFromDictValue(rowValue, rowIndex, order, expected, at, diags)
		if !ok {
			return Null()
		}
		rows = append(rows, row)
	}

	return CombValue(&Comb{Order: order, Rows: rows})
}

func rowDictSchema(value Value, rowIndex int, at diag.Span, diags *diag.Diagnostics) ([]string, map[string]struct{}, bool) {
	if value.Kind != KindDict || value.D == nil {
		diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d must be a dictionary", rowIndex+1), at, "use a list whose elements are dictionaries")
		return nil, nil, false
	}

	order := make([]string, 0, len(value.D.Order))
	seen := make(map[string]struct{}, len(value.D.Order))
	for _, key := range value.D.Order {
		name, ok := rowTableColumnName(key, rowIndex, at, diags)
		if !ok {
			return nil, nil, false
		}
		if _, exists := seen[name]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d duplicate column name '%s'", rowIndex+1, name), at, "use each row key at most once")
			return nil, nil, false
		}
		seen[name] = struct{}{}
		order = append(order, name)
	}

	return order, seen, true
}

func tableRowFromDictValue(value Value, rowIndex int, order []string, expected map[string]struct{}, at diag.Span, diags *diag.Diagnostics) (Row, bool) {
	if value.Kind != KindDict || value.D == nil {
		diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d must be a dictionary", rowIndex+1), at, "use a list whose elements are dictionaries")
		return Row{}, false
	}

	values := make(map[string]Cell, len(order))
	seen := make(map[string]struct{}, len(order))
	for _, key := range value.D.Order {
		name, ok := rowTableColumnName(key, rowIndex, at, diags)
		if !ok {
			return Row{}, false
		}
		if _, want := expected[name]; !want {
			addRowKeyMismatchDiag(rowIndex, at, diags)
			return Row{}, false
		}
		if _, exists := seen[name]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d duplicate column name '%s'", rowIndex+1, name), at, "use each row key at most once")
			return Row{}, false
		}
		cellValue, ok := value.D.Entries[key]
		if !ok {
			addRowKeyMismatchDiag(rowIndex, at, diags)
			return Row{}, false
		}
		if !cellValue.IsScalar() {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d column '%s' must be scalar", rowIndex+1, name), at, "use int, float, string, or bool row values")
			return Row{}, false
		}
		seen[name] = struct{}{}
		values[name] = Cell{Value: CloneValue(cellValue), Origin: at}
	}

	if len(seen) != len(expected) {
		addRowKeyMismatchDiag(rowIndex, at, diags)
		return Row{}, false
	}
	for _, name := range order {
		if _, ok := values[name]; !ok {
			addRowKeyMismatchDiag(rowIndex, at, diags)
			return Row{}, false
		}
	}

	return Row{Values: values}, true
}

func rowTableColumnName(key DictKey, rowIndex int, at diag.Span, diags *diag.Diagnostics) (string, bool) {
	if key.Kind != DictKeyString {
		diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d keys must be strings", rowIndex+1), at, "use string keys that are valid shell variable names")
		return "", false
	}
	if !isValidCombColumnName(key.S) {
		diags.AddError(diag.CodeE106, fmt.Sprintf("table() invalid row key '%s'", key.S), at, "use shell variable names such as x, system_name, or _tmp")
		return "", false
	}
	return key.S, true
}

func addRowKeyMismatchDiag(rowIndex int, at diag.Span, diags *diag.Diagnostics) {
	diags.AddError(diag.CodeE106, fmt.Sprintf("table() row %d has keys that do not match row 1", rowIndex+1), at, "make every row dictionary use the same key set")
}

func buildTableFromColumns(columns []tableColumnInput, at diag.Span, diags *diag.Diagnostics) Value {
	order := make([]string, 0, len(columns))
	maxLen := 0
	for _, col := range columns {
		order = append(order, col.Name)
		if len(col.Values) > maxLen {
			maxLen = len(col.Values)
		}
	}
	if maxLen == 0 {
		return CombValue(&Comb{
			Order: append([]string(nil), order...),
			Rows:  nil,
		})
	}
	for _, col := range columns {
		if len(col.Values) == 0 {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("table() column '%s' is empty and cannot be broadcast to length %d", col.Name, maxLen),
				nonZeroSpan(col.Span, at),
				"use a non-empty column or make all table columns empty",
			)
			return Null()
		}
		if len(col.Values) != maxLen && maxLen%len(col.Values) != 0 {
			diags.AddWarning(
				diag.CodeW101,
				fmt.Sprintf("length mismatch in table(): column '%s' has length %d; cyclic broadcast to length %d", col.Name, len(col.Values), maxLen),
				nonZeroSpan(col.Span, at),
				"align column lengths to avoid cyclic broadcast",
			)
		}
	}
	rows := make([]Row, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		values := make(map[string]Cell, len(columns))
		for _, col := range columns {
			values[col.Name] = Cell{
				Value:  CloneValue(col.Values[i%len(col.Values)]),
				Origin: nonZeroSpan(col.Span, at),
			}
		}
		rows = append(rows, Row{Values: values})
	}
	return CombValue(&Comb{Order: order, Rows: rows})
}

func hasPositionalValueArg(args []CallValueArg) bool {
	for _, arg := range args {
		if arg.Name == "" {
			return true
		}
	}
	return false
}

func firstPositionalValueSpan(args []CallValueArg) diag.Span {
	for _, arg := range args {
		if arg.Name == "" {
			return arg.Span
		}
	}
	return diag.Span{}
}

func nonZeroSpan(primary, fallback diag.Span) diag.Span {
	if !primary.IsZero() {
		return primary
	}
	return fallback
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
	if ctx.recursionLimitHit() {
		return Null()
	}
	if !IsComb(tableValue) {
		diags.AddError(diag.CodeE106, "select() first argument must be a table value", rawArgs[0].Span, "pass a table built by table(), rename(), or read_csv()")
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

func evalRowsValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	bound, ok := bindBuiltinArgs("rows", args, builtinSignature{Name: "rows", Params: []builtinParam{{Name: "table", Required: true}}}, at, diags)
	if !ok {
		return Null()
	}
	tableArg := bound.ByName["table"]
	if !IsComb(tableArg.Value) {
		diags.AddError(diag.CodeE106, "rows() argument must be a table value", tableArg.Span, "pass a table built by table(), rename(), or read_csv()")
		return Null()
	}
	return rowsFromTable(tableArg.Value, tableArg.Span, diags)
}

func rowsFromTable(tableValue Value, at diag.Span, diags *diag.Diagnostics) Value {
	names := CombNames(tableValue)
	out := make([]Value, 0, len(tableValue.C.Rows))

	for _, row := range tableValue.C.Rows {
		rowDict, ok := dictFromTableRow("rows", names, row, at, diags)
		if !ok {
			return Null()
		}
		out = append(out, rowDict)
	}

	return RowList(out, names)
}

func dictFromTableRow(caller string, names []string, row Row, at diag.Span, diags *diag.Diagnostics) (Value, bool) {
	entries := make([]DictEntry, 0, len(names))
	for _, name := range names {
		cell, ok := row.Values[name]
		if !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() could not read table column '%s'", caller, name), at, "convert well-formed table values only")
			return Null(), false
		}
		entries = append(entries, DictEntry{
			Key:   DictKey{Kind: DictKeyString, S: name},
			Value: CloneValue(cell.Value),
		})
	}
	return DictValue(entries), true
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
		if ctx.recursionLimitHit() {
			return nil, false
		}
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
	ident, ok := expr.(ast.IdentExpr)
	if !ok || !shellvar.ValidName(ident.Name) {
		return "", false
	}
	return ident.Name, true
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
