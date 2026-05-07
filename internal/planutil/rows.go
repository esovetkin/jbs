// row-grouping utilities for import and runtime planning
//
// group row indices by identical tuples of variable values
// (`BuildRowGroups`), using a stable tuple-key encoding
// (`tupleKeyAt`), provide sequential index generation
// (`SequentialIndices`) used for row-representative/index helper
// logic.
package planutil

import (
	"strconv"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type RowGroup struct {
	Rep  int
	Rows []int
}

type ValueKeyFunc func(eval.Value) string

func BuildRowGroups(vars []string, valuesByName map[string][]eval.Value, rowCount int, keyFn ValueKeyFunc) []RowGroup {
	if rowCount <= 0 {
		return nil
	}
	if len(vars) == 0 {
		return []RowGroup{{Rep: 0, Rows: SequentialIndices(rowCount)}}
	}
	if keyFn == nil {
		keyFn = func(v eval.Value) string {
			return v.String()
		}
	}
	indexByKey := make(map[string]int)
	groups := make([]RowGroup, 0, rowCount)
	for row := 0; row < rowCount; row++ {
		key := tupleKeyAt(vars, valuesByName, row, keyFn)
		if idx, exists := indexByKey[key]; exists {
			groups[idx].Rows = append(groups[idx].Rows, row)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, RowGroup{Rep: row, Rows: []int{row}})
	}
	return groups
}

// BuildProjectedRowGroups regroups only the allowed rows; full imports keep one group per raw row.
func BuildProjectedRowGroups(allowedRows []int, vars []string, valuesByName map[string][]eval.Value, full bool, keyFn ValueKeyFunc) []RowGroup {
	if len(allowedRows) == 0 {
		return nil
	}
	if full {
		groups := make([]RowGroup, 0, len(allowedRows))
		for _, row := range allowedRows {
			groups = append(groups, RowGroup{Rep: row, Rows: []int{row}})
		}
		return groups
	}
	if len(vars) == 0 {
		rows := append([]int(nil), allowedRows...)
		return []RowGroup{{Rep: allowedRows[0], Rows: rows}}
	}
	if keyFn == nil {
		keyFn = func(v eval.Value) string {
			return v.String()
		}
	}
	indexByKey := make(map[string]int)
	groups := make([]RowGroup, 0, len(allowedRows))
	for _, row := range allowedRows {
		key := tupleKeyAt(vars, valuesByName, row, keyFn)
		if idx, exists := indexByKey[key]; exists {
			groups[idx].Rows = append(groups[idx].Rows, row)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, RowGroup{Rep: row, Rows: []int{row}})
	}
	return groups
}

func tupleKeyAt(vars []string, valuesByName map[string][]eval.Value, row int, keyFn ValueKeyFunc) string {
	var b strings.Builder
	for _, name := range vars {
		values := valuesByName[name]
		value := eval.Null()
		if row >= 0 && row < len(values) {
			value = values[row]
		}
		lit := keyFn(value)
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(strconv.Itoa(len(lit)))
		b.WriteByte(':')
		b.WriteString(lit)
		b.WriteByte('|')
	}
	return b.String()
}

func SequentialIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = i
	}
	return out
}
