package parser

import (
	"math"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
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
		name string
		src  string
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
			name: "and binds tighter than or",
			src:  "a and b or c",
			check: func(t *testing.T, expr ast.Expr) {
				top, ok := expr.(ast.BinaryExpr)
				if !ok || top.Op != "or" {
					t.Fatalf("expected top 'or' binary, got %#v", expr)
				}
				left, ok := top.Left.(ast.BinaryExpr)
				if !ok || left.Op != "and" {
					t.Fatalf("expected left 'and' binary, got %#v", top.Left)
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

func TestParsePrimaryBoolModeConvertAndQualified(t *testing.T) {
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
			src:  "False",
			want: func(t *testing.T, expr ast.Expr) {
				b, ok := expr.(ast.BoolExpr)
				if !ok || b.Value {
					t.Fatalf("expected bool false, got %#v", expr)
				}
			},
		},
		{
			name: "mode expr shell",
			src:  `shell("x")`,
			want: func(t *testing.T, expr ast.Expr) {
				m, ok := expr.(ast.ModeExpr)
				if !ok || m.Mode != "shell" {
					t.Fatalf("expected shell ModeExpr, got %#v", expr)
				}
			},
		},
		{
			name: "conversion expr list",
			src:  "list(1)",
			want: func(t *testing.T, expr ast.Expr) {
				c, ok := expr.(ast.ConvertExpr)
				if !ok || c.Target != "list" {
					t.Fatalf("expected list ConvertExpr, got %#v", expr)
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
