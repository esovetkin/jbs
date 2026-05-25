package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalUniqueDuplicatedValueCall(name string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	bound, ok := bindBuiltinArgs(name, args, builtinSignature{
		Name: name,
		Params: []builtinParam{
			{Name: "values", Required: true},
		},
	}, at, diags)
	if !ok {
		return Null()
	}

	value := bound.ByName["values"].Value
	switch name {
	case "unique":
		return evalUniqueValue(value, at, diags)
	case "duplicated":
		mask, ok := evalDuplicatedMask(value, at, diags)
		if !ok {
			return Null()
		}
		return boolList(mask)
	default:
		diags.AddError(diag.CodeE199, "unknown uniqueness builtin", at, "use unique() or duplicated()")
		return Null()
	}
}

func evalUniqueValue(value Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch value.Kind {
	case KindList, KindTuple:
		return uniqueSequence(value)
	case KindComb:
		return uniqueTable(value, at, diags)
	default:
		diags.AddError(diag.CodeE106, "unique() expects list/tuple/table as first argument", at, "pass a list, tuple, or table value")
		return Null()
	}
}

func evalDuplicatedMask(value Value, at diag.Span, diags *diag.Diagnostics) ([]bool, bool) {
	switch value.Kind {
	case KindList, KindTuple:
		return duplicatedSequenceMask(value.L), true
	case KindComb:
		return duplicatedTableMask("duplicated", value, at, diags)
	default:
		diags.AddError(diag.CodeE106, "duplicated() expects list/tuple/table as first argument", at, "pass a list, tuple, or table value")
		return nil, false
	}
}

func boolList(mask []bool) Value {
	out := make([]Value, len(mask))
	for i, duplicate := range mask {
		out[i] = Bool(duplicate)
	}
	return List(out)
}

func uniqueSequence(value Value) Value {
	mask := duplicatedSequenceMask(value.L)
	out := make([]Value, 0, len(value.L))
	for i, duplicate := range mask {
		if duplicate {
			continue
		}
		out = append(out, CloneValue(value.L[i]))
	}
	if value.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

func duplicatedSequenceMask(items []Value) []bool {
	seen := make([]Value, 0, len(items))
	mask := make([]bool, len(items))
	for i, item := range items {
		if seenEqualValue(seen, item) {
			mask[i] = true
			continue
		}
		seen = append(seen, CloneValue(item))
	}
	return mask
}

func seenEqualValue(seen []Value, value Value) bool {
	for _, candidate := range seen {
		if Equal(candidate, value) {
			return true
		}
	}
	return false
}

func uniqueTable(value Value, at diag.Span, diags *diag.Diagnostics) Value {
	order := CombNames(value)
	mask, ok := duplicatedTableMask("unique", value, at, diags)
	if !ok {
		return Null()
	}

	rows := make([]Row, 0, len(value.C.Rows))
	for rowIndex, duplicate := range mask {
		if duplicate {
			continue
		}
		row, ok := cloneTableRowPreservingProjection(order, value.C.Rows[rowIndex], at, diags)
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

func duplicatedTableMask(caller string, value Value, at diag.Span, diags *diag.Diagnostics) ([]bool, bool) {
	if !IsComb(value) {
		diags.AddError(diag.CodeE106, caller+"() received a malformed table value", at, "use well-formed table values")
		return nil, false
	}

	order := CombNames(value)
	seen := make([][]Value, 0, len(value.C.Rows))
	mask := make([]bool, len(value.C.Rows))
	for rowIndex, row := range value.C.Rows {
		values, ok := rowValuesForUnique(caller, order, row, at, diags)
		if !ok {
			return nil, false
		}
		if seenEqualTuple(seen, values) {
			mask[rowIndex] = true
			continue
		}
		seen = append(seen, CloneValues(values))
	}
	return mask, true
}

func rowValuesForUnique(caller string, order []string, row Row, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	values := make([]Value, 0, len(order))
	for _, name := range order {
		cell, ok := row.Values[name]
		if !ok {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("%s() could not read table column '%s'", caller, name),
				at,
				"use well-formed table values",
			)
			return nil, false
		}
		values = append(values, cell.Value)
	}
	return values, true
}

func seenEqualTuple(seen [][]Value, values []Value) bool {
	for _, candidate := range seen {
		if equalValueTuple(candidate, values) {
			return true
		}
	}
	return false
}

func equalValueTuple(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
