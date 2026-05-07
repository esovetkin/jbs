package parser

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func newTopLevelParser(src string, diags *diag.Diagnostics) *Parser {
	return &Parser{
		file:  "in.jbs",
		src:   []rune(src),
		line:  1,
		col:   1,
		diags: diags,
	}
}

func hasDiag(diags *diag.Diagnostics, code string) bool {
	for _, item := range diags.Items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func parseUseDirect(src string) (ast.UseStmt, *diag.Diagnostics) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(src, diags)
	start := p.pos()
	p.consumeWord() // consume "use"
	stmt := p.parseUseStmt(start)
	return stmt, diags
}

func TestIsTopLevelAssignmentStart(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{src: `jbs_name = "x"`, want: true},
		{src: `let = "x"`, want: true},
		{src: `param = "x"`, want: true},
		{src: `jbs_name+= "x"`, want: true},
		{src: `jbs_name -= "x"`, want: true},
		{src: `jbs_name *= "x"`, want: true},
		{src: `jbs_name /= "x"`, want: true},
		{src: `jbs_name %= "x"`, want: true},
		{src: `jbs_name + "x"`, want: false},
		{src: `param p {`, want: false},
		{src: `do run {`, want: false},
		{src: `unknown run {`, want: false},
		{src: `let x {`, want: false},
		{src: `analyse run {`, want: false},
		{src: `use lib`, want: false},
		{src: `1x = 2`, want: false},
		{src: `name`, want: false},
		{src: `lib.value`, want: false},
	}

	for _, tt := range tests {
		p := newTopLevelParser(tt.src, &diag.Diagnostics{})
		if got := p.isTopLevelAssignmentStart(); got != tt.want {
			t.Fatalf("isTopLevelAssignmentStart(%q)=%v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestParseGlobalAssignMalformedStart(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("not an assignment\n", diags)
	start := p.pos()
	got := p.parseGlobalAssign(start)
	if !hasDiag(diags, "E012") {
		t.Fatalf("expected E012 for malformed top-level assignment, got: %s", diags.String())
	}
	if got.Span.Start != start || got.Span.End != start {
		t.Fatalf("expected zero-length span at start for malformed global assignment, got %+v", got.Span)
	}
}

func TestParseGlobalAssignSuccess(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`jbs_name = "bench"`+"\n", diags)
	start := p.pos()
	got := p.parseGlobalAssign(start)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got.Name != "jbs_name" {
		t.Fatalf("unexpected assignment name: %#v", got)
	}
	if got.Op != ast.AssignEq {
		t.Fatalf("unexpected assignment op: %#v", got.Op)
	}
	if _, ok := got.Expr.(ast.StringExpr); !ok {
		t.Fatalf("expected string expression, got %#v", got.Expr)
	}
}

func TestParseTopLevelExprStmtSuccess(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("lib.value\n", diags)
	start := p.pos()
	got := p.parseTopLevelExprStmt(start)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if _, ok := got.Expr.(ast.QualifiedIdentExpr); !ok {
		t.Fatalf("expected qualified identifier expression, got %#v", got.Expr)
	}
	if got.Span.IsZero() {
		t.Fatalf("expected non-zero statement span")
	}
}

func TestParseTopLevelExprStmtLineContinuation(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("1 + \\\n2\n", diags)
	start := p.pos()
	got := p.parseTopLevelExprStmt(start)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if _, ok := got.Expr.(ast.BinaryExpr); !ok {
		t.Fatalf("expected binary expression, got %#v", got.Expr)
	}
	if p.pos().Line != 3 || p.pos().Column != 1 {
		t.Fatalf("unexpected parser position after continued expression: %+v", p.pos())
	}
}

func TestParseTopLevelExprStmtTrailingTokens(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("unknownblock x\n", diags)
	start := p.pos()
	got := p.parseTopLevelExprStmt(start)
	if got.Expr == nil {
		t.Fatalf("expected expression node")
	}
	if !hasDiag(diags, "E061") {
		t.Fatalf("expected E061, got: %s", diags.String())
	}
}

func TestParseTopLevelExprStmtFunctionLiteral(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("function(x) {\n  x\n}(1)\n", diags)
	start := p.pos()
	got := p.parseTopLevelExprStmt(start)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	call, ok := got.Expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression, got %#v", got.Expr)
	}
	if _, ok := call.Callee.(ast.FunctionExpr); !ok {
		t.Fatalf("expected function literal callee, got %#v", call.Callee)
	}
}

func TestParseUseStmtErrorBranches(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "empty after use", src: "use\n"},
		{name: "invalid first token", src: "use 123\n"},
		{name: "path missing alias", src: `use "./x.jbs"` + "\n"},
		{name: "path alias missing identifier", src: `use "./x.jbs" as 1` + "\n"},
		{name: "from invalid source token", src: "use x from 1\n"},
		{name: "namespace import with many names", src: "use a, b\n"},
		{name: "trailing tokens", src: "use lib extra\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := parseUseDirect(tt.src)
			if !hasDiag(diags, "E430") {
				t.Fatalf("expected E430 for %q, got: %s", tt.src, diags.String())
			}
		})
	}
}

func TestParseUseStmtPathMissingAliasKeepsSource(t *testing.T) {
	stmt, diags := parseUseDirect(`use "./x.jbs"` + "\n")
	if !hasDiag(diags, "E430") {
		t.Fatalf("expected E430, got: %s", diags.String())
	}
	if stmt.Source.Kind != ast.UseSourcePath || stmt.Source.Value != "./x.jbs" {
		t.Fatalf("expected path source to be preserved, got %#v", stmt.Source)
	}
}

func TestReadTopLevelStatement(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantPrefix string
		wantLine   int
		wantCol    int
	}{
		{
			name:       "inline comment terminates statement",
			src:        "x = 1 # c\nnext = 2\n",
			wantPrefix: "x = 1 ",
			wantLine:   2,
			wantCol:    1,
		},
		{
			name:       "semicolon outside quotes terminates statement",
			src:        `x = "a;b"; y = 2`,
			wantPrefix: `x = "a;b"`,
			wantLine:   1,
			wantCol:    11,
		},
		{
			name:       "line continuation keeps statement open",
			src:        "x = \"a\" +\\\n\"b\"\nnext\n",
			wantPrefix: "x = \"a\" +\\\n\"b\"",
			wantLine:   3,
			wantCol:    1,
		},
		{
			name:       "hash inside quotes is not comment",
			src:        "x = \"#not_comment\"\nnext\n",
			wantPrefix: "x = \"#not_comment\"",
			wantLine:   2,
			wantCol:    1,
		},
		{
			name:       "escaped quotes in strings are handled",
			src:        "x = \"a\\\"#b\" + 'c\\'d#e'; z = 1\n",
			wantPrefix: "x = \"a\\\"#b\" + 'c\\'d#e'",
			wantLine:   1,
			wantCol:    24,
		},
		{
			name: "multiline function assignment stays one statement",
			src: "f = function(x, y = 1) {\n" +
				"  x + y\n" +
				"}\n" +
				"next\n",
			wantPrefix: "f = function(x, y = 1) {\n  x + y\n}",
			wantLine:   4,
			wantCol:    1,
		},
		{
			name: "multiline anonymous function call stays one statement",
			src: "function(x) {\n" +
				"  x\n" +
				"}(1)\n" +
				"next\n",
			wantPrefix: "function(x) {\n  x\n}(1)",
			wantLine:   4,
			wantCol:    1,
		},
		{
			name: "comments inside function body do not terminate statement",
			src: "f = function(x) {\n" +
				"  # inner\n" +
				"  x\n" +
				"} # trailing\n" +
				"next\n",
			wantPrefix: "f = function(x) {\n  # inner\n  x\n} ",
			wantLine:   5,
			wantCol:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTopLevelParser(tt.src, &diag.Diagnostics{})
			stmt, start := p.readTopLevelStatement()
			if start.Line != 1 || start.Column != 1 || start.Offset != 0 {
				t.Fatalf("unexpected start position: %+v", start)
			}
			if stmt != tt.wantPrefix {
				t.Fatalf("unexpected statement text: got %q want %q", stmt, tt.wantPrefix)
			}
			if p.pos().Line != tt.wantLine || p.pos().Column != tt.wantCol {
				t.Fatalf("unexpected parser position after statement: got=%+v want line=%d col=%d", p.pos(), tt.wantLine, tt.wantCol)
			}
		})
	}
}

func TestReadTopLevelStatementStopsAtHashEvenWithoutBoundary(t *testing.T) {
	p := newTopLevelParser("x=1#comment\ny=2\n", &diag.Diagnostics{})
	stmt, _ := p.readTopLevelStatement()
	if stmt != "x=1" {
		t.Fatalf("expected statement to stop at '#', got %q", stmt)
	}
}

func TestReadTopLevelStatementAtEOF(t *testing.T) {
	p := newTopLevelParser("x = 1", &diag.Diagnostics{})
	stmt, start := p.readTopLevelStatement()
	if start.Line != 1 || start.Column != 1 || start.Offset != 0 {
		t.Fatalf("unexpected start position: %+v", start)
	}
	if stmt != "x = 1" {
		t.Fatalf("unexpected statement at EOF: %q", stmt)
	}
	if !p.eof() {
		t.Fatalf("expected parser at EOF after reading statement")
	}
}

func TestParseUseStmtSelectiveFromPath(t *testing.T) {
	stmt, diags := parseUseDirect(`use helper, tool from "./lib.jbs"` + "\n")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(stmt.Names) != 2 || stmt.Names[0] != "helper" || stmt.Names[1] != "tool" {
		t.Fatalf("unexpected selective names: %#v", stmt.Names)
	}
	if stmt.Source.Kind != ast.UseSourcePath || stmt.Source.Value != "./lib.jbs" {
		t.Fatalf("unexpected selective path source: %#v", stmt.Source)
	}
}

func TestParseUseStmtSelectiveFromBareModule(t *testing.T) {
	stmt, diags := parseUseDirect("use helper, tool from lib\n")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(stmt.Names) != 2 || stmt.Names[0] != "helper" || stmt.Names[1] != "tool" {
		t.Fatalf("unexpected selective names: %#v", stmt.Names)
	}
	if stmt.Source.Kind != ast.UseSourceBare || stmt.Source.Value != "lib" {
		t.Fatalf("unexpected selective source: %#v", stmt.Source)
	}
}

func TestParseUseStmtPathAliasSuccess(t *testing.T) {
	stmt, diags := parseUseDirect(`use "./x.jbs" as util` + "\n")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if stmt.Source.Kind != ast.UseSourcePath || stmt.Source.Value != "./x.jbs" {
		t.Fatalf("unexpected source: %#v", stmt.Source)
	}
	if stmt.Alias != "util" {
		t.Fatalf("unexpected alias: %#v", stmt.Alias)
	}
}

func TestParseUseStmtNamespaceImportSuccess(t *testing.T) {
	stmt, diags := parseUseDirect("use lib\n")
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if stmt.Source.Kind != ast.UseSourceBare || stmt.Source.Value != "lib" {
		t.Fatalf("unexpected source: %#v", stmt.Source)
	}
	if stmt.Alias != "lib" {
		t.Fatalf("unexpected alias: %#v", stmt.Alias)
	}
}

func TestParseUseStmtMissingIdentifierAfterComma(t *testing.T) {
	_, diags := parseUseDirect("use a, from lib\n")
	if !hasDiag(diags, "E430") {
		t.Fatalf("expected E430, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "expected identifier in use statement") {
		t.Fatalf("expected missing-identifier message, got: %s", diags.String())
	}
}

func TestParseUseStmtUnexpectedTrailingTokensMessage(t *testing.T) {
	_, diags := parseUseDirect("use lib trailing\n")
	if !hasDiag(diags, "E430") {
		t.Fatalf("expected E430, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "unexpected trailing tokens in use statement") {
		t.Fatalf("expected trailing-token message, got: %s", diags.String())
	}
}
