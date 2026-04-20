package eval

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"jbs/internal/diag"
)

type FileAccess struct {
	BaseDir  string
	ReadFile func(string) ([]byte, error)
}

func (f *FileAccess) reader() func(string) ([]byte, error) {
	if f != nil && f.ReadFile != nil {
		return f.ReadFile
	}
	return os.ReadFile
}

func (f *FileAccess) Resolve(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := "."
	if f != nil && strings.TrimSpace(f.BaseDir) != "" {
		base = f.BaseDir
	}
	return filepath.Clean(filepath.Join(base, path))
}

type tableColumnKind string

const (
	tableColumnBool   tableColumnKind = "bool"
	tableColumnInt    tableColumnKind = "int"
	tableColumnFloat  tableColumnKind = "float"
	tableColumnString tableColumnKind = "string"
)

type parsedDelimitedTable struct {
	Header []string
	Rows   [][]string
	Kinds  []tableColumnKind
}

func evalReadCSVCall(args []Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, "read_csv() expects exactly one argument", at, "use read_csv(path)")
		return Null()
	}
	if args[0].Kind != KindString {
		diags.AddError(diag.CodeE106, "read_csv() expects string path", at, `use read_csv("file.csv")`)
		return Null()
	}
	if opts.Files == nil {
		diags.AddError(diag.CodeE106, "read_csv() requires file access context in this evaluation environment", at, "call read_csv() in normal compiled file or REPL contexts")
		return Null()
	}
	resolvedPath := opts.Files.Resolve(args[0].S)
	data, err := opts.Files.reader()(resolvedPath)
	if err != nil {
		diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() could not read '%s'", args[0].S), at, "check the path and file permissions")
		return Null()
	}
	table, ok := parseDelimitedTable(resolvedPath, data, at, diags)
	if !ok {
		return Null()
	}
	return tableToCombValue(table, at, diags)
}

func parseDelimitedTable(path string, data []byte, at diag.Span, diags *diag.Diagnostics) (*parsedDelimitedTable, bool) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.Comma = detectDelimiter(path, data)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err == io.EOF {
		diags.AddError(diag.CodeE106, "read_csv() file must contain a header row", at, "add a header row with valid column names")
		return nil, false
	}
	if err != nil {
		diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() could not parse '%s': %v", path, err), at, "fix malformed quoting or delimiters in the input file")
		return nil, false
	}
	header = normalizeDelimitedHeader(header)
	if !validateDelimitedHeader(header, at, diags) {
		return nil, false
	}

	rows := make([][]string, 0)
	rowNumber := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() could not parse '%s': %v", path, err), at, "fix malformed quoting or delimiters in the input file")
			return nil, false
		}
		rowNumber++
		if len(record) != len(header) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() row %d has %d fields; expected %d", rowNumber, len(record), len(header)), at, "ensure every data row has the same number of fields as the header")
			return nil, false
		}
		rows = append(rows, append([]string(nil), record...))
	}

	return &parsedDelimitedTable{
		Header: header,
		Rows:   rows,
		Kinds:  inferDelimitedColumnKinds(rows, len(header)),
	}, true
}

func detectDelimiter(path string, data []byte) rune {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tsv":
		return '\t'
	case ".csv":
		return ','
	}
	line := firstNonEmptyPhysicalLine(string(data))
	if strings.ContainsRune(line, '\t') && !strings.ContainsRune(line, ',') {
		return '\t'
	}
	return ','
}

func firstNonEmptyPhysicalLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		return line
	}
	return ""
}

func normalizeDelimitedHeader(header []string) []string {
	if len(header) == 0 {
		return nil
	}
	out := append([]string(nil), header...)
	out[0] = strings.TrimPrefix(out[0], "\ufeff")
	return out
}

func validateDelimitedHeader(header []string, at diag.Span, diags *diag.Diagnostics) bool {
	if len(header) == 0 {
		diags.AddError(diag.CodeE106, "read_csv() file must contain a header row", at, "add a header row with valid column names")
		return false
	}
	ok := true
	seen := make(map[string]struct{}, len(header))
	for _, name := range header {
		if name == "" {
			diags.AddError(diag.CodeE106, "read_csv() empty column name is not allowed", at, "use non-empty table column names in the header row")
			ok = false
			continue
		}
		if !isValidCombColumnName(name) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() invalid table column name '%s'", name), at, "use identifier-like names such as x, system_name, or ns.value")
			ok = false
			continue
		}
		if _, exists := seen[name]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() duplicate column name '%s'", name), at, "use unique header names")
			ok = false
			continue
		}
		seen[name] = struct{}{}
	}
	return ok
}

func isValidCombColumnName(name string) bool {
	if name == "" {
		return false
	}
	parts := strings.Split(name, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !isValidIdentifier(part) {
			return false
		}
	}
	return true
}

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !(unicode.IsLetter(r) || r == '_') {
				return false
			}
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return false
		}
	}
	return true
}

func inferDelimitedColumnKinds(rows [][]string, width int) []tableColumnKind {
	kinds := make([]tableColumnKind, width)
	for col := 0; col < width; col++ {
		values := make([]string, 0, len(rows))
		for _, row := range rows {
			if col >= len(row) {
				continue
			}
			values = append(values, row[col])
		}
		kinds[col] = inferDelimitedColumnKind(values)
	}
	return kinds
}

func inferDelimitedColumnKind(values []string) tableColumnKind {
	if len(values) == 0 {
		return tableColumnString
	}
	allBool := true
	allInt := true
	allFloat := true
	for _, raw := range values {
		if _, ok := parseDelimitedBool(raw); !ok {
			allBool = false
		}
		if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
			allInt = false
		}
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			allFloat = false
		}
	}
	switch {
	case allBool:
		return tableColumnBool
	case allInt:
		return tableColumnInt
	case allFloat:
		return tableColumnFloat
	default:
		return tableColumnString
	}
}

func parseDelimitedBool(raw string) (bool, bool) {
	if strings.EqualFold(raw, "true") {
		return true, true
	}
	if strings.EqualFold(raw, "false") {
		return false, true
	}
	return false, false
}

func tableToCombValue(table *parsedDelimitedTable, at diag.Span, diags *diag.Diagnostics) Value {
	if table == nil {
		return Null()
	}
	rows := make([]Row, 0, len(table.Rows))
	for _, rawRow := range table.Rows {
		values := make(map[string]Cell, len(table.Header))
		for col, name := range table.Header {
			value, ok := convertDelimitedValue(rawRow[col], table.Kinds[col])
			if !ok {
				diags.AddError(diag.CodeE106, fmt.Sprintf("read_csv() internal conversion failure for column '%s'", name), at, "check the input values for that column")
				return Null()
			}
			values[name] = Cell{Value: value, Origin: at}
		}
		rows = append(rows, Row{Values: values})
	}
	return CombValue(&Comb{
		Order: append([]string(nil), table.Header...),
		Rows:  rows,
	})
}

func convertDelimitedValue(raw string, kind tableColumnKind) (Value, bool) {
	switch kind {
	case tableColumnBool:
		v, ok := parseDelimitedBool(raw)
		if !ok {
			return Null(), false
		}
		return Bool(v), true
	case tableColumnInt:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Null(), false
		}
		return Int(v), true
	case tableColumnFloat:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return Null(), false
		}
		return Float(v), true
	case tableColumnString:
		return String(raw), true
	default:
		return Null(), false
	}
}
