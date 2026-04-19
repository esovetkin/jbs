package eval

import (
	"fmt"
	"strings"
)

func IsComb(v Value) bool {
	return v.Kind == KindComb && v.C != nil
}

func CombRowCount(v Value) int {
	if !IsComb(v) {
		return 0
	}
	return len(v.C.Rows)
}

func CombColumn(v Value, name string) ([]Value, bool) {
	if !IsComb(v) || name == "" {
		return nil, false
	}
	if !containsCombColumn(v.C.Order, name) {
		return nil, false
	}
	out := make([]Value, 0, len(v.C.Rows))
	for _, row := range v.C.Rows {
		cell, ok := row.Values[name]
		if !ok {
			return nil, false
		}
		out = append(out, cell.Value)
	}
	return out, true
}

func CombProject(v Value, cols []string) (Value, bool) {
	if !IsComb(v) || len(cols) == 0 {
		return Null(), false
	}
	seenCols := make(map[string]struct{}, len(cols))
	order := make([]string, 0, len(cols))
	for _, col := range cols {
		if col == "" {
			return Null(), false
		}
		if _, exists := seenCols[col]; exists {
			continue
		}
		if !containsCombColumn(v.C.Order, col) {
			return Null(), false
		}
		seenCols[col] = struct{}{}
		order = append(order, col)
	}
	if len(order) == 0 {
		return Null(), false
	}
	outRows := make([]Row, 0, len(v.C.Rows))
	seenKeys := make(map[string]struct{}, len(v.C.Rows))
	for _, row := range v.C.Rows {
		projected := Row{Values: make(map[string]Cell, len(order))}
		keyParts := make([]string, 0, len(order))
		for _, col := range order {
			cell, ok := row.Values[col]
			if !ok {
				return Null(), false
			}
			projected.Values[col] = cell
			keyParts = append(keyParts, valueKey(cell.Value))
		}
		key := strings.Join(keyParts, "\x1f")
		if _, exists := seenKeys[key]; exists {
			continue
		}
		seenKeys[key] = struct{}{}
		outRows = append(outRows, projected)
	}
	return CombValue(&Comb{
		Order: append([]string(nil), order...),
		Rows:  outRows,
	}), true
}

func CombNames(v Value) []string {
	if !IsComb(v) {
		return nil
	}
	if len(v.C.Order) > 0 {
		return uniqueStringsPreserveOrder(v.C.Order)
	}
	return RowVariableNames(v.C.Rows)
}

func containsCombColumn(order []string, name string) bool {
	for _, col := range order {
		if col == name {
			return true
		}
	}
	return false
}

func valueKey(v Value) string {
	switch v.Kind {
	case KindNull:
		return "n:"
	case KindInt:
		return fmt.Sprintf("i:%d", v.I)
	case KindFloat:
		return fmt.Sprintf("f:%g", v.F)
	case KindString:
		return "s:" + v.S
	case KindBool:
		if v.B {
			return "b:1"
		}
		return "b:0"
	case KindList, KindTuple:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, valueKey(item))
		}
		prefix := "l:"
		if v.Kind == KindTuple {
			prefix = "t:"
		}
		return prefix + strings.Join(parts, ",")
	case KindComb:
		if v.C == nil {
			return "c:nil"
		}
		return fmt.Sprintf("c:%d:%d", len(v.C.Order), len(v.C.Rows))
	default:
		return "u:"
	}
}
