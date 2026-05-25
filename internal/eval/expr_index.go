package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type sequenceSelectorKind int

const (
	sequenceSelectorEmpty sequenceSelectorKind = iota
	sequenceSelectorInts
	sequenceSelectorBools
)

type tableRowSelectorKind int

const (
	tableRowSelectorEmpty tableRowSelectorKind = iota
	tableRowSelectorInts
	tableRowSelectorBools
)

func evalSequenceIndex(base Value, items []ast.Expr, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(items) != 1 {
		diags.AddError(diag.CodeE106, "sequence index expects exactly one selector", at, "use value[index], value[[0, -1]], or value[[true, false, ...]]")
		return Null()
	}
	selector := evalExprWithCtx(items[0], env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	switch selector.Kind {
	case KindInt:
		return evalSequenceIntegerIndex(base, selector.I, items[0].GetSpan(), diags)
	case KindList, KindTuple:
		return evalSequenceListSelector(base, selector, items[0].GetSpan(), diags)
	default:
		diags.AddError(diag.CodeE106, "sequence index must be an integer, integer selector, or boolean mask", items[0].GetSpan(), "use value[0], value[[0, -1]], or value[[true, false, ...]]")
		return Null()
	}
}

func normalizeSequenceIndex(n int, raw int64, at diag.Span, diags *diag.Diagnostics) (int, bool) {
	idx := raw
	if idx < 0 {
		idx = int64(n) + idx
	}
	if idx < 0 || idx >= int64(n) {
		diags.AddError(diag.CodeE106, "sequence index out of range", at, "use an index in range -len(value) <= index < len(value)")
		return 0, false
	}
	return int(idx), true
}

func evalSequenceIntegerIndex(base Value, raw int64, at diag.Span, diags *diag.Diagnostics) Value {
	idx, ok := normalizeSequenceIndex(len(base.L), raw, at, diags)
	if !ok {
		return Null()
	}
	return CloneValue(base.L[idx])
}

func classifySequenceSelector(selector Value, at diag.Span, diags *diag.Diagnostics) (sequenceSelectorKind, bool) {
	if len(selector.L) == 0 {
		return sequenceSelectorEmpty, true
	}
	first := selector.L[0].Kind
	switch first {
	case KindInt:
		for _, item := range selector.L {
			if item.Kind != KindInt {
				diags.AddError(diag.CodeE106, "sequence selector cannot mix integer indexes and boolean mask values", at, "use all integers or all booleans")
				return sequenceSelectorEmpty, false
			}
		}
		return sequenceSelectorInts, true
	case KindBool:
		for _, item := range selector.L {
			if item.Kind != KindBool {
				diags.AddError(diag.CodeE106, "sequence selector cannot mix integer indexes and boolean mask values", at, "use all integers or all booleans")
				return sequenceSelectorEmpty, false
			}
		}
		return sequenceSelectorBools, true
	default:
		diags.AddError(diag.CodeE106, "sequence selector must contain only integers or only booleans", at, "use value[[0, -1]] or value[[true, false, ...]]")
		return sequenceSelectorEmpty, false
	}
}

func evalSequenceListSelector(base Value, selector Value, at diag.Span, diags *diag.Diagnostics) Value {
	kind, ok := classifySequenceSelector(selector, at, diags)
	if !ok {
		return Null()
	}
	switch kind {
	case sequenceSelectorEmpty:
		return emptySequenceResult(base)
	case sequenceSelectorInts:
		return evalSequenceGatherIndex(base, selector, at, diags)
	case sequenceSelectorBools:
		return evalSequenceMaskIndex(base, selector, at, diags)
	default:
		return Null()
	}
}

func evalSequenceGatherIndex(base Value, selector Value, at diag.Span, diags *diag.Diagnostics) Value {
	out := make([]Value, 0, len(selector.L))
	for _, item := range selector.L {
		idx, ok := normalizeSequenceIndex(len(base.L), item.I, at, diags)
		if !ok {
			return Null()
		}
		out = append(out, CloneValue(base.L[idx]))
	}
	return sequenceResult(base, out)
}

func evalSequenceMaskIndex(base Value, selector Value, at diag.Span, diags *diag.Diagnostics) Value {
	bools := make([]bool, len(selector.L))
	for i, item := range selector.L {
		bools[i] = item.B
	}
	if len(base.L)%len(bools) != 0 {
		diags.AddWarning(diag.CodeW101, fmt.Sprintf("length mismatch in sequence mask: values=%d mask=%d; cyclic broadcast to length %d", len(base.L), len(bools), len(base.L)), at, "align mask length with indexed value length")
	}
	out := make([]Value, 0, len(base.L))
	for i, item := range base.L {
		if bools[i%len(bools)] {
			out = append(out, CloneValue(item))
		}
	}
	return sequenceResult(base, out)
}

func sequenceResult(base Value, out []Value) Value {
	if base.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

func emptySequenceResult(base Value) Value {
	return sequenceResult(base, nil)
}

func evalCombIndex(base Value, items []ast.Expr, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(items) == 0 {
		diags.AddError(diag.CodeE106, "table index selectors cannot be empty", at, `use table_value["col"] or table_value[[0, 2]]`)
		return Null()
	}

	if len(items) == 1 {
		selector := evalExprWithCtx(items[0], env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		switch selector.Kind {
		case KindString:
			return evalCombColumnIndex(base, []string{selector.S}, at, diags)
		case KindList:
			return evalCombRowIndex(base, selector, items[0].GetSpan(), diags)
		default:
			diags.AddError(diag.CodeE106, "table index selector must be a string column name or list row selector", items[0].GetSpan(), `use table_value["col"] or table_value[[0, 2]]`)
			return Null()
		}
	}

	selectors := make([]string, 0, len(items))
	for _, item := range items {
		value := evalExprWithCtx(item, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		if value.Kind != KindString {
			diags.AddError(diag.CodeE106, "multi-column table index selectors must evaluate to strings", item.GetSpan(), `use table_value["x", "y"]; use table_value[[0, 2]] for row indexing`)
			return Null()
		}
		selectors = append(selectors, value.S)
	}
	return evalCombColumnIndex(base, selectors, at, diags)
}

func evalCombColumnIndex(base Value, selectors []string, at diag.Span, diags *diag.Diagnostics) Value {
	projected, ok := CombProject(base, selectors)
	if !ok {
		diags.AddError(diag.CodeE106, "invalid table projection selector", at, "select existing table columns only")
		return Null()
	}
	return projected
}

func evalCombRowIndex(base Value, selector Value, selectorSpan diag.Span, diags *diag.Diagnostics) Value {
	kind, ok := classifyTableRowSelector(selector, selectorSpan, diags)
	if !ok {
		return Null()
	}
	switch kind {
	case tableRowSelectorEmpty:
		return CombValue(&Comb{Order: append([]string(nil), base.C.Order...), Rows: nil})
	case tableRowSelectorInts:
		return evalCombGatherRows(base, selector, selectorSpan, diags)
	case tableRowSelectorBools:
		return evalCombMaskRows(base, selector, selectorSpan, diags)
	default:
		return Null()
	}
}

func classifyTableRowSelector(selector Value, at diag.Span, diags *diag.Diagnostics) (tableRowSelectorKind, bool) {
	if selector.Kind != KindList {
		diags.AddError(diag.CodeE106, "table row selector must be a list", at, `use table_value[[0, 2]] or table_value[[true, false]]`)
		return tableRowSelectorEmpty, false
	}
	if len(selector.L) == 0 {
		return tableRowSelectorEmpty, true
	}
	first := selector.L[0].Kind
	switch first {
	case KindInt:
		for _, item := range selector.L {
			if item.Kind != KindInt {
				diags.AddError(diag.CodeE106, "table row selector cannot mix integer indexes and boolean mask values", at, "use all integers or all booleans")
				return tableRowSelectorEmpty, false
			}
		}
		return tableRowSelectorInts, true
	case KindBool:
		for _, item := range selector.L {
			if item.Kind != KindBool {
				diags.AddError(diag.CodeE106, "table row selector cannot mix integer indexes and boolean mask values", at, "use all integers or all booleans")
				return tableRowSelectorEmpty, false
			}
		}
		return tableRowSelectorBools, true
	default:
		diags.AddError(diag.CodeE106, "table row selector must contain only integers or only booleans", at, `use table_value[[0, 2]] or table_value[[true, false]]`)
		return tableRowSelectorEmpty, false
	}
}

func evalCombGatherRows(base Value, selector Value, at diag.Span, diags *diag.Diagnostics) Value {
	order := CombNames(base)
	rows := make([]Row, 0, len(selector.L))
	for _, item := range selector.L {
		idx, ok := normalizeTableRowIndex(len(base.C.Rows), item.I, at, diags)
		if !ok {
			return Null()
		}
		row, ok := cloneTableRowPreservingProjection(order, base.C.Rows[idx], at, diags)
		if !ok {
			return Null()
		}
		rows = append(rows, row)
	}
	return CombValue(&Comb{
		Order: append([]string(nil), order...),
		Rows:  rows,
	})
}

func evalCombMaskRows(base Value, selector Value, at diag.Span, diags *diag.Diagnostics) Value {
	order := CombNames(base)
	if len(selector.L) == 0 {
		return CombValue(&Comb{Order: append([]string(nil), order...), Rows: nil})
	}
	if len(base.C.Rows)%len(selector.L) != 0 {
		diags.AddWarning(
			diag.CodeW101,
			fmt.Sprintf("length mismatch in table row mask: rows=%d mask=%d; cyclic broadcast to length %d", len(base.C.Rows), len(selector.L), len(base.C.Rows)),
			at,
			"align mask length with table row count",
		)
	}

	rows := make([]Row, 0, len(base.C.Rows))
	for rowIndex, row := range base.C.Rows {
		if !selector.L[rowIndex%len(selector.L)].B {
			continue
		}
		cloned, ok := cloneTableRowPreservingProjection(order, row, at, diags)
		if !ok {
			return Null()
		}
		rows = append(rows, cloned)
	}
	return CombValue(&Comb{Order: append([]string(nil), order...), Rows: rows})
}

func normalizeTableRowIndex(n int, raw int64, at diag.Span, diags *diag.Diagnostics) (int, bool) {
	if raw < 0 || raw >= int64(n) {
		diags.AddError(diag.CodeE106, "table row index out of range", at, "use an index in range 0 <= index < nrow(table)")
		return 0, false
	}
	return int(raw), true
}

func cloneTableRowPreservingProjection(order []string, row Row, at diag.Span, diags *diag.Diagnostics) (Row, bool) {
	out := Row{Values: make(map[string]Cell, len(order))}
	for _, name := range order {
		cell, ok := row.Values[name]
		if !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("table row index could not read table column '%s'", name), at, "use well-formed table values")
			return Row{}, false
		}
		cell.Value = CloneValue(cell.Value)
		out.Values[name] = cell
	}
	return out, true
}
