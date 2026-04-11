// render `printparam` tables
//
// expose `Render` dispatch for pretty-table and CSV output, compute column
// widths/alignment for pretty rendering
package printparam

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

func Render(t Table, rt RenderType) (string, error) {
	switch rt {
	case RenderPretty:
		return renderPretty(t), nil
	case RenderCSV:
		return renderCSV(t)
	default:
		return "", fmt.Errorf("unknown render type %q", rt)
	}
}

func renderPretty(t Table) string {
	headers := append([]string{}, t.Columns...)
	headers = append(headers, "step")
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range t.Rows {
		for i, col := range t.Columns {
			if n := len(row.Values[col]); n > widths[i] {
				widths[i] = n
			}
		}
		if n := len(stepLabel(row)); n > widths[len(widths)-1] {
			widths[len(widths)-1] = n
		}
	}

	var b strings.Builder
	writePrettyRow(&b, headers, widths)
	writePrettySeparator(&b, widths)
	for _, row := range t.Rows {
		cells := make([]string, 0, len(headers))
		for _, col := range t.Columns {
			cells = append(cells, row.Values[col])
		}
		cells = append(cells, stepLabel(row))
		writePrettyRow(&b, cells, widths)
	}
	return b.String()
}

func writePrettyRow(b *strings.Builder, cells []string, widths []int) {
	b.WriteString("|")
	for i, cell := range cells {
		b.WriteString(" ")
		b.WriteString(padRight(cell, widths[i]))
		b.WriteString(" |")
	}
	b.WriteByte('\n')
}

func writePrettySeparator(b *strings.Builder, widths []int) {
	b.WriteString("|")
	for _, width := range widths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteString("|")
	}
	b.WriteByte('\n')
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func renderCSV(t Table) (string, error) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	headers := append([]string{}, t.Columns...)
	headers = append(headers, "step")
	if err := w.Write(headers); err != nil {
		return "", err
	}
	for _, row := range t.Rows {
		record := make([]string, 0, len(headers))
		for _, col := range t.Columns {
			record = append(record, row.Values[col])
		}
		record = append(record, stepLabel(row))
		if err := w.Write(record); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func stepLabel(row Row) string {
	kind := row.StepKind
	if kind == "" {
		kind = "step"
	}
	return kind + ": " + row.StepName
}
