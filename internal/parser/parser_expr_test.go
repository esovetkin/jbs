package parser

import (
	"math"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
)

func parseExprTP(src string, diags *diag.Diagnostics) *tokenParser {
	tokens := lexer.LexFrom("expr.jbs", src, diag.NewPos(0, 1, 1), diags)
	return &tokenParser{tokens: tokens, diags: diags}
}

func TestIsAssignTokenAndTokenToAssignOp(t *testing.T) {
	tests := []struct {
		tt     lexer.TokenType
		isOp   bool
		assign ast.AssignOp
	}{
		{tt: lexer.TokenEqual, isOp: true, assign: ast.AssignEq},
		{tt: lexer.TokenPlusEqual, isOp: true, assign: ast.AssignPlusEq},
		{tt: lexer.TokenMinusEqual, isOp: true, assign: ast.AssignMinusEq},
		{tt: lexer.TokenStarEqual, isOp: true, assign: ast.AssignStarEq},
		{tt: lexer.TokenSlashEqual, isOp: true, assign: ast.AssignSlashEq},
		{tt: lexer.TokenPercentEqual, isOp: true, assign: ast.AssignPctEq},
		{tt: lexer.TokenPlus, isOp: false, assign: ast.AssignEq},
		{tt: lexer.TokenIdent, isOp: false, assign: ast.AssignEq},
	}

	for _, tc := range tests {
		gotIs := isAssignToken(tc.tt)
		if gotIs != tc.isOp {
			t.Fatalf("isAssignToken(%s) expected %v, got %v", tc.tt, tc.isOp, gotIs)
		}
		gotAssign := tokenToAssignOp(tc.tt)
		if gotAssign != tc.assign {
			t.Fatalf("tokenToAssignOp(%s) expected %v, got %v", tc.tt, tc.assign, gotAssign)
		}
	}
}

func TestParseAssignOp(t *testing.T) {
	diags0 := &diag.Diagnostics{}
	tp0 := parseExprTP("+= 1", diags0)
	op, span, ok := tp0.parseAssignOp()
	if !ok {
		t.Fatalf("expected parseAssignOp to succeed")
	}
	if op != ast.AssignPlusEq {
		t.Fatalf("expected += op, got %v", op)
	}
	if span.IsZero() {
		t.Fatalf("expected non-zero span for parsed assign op")
	}

	diags1 := &diag.Diagnostics{}
	tp1 := parseExprTP("x", diags1)
	op, span, ok = tp1.parseAssignOp()
	if ok {
		t.Fatalf("expected parseAssignOp to fail on non-assign token")
	}
	if op != ast.AssignEq {
		t.Fatalf("expected default assign op on failure, got %v", op)
	}
	if span.IsZero() {
		t.Fatalf("expected token span on parseAssignOp failure")
	}
}

func TestParseAssignmentBranches(t *testing.T) {
	t.Run("missing identifier", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("= 1\n", diags)
		asn := tp.parseAssignment()
		if asn.Name != "=" {
			t.Fatalf("expected fallback assignment name from unexpected token, got %q", asn.Name)
		}
		if !hasCode(diags, "E050") {
			t.Fatalf("expected E050, got: %s", diags.String())
		}
	})

	t.Run("missing assignment operator", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("x 1\n", diags)
		asn := tp.parseAssignment()
		if asn.Name != "x" {
			t.Fatalf("expected assignment name x, got %q", asn.Name)
		}
		if asn.Expr != nil {
			t.Fatalf("expected nil expression on missing assign op, got %#v", asn.Expr)
		}
		if !hasCode(diags, "E051") {
			t.Fatalf("expected E051, got: %s", diags.String())
		}
	})

	t.Run("trailing tokens after expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("x = 1 trailing\n", diags)
		asn := tp.parseAssignment()
		if asn.Name != "x" || asn.Expr == nil {
			t.Fatalf("unexpected assignment parse result: %#v", asn)
		}
		if !hasCode(diags, "E061") {
			t.Fatalf("expected E061, got: %s", diags.String())
		}
	})

	t.Run("compound assignment", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("x *= 2\n", diags)
		asn := tp.parseAssignment()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		if asn.Name != "x" || asn.Op != ast.AssignStarEq {
			t.Fatalf("unexpected compound assignment parse result: %#v", asn)
		}
	})
}

func TestParseRegexStringOutsideAnalyseFileTargetReportsError(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`re"job.*"`, diags)
	expr := tp.parseExpr()
	if _, ok := expr.(ast.StringExpr); !ok {
		t.Fatalf("expected fallback string expression, got %T", expr)
	}
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for regex string outside analyse file target, got %s", diags.String())
	}
}

func TestParseExprPrecedenceAndAssociativity(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		check func(t *testing.T, expr ast.Expr)
	}{
		{
			name: "mul binds tighter than add/sub",
			src:  "a + b * c - d",
			check: func(t *testing.T, expr ast.Expr) {
				top, ok := expr.(ast.BinaryExpr)
				if !ok || top.Op != "-" {
					t.Fatalf("expected top '-' binary, got %#v", expr)
				}
				left, ok := top.Left.(ast.BinaryExpr)
				if !ok || left.Op != "+" {
					t.Fatalf("expected left '+' binary, got %#v", top.Left)
				}
				mul, ok := left.Right.(ast.BinaryExpr)
				if !ok || mul.Op != "*" {
					t.Fatalf("expected nested '*' binary, got %#v", left.Right)
				}
			},
		},
		{
			name: "amp binds tighter than pipe",
			src:  "a | b & c",
			check: func(t *testing.T, expr ast.Expr) {
				top, ok := expr.(ast.BinaryExpr)
				if !ok || top.Op != "|" {
					t.Fatalf("expected top '|' binary, got %#v", expr)
				}
				right, ok := top.Right.(ast.BinaryExpr)
				if !ok || right.Op != "&" {
					t.Fatalf("expected right '&' binary, got %#v", top.Right)
				}
			},
		},
		{
			name: "unary bang binds tighter than amp",
			src:  "!a & b",
			check: func(t *testing.T, expr ast.Expr) {
				top, ok := expr.(ast.BinaryExpr)
				if !ok || top.Op != "&" {
					t.Fatalf("expected top '&' binary, got %#v", expr)
				}
				left, ok := top.Left.(ast.UnaryExpr)
				if !ok || left.Op != "!" {
					t.Fatalf("expected unary '!' on left side, got %#v", top.Left)
				}
			},
		},
		{
			name: "compare consumes additive rhs",
			src:  "a < b + c",
			check: func(t *testing.T, expr ast.Expr) {
				cmp, ok := expr.(ast.CompareExpr)
				if !ok || cmp.Op != "<" {
					t.Fatalf("expected compare '<', got %#v", expr)
				}
				if _, ok := cmp.Right.(ast.BinaryExpr); !ok {
					t.Fatalf("expected additive expression on compare rhs, got %#v", cmp.Right)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			expr := tp.parseExpr()
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors for %q: %s", tc.src, diags.String())
			}
			tc.check(t, expr)
		})
	}
}

func TestParseUnaryBangGroupedCompare(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("!(a == b)", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	unary, ok := expr.(ast.UnaryExpr)
	if !ok || unary.Op != "!" {
		t.Fatalf("expected unary '!' expression, got %#v", expr)
	}
	if _, ok := unary.Expr.(ast.CompareExpr); !ok {
		t.Fatalf("expected compare expression under unary '!', got %#v", unary.Expr)
	}
}

func TestParseGroupedExpressionSpansIncludeParentheses(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "grouped arithmetic", src: "(1 + 2) * 3"},
		{name: "grouped unary operand", src: "-(a + b)"},
		{name: "grouped conditional", src: "(a if b else c) + d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			expr := tp.parseExpr()
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors: %s", diags.String())
			}
			if got := expr.GetSpan().Start.Offset; got != 0 {
				t.Fatalf("expected expression span to start at opening grouping, got offset %d", got)
			}
		})
	}
}

func TestExprWithSpanCoversExpressionVariants(t *testing.T) {
	span := diag.NewSpan("expr.jbs", diag.NewPos(10, 2, 1), diag.NewPos(20, 2, 11))
	leaf := ast.IdentExpr{Name: "x"}
	cases := []ast.Expr{
		ast.IdentExpr{Name: "x"},
		ast.QualifiedIdentExpr{Namespace: "pkg", Name: "x"},
		ast.MemberExpr{Base: leaf, Name: "x"},
		ast.IndexExpr{Base: leaf, Items: []ast.Expr{leaf}},
		ast.StringExpr{Value: "x"},
		ast.NumberExpr{Raw: "1", Int: true, IntValue: 1},
		ast.BoolExpr{Value: true},
		ast.ListExpr{Items: []ast.Expr{leaf}},
		ast.TupleExpr{Items: []ast.Expr{leaf}},
		ast.DictExpr{Entries: []ast.DictEntryExpr{{Key: leaf, Value: leaf}}},
		ast.RangeExpr{Start: leaf, Stop: leaf},
		ast.CallExpr{Callee: leaf},
		ast.FunctionExpr{},
		ast.AliasExpr{Expr: leaf, Alias: "y"},
		ast.UnaryExpr{Op: "-", Expr: leaf},
		ast.BinaryExpr{Left: leaf, Op: "+", Right: leaf},
		ast.CompareExpr{Left: leaf, Op: "==", Right: leaf},
		ast.ConditionalExpr{Then: leaf, Cond: leaf, Else: leaf},
	}
	for _, expr := range cases {
		got := exprWithSpan(expr, span)
		if got.GetSpan() != span {
			t.Fatalf("expected rewritten span for %T, got %#v", expr, got.GetSpan())
		}
	}
	if got := exprWithSpan(nil, span); got != nil {
		t.Fatalf("expected nil fallback to stay nil, got %#v", got)
	}
}

func TestParseLogicalOperatorAliasesCanonicalized(t *testing.T) {
	tests := []struct {
		src    string
		wantOp string
	}{
		{src: "a & b", wantOp: "&"},
		{src: "a && b", wantOp: "&"},
		{src: "a and b", wantOp: "&"},
		{src: "a | b", wantOp: "|"},
		{src: "a || b", wantOp: "|"},
		{src: "a or b", wantOp: "|"},
	}
	for _, tc := range tests {
		diags := &diag.Diagnostics{}
		tp := parseExprTP(tc.src, diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors for %q: %s", tc.src, diags.String())
		}
		bin, ok := expr.(ast.BinaryExpr)
		if !ok || bin.Op != tc.wantOp {
			t.Fatalf("expected canonical op %q for %q, got %#v", tc.wantOp, tc.src, expr)
		}
	}
}

func TestCanonicalLogicalOpRejectsOtherTokens(t *testing.T) {
	if op, ok := canonicalLogicalOp(lexer.TokenPlus); ok || op != "" {
		t.Fatalf("expected non-logical token to be rejected, got op=%q ok=%v", op, ok)
	}
}

func TestParseRangeExprShortcut(t *testing.T) {
	tests := []struct {
		src     string
		hasStep bool
	}{
		{src: "1:10"},
		{src: "-1:2"},
		{src: "1:10:2", hasStep: true},
		{src: "10:-2:-2", hasStep: true},
		{src: "0.1:10:0.5", hasStep: true},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			expr := tp.parseExpr()
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors: %s", diags.String())
			}
			r, ok := expr.(ast.RangeExpr)
			if !ok {
				t.Fatalf("expected RangeExpr, got %#v", expr)
			}
			if (r.Step != nil) != tc.hasStep {
				t.Fatalf("unexpected step presence for %q", tc.src)
			}
			if r.Start == nil || r.Stop == nil {
				t.Fatalf("expected start and stop expressions, got %#v", r)
			}
		})
	}
}

func TestParseRangeExprPrecedence(t *testing.T) {
	t.Run("binds tighter than product", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("0:2 * 4", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		top, ok := expr.(ast.BinaryExpr)
		if !ok || top.Op != "*" {
			t.Fatalf("expected top-level multiplication, got %#v", expr)
		}
		if _, ok := top.Left.(ast.RangeExpr); !ok {
			t.Fatalf("expected range on multiplication lhs, got %#v", top.Left)
		}
		if _, ok := top.Right.(ast.NumberExpr); !ok {
			t.Fatalf("expected number on multiplication rhs, got %#v", top.Right)
		}
	})

	t.Run("grouped range still binds as product lhs", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("(0:2) * 4", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		top, ok := expr.(ast.BinaryExpr)
		if !ok || top.Op != "*" {
			t.Fatalf("expected top-level multiplication, got %#v", expr)
		}
		if _, ok := top.Left.(ast.RangeExpr); !ok {
			t.Fatalf("expected grouped range on multiplication lhs, got %#v", top.Left)
		}
	})

	t.Run("index applies to completed range", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("0:10[0:10:2]", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		idx, ok := expr.(ast.IndexExpr)
		if !ok {
			t.Fatalf("expected top-level index expression, got %#v", expr)
		}
		if _, ok := idx.Base.(ast.RangeExpr); !ok {
			t.Fatalf("expected indexed base to be range expression, got %#v", idx.Base)
		}
		if len(idx.Items) != 1 {
			t.Fatalf("expected one index selector, got %d", len(idx.Items))
		}
		item, ok := idx.Items[0].(ast.RangeExpr)
		if !ok || item.Step == nil {
			t.Fatalf("expected stepped range index selector, got %#v", idx.Items[0])
		}
	})

	t.Run("range bound keeps call suffix", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("0:len(xs)", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		r, ok := expr.(ast.RangeExpr)
		if !ok {
			t.Fatalf("expected range expression, got %#v", expr)
		}
		call, ok := r.Stop.(ast.CallExpr)
		if !ok {
			t.Fatalf("expected call as range stop, got %#v", r.Stop)
		}
		callee, ok := call.Callee.(ast.IdentExpr)
		if !ok || callee.Name != "len" {
			t.Fatalf("expected len call stop, got %#v", call.Callee)
		}
	})

	t.Run("parenthesized indexed bound remains range stop", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("0:(xs[0])", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		r, ok := expr.(ast.RangeExpr)
		if !ok {
			t.Fatalf("expected range expression, got %#v", expr)
		}
		if _, ok := r.Stop.(ast.IndexExpr); !ok {
			t.Fatalf("expected indexed stop bound, got %#v", r.Stop)
		}
	})
}

func TestParseRangeExprInPostfixAndCalls(t *testing.T) {
	t.Run("index selector", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("xs[1:3]", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		idx, ok := expr.(ast.IndexExpr)
		if !ok || len(idx.Items) != 1 {
			t.Fatalf("expected one index selector, got %#v", expr)
		}
		if _, ok := idx.Items[0].(ast.RangeExpr); !ok {
			t.Fatalf("expected range index selector, got %#v", idx.Items[0])
		}
	})

	t.Run("named call argument", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("table(x = 1:3)", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		call, ok := expr.(ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			t.Fatalf("expected call expression, got %#v", expr)
		}
		if call.Args[0].Name != "x" {
			t.Fatalf("expected named argument x, got %#v", call.Args[0])
		}
		if _, ok := call.Args[0].Expr.(ast.RangeExpr); !ok {
			t.Fatalf("expected range named argument, got %#v", call.Args[0].Expr)
		}
	})

	t.Run("positional call argument", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("map(int, 1:3)", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		call, ok := expr.(ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			t.Fatalf("expected call expression, got %#v", expr)
		}
		if _, ok := call.Args[1].Expr.(ast.RangeExpr); !ok {
			t.Fatalf("expected range positional argument, got %#v", call.Args[1].Expr)
		}
	})
}

func TestParseDictionaryColonStillSeparatesKeyValue(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`{"a": 1:3}`, diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	dict, ok := expr.(ast.DictExpr)
	if !ok || len(dict.Entries) != 1 {
		t.Fatalf("expected one-entry DictExpr, got %#v", expr)
	}
	if _, ok := dict.Entries[0].Key.(ast.StringExpr); !ok {
		t.Fatalf("expected string key, got %#v", dict.Entries[0].Key)
	}
	if _, ok := dict.Entries[0].Value.(ast.RangeExpr); !ok {
		t.Fatalf("expected range value, got %#v", dict.Entries[0].Value)
	}
}

func TestParseRangeExprReportsTrailingColonTokens(t *testing.T) {
	diags := &diag.Diagnostics{}
	expr, ok := ParseStandaloneExpr("expr.jbs", "1:2:3:4", diag.NewPos(0, 1, 1), diags)
	if !ok {
		t.Fatalf("expected standalone expression")
	}
	if _, ok := expr.(ast.RangeExpr); !ok {
		t.Fatalf("expected range expression, got %#v", expr)
	}
	if !hasCode(diags, "E061") {
		t.Fatalf("expected trailing token diagnostic, got: %s", diags.String())
	}
}

func TestParseRangeExprReportsMissingBounds(t *testing.T) {
	t.Run("missing stop", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("1:", diags)
		expr := tp.parseExpr()
		r, ok := expr.(ast.RangeExpr)
		if !ok {
			t.Fatalf("expected range expression fallback, got %#v", expr)
		}
		if _, ok := r.Stop.(ast.StringExpr); !ok {
			t.Fatalf("expected empty string fallback range stop, got %#v", r.Stop)
		}
		if !hasCode(diags, "E058") {
			t.Fatalf("expected E058 for missing range stop, got: %s", diags.String())
		}
	})

	t.Run("missing step", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("1:2:", diags)
		expr := tp.parseExpr()
		r, ok := expr.(ast.RangeExpr)
		if !ok {
			t.Fatalf("expected partial range fallback, got %#v", expr)
		}
		if _, ok := r.Step.(ast.StringExpr); !ok {
			t.Fatalf("expected empty string fallback range step, got %#v", r.Step)
		}
		if !hasCode(diags, "E058") {
			t.Fatalf("expected E058 for missing range step, got: %s", diags.String())
		}
	})
}

func TestParseLogicalAliasPrecedence(t *testing.T) {
	tests := []string{
		"a || b && c",
		"a or b and c",
	}
	for _, src := range tests {
		diags := &diag.Diagnostics{}
		tp := parseExprTP(src, diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors for %q: %s", src, diags.String())
		}
		top, ok := expr.(ast.BinaryExpr)
		if !ok || top.Op != "|" {
			t.Fatalf("expected top-level disjunction for %q, got %#v", src, expr)
		}
		right, ok := top.Right.(ast.BinaryExpr)
		if !ok || right.Op != "&" {
			t.Fatalf("expected conjunction on right side for %q, got %#v", src, top.Right)
		}
	}
}

func TestParseExprAliasPrecedence(t *testing.T) {
	t.Run("a + a as b parses alias on rhs", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("a + a as b", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		top, ok := expr.(ast.BinaryExpr)
		if !ok || top.Op != "+" {
			t.Fatalf("expected top '+' binary, got %#v", expr)
		}
		alias, ok := top.Right.(ast.AliasExpr)
		if !ok {
			t.Fatalf("expected alias expression on rhs, got %#v", top.Right)
		}
		if alias.Alias != "b" {
			t.Fatalf("expected alias name b, got %q", alias.Alias)
		}
		id, ok := alias.Expr.(ast.IdentExpr)
		if !ok || id.Name != "a" {
			t.Fatalf("expected aliased identifier a, got %#v", alias.Expr)
		}
	})

	t.Run("(a + a) as b parses alias on parenthesized expression", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("(a + a) as b", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		alias, ok := expr.(ast.AliasExpr)
		if !ok {
			t.Fatalf("expected top-level alias expression, got %#v", expr)
		}
		if alias.Alias != "b" {
			t.Fatalf("expected alias name b, got %q", alias.Alias)
		}
		if _, ok := alias.Expr.(ast.BinaryExpr); !ok {
			t.Fatalf("expected aliased binary expression, got %#v", alias.Expr)
		}
	})
}

func TestParseConditionalExpressions(t *testing.T) {
	t.Run("valid conditional", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("x if cond else y", diags)
		expr := tp.parseExpr()
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
		if _, ok := expr.(ast.ConditionalExpr); !ok {
			t.Fatalf("expected conditional expression, got %T", expr)
		}
	})

	t.Run("missing else reports E052", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("x if cond", diags)
		expr := tp.parseExpr()
		if _, ok := expr.(ast.ConditionalExpr); !ok {
			t.Fatalf("expected conditional expression even on missing else, got %T", expr)
		}
		if !hasCode(diags, "E052") {
			t.Fatalf("expected E052, got: %s", diags.String())
		}
	})
}

func TestParsePrimaryBoolModeCallAndQualified(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want func(t *testing.T, expr ast.Expr)
	}{
		{
			name: "bool true literal",
			src:  "true",
			want: func(t *testing.T, expr ast.Expr) {
				b, ok := expr.(ast.BoolExpr)
				if !ok || !b.Value {
					t.Fatalf("expected bool true, got %#v", expr)
				}
			},
		},
		{
			name: "bool false literal with uppercase",
			src:  "FALSE",
			want: func(t *testing.T, expr ast.Expr) {
				b, ok := expr.(ast.BoolExpr)
				if !ok || b.Value {
					t.Fatalf("expected bool false, got %#v", expr)
				}
			},
		},
		{
			name: "bool true literal with uppercase",
			src:  "TRUE",
			want: func(t *testing.T, expr ast.Expr) {
				b, ok := expr.(ast.BoolExpr)
				if !ok || !b.Value {
					t.Fatalf("expected bool true, got %#v", expr)
				}
			},
		},
		{
			name: "kernel call list",
			src:  "list(1)",
			want: func(t *testing.T, expr ast.Expr) {
				c, ok := expr.(ast.CallExpr)
				if !ok {
					t.Fatalf("expected CallExpr, got %#v", expr)
				}
				callee, ok := c.Callee.(ast.IdentExpr)
				if !ok || callee.Name != "list" {
					t.Fatalf("expected call callee list, got %#v", c.Callee)
				}
				if len(c.Args) != 1 {
					t.Fatalf("expected 1 arg, got %d", len(c.Args))
				}
				if c.Args[0].Name != "" {
					t.Fatalf("expected positional arg, got %#v", c.Args[0])
				}
				if _, ok := c.Args[0].Expr.(ast.NumberExpr); !ok {
					t.Fatalf("expected numeric positional arg expr, got %#v", c.Args[0].Expr)
				}
			},
		},
		{
			name: "kernel call range",
			src:  "range(0,10,2)",
			want: func(t *testing.T, expr ast.Expr) {
				c, ok := expr.(ast.CallExpr)
				if !ok {
					t.Fatalf("expected CallExpr, got %#v", expr)
				}
				callee, ok := c.Callee.(ast.IdentExpr)
				if !ok || callee.Name != "range" {
					t.Fatalf("expected call callee range, got %#v", c.Callee)
				}
				if len(c.Args) != 3 {
					t.Fatalf("expected 3 args, got %d", len(c.Args))
				}
				for i, arg := range c.Args {
					if arg.Name != "" {
						t.Fatalf("expected positional arg %d, got %#v", i, arg)
					}
				}
			},
		},
		{
			name: "qualified identifier",
			src:  "ns.value",
			want: func(t *testing.T, expr ast.Expr) {
				q, ok := expr.(ast.QualifiedIdentExpr)
				if !ok || q.Namespace != "ns" || q.Name != "value" {
					t.Fatalf("expected qualified identifier ns.value, got %#v", expr)
				}
			},
		},
		{
			name: "qualified identifier missing member reports E064",
			src:  "ns.",
			want: func(t *testing.T, expr ast.Expr) {
				q, ok := expr.(ast.QualifiedIdentExpr)
				if !ok || q.Namespace != "ns" {
					t.Fatalf("expected partial qualified identifier, got %#v", expr)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			expr := tp.parseExpr()
			tc.want(t, expr)
			if tc.name == "qualified identifier missing member reports E064" {
				if !hasCode(diags, "E064") {
					t.Fatalf("expected E064, got: %s", diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors: %s", diags.String())
			}
		})
	}
}

func TestParseAliasMissingIdentifierReportsE058(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`a as "b"`, diags)
	expr := tp.parseExpr()
	if _, ok := expr.(ast.AliasExpr); !ok {
		t.Fatalf("expected alias expression fallback, got %#v", expr)
	}
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for missing alias identifier, got: %s", diags.String())
	}
}

func TestParsePrimaryNumberAndFallbackErrors(t *testing.T) {
	t.Run("integer overflow reports E065", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("9223372036854775808", diags)
		expr := tp.parseExpr()
		num, ok := expr.(ast.NumberExpr)
		if !ok || !num.Int {
			t.Fatalf("expected int NumberExpr, got %#v", expr)
		}
		if !hasCode(diags, "E065") {
			t.Fatalf("expected E065, got: %s", diags.String())
		}
	})

	t.Run("scientific notation is parsed as float", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("1e3", diags)
		expr := tp.parseExpr()
		num, ok := expr.(ast.NumberExpr)
		if !ok || num.Int {
			t.Fatalf("expected float NumberExpr, got %#v", expr)
		}
		if num.FloatValue != 1000 {
			t.Fatalf("unexpected float value: got=%v want=1000", num.FloatValue)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("leading dot scientific notation parses as float", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP(".1E-12", diags)
		expr := tp.parseExpr()
		num, ok := expr.(ast.NumberExpr)
		if !ok || num.Int {
			t.Fatalf("expected float NumberExpr, got %#v", expr)
		}
		if math.Abs(num.FloatValue-1e-13) > 1e-20 {
			t.Fatalf("unexpected float value: got=%v want=%v", num.FloatValue, 1e-13)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("unary minus over leading dot float", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("-.2", diags)
		expr := tp.parseExpr()
		unary, ok := expr.(ast.UnaryExpr)
		if !ok || unary.Op != "-" {
			t.Fatalf("expected unary minus expression, got %#v", expr)
		}
		num, ok := unary.Expr.(ast.NumberExpr)
		if !ok || num.Int {
			t.Fatalf("expected float NumberExpr as unary child, got %#v", unary.Expr)
		}
		if math.Abs(num.FloatValue-0.2) > 1e-15 {
			t.Fatalf("unexpected float value: got=%v want=0.2", num.FloatValue)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("invalid float token reports E066", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		bad := lexer.Token{
			Type:  lexer.TokenNumber,
			Text:  "1.2.3",
			Value: "1.2.3",
			Span:  diag.NewSpan("expr.jbs", diag.NewPos(0, 1, 1), diag.NewPos(5, 1, 6)),
		}
		eof := lexer.Token{Type: lexer.TokenEOF, Span: bad.Span}
		tp := &tokenParser{tokens: []lexer.Token{bad, eof}, diags: diags}
		expr := tp.parseExpr()
		num, ok := expr.(ast.NumberExpr)
		if !ok || num.Int {
			t.Fatalf("expected float NumberExpr, got %#v", expr)
		}
		if !hasCode(diags, "E066") {
			t.Fatalf("expected E066, got: %s", diags.String())
		}
	})

	t.Run("unexpected token fallback reports E058", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		bad := lexer.Token{
			Type:  lexer.TokenEOF,
			Text:  "",
			Value: "",
			Span:  diag.NewSpan("expr.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 1)),
		}
		tp := &tokenParser{tokens: []lexer.Token{bad}, diags: diags}
		expr := tp.parseExpr()
		s, ok := expr.(ast.StringExpr)
		if !ok || s.Value != "" {
			t.Fatalf("expected empty StringExpr fallback, got %#v", expr)
		}
		if !hasCode(diags, "E058") {
			t.Fatalf("expected E058, got: %s", diags.String())
		}
	})
}

func TestParseCallMalformedSyntax(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "missing closing paren", src: "range(1,2"},
		{name: "empty middle arg", src: "range(1,,3)"},
		{name: "empty first arg", src: "rev(,)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			_ = tp.parseExpr()
			if !diags.HasErrors() {
				t.Fatalf("expected parse errors for %q", tc.src)
			}
		})
	}
}

func TestParsePrimaryTupleAndListBranches(t *testing.T) {
	t.Run("empty tuple", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("()", diags)
		expr := tp.parseExpr()
		tuple, ok := expr.(ast.TupleExpr)
		if !ok {
			t.Fatalf("expected tuple expr, got %#v", expr)
		}
		if len(tuple.Items) != 0 {
			t.Fatalf("expected empty tuple items, got %#v", tuple.Items)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("tuple with trailing comma", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("(1,2,)", diags)
		expr := tp.parseExpr()
		tuple, ok := expr.(ast.TupleExpr)
		if !ok || len(tuple.Items) != 2 {
			t.Fatalf("expected tuple with two items, got %#v", expr)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("grouped expression missing close reports E054", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("(1+2", diags)
		expr := tp.parseExpr()
		if _, ok := expr.(ast.BinaryExpr); !ok {
			t.Fatalf("expected grouped binary expression, got %#v", expr)
		}
		if !hasCode(diags, "E054") {
			t.Fatalf("expected E054, got: %s", diags.String())
		}
	})

	t.Run("tuple missing close reports E053", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("(1,2", diags)
		expr := tp.parseExpr()
		if _, ok := expr.(ast.TupleExpr); !ok {
			t.Fatalf("expected tuple expression, got %#v", expr)
		}
		if !hasCode(diags, "E053") {
			t.Fatalf("expected E053, got: %s", diags.String())
		}
	})

	t.Run("list with trailing comma", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("[1,2,]", diags)
		expr := tp.parseExpr()
		list, ok := expr.(ast.ListExpr)
		if !ok || len(list.Items) != 2 {
			t.Fatalf("expected list with two items, got %#v", expr)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("list missing close reports E055", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("[1,2", diags)
		expr := tp.parseExpr()
		if _, ok := expr.(ast.ListExpr); !ok {
			t.Fatalf("expected list expression, got %#v", expr)
		}
		if !hasCode(diags, "E055") {
			t.Fatalf("expected E055, got: %s", diags.String())
		}
	})
}

func TestParseDictionaryLiteral(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "empty", src: "{}", want: 0},
		{name: "string and int keys", src: `{"a": 1, 2: "two"}`, want: 2},
		{name: "expression key and nested value", src: `{name: {"inner": [1, (2,)]}}`, want: 1},
		{name: "trailing comma", src: `{"a": 1,}`, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			expr := tp.parseExpr()
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors: %s", diags.String())
			}
			dict, ok := expr.(ast.DictExpr)
			if !ok {
				t.Fatalf("expected DictExpr, got %#v", expr)
			}
			if len(dict.Entries) != tc.want {
				t.Fatalf("expected %d entries, got %#v", tc.want, dict.Entries)
			}
		})
	}
}

func TestParseDictionaryMissingColonReportsE058(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`{"a" 1}`, diags)
	_ = tp.parseExpr()
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for missing dictionary colon, got: %s", diags.String())
	}
}

func TestParseDictionaryMissingCloseReportsE055(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`{"a": 1`, diags)
	expr := tp.parseExpr()
	if _, ok := expr.(ast.DictExpr); !ok {
		t.Fatalf("expected dictionary expression fallback, got %#v", expr)
	}
	if !hasCode(diags, "E055") {
		t.Fatalf("expected E055 for missing dictionary close, got: %s", diags.String())
	}
}

func TestParseDictionaryZeroSpanRecovery(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := &tokenParser{
		tokens: []lexer.Token{
			{Type: lexer.TokenLBrace, Text: "{"},
			{Type: lexer.TokenColon, Text: ":"},
			{Type: lexer.TokenNumber, Text: "1", Value: "1"},
			{Type: lexer.TokenRBrace, Text: "}"},
			{Type: lexer.TokenEOF},
		},
		diags: diags,
	}
	expr := tp.parseExpr()
	dict, ok := expr.(ast.DictExpr)
	if !ok || len(dict.Entries) != 1 {
		t.Fatalf("expected one-entry dictionary fallback, got %#v", expr)
	}
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 from malformed dictionary, got: %s", diags.String())
	}
}

func TestParseDictionaryConditionalKeyUsesNoRangeParser(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`{a if ok else b: 1}`, diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	dict, ok := expr.(ast.DictExpr)
	if !ok || len(dict.Entries) != 1 {
		t.Fatalf("expected one-entry dictionary, got %#v", expr)
	}
	if _, ok := dict.Entries[0].Key.(ast.ConditionalExpr); !ok {
		t.Fatalf("expected conditional key, got %#v", dict.Entries[0].Key)
	}
}

func TestParsePostfixChainedQualifiedIdentifier(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("a.b.c", diags)
	expr := tp.parseExpr()
	q, ok := expr.(ast.QualifiedIdentExpr)
	if !ok {
		t.Fatalf("expected qualified identifier, got %#v", expr)
	}
	if q.Namespace != "a.b" || q.Name != "c" {
		t.Fatalf("expected qualified identifier a.b.c, got namespace=%q name=%q", q.Namespace, q.Name)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParsePostfixDotOnNonNamespaceBuildsMemberExpr(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("(1).x", diags)
	expr := tp.parseExpr()
	member, ok := expr.(ast.MemberExpr)
	if !ok {
		t.Fatalf("expected member expression, got %#v", expr)
	}
	if member.Name != "x" {
		t.Fatalf("expected member name x, got %q", member.Name)
	}
	if _, ok := member.Base.(ast.NumberExpr); !ok {
		t.Fatalf("expected number base, got %#v", member.Base)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseIndexExprBranches(t *testing.T) {
	t.Run("index with items and trailing comma", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("a[1,2,]", diags)
		expr := tp.parseExpr()
		idx, ok := expr.(ast.IndexExpr)
		if !ok {
			t.Fatalf("expected index expression, got %#v", expr)
		}
		if _, ok := idx.Base.(ast.IdentExpr); !ok {
			t.Fatalf("expected ident base, got %#v", idx.Base)
		}
		if len(idx.Items) != 2 {
			t.Fatalf("expected 2 index items, got %#v", idx.Items)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("empty index selector list", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("a[]", diags)
		expr := tp.parseExpr()
		idx, ok := expr.(ast.IndexExpr)
		if !ok {
			t.Fatalf("expected index expression, got %#v", expr)
		}
		if len(idx.Items) != 0 {
			t.Fatalf("expected empty index items, got %#v", idx.Items)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("index followed by member access", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("p0[x].x", diags)
		expr := tp.parseExpr()
		member, ok := expr.(ast.MemberExpr)
		if !ok {
			t.Fatalf("expected member expression, got %#v", expr)
		}
		if member.Name != "x" {
			t.Fatalf("expected member name x, got %q", member.Name)
		}
		idx, ok := member.Base.(ast.IndexExpr)
		if !ok {
			t.Fatalf("expected index base, got %#v", member.Base)
		}
		base, ok := idx.Base.(ast.IdentExpr)
		if !ok || base.Name != "p0" {
			t.Fatalf("expected p0 index base, got %#v", idx.Base)
		}
		if len(idx.Items) != 1 {
			t.Fatalf("expected one index selector, got %#v", idx.Items)
		}
		sel, ok := idx.Items[0].(ast.IdentExpr)
		if !ok || sel.Name != "x" {
			t.Fatalf("expected x selector, got %#v", idx.Items[0])
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("nested integer selector list", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("xs[[0,-1]]", diags)
		expr := tp.parseExpr()
		idx, ok := expr.(ast.IndexExpr)
		if !ok {
			t.Fatalf("expected index expression, got %#v", expr)
		}
		if len(idx.Items) != 1 {
			t.Fatalf("expected one selector item, got %#v", idx.Items)
		}
		list, ok := idx.Items[0].(ast.ListExpr)
		if !ok {
			t.Fatalf("expected nested list selector, got %#v", idx.Items[0])
		}
		if len(list.Items) != 2 {
			t.Fatalf("expected two selector values, got %#v", list.Items)
		}
		if first, ok := list.Items[0].(ast.NumberExpr); !ok || !first.Int || first.IntValue != 0 {
			t.Fatalf("expected first selector value 0, got %#v", list.Items[0])
		}
		second, ok := list.Items[1].(ast.UnaryExpr)
		if !ok || second.Op != "-" {
			t.Fatalf("expected second selector value to be unary -1, got %#v", list.Items[1])
		}
		if inner, ok := second.Expr.(ast.NumberExpr); !ok || !inner.Int || inner.IntValue != 1 {
			t.Fatalf("expected unary selector inner value 1, got %#v", second.Expr)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("nested boolean selector list", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("xs[[true,false]]", diags)
		expr := tp.parseExpr()
		idx, ok := expr.(ast.IndexExpr)
		if !ok {
			t.Fatalf("expected index expression, got %#v", expr)
		}
		if len(idx.Items) != 1 {
			t.Fatalf("expected one selector item, got %#v", idx.Items)
		}
		list, ok := idx.Items[0].(ast.ListExpr)
		if !ok {
			t.Fatalf("expected nested list selector, got %#v", idx.Items[0])
		}
		if len(list.Items) != 2 {
			t.Fatalf("expected two selector values, got %#v", list.Items)
		}
		first, ok := list.Items[0].(ast.BoolExpr)
		if !ok || !first.Value {
			t.Fatalf("expected first selector value true, got %#v", list.Items[0])
		}
		second, ok := list.Items[1].(ast.BoolExpr)
		if !ok || second.Value {
			t.Fatalf("expected second selector value false, got %#v", list.Items[1])
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("missing closing bracket reports E055", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("a[1,2", diags)
		expr := tp.parseExpr()
		if _, ok := expr.(ast.IndexExpr); !ok {
			t.Fatalf("expected index expression fallback, got %#v", expr)
		}
		if !hasCode(diags, "E055") {
			t.Fatalf("expected E055, got: %s", diags.String())
		}
	})
}

func TestParseMemberAliasAndProjectionCombExpr(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("p0[x].x as y + p1[x]", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	top, ok := expr.(ast.BinaryExpr)
	if !ok || top.Op != "+" {
		t.Fatalf("expected top-level + binary expression, got %#v", expr)
	}
	leftAlias, ok := top.Left.(ast.AliasExpr)
	if !ok || leftAlias.Alias != "y" {
		t.Fatalf("expected left alias y, got %#v", top.Left)
	}
	leftMember, ok := leftAlias.Expr.(ast.MemberExpr)
	if !ok || leftMember.Name != "x" {
		t.Fatalf("expected left member access .x, got %#v", leftAlias.Expr)
	}
	leftIndex, ok := leftMember.Base.(ast.IndexExpr)
	if !ok {
		t.Fatalf("expected left member base to be index expr, got %#v", leftMember.Base)
	}
	leftBase, ok := leftIndex.Base.(ast.IdentExpr)
	if !ok || leftBase.Name != "p0" {
		t.Fatalf("expected left index base p0, got %#v", leftIndex.Base)
	}
	rightIndex, ok := top.Right.(ast.IndexExpr)
	if !ok {
		t.Fatalf("expected right index expr, got %#v", top.Right)
	}
	rightBase, ok := rightIndex.Base.(ast.IdentExpr)
	if !ok || rightBase.Name != "p1" {
		t.Fatalf("expected right index base p1, got %#v", rightIndex.Base)
	}
}

func TestParseCallArgsBranches(t *testing.T) {
	t.Run("empty call args", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("f()", diags)
		expr := tp.parseExpr()
		call, ok := expr.(ast.CallExpr)
		if !ok {
			t.Fatalf("expected call expression, got %#v", expr)
		}
		if len(call.Args) != 0 {
			t.Fatalf("expected no args, got %#v", call.Args)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("trailing comma call args", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		tp := parseExprTP("f(1,)", diags)
		expr := tp.parseExpr()
		call, ok := expr.(ast.CallExpr)
		if !ok {
			t.Fatalf("expected call expression, got %#v", expr)
		}
		if len(call.Args) != 1 {
			t.Fatalf("expected one arg, got %#v", call.Args)
		}
		if call.Args[0].Name != "" {
			t.Fatalf("expected positional arg, got %#v", call.Args[0])
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})
}

func TestParseAliasMissingIdentifierAtEOFReportsE058(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("a as", diags)
	expr := tp.parseExpr()
	alias, ok := expr.(ast.AliasExpr)
	if !ok {
		t.Fatalf("expected alias expression fallback, got %#v", expr)
	}
	if alias.Alias != "" {
		t.Fatalf("expected empty alias name fallback, got %q", alias.Alias)
	}
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for missing alias identifier at EOF, got: %s", diags.String())
	}
}

func TestParseFunctionLiteral(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("function(x, y = 1) { x + y }", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	fn, ok := expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected function expression, got %#v", expr)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %#v", fn.Params)
	}
	if fn.Params[0].Name != "x" || fn.Params[0].Default != nil {
		t.Fatalf("unexpected first param: %#v", fn.Params[0])
	}
	if fn.Params[1].Name != "y" {
		t.Fatalf("unexpected second param: %#v", fn.Params[1])
	}
	if _, ok := fn.Params[1].Default.(ast.NumberExpr); !ok {
		t.Fatalf("expected numeric default for y, got %#v", fn.Params[1].Default)
	}
	if len(fn.Body) != 1 {
		t.Fatalf("expected one body statement, got %#v", fn.Body)
	}
	stmt, ok := fn.Body[0].(ast.ExprStmt)
	if !ok {
		t.Fatalf("expected trailing expr stmt, got %#v", fn.Body[0])
	}
	if _, ok := stmt.Expr.(ast.BinaryExpr); !ok {
		t.Fatalf("expected binary expr body, got %#v", stmt.Expr)
	}
}

func TestParseFunctionRestParameters(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("function(a, b = 1, *args, **kwargs) { args }", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	fn, ok := expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected function expression, got %#v", expr)
	}
	if len(fn.Params) != 4 {
		t.Fatalf("expected 4 params, got %#v", fn.Params)
	}
	if fn.Params[0].Kind != ast.FuncParamValue || fn.Params[0].Name != "a" {
		t.Fatalf("unexpected first param: %#v", fn.Params[0])
	}
	if fn.Params[1].Kind != ast.FuncParamValue || fn.Params[1].Name != "b" || fn.Params[1].Default == nil {
		t.Fatalf("unexpected second param: %#v", fn.Params[1])
	}
	if fn.Params[2].Kind != ast.FuncParamArgs || fn.Params[2].Name != "args" {
		t.Fatalf("unexpected *args param: %#v", fn.Params[2])
	}
	if fn.Params[3].Kind != ast.FuncParamKwargs || fn.Params[3].Name != "kwargs" {
		t.Fatalf("unexpected **kwargs param: %#v", fn.Params[3])
	}
}

func TestParseFunctionRestParameterDiagnostics(t *testing.T) {
	tests := []struct {
		src  string
		code string
	}{
		{src: "function(*args, x) { x }", code: "E058"},
		{src: "function(**kwargs, x) { x }", code: "E058"},
		{src: "function(**kwargs, *args) { args }", code: "E058"},
		{src: "function(*args, *more) { args }", code: "E058"},
		{src: "function(**a, **b) { a }", code: "E058"},
		{src: "function(*args = []) { args }", code: "E058"},
		{src: "function(a = 1, b) { b }", code: "E058"},
		{src: "function(a, a) { a }", code: "E058"},
		{src: "function(,) { 1 }", code: "E050"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			_ = tp.parseExpr()
			if !hasCode(diags, tc.code) {
				t.Fatalf("expected %s for %q, got: %s", tc.code, tc.src, diags.String())
			}
		})
	}
}

func TestParseFunctionParameterTrailingComma(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("function(a,) { a }", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	fn, ok := expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected function expression, got %#v", expr)
	}
	if len(fn.Params) != 1 || fn.Params[0].Name != "a" {
		t.Fatalf("expected one parameter a, got %#v", fn.Params)
	}
}

func TestParseFunctionLiteralMissingBodyBraceReportsE025(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("function(x) x", diags)
	expr := tp.parseExpr()
	fn, ok := expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected function expression fallback, got %#v", expr)
	}
	if len(fn.Body) != 0 {
		t.Fatalf("expected empty body when opening brace is missing, got %#v", fn.Body)
	}
	if !hasCode(diags, "E025") {
		t.Fatalf("expected E025 for missing function body brace, got: %s", diags.String())
	}
}

func TestParseFunctionLiteralZeroSpanRecovery(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := &tokenParser{
		tokens: []lexer.Token{
			{Type: lexer.TokenFunction, Text: "function"},
			{Type: lexer.TokenLParen, Text: "("},
			{Type: lexer.TokenRParen, Text: ")"},
			{Type: lexer.TokenEOF},
		},
		diags: diags,
	}
	expr := tp.parseExpr()
	fn, ok := expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected function expression fallback, got %#v", expr)
	}
	if !fn.Span.IsZero() {
		t.Fatalf("expected zero span fallback with zero-span tokens, got %+v", fn.Span)
	}
	if !hasCode(diags, "E025") {
		t.Fatalf("expected E025 for missing function body brace, got: %s", diags.String())
	}
}

func TestParseFunctionIfConditionWithDictionaryLiteral(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP(`function(d) { if d == {"a": 1} { return true } else { return false } }`, diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	fn, ok := expr.(ast.FunctionExpr)
	if !ok || len(fn.Body) != 1 {
		t.Fatalf("expected function with one body statement, got %#v", expr)
	}
	stmt, ok := fn.Body[0].(ast.FuncIfStmt)
	if !ok {
		t.Fatalf("expected function if statement, got %#v", fn.Body[0])
	}
	cmp, ok := stmt.Cond.(ast.CompareExpr)
	if !ok {
		t.Fatalf("expected compare condition, got %#v", stmt.Cond)
	}
	if _, ok := cmp.Right.(ast.DictExpr); !ok {
		t.Fatalf("expected dictionary literal on right side, got %#v", cmp.Right)
	}
}

func TestParseNestedFunctionLiteralInsideFunctionBody(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("function(x) { inner = function(y) { return y }\ninner }", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	fn, ok := expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected function expression, got %#v", expr)
	}
	if len(fn.Body) != 2 {
		t.Fatalf("expected two body statements, got %#v", fn.Body)
	}
	assign, ok := fn.Body[0].(ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("expected local assignment, got %#v", fn.Body[0])
	}
	inner, ok := assign.Expr.(ast.FunctionExpr)
	if !ok {
		t.Fatalf("expected nested function rhs, got %#v", assign.Expr)
	}
	if len(inner.Body) != 1 {
		t.Fatalf("expected one inner body statement, got %#v", inner.Body)
	}
	if _, ok := inner.Body[0].(ast.ReturnStmt); !ok {
		t.Fatalf("expected return stmt inside nested function, got %#v", inner.Body[0])
	}
}

func TestParseReturnOutsideFunctionBodyReportsE058(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("return x", diags)
	_ = tp.parseExpr()
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for return outside function body, got: %s", diags.String())
	}
}

func TestParseFunctionBodyRejectsUnsupportedStatements(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("function() { use lib }", diags)
	_ = tp.parseExpr()
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for unsupported function body statement, got: %s", diags.String())
	}
}

func TestParseFunctionBodyMalformedStatementTails(t *testing.T) {
	tests := []struct {
		name string
		src  string
		code string
	}{
		{name: "else without brace and not if", src: "function() { if true { 1 } else x }", code: "E080"},
		{name: "else missing closing brace preserves previous span", src: "function() { if true { 1 } else { 2 ", code: "E025"},
		{name: "break with trailing expression", src: "function() { while true { break 1 } }", code: "E061"},
		{name: "assignment with trailing expression", src: "function() { x = 1 2 }", code: "E061"},
		{name: "return with trailing expression", src: "function() { return 1 2 }", code: "E061"},
		{name: "expression with trailing expression", src: "function() { x y }", code: "E061"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			tp := parseExprTP(tc.src, diags)
			_ = tp.parseExpr()
			if !hasCode(diags, tc.code) {
				t.Fatalf("expected %s for %q, got: %s", tc.code, tc.src, diags.String())
			}
		})
	}
}

func TestParseLocalAssignStmtReportsMissingOperatorWhenCalledDirectly(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("x y", diags)
	stmt := tp.parseLocalAssignStmt()
	assign, ok := stmt.(ast.LocalAssignStmt)
	if !ok || assign.Name != "x" || assign.Expr != nil {
		t.Fatalf("expected fallback local assignment, got %#v", stmt)
	}
	if !hasCode(diags, "E051") {
		t.Fatalf("expected E051 for missing local assignment operator, got: %s", diags.String())
	}
}

func TestParseNamedCallArguments(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("f(1, b = 2)", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	call, ok := expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression, got %#v", expr)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected two call args, got %#v", call.Args)
	}
	if call.Args[0].Name != "" {
		t.Fatalf("expected first arg to be positional, got %#v", call.Args[0])
	}
	if call.Args[0].EffectiveKind() != ast.CallArgPositional {
		t.Fatalf("expected first arg kind positional, got %#v", call.Args[0])
	}
	if call.Args[1].Name != "b" {
		t.Fatalf("expected second arg to be named b, got %#v", call.Args[1])
	}
	if call.Args[1].EffectiveKind() != ast.CallArgNamed {
		t.Fatalf("expected second arg kind named, got %#v", call.Args[1])
	}
}

func TestParseCallArgumentSpreads(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("f(1, *args, x = 2, **kwargs)", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	call, ok := expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression, got %#v", expr)
	}
	if len(call.Args) != 4 {
		t.Fatalf("expected four call args, got %#v", call.Args)
	}
	want := []ast.CallArgKind{
		ast.CallArgPositional,
		ast.CallArgPositionalSpread,
		ast.CallArgNamed,
		ast.CallArgKeywordSpread,
	}
	for i, kind := range want {
		if call.Args[i].EffectiveKind() != kind {
			t.Fatalf("arg %d kind=%v want %v: %#v", i, call.Args[i].EffectiveKind(), kind, call.Args[i])
		}
	}
}

func TestParseNamedCallArgumentsRejectPositionalAfterNamed(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("f(a = 1, 2)", diags)
	_ = tp.parseExpr()
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for positional-after-named call args, got: %s", diags.String())
	}
}

func TestParseNamedCallArgumentsRejectDuplicateNames(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("f(a = 1, a = 2)", diags)
	_ = tp.parseExpr()
	if !hasCode(diags, "E058") {
		t.Fatalf("expected E058 for duplicate named call args, got: %s", diags.String())
	}
}

func TestParseInlineAnonymousCallee(t *testing.T) {
	diags := &diag.Diagnostics{}
	tp := parseExprTP("(function(x) { x + 1 })(1)", diags)
	expr := tp.parseExpr()
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	call, ok := expr.(ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression, got %#v", expr)
	}
	if _, ok := call.Callee.(ast.FunctionExpr); !ok {
		t.Fatalf("expected function literal callee, got %#v", call.Callee)
	}
}

func TestIsDecimalIntegerLiteralEmptyString(t *testing.T) {
	if isDecimalIntegerLiteral("") {
		t.Fatalf("expected empty string to be non-integer literal")
	}
}
