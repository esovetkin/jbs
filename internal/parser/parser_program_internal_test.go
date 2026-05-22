package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func parseProgramWithTimeout(t *testing.T, src string) (ast.Program, *diag.Diagnostics) {
	t.Helper()
	type parseResult struct {
		prog  ast.Program
		diags *diag.Diagnostics
	}
	done := make(chan parseResult, 1)
	go func() {
		diags := &diag.Diagnostics{}
		prog := Parse("timeout.jbs", src, diags)
		done <- parseResult{prog: prog, diags: diags}
	}()
	select {
	case result := <-done:
		return result.prog, result.diags
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("parser did not return for %q", src)
		return ast.Program{}, nil
	}
}

func TestParseProgramDispatchAndSpan(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
jbs_name = "bench"
x = (1, 2)
x
do run {
  echo hi
}
analyse run {
  n = "N: %d" in "out.log"
  (n)
}
	`
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 5 {
		t.Fatalf("expected 5 top-level statements, got %d (%#v)", len(prog.Stmts), prog.Stmts)
	}
	if prog.File != "in.jbs" {
		t.Fatalf("unexpected program file: %q", prog.File)
	}
	if prog.Span.Start.Offset >= prog.Span.End.Offset {
		t.Fatalf("expected non-empty merged program span, got %+v", prog.Span)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid program: %s", diags.String())
	}
}

func TestParseProgramReportsExpressionErrorsForMalformedTopLevelInput(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := "@\nunknownblock x\n"
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two expression statements for malformed source, got %#v", prog.Stmts)
	}
	if !hasDiag(diags, "E058") {
		t.Fatalf("expected E058 for invalid expression token, got: %s", diags.String())
	}
	if !hasDiag(diags, "E061") {
		t.Fatalf("expected E061 for trailing tokens in malformed expr line, got: %s", diags.String())
	}
}

func TestParseProgramReportsStrayTopLevelClosingBrace(t *testing.T) {
	prog, diags := parseProgramWithTimeout(t, "}\n")
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one recovery statement, got %#v", prog.Stmts)
	}
	if !hasDiag(diags, "E058") || !strings.Contains(diags.String(), "unexpected closing brace") {
		t.Fatalf("expected unexpected closing brace diagnostic, got: %s", diags.String())
	}
}

func TestParseProgramRecoversAfterStrayTopLevelClosingBrace(t *testing.T) {
	prog, diags := parseProgramWithTimeout(t, "x = 1\n}\ny = 2\n")
	if !strings.Contains(diags.String(), "unexpected closing brace") {
		t.Fatalf("expected unexpected closing brace diagnostic, got: %s", diags.String())
	}
	if len(prog.Stmts) != 3 {
		t.Fatalf("expected x assignment, recovery statement, and y assignment, got %#v", prog.Stmts)
	}
	first, ok := prog.Stmts[0].(ast.GlobalAssign)
	if !ok || first.Name != "x" {
		t.Fatalf("first statement = %#v, want x assignment", prog.Stmts[0])
	}
	third, ok := prog.Stmts[2].(ast.GlobalAssign)
	if !ok || third.Name != "y" {
		t.Fatalf("third statement = %#v, want y assignment", prog.Stmts[2])
	}
}

func TestParseProgramHandlesNestedAndExtraClosingBraces(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("nested.jbs", "if true {\n  x = 1\n}\n", diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid nested block: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one if statement, got %#v", prog.Stmts)
	}

	prog, diags = parseProgramWithTimeout(t, "if true {\n  x = 1\n}}\n")
	if !strings.Contains(diags.String(), "unexpected closing brace") {
		t.Fatalf("expected extra closing brace diagnostic, got: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected if statement plus recovery statement, got %#v", prog.Stmts)
	}
}

func TestParseDoBlockKeepsBraceInsideHereDoc(t *testing.T) {
	src := "do run {\ncat > out <<EOF\n}\nEOF\necho after\n}\n"
	body, diags := parseSingleDoBody(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(body, "echo after") {
		t.Fatalf("do body closed before echo after: %q", body)
	}
	if strings.Count(body, "}") != 1 {
		t.Fatalf("expected heredoc brace to remain in body, got %q", body)
	}
}

func TestParseDoBlockHereDocQuotedDelimiter(t *testing.T) {
	src := "do run {\ncat <<'JSON'\n{\"a\": {\"b\": 1}}\nJSON\necho done\n}\n"
	body, diags := parseSingleDoBody(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(body, "echo done") {
		t.Fatalf("do body closed before echo done: %q", body)
	}
}

func TestParseDoBlockHereDocStripTabs(t *testing.T) {
	src := "do run {\ncat <<-EOF\n\t}\n\tEOF\necho done\n}\n"
	body, diags := parseSingleDoBody(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(body, "echo done") {
		t.Fatalf("do body closed before echo done: %q", body)
	}
}

func TestParseDoBlockMultipleHereDocs(t *testing.T) {
	src := "do run {\ncat <<A <<B\n}\nA\n{\nB\necho done\n}\n"
	body, diags := parseSingleDoBody(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(body, "echo done") {
		t.Fatalf("do body closed before echo done: %q", body)
	}
}

func TestParseDoBlockKeepsShellParameterExpansion(t *testing.T) {
	cases := []string{
		"echo ${file#*.}",
		"echo ${file##*/}",
		"echo ${file%.*}",
		"echo ${file%%.*}",
		"echo \"${file#*.}\"",
		"echo ${file:-${fallback#*.}}",
	}

	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			src := "do run {\nfile=name.txt\nfallback=archive.tar.gz\n" + line + "\necho after\n}\n"
			body, diags := parseSingleDoBody(t, src)
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if !strings.Contains(body, line) {
				t.Fatalf("do body lost parameter expansion %q: %q", line, body)
			}
			if !strings.Contains(body, "echo after") {
				t.Fatalf("do body closed before echo after: %q", body)
			}
		})
	}
}

func TestParseDoBlockKeepsShellCommentBehavior(t *testing.T) {
	src := "do run {\necho before # } ignored by shell comment\necho after\n}\n"
	body, diags := parseSingleDoBody(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(body, "echo after") {
		t.Fatalf("do body closed before echo after: %q", body)
	}
}

func TestParseDoBlockMissingHereDocDelimiterIsUnterminated(t *testing.T) {
	diags := &diag.Diagnostics{}
	_ = Parse("missing.jbs", "do run {\ncat <<EOF\n}\n", diags)
	if !hasDiag(diags, "E025") {
		t.Fatalf("expected unterminated block diagnostic, got: %s", diags.String())
	}
}

func TestExampleDoSbatchParses(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "do_sbatch.jbs"))
	if err != nil {
		t.Fatal(err)
	}
	diags := &diag.Diagnostics{}
	_ = Parse("examples/do_sbatch.jbs", string(src), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func parseSingleDoBody(t *testing.T, src string) (string, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := Parse("heredoc.jbs", src, diags)
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %#v; diagnostics: %s", prog.Stmts, diags.String())
	}
	block, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block, got %#v", prog.Stmts[0])
	}
	return block.Body, diags
}

func TestParseProgramWithFunctionSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
f = function(x, y = 1) {
  x + y
}
function(a) {
  return a
}(1)
`
	prog := Parse("functions.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two top-level statements, got %d", len(prog.Stmts))
	}
	assign, ok := prog.Stmts[0].(ast.GlobalAssign)
	if !ok {
		t.Fatalf("expected first stmt to be global assign, got %#v", prog.Stmts[0])
	}
	if _, ok := assign.Expr.(ast.FunctionExpr); !ok {
		t.Fatalf("expected function literal rhs, got %#v", assign.Expr)
	}
	exprStmt, ok := prog.Stmts[1].(ast.ExprStmt)
	if !ok {
		t.Fatalf("expected second stmt to be expr stmt, got %#v", prog.Stmts[1])
	}
	call, ok := exprStmt.Expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression, got %#v", exprStmt.Expr)
	}
	if _, ok := call.Callee.(ast.FunctionExpr); !ok {
		t.Fatalf("expected function literal callee, got %#v", call.Callee)
	}
}

func TestParseProgramTreatsLetAndParamBlocksAsGenericInvalidExpressions(t *testing.T) {
	tests := []string{
		"let defaults {\n  queue = \"batch\"\n}\n",
		"param cases {\n  x = (1, 2)\n  x\n}\n",
	}

	for _, src := range tests {
		diags := &diag.Diagnostics{}
		prog := Parse("in.jbs", src, diags)
		if len(prog.Stmts) != 1 {
			t.Fatalf("expected one statement for %q, got %#v", src, prog.Stmts)
		}
		if !hasDiag(diags, "E061") {
			t.Fatalf("expected generic trailing-token diagnostic for %q, got: %s", src, diags.String())
		}
		if got := len(diags.Items); got != 1 {
			t.Fatalf("expected exactly one diagnostic for %q, got %d: %s", src, got, diags.String())
		}
	}
}

func TestParseProgramRecoversAfterFormerKeywordShapedBlocks(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
param cases {
  x = (1, 2)
  x
}
let defaults {
  queue = "batch"
}
do run {
  echo ok
}
`
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 3 {
		t.Fatalf("expected three top-level statements, got %#v", prog.Stmts)
	}
	count := 0
	for _, item := range diags.Items {
		if item.Code == "E061" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected two generic expression diagnostics, got %d: %s", count, diags.String())
	}
	if _, ok := prog.Stmts[2].(ast.DoBlock); !ok {
		t.Fatalf("expected parser to recover and keep trailing do block, got %#v", prog.Stmts[2])
	}
}

func TestParseProgramDoesNotTreatLetOrParamAssignmentsAsLegacyBlocks(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := "let = 1\nparam = 2\n"
	prog := Parse("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(prog.Stmts) != 2 {
		t.Fatalf("expected two assignments, got %#v", prog.Stmts)
	}
	if _, ok := prog.Stmts[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected first statement to stay a global assignment, got %#v", prog.Stmts[0])
	}
	if _, ok := prog.Stmts[1].(ast.GlobalAssign); !ok {
		t.Fatalf("expected second statement to stay a global assignment, got %#v", prog.Stmts[1])
	}
}
