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
			"use syntax: name = expression [in \"filename\" | in re\"file-regex\"]",
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
			"use syntax: name = expression [in \"filename\" | in re\"file-regex\"]",
		)
		tp.consumeUntilStmtEnd()
		return ast.AnalyseAssign{}
	}

	expr := tp.parseExpr()

	fileName := ""
	var fileTarget ast.AnalyseFileTarget
	span := diag.Merge(nameTok.Span, expr.GetSpan())
	if tp.peek().Type == lexer.TokenIn {
		tp.next()
		target, ok := parseAnalyseFileTarget(
			tp,
			diag.CodeE416,
			"malformed analyse extraction; expected quoted file name or regex file target after 'in'",
			"use syntax: alias = expression in \"filename\" or alias = expression in re\"file-regex\"",
			diags,
		)
		if !ok {
			tp.consumeUntilStmtEnd()
			return ast.AnalyseAssign{}
		}
		fileTarget = target
		fileName = target.Value
		span = diag.Merge(nameTok.Span, target.Span)
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
		Name:       nameTok.Value,
		Op:         op,
		Expr:       expr,
		File:       fileName,
		FileTarget: fileTarget,
		Span:       span,
	}
}

func parseAnalyseFileTarget(tp *tokenParser, code diag.Code, message, hint string, diags *diag.Diagnostics) (ast.AnalyseFileTarget, bool) {
	tok := tp.peek()
	switch tok.Type {
	case lexer.TokenString:
		tp.next()
		return ast.ExactAnalyseFile(tok.Value, tok.Span), true
	case lexer.TokenRegexString:
		tp.next()
		return ast.RegexAnalyseFile(tok.Value, tok.Span), true
	default:
		if tok.Type == lexer.TokenIdent && tok.Value == "re" {
			hint = `write regex file targets without whitespace, for example re"job.*"`
		}
		diags.AddError(code, message, tok.Span, hint)
		return ast.AnalyseFileTarget{}, false
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
		if tok.Type == lexer.TokenComma {
			diags.AddError(diag.CodeE417,
				"expected column identifier in analyse result tuple",
				tok.Span,
				"use syntax: (name, other as \"Title\", \"pattern %f\" in \"file\" as \"Title\")",
			)
			tp.next()
			continue
		}

		col, ok := parseAnalyseTupleColumn(tp, file, diags)
		if ok {
			columns = append(columns, col)
		}

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

func parseAnalyseTupleColumn(tp *tokenParser, file string, diags *diag.Diagnostics) (ast.AnalyseColumn, bool) {
	if col, ok := parseAnalyseNamedColumnIfPresent(tp, diags); ok {
		return col, true
	}
	return parseAnalyseInlinePatternColumn(tp, file, diags)
}

func parseAnalyseNamedColumnIfPresent(tp *tokenParser, diags *diag.Diagnostics) (ast.AnalyseColumn, bool) {
	mark := tp.idx
	first := tp.peek()
	if first.Type != lexer.TokenIdent {
		return ast.AnalyseColumn{}, false
	}

	nameTok := tp.next()
	name, span := tp.parseQualifiedNameAfterFirst(nameTok, diag.CodeE417, "expected identifier after '.' in analyse result tuple")
	switch tp.peek().Type {
	case lexer.TokenAs, lexer.TokenComma, lexer.TokenRParen:
		title, itemSpan, ok := parseOptionalAnalyseColumnTitle(tp, span, diags)
		if !ok {
			return ast.AnalyseColumn{}, false
		}
		return ast.AnalyseColumn{
			Kind:  ast.AnalyseColumnNamed,
			Name:  name,
			Title: title,
			Span:  itemSpan,
		}, true
	case lexer.TokenIn, lexer.TokenLParen, lexer.TokenLBracket, lexer.TokenColon,
		lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent,
		lexer.TokenAmp, lexer.TokenAnd, lexer.TokenPipe, lexer.TokenOr,
		lexer.TokenEqEq, lexer.TokenNeq, lexer.TokenLT, lexer.TokenGT, lexer.TokenLE, lexer.TokenGE,
		lexer.TokenIf:
		tp.idx = mark
		return ast.AnalyseColumn{}, false
	default:
		return ast.AnalyseColumn{
			Kind: ast.AnalyseColumnNamed,
			Name: name,
			Span: span,
		}, true
	}
}

func parseAnalyseInlinePatternColumn(tp *tokenParser, file string, diags *diag.Diagnostics) (ast.AnalyseColumn, bool) {
	tok := tp.peek()
	if tok.Type == lexer.TokenRParen || tok.Type == lexer.TokenComma || tok.Type == lexer.TokenEOF {
		diags.AddError(diag.CodeE417,
			"expected column identifier or inline pattern in analyse result tuple",
			tok.Span,
			"use syntax: (name, other as \"Title\", \"pattern %f\" in \"file\" as \"Title\")",
		)
		if tok.Type != lexer.TokenEOF {
			tp.next()
		}
		return ast.AnalyseColumn{}, false
	}

	expr := tp.parseExpr()
	if expr == nil {
		return ast.AnalyseColumn{}, false
	}
	return parseAnalyseInlinePatternAfterExpr(tp, expr, expr.GetSpan(), file, diags)
}

func parseAnalyseInlinePatternAfterExpr(tp *tokenParser, expr ast.Expr, span diag.Span, file string, diags *diag.Diagnostics) (ast.AnalyseColumn, bool) {
	inTok := tp.peek()
	if inTok.Type != lexer.TokenIn {
		diags.AddError(diag.CodeE417,
			"expected 'in' after inline analyse pattern expression",
			inTok.Span,
			"use syntax: (\"pattern %f\" in \"file\" as \"Title\")",
		)
		return ast.AnalyseColumn{}, false
	}
	tp.next()

	target, ok := parseAnalyseFileTarget(
		tp,
		diag.CodeE417,
		"expected quoted file name or regex file target after 'in' in analyse result tuple",
		"use syntax: (\"pattern %f\" in \"file\") or (\"pattern %f\" in re\"file-regex\")",
		diags,
	)
	if !ok {
		tp.consumeUntilNewline()
		return ast.AnalyseColumn{}, false
	}

	title, itemSpan, ok := parseOptionalAnalyseColumnTitle(tp, diag.Merge(span, target.Span), diags)
	if !ok {
		return ast.AnalyseColumn{}, false
	}
	return ast.AnalyseColumn{
		Kind:       ast.AnalyseColumnInlinePattern,
		Expr:       expr,
		File:       target.Value,
		FileTarget: target,
		Title:      title,
		Span:       itemSpan,
	}, true
}

func parseOptionalAnalyseColumnTitle(tp *tokenParser, span diag.Span, diags *diag.Diagnostics) (string, diag.Span, bool) {
	if tp.peek().Type != lexer.TokenAs {
		return "", span, true
	}
	tp.next()
	titleTok := tp.peek()
	if titleTok.Type != lexer.TokenString {
		diags.AddError(diag.CodeE417,
			"expected quoted title after 'as' in analyse result tuple",
			titleTok.Span,
			"use syntax: name as \"Title\" or \"pattern\" in \"file\" as \"Title\"",
		)
		tp.consumeUntilNewline()
		return "", span, false
	}
	tp.next()
	return titleTok.Value, diag.Merge(span, titleTok.Span), true
}

func (p *tokenParser) parseQualifiedNameAfterFirst(first lexer.Token, code diag.Code, message string) (string, diag.Span) {
	name := first.Value
	span := first.Span
	for p.peek().Type == lexer.TokenDot {
		p.next()
		partTok := p.expect(lexer.TokenIdent, code, message)
		name += "." + partTok.Value
		span = diag.Merge(span, partTok.Span)
		if partTok.Type != lexer.TokenIdent {
			break
		}
	}
	return name, span
}
