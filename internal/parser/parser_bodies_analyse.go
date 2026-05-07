// parse analyse block statement bodies
//
// tokenize analyse body text, parse assignment/extraction statements
// and the final result tuple, and emit analyse-specific diagnostics
// for malformed statements.
package parser

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
)

func parseAnalyseBody(file, body string, start diag.Position, diags *diag.Diagnostics) ([]ast.AnalyseAssign, []ast.AnalyseColumn) {
	tokens := lexer.LexFrom(file, body, start, diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	assignments := make([]ast.AnalyseAssign, 0)
	var columns []ast.AnalyseColumn

	for {
		tp.skipStmtSeparators()
		tok := tp.peek()
		if tok.Type == lexer.TokenEOF {
			break
		}
		if tok.Type == lexer.TokenLParen {
			columns = parseAnalyseTuple(tp, file, diags)
			tp.skipStmtSeparators()
			if tp.peek().Type != lexer.TokenEOF {
				diags.AddError(diag.CodeE417,
					"unexpected tokens after analyse result tuple",
					tp.peek().Span,
					"result tuple must be the last statement in analyse block",
				)
			}
			break
		}
		assign := parseAnalyseAssignment(tp, file, diags)
		if assign.Name != "" {
			assignments = append(assignments, assign)
		}
	}

	if columns == nil {
		diags.AddError(diag.CodeE417,
			"analyse block missing final result tuple",
			diag.NewSpan(file, start, start),
			"add a final tuple like (a, x, p0)",
		)
	}
	return assignments, columns
}

func parseAnalyseAssignment(tp *tokenParser, file string, diags *diag.Diagnostics) ast.AnalyseAssign {
	stmtStart := tp.peek()
	if stmtStart.Type != lexer.TokenIdent {
		diags.AddError(diag.CodeE416,
			"malformed analyse statement; expected 'name = expression' or 'name = expression in \"file\"'",
			stmtStart.Span,
			"use syntax: name = expression [in \"filename\"]",
		)
		tp.consumeUntilStmtEnd()
		return ast.AnalyseAssign{}
	}
	nameTok := tp.next()

	op, _, ok := tp.parseAssignOp()
	if !ok {
		diags.AddError(diag.CodeE416,
			"malformed analyse statement; expected assignment operator after variable name",
			nameTok.Span,
			"use syntax: name = expression [in \"filename\"]",
		)
		tp.consumeUntilStmtEnd()
		return ast.AnalyseAssign{}
	}

	expr := tp.parseExpr()

	fileName := ""
	span := diag.Merge(nameTok.Span, expr.GetSpan())
	if tp.peek().Type == lexer.TokenIn {
		tp.next()
		fileTok := tp.peek()
		if fileTok.Type != lexer.TokenString {
			diags.AddError(diag.CodeE416,
				"malformed analyse extraction; expected quoted file name after 'in'",
				fileTok.Span,
				"use syntax: alias = expression in \"filename\"",
			)
			tp.consumeUntilStmtEnd()
			return ast.AnalyseAssign{}
		}
		tp.next()
		fileName = fileTok.Value
		span = diag.Merge(nameTok.Span, fileTok.Span)
	}

	if tp.peek().Type != lexer.TokenEOF &&
		tp.peek().Type != lexer.TokenNewline &&
		tp.peek().Type != lexer.TokenSemicolon &&
		tp.peek().Type != lexer.TokenComment {
		diags.AddError(diag.CodeE416,
			"unexpected trailing tokens in analyse statement",
			tp.peek().Span,
			"separate statements with newline or ';'",
		)
	}
	tp.consumeUntilStmtEnd()

	return ast.AnalyseAssign{
		Name: nameTok.Value,
		Op:   op,
		Expr: expr,
		File: fileName,
		Span: span,
	}
}

func parseAnalyseTuple(tp *tokenParser, file string, diags *diag.Diagnostics) []ast.AnalyseColumn {
	open := tp.next()
	columns := make([]ast.AnalyseColumn, 0)
	tp.skipNewlines()
	if tp.peek().Type == lexer.TokenRParen {
		tp.next()
		return columns
	}

	for {
		tp.skipNewlines()
		tok := tp.peek()
		if tok.Type == lexer.TokenEOF {
			diags.AddError(diag.CodeE417,
				"unterminated analyse result tuple",
				open.Span,
				"close the tuple with ')'",
			)
			return columns
		}
		if tok.Type == lexer.TokenRParen {
			tp.next()
			return columns
		}
		if tok.Type != lexer.TokenIdent {
			diags.AddError(diag.CodeE417,
				"expected column identifier in analyse result tuple",
				tok.Span,
				"use syntax: (name, other as \"Title\")",
			)
			tp.next()
			continue
		}

		nameTok := tp.next()
		name := nameTok.Value
		span := nameTok.Span
		if tp.peek().Type == lexer.TokenDot {
			tp.next()
			memberTok := tp.expect(lexer.TokenIdent, diag.CodeE417, "expected identifier after '.' in analyse result tuple")
			name = name + "." + memberTok.Value
			span = diag.Merge(span, memberTok.Span)
		}
		title := ""
		if tp.peek().Type == lexer.TokenAs {
			tp.next()
			titleTok := tp.peek()
			if titleTok.Type != lexer.TokenString {
				diags.AddError(diag.CodeE417,
					"expected quoted title after 'as' in analyse result tuple",
					titleTok.Span,
					"use syntax: name as \"Title\"",
				)
				tp.consumeUntilNewline()
				return columns
			}
			tp.next()
			title = titleTok.Value
			span = diag.Merge(span, titleTok.Span)
		}

		columns = append(columns, ast.AnalyseColumn{
			Name:  name,
			Title: title,
			Span:  span,
		})

		tp.skipNewlines()
		if tp.peek().Type == lexer.TokenComma {
			tp.next()
			tp.skipNewlines()
			if tp.peek().Type == lexer.TokenRParen {
				tp.next()
				return columns
			}
			continue
		}
		if tp.peek().Type == lexer.TokenRParen {
			tp.next()
			return columns
		}

		diags.AddError(diag.CodeE417,
			"expected ',' or ')' in analyse result tuple",
			tp.peek().Span,
			"separate tuple items with commas",
		)
		tp.consumeUntilNewline()
		return columns
	}
}
