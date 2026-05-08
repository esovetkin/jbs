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

func TestParseKeywordLogicalOperatorsRejected(t *testing.T) {
	tests := []string{
		"a and b",
		"a or b",
	}
	for _, src := range tests {
		diags := &diag.Diagnostics{}
		tp := parseExprTP(src, diags)
		_ = tp.parseExpr()
		if !hasCode(diags, "E058") {
			t.Fatalf("expected E058 for %q, got: %s", src, diags.String())
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
	if call.Args[1].Name != "b" {
		t.Fatalf("expected second arg to be named b, got %#v", call.Args[1])
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
