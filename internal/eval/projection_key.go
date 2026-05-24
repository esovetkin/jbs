package eval

import (
	"strconv"
	"strings"
	"sync/atomic"
)

type ProjectionKey struct {
	Source uint64
	Index  int
}

func (k ProjectionKey) Valid() bool {
	return k.Source != 0 && k.Index >= 0
}

var nextProjectionSource atomic.Uint64

func NewProjectionSource() uint64 {
	return nextProjectionSource.Add(1)
}

const fallbackProjectionSource = ^uint64(0)

func ProjectionFallbackKey(row int) ProjectionKey {
	return ProjectionKey{Source: fallbackProjectionSource, Index: row}
}

func ProjectionTupleKey(keys []ProjectionKey) string {
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(strconv.FormatUint(key.Source, 10))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(key.Index))
		b.WriteByte(';')
	}
	return b.String()
}

func rebaseRowsByOutputRow(order []string, rows []Row) []Row {
	if len(rows) == 0 {
		return nil
	}
	source := NewProjectionSource()
	out := make([]Row, 0, len(rows))
	for rowIndex, row := range rows {
		values := make(map[string]Cell, len(order))
		for _, name := range order {
			cell, ok := row.Values[name]
			if !ok {
				continue
			}
			cell.Value = CloneValue(cell.Value)
			cell.Projection = ProjectionKey{Source: source, Index: rowIndex}
			values[name] = cell
		}
		out = append(out, Row{Values: values})
	}
	return out
}
