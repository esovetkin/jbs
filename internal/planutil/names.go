package planutil

import (
	"sort"

	"jbs/internal/eval"
)

func SourceVarNames(order []string, vars map[string][]eval.Value) []string {
	if len(order) > 0 {
		out := make([]string, len(order))
		copy(out, order)
		return out
	}
	if len(vars) == 0 {
		return nil
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func SourceRowCount(order []string, vars map[string][]eval.Value) int {
	rowCount := 0
	for _, name := range SourceVarNames(order, vars) {
		if n := len(vars[name]); n > rowCount {
			rowCount = n
		}
	}
	return rowCount
}

func ExpandValues(base []eval.Value, rowCount int) []eval.Value {
	if rowCount <= 0 {
		return nil
	}
	values := make([]eval.Value, 0, rowCount)
	if len(base) == 0 {
		for i := 0; i < rowCount; i++ {
			values = append(values, eval.Null())
		}
		return values
	}
	for i := 0; i < rowCount; i++ {
		values = append(values, base[i%len(base)])
	}
	return values
}
