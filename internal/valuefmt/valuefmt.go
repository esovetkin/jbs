package valuefmt

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

const (
	DefaultNRow  = 10
	DefaultWidth = 80
)

type Options struct {
	NRow         int
	Width        int
	QuoteStrings bool
}

type formatContext struct {
	Depth  int
	Inline bool
}

func DefaultOptions() Options {
	return Options{NRow: DefaultNRow, Width: DefaultWidth}
}

func ReplValue(v eval.Value) string {
	return ReplValueWithOptions(v, DefaultOptions())
}

func ReplValueWithOptions(v eval.Value, opts Options) string {
	return PrintLineWithOptions([]eval.Value{v}, opts)
}

func PrintLine(values []eval.Value) string {
	return PrintLineWithOptions(values, DefaultOptions())
}

func PrintLineWithOptions(values []eval.Value, opts Options) string {
	parts := make([]string, 0, len(values))
	multiline := false
	opts = normalizeOptions(opts)
	opts.QuoteStrings = true
	for _, value := range values {
		part := formatValue(value, opts, formatContext{})
		if strings.Contains(part, "\n") {
			multiline = true
		}
		parts = append(parts, part)
	}
	if !multiline {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts, "\n")
}

func normalizeOptions(opts Options) Options {
	if opts.NRow < 0 {
		opts.NRow = DefaultNRow
	}
	if opts.Width <= 0 {
		opts.Width = DefaultWidth
	}
	return opts
}

func (c formatContext) child() formatContext {
	return formatContext{Depth: c.Depth + 1, Inline: true}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func formatValue(v eval.Value, opts Options, ctx formatContext) string {
	if ctx.Inline {
		return formatInlineValue(v, opts, ctx)
	}
	switch v.Kind {
	case eval.KindString:
		if opts.QuoteStrings {
			return strconv.Quote(v.S)
		}
		return v.String()
	case eval.KindFloat:
		return formatFloat(v.F)
	case eval.KindList:
		return formatSequence("[", "]", false, v.L, opts, ctx)
	case eval.KindTuple:
		return formatSequence("(", ")", true, v.L, opts, ctx)
	case eval.KindDict:
		return formatDict(v.D, opts, ctx)
	case eval.KindComb:
		return formatTable(v.C, opts)
	default:
		return v.String()
	}
}

func formatInlineValue(v eval.Value, opts Options, ctx formatContext) string {
	switch v.Kind {
	case eval.KindString:
		return strconv.Quote(v.S)
	case eval.KindFloat:
		return formatFloat(v.F)
	case eval.KindList:
		return compactSequence("[", "]", false, v.L, opts, ctx)
	case eval.KindTuple:
		return compactSequence("(", ")", true, v.L, opts, ctx)
	case eval.KindDict:
		return compactDict(v.D, ctx.Depth)
	case eval.KindComb:
		return compactTable(v.C)
	default:
		return v.String()
	}
}

func formatSequence(open, close string, tuple bool, items []eval.Value, opts Options, ctx formatContext) string {
	if len(items) == 0 {
		return open + close
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, formatInlineValue(item, opts, ctx.child()))
	}
	if inline, ok := joinSequenceInline(open, close, tuple, parts, opts.Width); ok {
		return inline
	}
	return joinSequenceRows(open, close, tuple, parts, opts)
}

func joinSequenceInline(open, close string, tuple bool, parts []string, width int) (string, bool) {
	if len(parts) == 0 {
		return open + close, true
	}
	sep := ", "
	body := strings.Join(parts, sep)
	if tuple && len(parts) == 1 {
		body += ","
	}
	out := open + body + close
	return out, runeLen(out) <= width
}

func joinSequenceRows(open, close string, tuple bool, parts []string, opts Options) string {
	rows := make([]string, 0)
	current := open
	indent := strings.Repeat(" ", runeLen(open))
	written := 0
	unlimited := opts.NRow == 0

	for i, part := range parts {
		sep := ""
		if written > 0 {
			sep = ", "
		}
		candidate := current + sep + part
		if written > 0 && runeLen(candidate+close) > opts.Width {
			rows = append(rows, current+",")
			if !unlimited && len(rows) >= opts.NRow {
				return finishTruncatedSequence(rows, close, opts.Width)
			}
			current = indent + part
		} else {
			current = candidate
		}
		written = i + 1
	}

	suffix := close
	if tuple && len(parts) == 1 {
		suffix = "," + close
	}
	rows = append(rows, current+suffix)
	return strings.Join(rows, "\n")
}

func finishTruncatedSequence(rows []string, close string, width int) string {
	if len(rows) == 0 {
		return "..." + close
	}
	last := strings.TrimSuffix(rows[len(rows)-1], ",")
	suffix := ", ..." + close
	for runeLen(last+suffix) > width {
		idx := strings.LastIndex(last, ", ")
		if idx < 0 {
			break
		}
		last = last[:idx]
	}
	rows[len(rows)-1] = last + suffix
	return strings.Join(rows, "\n")
}

func compactSequence(open, close string, tuple bool, items []eval.Value, opts Options, ctx formatContext) string {
	if len(items) == 0 {
		return open + close
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, compactValueDepth(item, ctx.Depth+1))
	}
	return compactSequenceParts(open, close, tuple, parts, opts.Width)
}

func compactSequenceParts(open, close string, tuple bool, parts []string, width int) string {
	out := open
	for i, part := range parts {
		sep := ""
		if i > 0 {
			sep = ", "
		}
		suffix := close
		if tuple && len(parts) == 1 {
			suffix = "," + close
		}
		if i == 0 || runeLen(out+sep+part+suffix) <= width {
			out += sep + part
			continue
		}
		out += sep + "..."
		return out + close
	}
	if tuple && len(parts) == 1 {
		return out + "," + close
	}
	return out + close
}

func formatDict(d *eval.Dict, opts Options, ctx formatContext) string {
	entries := dictEntries(d)
	if len(entries) == 0 {
		return "{}"
	}
	limit := len(entries)
	if opts.NRow > 0 && limit > opts.NRow {
		limit = opts.NRow
	}
	lines := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		prefix := "{"
		if i > 0 {
			prefix = " "
		}
		suffix := ","
		if i == limit-1 && len(entries) <= limit {
			suffix = "}"
		}
		lines = append(lines, prefix+formatDictKey(entries[i].Key)+": "+formatNestedValue(entries[i].Value, opts, ctx.child())+suffix)
	}
	if len(entries) > limit {
		lines = append(lines, " ...}")
	}
	return strings.Join(lines, "\n")
}

func formatNestedValue(v eval.Value, opts Options, ctx formatContext) string {
	if ctx.Depth >= 2 {
		return compactValue(v)
	}
	return formatInlineValue(v, opts, ctx)
}

func compactValue(v eval.Value) string {
	return compactValueDepth(v, 0)
}

func compactValueDepth(v eval.Value, depth int) string {
	if depth >= 3 {
		switch v.Kind {
		case eval.KindList:
			return "[...]"
		case eval.KindTuple:
			return "(...)"
		case eval.KindDict:
			return "{...}"
		case eval.KindComb:
			return compactTable(v.C)
		case eval.KindString:
			return strconv.Quote(v.S)
		case eval.KindFloat:
			return formatFloat(v.F)
		default:
			return v.String()
		}
	}
	switch v.Kind {
	case eval.KindString:
		return strconv.Quote(v.S)
	case eval.KindFloat:
		return formatFloat(v.F)
	case eval.KindList:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, compactValueDepth(item, depth+1))
		}
		return compactSequenceParts("[", "]", false, parts, DefaultWidth)
	case eval.KindTuple:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, compactValueDepth(item, depth+1))
		}
		return compactSequenceParts("(", ")", true, parts, DefaultWidth)
	case eval.KindDict:
		return compactDict(v.D, depth)
	case eval.KindComb:
		return compactTable(v.C)
	default:
		return v.String()
	}
}

type dictEntry struct {
	Key   eval.DictKey
	Value eval.Value
}

func dictEntries(d *eval.Dict) []dictEntry {
	if d == nil || len(d.Entries) == 0 {
		return nil
	}
	entries := make([]dictEntry, 0, len(d.Order))
	for _, key := range d.Order {
		value, ok := d.Entries[key]
		if ok {
			entries = append(entries, dictEntry{Key: key, Value: value})
		}
	}
	return entries
}

func compactDict(d *eval.Dict, depth int) string {
	entries := dictEntries(d)
	if len(entries) == 0 {
		return "{}"
	}
	out := "{"
	for i, entry := range entries {
		sep := ""
		if i > 0 {
			sep = ", "
		}
		part := formatDictKey(entry.Key) + ": " + compactValueDepth(entry.Value, depth+1)
		if i == 0 || runeLen(out+sep+part+"}") <= DefaultWidth {
			out += sep + part
			continue
		}
		out += sep + "..."
		return out + "}"
	}
	return out + "}"
}

func formatDictKey(key eval.DictKey) string {
	switch key.Kind {
	case eval.DictKeyString:
		return strconv.Quote(key.S)
	case eval.DictKeyInt:
		return strconv.FormatInt(key.I, 10)
	case eval.DictKeyBool:
		if key.B {
			return "true"
		}
		return "false"
	default:
		return strconv.Quote("")
	}
}

func formatTable(c *eval.Comb, opts Options) string {
	if c == nil {
		c = &eval.Comb{}
	}
	cols := tableColumns(c)
	dataLimit := len(c.Rows)
	if opts.NRow > 0 {
		allowedData := opts.NRow - 1
		if dataLimit > allowedData {
			dataLimit = allowedData
		}
	}
	rows := make([][]string, 0, dataLimit+1)
	rows = append(rows, cols)
	for i := 0; i < dataLimit; i++ {
		rows = append(rows, formatTableRow(c.Rows[i], cols, opts))
	}
	widths := tableWidths(rows)
	var b strings.Builder
	writeTableRow(&b, rows[0], widths)
	writeTableSeparator(&b, widths)
	for _, row := range rows[1:] {
		writeTableRow(&b, row, widths)
	}
	if dataLimit < len(c.Rows) {
		fmt.Fprintf(&b, "... %d more rows\n", len(c.Rows)-dataLimit)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func compactTable(c *eval.Comb) string {
	if c == nil {
		return "table(rows=0, cols=[])"
	}
	return fmt.Sprintf("table(rows=%d, cols=[%s])", len(c.Rows), strings.Join(tableColumns(c), ", "))
}

func tableColumns(c *eval.Comb) []string {
	if c == nil {
		return nil
	}
	cols := slices.Clone(c.Order)
	if len(cols) > 0 {
		return cols
	}
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
	return cols
}

func formatTableRow(row eval.Row, cols []string, opts Options) []string {
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		cell, ok := row.Values[col]
		if !ok {
			out = append(out, "")
			continue
		}
		out = append(out, tableCellString(cell.Value, opts))
	}
	return out
}

func tableCellString(v eval.Value, opts Options) string {
	if v.Kind == eval.KindString && opts.QuoteStrings {
		return strconv.Quote(v.S)
	}
	if v.Kind == eval.KindFloat {
		return formatFloat(v.F)
	}
	if v.IsScalar() {
		return v.String()
	}
	return compactValue(v)
}

func tableWidths(rows [][]string) []int {
	widths := make([]int, 0)
	for _, row := range rows {
		if len(row) > len(widths) {
			widths = append(widths, make([]int, len(row)-len(widths))...)
		}
		for i, cell := range row {
			if n := runeLen(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

func writeTableRow(b *strings.Builder, cells []string, widths []int) {
	if len(widths) == 0 {
		b.WriteString("| |\n")
		return
	}
	b.WriteString("|")
	for i, cell := range cells {
		b.WriteString(" ")
		b.WriteString(padRight(cell, widths[i]))
		b.WriteString(" |")
	}
	b.WriteByte('\n')
}

func writeTableSeparator(b *strings.Builder, widths []int) {
	if len(widths) == 0 {
		b.WriteString("|-|\n")
		return
	}
	b.WriteString("|")
	for _, width := range widths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteString("|")
	}
	b.WriteByte('\n')
}

func padRight(s string, width int) string {
	if n := runeLen(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
