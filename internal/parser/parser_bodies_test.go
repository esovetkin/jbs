package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

func parseBodyTP(file, body string, diags *diag.Diagnostics) *tokenParser {
	tokens := lexer.LexFrom(file, body, diag.NewPos(0, 1, 1), diags)
	return &tokenParser{tokens: tokens, diags: diags}
}

func hasCode(diags *diag.Diagnostics, code string) bool {
	for _, d := range diags.Items {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestParseParamBodyUnexpectedTokensAfterFinal(t *testing.T) {
	body := `
a = 1
b = 2
a + b
c = 3
`
	diags := &diag.Diagnostics{}
	assignments, final, finalExpr := parseParamBody("param_body.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments before final expression, got %d", len(assignments))
	}
	if final == nil {
		t.Fatalf("expected final combination expression")
	}
	if finalExpr != nil {
		t.Fatalf("did not expect final expression fallback, got %#v", finalExpr)
	}
	if !hasCode(diags, "E026") {
		t.Fatalf("expected E026 for trailing tokens after final expression, got: %s", diags.String())
	}
}

func TestParseParamBodyFinalFunctionCallParsesAsFinalExpr(t *testing.T) {
	body := `
a = 1
a * range(2)
`
	diags := &diag.Diagnostics{}
	assignments, final, finalExpr := parseParamBody("param_body_final_call.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 1 {
		t.Fatalf("expected one assignment before final expression, got %d", len(assignments))
	}
	if final != nil {
		t.Fatalf("did not expect legacy comb final for call expression, got %#v", final)
	}
	if finalExpr == nil {
		t.Fatalf("expected final expression fallback for call-based final statement")
	}
	if hasCode(diags, "E026") {
		t.Fatalf("did not expect E026 trailing-token cascade, got: %s", diags.String())
	}
	if diags.HasErrors() {
		t.Fatalf("did not expect parser errors for final expression fallback, got: %s", diags.String())
	}
}

func TestParseParamBodyFinalCombCallParsesAsFinalExpr(t *testing.T) {
	body := `
x = (1,2)
y = (3,4)
comb(x*x as b)
`
	diags := &diag.Diagnostics{}
	assignments, final, finalExpr := parseParamBody("param_body_final_comb_call.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 2 {
		t.Fatalf("expected two assignments before final expression, got %d", len(assignments))
	}
	if final != nil {
		t.Fatalf("did not expect legacy comb final for comb(...) call, got %#v", final)
	}
	if finalExpr == nil {
		t.Fatalf("expected final expression fallback for comb(...) final")
	}
	call, ok := finalExpr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected final expression to be call expr, got %T", finalExpr)
	}
	callee, ok := call.Callee.(ast.IdentExpr)
	if !ok || callee.Name != "comb" {
		t.Fatalf("expected comb(...) call, got %#v", call.Callee)
	}
	if diags.HasErrors() {
		t.Fatalf("did not expect parser errors, got: %s", diags.String())
	}
}

func TestParseParamBodyMissingFinalExpression(t *testing.T) {
	body := `
a = 1
b = 2
`
	diags := &diag.Diagnostics{}
	assignments, final, finalExpr := parseParamBody("param_body_missing_final.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 2 {
		t.Fatalf("expected two assignments, got %d", len(assignments))
	}
	if final != nil {
		t.Fatalf("expected nil final expression when missing, got %#v", final)
	}
	if finalExpr != nil {
		t.Fatalf("expected nil final fallback expression when missing, got %#v", finalExpr)
	}
	if !hasCode(diags, "E027") {
		t.Fatalf("expected E027 for missing final expression, got: %s", diags.String())
	}
}

func TestCombExprFromExprIdentConversion(t *testing.T) {
	span := diag.NewSpan("param_body_ident.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	comb, ok := combExprFromExpr(ast.IdentExpr{Name: "x", Span: span})
	if !ok {
		t.Fatalf("expected ident to convert to comb ident")
	}
	got, ok := comb.(ast.CombIdent)
	if !ok {
		t.Fatalf("expected ast.CombIdent, got %T", comb)
	}
	if got.Name != "x" {
		t.Fatalf("expected comb name x, got %q", got.Name)
	}
	if got.Span != span {
		t.Fatalf("expected comb span %v, got %v", span, got.Span)
	}
}

func TestParseLetBodyMalformedStatementContinues(t *testing.T) {
	body := `
x = 1
(x + 1)
y = 2
`
	diags := &diag.Diagnostics{}
	assignments := parseLetBody("let_body.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 2 {
		t.Fatalf("expected two valid assignments, got %d", len(assignments))
	}
	if assignments[0].Name != "x" || assignments[1].Name != "y" {
		t.Fatalf("unexpected parsed assignments: %#v", assignments)
	}
	if !hasCode(diags, "E418") {
		t.Fatalf("expected E418 for malformed let statement, got: %s", diags.String())
	}
}

func TestParseAnalyseBodySuccess(t *testing.T) {
	body := `
p0 = p.number in "out.log"
p1 = "Number: %d" in "out.log"
(a, p0 as "P0", p1)
`
	diags := &diag.Diagnostics{}
	assignments, columns := parseAnalyseBody("analyse_body.jbs", body, diag.NewPos(0, 1, 1), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(assignments) != 2 {
		t.Fatalf("expected 2 analyse assignments, got %d", len(assignments))
	}
	if assignments[0].Name != "p0" || assignments[0].File != "out.log" {
		t.Fatalf("unexpected first analyse assignment: %#v", assignments[0])
	}
	if len(columns) != 3 {
		t.Fatalf("expected 3 analyse columns, got %d", len(columns))
	}
	if columns[1].Name != "p0" || columns[1].Title != "P0" {
		t.Fatalf("unexpected titled analyse column: %#v", columns[1])
	}
}

func TestParseAnalyseBodyMissingFinalTuple(t *testing.T) {
	body := `
p0 = p.number in "out.log"
`
	diags := &diag.Diagnostics{}
	assignments, columns := parseAnalyseBody("analyse_missing_tuple.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 1 {
		t.Fatalf("expected one analyse assignment, got %d", len(assignments))
	}
	if columns != nil {
		t.Fatalf("expected nil columns when final tuple is missing, got %#v", columns)
	}
	if !hasCode(diags, "E417") {
		t.Fatalf("expected E417 for missing final tuple, got: %s", diags.String())
	}
}

func TestParseAnalyseBodyTrailingTokensAfterTuple(t *testing.T) {
	body := `
(a)
p0 = p.number in "out.log"
`
	diags := &diag.Diagnostics{}
	assignments, columns := parseAnalyseBody("analyse_trailing_after_tuple.jbs", body, diag.NewPos(0, 1, 1), diags)
	if len(assignments) != 0 {
		t.Fatalf("expected no assignments after tuple-first body, got %#v", assignments)
	}
	if len(columns) != 1 || columns[0].Name != "a" {
		t.Fatalf("unexpected analyse columns: %#v", columns)
	}
	if !hasCode(diags, "E417") {
		t.Fatalf("expected E417 for trailing tokens after final tuple, got: %s", diags.String())
	}
}

func TestParseAnalyseAssignmentErrorBranches(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantName string
		wantFile string
		wantDiag string
	}{
		{
			name:     "statement does not start with ident",
			body:     "1 = p in \"out\"\n",
			wantDiag: "E416",
		},
		{
			name:     "missing assignment operator",
			body:     "x p\n",
			wantDiag: "E416",
		},
		{
			name:     "in without quoted filename",
			body:     "x = p in out\n",
			wantDiag: "E416",
		},
		{
			name:     "unexpected trailing tokens",
			body:     "x = p in \"out\" trailing\n",
			wantName: "x",
			wantFile: "out",
			wantDiag: "E416",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseBodyTP("analyse_assign.jbs", tc.body, diags)
			tp.skipStmtSeparators()
			assign := parseAnalyseAssignment(tp, "analyse_assign.jbs", diags)
			if assign.Name != tc.wantName {
				t.Fatalf("expected assignment name %q, got %q (assignment=%#v)", tc.wantName, assign.Name, assign)
			}
			if assign.File != tc.wantFile {
				t.Fatalf("expected assignment file %q, got %q", tc.wantFile, assign.File)
			}
			if !hasCode(diags, tc.wantDiag) {
				t.Fatalf("expected %s, got: %s", tc.wantDiag, diags.String())
			}
		})
	}
}

func TestParseAnalyseTupleBranches(t *testing.T) {
	t.Run("empty tuple", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_empty.jbs", "()", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_empty.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected empty columns for (), got %#v", cols)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("missing comma between items", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_comma.jbs", "(a b)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_comma.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "a" {
			t.Fatalf("expected first parsed column before comma error, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for missing comma, got: %s", diags.String())
		}
	})

	t.Run("as without string title", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_as.jbs", "(a as b)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_as.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected no columns when title is malformed, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for malformed title after as, got: %s", diags.String())
		}
	})

	t.Run("dotted name missing identifier after dot", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_dot.jbs", "(ns.)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_dot.jbs", diags)
		if len(cols) != 1 {
			t.Fatalf("expected one partial column despite dotted-name error, got %#v", cols)
		}
		if len(cols[0].Name) < 3 || cols[0].Name[:3] != "ns." {
			t.Fatalf("expected partial dotted name with ns. prefix, got %#v", cols[0])
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for missing identifier after dot, got: %s", diags.String())
		}
	})
}

func TestParseAnalyseAssignmentQualifiedReference(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseBodyTP("analyse_assign_ok.jbs", `p0 = ns.value in "out.log"`, diags)
	tp.skipStmtSeparators()
	assign := parseAnalyseAssignment(tp, "analyse_assign_ok.jbs", diags)
	if assign.Name != "p0" || assign.File != "out.log" {
		t.Fatalf("unexpected assignment: %#v", assign)
	}
	if _, ok := assign.Expr.(ast.QualifiedIdentExpr); !ok {
		t.Fatalf("expected qualified identifier expression, got %T", assign.Expr)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseAnalyseAssignmentNoInClauseAndCommentTerminator(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseBodyTP("analyse_assign_plus_eq.jbs", "x += ns.value # keep comment\n", diags)
	tp.skipStmtSeparators()
	assign := parseAnalyseAssignment(tp, "analyse_assign_plus_eq.jbs", diags)
	if assign.Name != "x" {
		t.Fatalf("expected assignment name x, got %#v", assign)
	}
	if assign.Op != ast.AssignPlusEq {
		t.Fatalf("expected += assignment op, got %q", assign.Op)
	}
	if assign.File != "" {
		t.Fatalf("expected empty file for assignment without in-clause, got %q", assign.File)
	}
	if _, ok := assign.Expr.(ast.QualifiedIdentExpr); !ok {
		t.Fatalf("expected qualified identifier expression, got %T", assign.Expr)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseAnalyseTupleAdditionalBranches(t *testing.T) {
	t.Run("unterminated tuple reports E417 and keeps parsed prefix", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_unterminated.jbs", "(a,\n", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_unterminated.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "a" {
			t.Fatalf("expected parsed prefix column a before unterminated tuple, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for unterminated tuple, got: %s", diags.String())
		}
	})

	t.Run("non-identifier tuple token is skipped and parsing continues", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_non_ident.jbs", "(1, a)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_non_ident.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "a" {
			t.Fatalf("expected tuple parser to recover and parse a, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for non-identifier tuple item, got: %s", diags.String())
		}
	})

	t.Run("loop-level right-paren branch after skipping invalid token", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_loop_rparen.jbs", "(,)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_loop_rparen.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected no columns for (,), got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for invalid leading tuple token, got: %s", diags.String())
		}
	})

	t.Run("trailing comma before closing parenthesis is accepted", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_trailing_comma.jbs", "(a,)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_trailing_comma.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "a" {
			t.Fatalf("expected single column for trailing-comma tuple, got %#v", cols)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for trailing comma tuple, got: %s", diags.String())
		}
	})

	t.Run("dotted tuple item parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_dot_ok.jbs", "(ns.value)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_dot_ok.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "ns.value" {
			t.Fatalf("expected one dotted tuple item ns.value, got %#v", cols)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for valid dotted tuple item, got: %s", diags.String())
		}
	})
}
