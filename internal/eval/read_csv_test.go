package eval

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func readCSVCallExpr(span diag.Span, args ...ast.Expr) ast.CallExpr {
	return ast.CallExpr{
		Callee: ast.IdentExpr{Name: "read_csv", Span: span},
		Args:   args,
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

func TestIsValidCombColumnName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "simple", in: "x", want: true},
		{name: "underscored", in: "system_name", want: true},
		{name: "qualified", in: "ns.value", want: true},
		{name: "nested qualified", in: "outer.inner.value", want: true},
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
