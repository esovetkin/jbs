package valuefmt

import (
	"slices"
	"strconv"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

const maxPreviewItems = 3

func ReplValue(v eval.Value) string {
	switch v.Kind {
	case eval.KindList:
		return replSequence("[", "]", v.L)
	case eval.KindTuple:
		return replSequence("(", ")", v.L)
	case eval.KindComb:
		return replTable(v.C)
	default:
		return v.String()
	}
}

func PrintLine(values []eval.Value) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, ReplValue(value))
	}
	return strings.Join(parts, " ")
}

func replSequence(open, close string, items []eval.Value) string {
	limit := len(items)
	if limit > maxPreviewItems {
		limit = maxPreviewItems
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, replInlineValue(items[i]))
	}
	if len(items) > limit {
		parts = append(parts, "...")
	}
	return open + strings.Join(parts, ", ") + close
}

func replInlineValue(v eval.Value) string {
	switch v.Kind {
	case eval.KindList:
		return replSequence("[", "]", v.L)
	case eval.KindTuple:
		return replSequence("(", ")", v.L)
	case eval.KindComb:
		return replTable(v.C)
	case eval.KindString:
		return strconv.Quote(v.S)
	default:
		return v.String()
	}
}

func replTable(c *eval.Comb) string {
	if c == nil {
		return "table(rows=0, cols=[], head=[])"
	}
	cols := slices.Clone(c.Order)
	if len(cols) == 0 {
		colSet := make(map[string]struct{})
		for _, row := range c.Rows {
			for name := range row.Values {
				colSet[name] = struct{}{}
			}
		}
		cols = make([]string, 0, len(colSet))
		for name := range colSet {
			cols = append(cols, name)
		}
		slices.Sort(cols)
	}

	headLimit := len(c.Rows)
	if headLimit > maxPreviewItems {
		headLimit = maxPreviewItems
	}
	headRows := make([]string, 0, headLimit+1)
	for i := 0; i < headLimit; i++ {
		row := c.Rows[i]
		cells := make([]string, 0, len(cols))
		for _, col := range cols {
			cell, ok := row.Values[col]
			if !ok {
				continue
			}
			cells = append(cells, col+":"+replInlineValue(cell.Value))
		}
		headRows = append(headRows, "{"+strings.Join(cells, ", ")+"}")
	}
	if len(c.Rows) > headLimit {
		headRows = append(headRows, "...")
	}
	return "table(rows=" + strconv.Itoa(len(c.Rows)) +
		", cols=[" + strings.Join(cols, ", ") +
		"], head=[" + strings.Join(headRows, ", ") + "])"
}
