package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type tableRenameEntry struct {
	Old string
	New string
}

func evalRenameCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) != 2 {
		diags.AddError(diag.CodeE106, "rename() expects exactly two positional arguments", at, `use rename(table_value, {"old": "new"})`)
		return Null()
	}
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "rename() does not accept named arguments", arg.Span, `use rename(table_value, {"old": "new"})`)
			return Null()
		}
	}

	tableValue := evalExprWithCtx(rawArgs[0].Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	mapping := evalExprWithCtx(rawArgs[1].Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}

	return renameTableValue(tableValue, mapping, rawArgs[0].Span, rawArgs[1].Span, at, diags)
}

func evalRenameValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 2 {
		diags.AddError(diag.CodeE106, "rename() expects exactly two positional arguments", at, `use rename(table_value, {"old": "new"})`)
		return Null()
	}
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "rename() does not accept named arguments", arg.Span, `use rename(table_value, {"old": "new"})`)
			return Null()
		}
	}
	return renameTableValue(args[0].Value, args[1].Value, args[0].Span, args[1].Span, at, diags)
}

func renameTableValue(tableValue Value, mapping Value, tableSpan, mapSpan, at diag.Span, diags *diag.Diagnostics) Value {
	if !IsComb(tableValue) {
		diags.AddError(diag.CodeE106, "rename() first argument must be a table value", tableSpan, "pass a table built by table(), zip(), product(), select(), rename(), or read_csv()")
		return Null()
	}

	entries, ok := tableRenameEntries(tableValue, mapping, mapSpan, diags)
	if !ok {
		return Null()
	}

	rename := make(map[string]string, len(entries))
	for _, entry := range entries {
		rename[entry.Old] = entry.New
	}

	oldOrder := CombNames(tableValue)
	newOrder := make([]string, 0, len(oldOrder))
	seen := make(map[string]struct{}, len(oldOrder))
	for _, oldName := range oldOrder {
		newName := oldName
		if mapped, ok := rename[oldName]; ok {
			newName = mapped
		}
		if _, exists := seen[newName]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("rename() duplicate output column name '%s'", newName), at, "choose unique replacement column names")
			return Null()
		}
		seen[newName] = struct{}{}
		newOrder = append(newOrder, newName)
	}

	rows := make([]Row, 0, len(tableValue.C.Rows))
	for _, row := range tableValue.C.Rows {
		out := Row{Values: make(map[string]Cell, len(newOrder))}
		for i, oldName := range oldOrder {
			cell, ok := row.Values[oldName]
			if !ok {
				diags.AddError(diag.CodeE106, fmt.Sprintf("rename() could not read table column '%s'", oldName), at, "rename well-formed table values only")
				return Null()
			}
			cell.Value = CloneValue(cell.Value)
			out.Values[newOrder[i]] = cell
		}
		rows = append(rows, out)
	}

	return CombValue(&Comb{Order: newOrder, Rows: rows})
}

func tableRenameEntries(tableValue Value, mapping Value, mapSpan diag.Span, diags *diag.Diagnostics) ([]tableRenameEntry, bool) {
	if mapping.Kind != KindDict || mapping.D == nil {
		diags.AddError(diag.CodeE106, "rename() second argument must be a dictionary", mapSpan, `use a mapping such as {"old": "new"}`)
		return nil, false
	}

	existing := make(map[string]struct{})
	for _, name := range CombNames(tableValue) {
		existing[name] = struct{}{}
	}

	entries := make([]tableRenameEntry, 0, len(mapping.D.Order))
	for _, key := range mapping.D.Order {
		if key.Kind != DictKeyString {
			diags.AddError(diag.CodeE106, "rename() old column names must be strings", mapSpan, `use a mapping such as {"old": "new"}`)
			return nil, false
		}
		oldName := key.S
		if _, ok := existing[oldName]; !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("rename() column '%s' does not exist", oldName), mapSpan, "rename existing table columns only")
			return nil, false
		}

		newValue, ok := mapping.D.Entries[key]
		if !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("rename() missing replacement for column '%s'", oldName), mapSpan, `use a mapping such as {"old": "new"}`)
			return nil, false
		}
		if newValue.Kind != KindString {
			diags.AddError(diag.CodeE106, fmt.Sprintf("rename() new name for column '%s' must be a string", oldName), mapSpan, `use a mapping such as {"old": "new"}`)
			return nil, false
		}
		if !isValidCombColumnName(newValue.S) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("rename() invalid new column name '%s'", newValue.S), mapSpan, "use valid table column names such as x, system_name, or ns.value")
			return nil, false
		}

		entries = append(entries, tableRenameEntry{Old: oldName, New: newValue.S})
	}

	return entries, true
}
