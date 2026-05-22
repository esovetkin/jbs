package eval

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func readCSVCallExpr(span diag.Span, args ...ast.Expr) ast.CallExpr {
	return ast.CallExpr{
		Callee: ast.IdentExpr{Name: "read_csv", Span: span},
		Args:   ast.PosCallArgs(args...),
		Span:   span,
	}
}

func TestInferDelimitedColumnKind(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   tableColumnKind
	}{
		{name: "empty rows default string", values: nil, want: tableColumnString},
		{name: "bool", values: []string{"true", "FALSE"}, want: tableColumnBool},
		{name: "int", values: []string{"1", "-2", "3"}, want: tableColumnInt},
		{name: "float", values: []string{"1", "2.5", "3e1"}, want: tableColumnFloat},
		{name: "mixed bool int becomes string", values: []string{"true", "1"}, want: tableColumnString},
		{name: "empty field becomes string", values: []string{"", "3"}, want: tableColumnString},
		{name: "whitespace numeric becomes string", values: []string{" 1", "2"}, want: tableColumnString},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferDelimitedColumnKind(tc.values); got != tc.want {
				t.Fatalf("inferDelimitedColumnKind(%#v)=%q want %q", tc.values, got, tc.want)
			}
		})
	}
}

func TestDetectDelimiterAndFirstNonEmptyPhysicalLine(t *testing.T) {
	delimiterTests := []struct {
		name string
		path string
		data string
		want rune
	}{
		{name: "tsv extension", path: "cases.tsv", data: "x,y\n1,2\n", want: '\t'},
		{name: "csv extension", path: "cases.csv", data: "x\ty\n1\t2\n", want: ','},
		{name: "no extension tsv sniff", path: "cases", data: "\r\n\nx\ty\n1\t2\n", want: '\t'},
		{name: "no extension csv fallback", path: "cases", data: "\r\n\nx,y\n1,2\n", want: ','},
		{name: "blank data fallback", path: "cases", data: "\n\r\n", want: ','},
	}
	for _, tc := range delimiterTests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectDelimiter(tc.path, []byte(tc.data)); got != tc.want {
				t.Fatalf("detectDelimiter()=%q want %q", got, tc.want)
			}
		})
	}

	lineTests := []struct {
		name string
		text string
		want string
	}{
		{name: "all blank", text: "\n\r\n", want: ""},
		{name: "skips blank CRLF", text: "\r\n\nheader,value\r\n1,2\r\n", want: "header,value"},
		{name: "comment like line is physical line", text: "\n# not a comment\nx,y\n", want: "# not a comment"},
		{name: "keeps spaces", text: "\n  x,y\n", want: "  x,y"},
	}
	for _, tc := range lineTests {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstNonEmptyPhysicalLine(tc.text); got != tc.want {
				t.Fatalf("firstNonEmptyPhysicalLine()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestIsValidCombColumnName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "simple", in: "x", want: true},
		{name: "underscored", in: "system_name", want: true},
		{name: "underscore leading", in: "_tmp", want: true},
		{name: "digit suffix", in: "x1", want: true},
		{name: "qualified", in: "ns.value", want: false},
		{name: "nested qualified", in: "outer.inner.value", want: false},
		{name: "empty", in: "", want: false},
		{name: "leading digit", in: "1x", want: false},
		{name: "dash", in: "x-y", want: false},
		{name: "space", in: "x y", want: false},
		{name: "leading dot", in: ".ns", want: false},
		{name: "trailing dot", in: "ns.", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidCombColumnName(tc.in); got != tc.want {
				t.Fatalf("isValidCombColumnName(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseDelimitedTableDirectCases(t *testing.T) {
	span := spanAt(1203, 1)

	tests := []struct {
		name      string
		path      string
		data      []byte
		wantOK    bool
		wantHead  []string
		wantRows  int
		wantKinds []tableColumnKind
	}{
		{
			name:   "empty file",
			path:   "empty.csv",
			data:   nil,
			wantOK: false,
		},
		{
			name:   "malformed quote",
			path:   "bad.csv",
			data:   []byte("x,y\n\"unterminated,2\n"),
			wantOK: false,
		},
		{
			name:   "malformed header quote",
			path:   "bad-header.csv",
			data:   []byte("\"unterminated\n"),
			wantOK: false,
		},
		{
			name:   "row width mismatch",
			path:   "width.csv",
			data:   []byte("x,y\n1\n"),
			wantOK: false,
		},
		{
			name:   "invalid header",
			path:   "invalid.csv",
			data:   []byte("bad-name,y\n1,2\n"),
			wantOK: false,
		},
		{
			name:      "bom header",
			path:      "bom.csv",
			data:      []byte("\ufeffx,y\n1,2\n"),
			wantOK:    true,
			wantHead:  []string{"x", "y"},
			wantRows:  1,
			wantKinds: []tableColumnKind{tableColumnInt, tableColumnInt},
		},
		{
			name:      "sniffed tsv without extension",
			path:      "cases",
			data:      []byte("\r\nx\ty\n1\ttrue\n"),
			wantOK:    true,
			wantHead:  []string{"x", "y"},
			wantRows:  1,
			wantKinds: []tableColumnKind{tableColumnInt, tableColumnBool},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got, ok := parseDelimitedTable(tc.path, tc.data, span, diags)
			if ok != tc.wantOK {
				t.Fatalf("parseDelimitedTable ok=%v want %v; table=%#v diags=%s", ok, tc.wantOK, got, diags.String())
			}
			if !tc.wantOK {
				if diagCount(diags, "E106") == 0 {
					t.Fatalf("expected E106, got %s", diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !reflect.DeepEqual(got.Header, tc.wantHead) || len(got.Rows) != tc.wantRows || !reflect.DeepEqual(got.Kinds, tc.wantKinds) {
				t.Fatalf("unexpected parsed table: %#v", got)
			}
		})
	}
}

func TestConvertDelimitedValueFailuresAndSpecialFloats(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		kind tableColumnKind
	}{
		{name: "invalid bool", raw: "yes", kind: tableColumnBool},
		{name: "invalid int", raw: "1.5", kind: tableColumnInt},
		{name: "invalid float", raw: "abc", kind: tableColumnFloat},
		{name: "nan float", raw: "NaN", kind: tableColumnFloat},
		{name: "inf float", raw: "+Inf", kind: tableColumnFloat},
		{name: "unknown kind", raw: "x", kind: tableColumnKind("unknown")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := convertDelimitedValue(tc.raw, tc.kind); ok || got.Kind != KindNull {
				t.Fatalf("expected failed conversion, got value=%#v ok=%v", got, ok)
			}
		})
	}

	got, ok := convertDelimitedValue("2.5", tableColumnFloat)
	if !ok || got.Kind != KindFloat || got.F != 2.5 {
		t.Fatalf("expected successful float conversion, got value=%#v ok=%v", got, ok)
	}
}

func TestEvalReadCSVCallCSVInferenceAndQuotedFields(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "cases.csv")
	data := "name,count,ratio,flag,comment\n" +
		"alice,1,2.5,true,\"hello, world\"\n" +
		"bob,2,3.5,false,plain text\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	span := spanAt(1200, 1)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(readCSVCallExpr(span, ast.StringExpr{Value: "./cases.csv", Span: span}), nil, diags, ExprOptions{
		Context: EvalCtxBindingAssign,
		Files:   &FileAccess{BaseDir: cwd},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) {
		t.Fatalf("expected comb result, got %#v", got)
	}
	if !reflect.DeepEqual(got.C.Order, []string{"name", "count", "ratio", "flag", "comment"}) {
		t.Fatalf("unexpected header order: %#v", got.C.Order)
	}
	if len(got.C.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %#v", got.C.Rows)
	}
	if cell := got.C.Rows[0].Values["name"].Value; cell.Kind != KindString || cell.S != "alice" {
		t.Fatalf("unexpected first name cell: %#v", cell)
	}
	if cell := got.C.Rows[0].Values["count"].Value; cell.Kind != KindInt || cell.I != 1 {
		t.Fatalf("unexpected first count cell: %#v", cell)
	}
	if cell := got.C.Rows[0].Values["ratio"].Value; cell.Kind != KindFloat || cell.F != 2.5 {
		t.Fatalf("unexpected first ratio cell: %#v", cell)
	}
	if cell := got.C.Rows[0].Values["flag"].Value; cell.Kind != KindBool || !cell.B {
		t.Fatalf("unexpected first flag cell: %#v", cell)
	}
	if cell := got.C.Rows[0].Values["comment"].Value; cell.Kind != KindString || cell.S != "hello, world" {
		t.Fatalf("unexpected first comment cell: %#v", cell)
	}

	diags = &diag.Diagnostics{}
	named := EvalExprWithOptions(callExpr(ident("read_csv"), namedArg("path", ast.StringExpr{Value: "./cases.csv", Span: span})), nil, diags, ExprOptions{
		Context: EvalCtxBindingAssign,
		Files:   &FileAccess{BaseDir: cwd},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected named read_csv diagnostics: %s", diags.String())
	}
	if !Equal(named, got) {
		t.Fatalf("named read_csv result differs: got=%#v want=%#v", named, got)
	}
}

func TestEvalReadCSVCallTSVAndEmptyFieldStringFallback(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "cases.tsv")
	data := "x\tlabel\n" +
		"1\t\"a\tb\"\n" +
		"2\t\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write tsv: %v", err)
	}

	span := spanAt(1201, 1)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(readCSVCallExpr(span, ast.StringExpr{Value: "./cases.tsv", Span: span}), nil, diags, ExprOptions{
		Context: EvalCtxBindingAssign,
		Files:   &FileAccess{BaseDir: cwd},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) {
		t.Fatalf("expected comb result, got %#v", got)
	}
	if !reflect.DeepEqual(got.C.Order, []string{"x", "label"}) {
		t.Fatalf("unexpected tsv order: %#v", got.C.Order)
	}
	if first := got.C.Rows[0].Values["x"].Value; first.Kind != KindInt || first.I != 1 {
		t.Fatalf("unexpected int x cell: %#v", first)
	}
	if first := got.C.Rows[0].Values["label"].Value; first.Kind != KindString || first.S != "a\tb" {
		t.Fatalf("unexpected quoted label cell: %#v", first)
	}
	if second := got.C.Rows[1].Values["label"].Value; second.Kind != KindString || second.S != "" {
		t.Fatalf("expected empty field to stay string, got %#v", second)
	}
}

func TestEvalReadCSVCallErrors(t *testing.T) {
	cwd := t.TempDir()
	writeFile := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cwd, name), []byte(data), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile("dup.csv", "x,x\n1,2\n")
	writeFile("invalid.csv", "x-y,z\n1,2\n")
	writeFile("dotted.csv", "a.b,x\n1,2\n")
	writeFile("empty.csv", ",z\n1,2\n")
	writeFile("width.csv", "x,y\n1,2\n3\n")

	span := spanAt(1202, 1)
	tests := []struct {
		name     string
		expr     ast.Expr
		opts     ExprOptions
		wantCode string
	}{
		{
			name:     "missing file context",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./cases.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign},
			wantCode: "E106",
		},
		{
			name:     "arity zero",
			expr:     readCSVCallExpr(span),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "arity two",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "a.csv", Span: span}, ast.StringExpr{Value: "b.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "non string path",
			expr:     readCSVCallExpr(span, ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "missing file",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./missing.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "duplicate header",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./dup.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "invalid header",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./invalid.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "dotted header",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./dotted.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "empty header",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./empty.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
		{
			name:     "inconsistent width",
			expr:     readCSVCallExpr(span, ast.StringExpr{Value: "./width.csv", Span: span}),
			opts:     ExprOptions{Context: EvalCtxBindingAssign, Files: &FileAccess{BaseDir: cwd}},
			wantCode: "E106",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, tc.opts)
			if got.Kind != KindNull {
				t.Fatalf("expected null result, got %#v", got)
			}
			if diagCount(diags, tc.wantCode) == 0 {
				t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
			}
		})
	}
}
