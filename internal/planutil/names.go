// small planning helpers for source variable series
//
// define deterministic source variable ordering (`SourceVarNames`),
// compute effective row count across imported variables deterministic
// source variable ordering (`SourceVarNames`), and expand a
// variable's base values to row-count length with cyclic
// indexing/null fill (`ExpandValues`) for downstream import and runtime planning
package planutil

import (
	"maps"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func SourceVarNames(order []string, vars map[string][]eval.Value) []string {
	if len(order) > 0 {
		return slices.Clone(order)
	}
	return slices.Sorted(maps.Keys(vars))
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
