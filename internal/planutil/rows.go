// row-grouping utilities for import and runtime planning
package planutil

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"

type RowGroup struct {
	Rep  int
	Rows []int
}

func BuildRowGroups(vars []string, valuesByName map[string][]eval.Value, rowCount int) []RowGroup {
	if rowCount <= 0 {
		return nil
	}
	if len(vars) == 0 {
		return []RowGroup{{Rep: 0, Rows: SequentialIndices(rowCount)}}
	}
	indexByKey := make(map[string]int)
	groups := make([]RowGroup, 0, rowCount)
	for row := 0; row < rowCount; row++ {
		key := tupleKeyAt(vars, valuesByName, row)
		if idx, exists := indexByKey[key]; exists {
			groups[idx].Rows = append(groups[idx].Rows, row)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, RowGroup{Rep: row, Rows: []int{row}})
	}
	return groups
}

func BuildProjectedRowGroups(allowedRows []int, vars []string, projectionsByName map[string][]eval.ProjectionKey, full bool) []RowGroup {
	if len(allowedRows) == 0 {
		return nil
	}
	if full {
		return oneGroupPerAllowedRow(allowedRows)
	}
	if len(vars) == 0 {
		rows := append([]int(nil), allowedRows...)
		return []RowGroup{{Rep: allowedRows[0], Rows: rows}}
	}
	indexByKey := make(map[string]int)
	groups := make([]RowGroup, 0, len(allowedRows))
	for _, row := range allowedRows {
		key := projectionTupleKeyAt(vars, projectionsByName, row)
		if idx, exists := indexByKey[key]; exists {
			groups[idx].Rows = append(groups[idx].Rows, row)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, RowGroup{Rep: row, Rows: []int{row}})
	}
	return groups
}

func oneGroupPerAllowedRow(allowedRows []int) []RowGroup {
	groups := make([]RowGroup, 0, len(allowedRows))
	for _, row := range allowedRows {
		groups = append(groups, RowGroup{Rep: row, Rows: []int{row}})
	}
	return groups
}

func projectionTupleKeyAt(vars []string, projectionsByName map[string][]eval.ProjectionKey, row int) string {
	keys := make([]eval.ProjectionKey, 0, len(vars))
	for _, name := range vars {
		keys = append(keys, projectionKeyAt(projectionsByName[name], row))
	}
	return eval.ProjectionTupleKey(keys)
}

func projectionKeyAt(keys []eval.ProjectionKey, row int) eval.ProjectionKey {
	if row >= 0 && row < len(keys) && keys[row].Valid() {
		return keys[row]
	}
	return eval.ProjectionFallbackKey(row)
}

func tupleKeyAt(vars []string, valuesByName map[string][]eval.Value, row int) string {
	parts := make([]eval.StableNamedValuePart, 0, len(vars))
	for _, name := range vars {
		parts = append(parts, eval.StableNamedValuePart{
			Name:  name,
			Value: rowValue(valuesByName[name], row),
		})
	}
	return eval.StableNamedValueTupleKey(parts)
}

func ExpandProjectionKeys(values []eval.ProjectionKey, rowCount int) []eval.ProjectionKey {
	if rowCount <= 0 || len(values) == 0 {
		return nil
	}
	out := make([]eval.ProjectionKey, rowCount)
	for i := 0; i < rowCount; i++ {
		out[i] = values[i%len(values)]
	}
	return out
}

func rowValue(values []eval.Value, row int) eval.Value {
	if row >= 0 && row < len(values) {
		return values[row]
	}
	return eval.Null()
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
