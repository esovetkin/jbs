package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
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

func TestParseAnalyseAssignmentFileTargets(t *testing.T) {
	tests := []struct {
		name string
		body string
		kind ast.AnalyseFileKind
		file string
	}{
		{name: "exact", body: `p0 = "Runtime %f" in "job.out"`, kind: ast.AnalyseFileExact, file: "job.out"},
		{name: "regex", body: `p0 = "Runtime %f" in re"^job\.[0-9]+\.out$"`, kind: ast.AnalyseFileRegex, file: `^job\.[0-9]+\.out$`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseBodyTP("analyse_assign_target.jbs", tt.body, diags)
			tp.skipStmtSeparators()
			assign := parseAnalyseAssignment(tp, "analyse_assign_target.jbs", diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors: %s", diags.String())
			}
			if assign.FileTarget.Kind != tt.kind || assign.FileTarget.Value != tt.file || assign.File != tt.file {
				t.Fatalf("unexpected file target: %#v", assign)
			}
		})
	}

	diags := &diag.Diagnostics{}
	tp := parseBodyTP("analyse_assign_spaced_regex.jbs", `p0 = "Runtime %f" in re "job.*"`, diags)
	tp.skipStmtSeparators()
	assign := parseAnalyseAssignment(tp, "analyse_assign_spaced_regex.jbs", diags)
	if assign.Name != "" {
		t.Fatalf("malformed spaced regex assignment should be skipped, got %#v", assign)
	}
	if !hasCode(diags, "E416") {
		t.Fatalf("expected E416 for spaced regex marker, got %s", diags.String())
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

	t.Run("deep dotted tuple item parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_deep_ok.jbs", "(pkg.ns.value)", diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_deep_ok.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "pkg.ns.value" {
			t.Fatalf("expected one deep dotted tuple item pkg.ns.value, got %#v", cols)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for valid deep dotted tuple item, got: %s", diags.String())
		}
	})

	t.Run("deep dotted tuple item with title parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_deep_title.jbs", `(pkg.ns.value as "Value")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_deep_title.jbs", diags)
		if len(cols) != 1 || cols[0].Name != "pkg.ns.value" || cols[0].Title != "Value" {
			t.Fatalf("expected titled deep dotted tuple item, got %#v", cols)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for titled deep dotted tuple item, got: %s", diags.String())
		}
	})

	t.Run("inline string pattern parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline.jbs", `("Runtime %f" in "job.out")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline.jbs", diags)
		if len(cols) != 1 || cols[0].Kind != ast.AnalyseColumnInlinePattern || cols[0].FileTarget.Kind != ast.AnalyseFileExact || cols[0].File != "job.out" || cols[0].Title != "" {
			t.Fatalf("unexpected inline columns: %#v", cols)
		}
		if s, ok := cols[0].Expr.(ast.StringExpr); !ok || s.Value != "Runtime %f" {
			t.Fatalf("unexpected inline expression: %#v", cols[0].Expr)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for inline pattern, got: %s", diags.String())
		}
	})

	t.Run("inline string pattern with title parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_title.jbs", `("Runtime %f" in "job.out" as "runtime")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_title.jbs", diags)
		if len(cols) != 1 || cols[0].Kind != ast.AnalyseColumnInlinePattern || cols[0].Title != "runtime" {
			t.Fatalf("unexpected titled inline columns: %#v", cols)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for titled inline pattern, got: %s", diags.String())
		}
	})

	t.Run("inline identifier pattern parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_ident.jbs", `(runtime_pattern in "job.out" as "runtime")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_ident.jbs", diags)
		if len(cols) != 1 || cols[0].Kind != ast.AnalyseColumnInlinePattern || cols[0].File != "job.out" || cols[0].Title != "runtime" {
			t.Fatalf("unexpected identifier inline columns: %#v", cols)
		}
		if id, ok := cols[0].Expr.(ast.IdentExpr); !ok || id.Name != "runtime_pattern" {
			t.Fatalf("unexpected identifier inline expression: %#v", cols[0].Expr)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for identifier inline pattern, got: %s", diags.String())
		}
	})

	t.Run("inline qualified identifier pattern parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_qualified.jbs", `(pkg.ns.value in "job.out")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_qualified.jbs", diags)
		if len(cols) != 1 || cols[0].Kind != ast.AnalyseColumnInlinePattern || cols[0].File != "job.out" {
			t.Fatalf("unexpected qualified inline columns: %#v", cols)
		}
		if q, ok := cols[0].Expr.(ast.QualifiedIdentExpr); !ok || q.Namespace != "pkg.ns" || q.Name != "value" {
			t.Fatalf("unexpected qualified inline expression: %#v", cols[0].Expr)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for qualified inline pattern, got: %s", diags.String())
		}
	})

	t.Run("inline regex file target parses successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_regex_file.jbs", `("Runtime %f" in re"^job\.[0-9]+\.out$" as "runtime")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_regex_file.jbs", diags)
		if len(cols) != 1 || cols[0].Kind != ast.AnalyseColumnInlinePattern || cols[0].FileTarget.Kind != ast.AnalyseFileRegex || cols[0].Title != "runtime" {
			t.Fatalf("unexpected regex inline columns: %#v", cols)
		}
		if cols[0].FileTarget.Value != `^job\.[0-9]+\.out$` {
			t.Fatalf("unexpected regex file target value: %#v", cols[0].FileTarget)
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for regex inline pattern, got: %s", diags.String())
		}
	})

	t.Run("mixed named and inline columns parse successfully", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_mixed.jbs", `(case_id, assigned as "assigned", "Runtime %f" in "job.out" as "runtime")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_mixed.jbs", diags)
		if len(cols) != 3 {
			t.Fatalf("expected three columns, got %#v", cols)
		}
		if cols[0].Name != "case_id" || cols[1].Name != "assigned" || cols[1].Title != "assigned" {
			t.Fatalf("unexpected named columns in mixed tuple: %#v", cols)
		}
		if cols[2].Kind != ast.AnalyseColumnInlinePattern || cols[2].Title != "runtime" {
			t.Fatalf("unexpected inline column in mixed tuple: %#v", cols[2])
		}
		if diags.HasErrors() {
			t.Fatalf("did not expect errors for mixed tuple, got: %s", diags.String())
		}
	})

	t.Run("inline pattern without in reports E417", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_no_in.jbs", `("Runtime %f")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_no_in.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected no columns for malformed inline pattern, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for missing inline in-clause, got: %s", diags.String())
		}
	})

	t.Run("inline pattern without quoted file reports E417", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_bad_file.jbs", `("Runtime %f" in out)`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_bad_file.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected no columns for malformed inline file, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for missing quoted inline file, got: %s", diags.String())
		}
	})

	t.Run("inline pattern with spaced regex marker reports E417", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_bad_regex_file.jbs", `("Runtime %f" in re "job.*")`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_bad_regex_file.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected no columns for spaced regex marker, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for spaced regex marker, got: %s", diags.String())
		}
	})

	t.Run("inline pattern without quoted title reports E417", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseBodyTP("analyse_tuple_inline_bad_title.jbs", `("Runtime %f" in "job.out" as runtime)`, diags)
		cols := parseAnalyseTuple(tp, "analyse_tuple_inline_bad_title.jbs", diags)
		if len(cols) != 0 {
			t.Fatalf("expected no columns for malformed inline title, got %#v", cols)
		}
		if !hasCode(diags, "E417") {
			t.Fatalf("expected E417 for missing quoted inline title, got: %s", diags.String())
		}
	})
}
