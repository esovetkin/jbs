// handle top-level statement scanning/parsing helpers
package parser

import (
	"fmt"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

func (p *Parser) isTopLevelAssignmentStart() bool {
	word, ok := p.peekWord()
	if !ok || word == "do" || word == "submit" || word == "analyse" || word == "use" {
		return false
	}
	i := p.off
	for i < len(p.src) {
		r := p.src[i]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			i++
			continue
		}
		break
	}
	for i < len(p.src) {
		r := p.src[i]
		if r == ' ' || r == '\t' || r == '\r' {
			i++
			continue
		}
		if r == '=' {
			return true
		}
		if (r == '+' || r == '-' || r == '*' || r == '/' || r == '%') && i+1 < len(p.src) && p.src[i+1] == '=' {
			return true
		}
		return false
	}
	return false
}

func (p *Parser) parseGlobalAssign(start diag.Position) ast.GlobalAssign {
	stmt, stmtStart := p.readTopLevelStatement()
	tokens := lexer.LexFrom(p.file, stmt, stmtStart, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenIdent || !isAssignToken(tp.peekN(1).Type) {
		tok := tp.peek()
		p.diags.AddError(diag.CodeE012,
			"expected top-level global assignment",
			tok.Span,
			"use syntax: name = expression",
		)
		return ast.GlobalAssign{
			Span: diag.NewSpan(p.file, start, start),
		}
	}
	asn := tp.parseAssignment()
	return ast.GlobalAssign(asn)
}

func (p *Parser) parseTopLevelExprStmt(start diag.Position) ast.ExprStmt {
	stmt, stmtStart := p.readTopLevelStatement()
	tokens := lexer.LexFrom(p.file, stmt, stmtStart, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	expr := tp.parseExpr()
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		tok := tp.peek()
		p.diags.AddError(
			diag.CodeE061,
			"unexpected trailing tokens after expression",
			tok.Span,
			"remove unsupported trailing syntax after the expression",
		)
		tp.consumeUntilStmtEnd()
	}
	span := diag.NewSpan(p.file, start, start)
	if expr != nil {
		span = expr.GetSpan()
	}
	return ast.ExprStmt{
		Expr: expr,
		Span: span,
	}
}

func (p *Parser) parseUseStmt(start diag.Position) ast.UseStmt {
	stmt, stmtStart := p.readTopLevelStatement()
	tokens := lexer.LexFrom(p.file, stmt, stmtStart, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()

	zero := diag.NewSpan(p.file, start, start)
	if tp.peek().Type == lexer.TokenEOF {
		p.diags.AddError(diag.CodeE430,
			"malformed use statement; expected module name, path, or selective import",
			zero,
			"use syntax: use <module> | use \"path.jbs\" as alias | use x,y from <module_or_path>",
		)
		return ast.UseStmt{Span: zero}
	}

	first := tp.peek()
	mergeSpan := func(a diag.Span, b diag.Span) diag.Span {
		if a.IsZero() {
			return b
		}
		return diag.Merge(a, b)
	}

	ensureEOF := func(span diag.Span) bool {
		if tp.peek().Type == lexer.TokenEOF {
			return true
		}
		p.diags.AddError(diag.CodeE430,
			"unexpected trailing tokens in use statement",
			tp.peek().Span,
			"use one use statement per line",
		)
		return false
	}

	parseUseSource := func(tok lexer.Token) (ast.UseSource, bool) {
		switch tok.Type {
		case lexer.TokenIdent:
			return ast.UseSource{
				Kind:  ast.UseSourceBare,
				Value: tok.Value,
				Span:  tok.Span,
			}, true
		case lexer.TokenString:
			return ast.UseSource{
				Kind:  ast.UseSourcePath,
				Value: tok.Value,
				Span:  tok.Span,
			}, true
		default:
			p.diags.AddError(diag.CodeE430,
				"expected module name or quoted path in use statement",
				tok.Span,
				"use an identifier or quoted .jbs path",
			)
			return ast.UseSource{}, false
		}
	}

	if first.Type == lexer.TokenString {
		pathTok := tp.next()
		asTok := tp.peek()
		if asTok.Type != lexer.TokenAs {
			p.diags.AddError(diag.CodeE430,
				"quoted path import requires alias",
				asTok.Span,
				"use syntax: use \"path.jbs\" as alias",
			)
			return ast.UseStmt{
				Source: ast.UseSource{Kind: ast.UseSourcePath, Value: pathTok.Value, Span: pathTok.Span},
				Span:   pathTok.Span,
			}
		}
		tp.next()
		aliasTok := tp.peek()
		if aliasTok.Type != lexer.TokenIdent {
			p.diags.AddError(diag.CodeE430,
				"expected alias identifier after 'as'",
				aliasTok.Span,
				"use syntax: use \"path.jbs\" as alias",
			)
			return ast.UseStmt{
				Source: ast.UseSource{Kind: ast.UseSourcePath, Value: pathTok.Value, Span: pathTok.Span},
				Span:   mergeSpan(pathTok.Span, asTok.Span),
			}
		}
		aliasTok = tp.next()
		span := mergeSpan(pathTok.Span, aliasTok.Span)
		ensureEOF(span)
		return ast.UseStmt{
			Source: ast.UseSource{
				Kind:  ast.UseSourcePath,
				Value: pathTok.Value,
				Span:  pathTok.Span,
			},
			Alias: aliasTok.Value,
			Span:  span,
		}
	}

	if first.Type != lexer.TokenIdent {
		p.diags.AddError(diag.CodeE430,
			"malformed use statement; expected identifier list or quoted path",
			first.Span,
			"use syntax: use <module> | use \"path.jbs\" as alias | use x,y from <module_or_path>",
		)
		return ast.UseStmt{Span: first.Span}
	}

	names := make([]string, 0, 4)
	span := diag.Span{}
	for {
		nameTok := tp.peek()
		if nameTok.Type != lexer.TokenIdent {
			p.diags.AddError(diag.CodeE430,
				"expected identifier in use statement",
				nameTok.Span,
				"use syntax: use x,y from module",
			)
			break
		}
		nameTok = tp.next()
		names = append(names, nameTok.Value)
		span = mergeSpan(span, nameTok.Span)
		if tp.peek().Type != lexer.TokenComma {
			break
		}
		commaTok := tp.next()
		span = mergeSpan(span, commaTok.Span)
	}

	if tp.peek().Type == lexer.TokenFrom {
		fromTok := tp.next()
		srcTok := tp.peek()
		src, ok := parseUseSource(srcTok)
		if !ok {
			return ast.UseStmt{
				Names: names,
				Span:  mergeSpan(span, fromTok.Span),
			}
		}
		tp.next()
		outSpan := mergeSpan(span, src.Span)
		ensureEOF(outSpan)
		return ast.UseStmt{
			Names:  names,
			Source: src,
			Span:   outSpan,
		}
	}

	if len(names) != 1 {
		p.diags.AddError(diag.CodeE430,
			"namespace import accepts exactly one module name",
			span,
			"use syntax: use <module> or use x,y from <module_or_path>",
		)
		return ast.UseStmt{Names: names, Span: span}
	}
	if !ensureEOF(span) {
		return ast.UseStmt{Names: names, Span: span}
	}
	source := ast.UseSource{
		Kind:  ast.UseSourceBare,
		Value: names[0],
		Span:  span,
	}
	return ast.UseStmt{
		Source: source,
		Alias:  names[0],
		Span:   span,
	}
}

func (p *Parser) legacyTopLevelBlockKeyword() (string, bool) {
	stmt, stmtStart := p.peekTopLevelStatement()
	if stmt == "" {
		return "", false
	}
	previewDiags := &diag.Diagnostics{}
	tokens := lexer.LexFrom(p.file, stmt, stmtStart, previewDiags)
	if len(tokens) == 0 {
		return "", false
	}

	idx := 0
	for idx < len(tokens) {
		tt := tokens[idx].Type
		if tt != lexer.TokenNewline && tt != lexer.TokenSemicolon && tt != lexer.TokenComment {
			break
		}
		idx++
	}
	if idx >= len(tokens) {
		return "", false
	}

	first := tokens[idx]
	if first.Type != lexer.TokenIdent || (first.Value != "let" && first.Value != "param") {
		return "", false
	}
	idx++
	if idx >= len(tokens) || tokens[idx].Type != lexer.TokenIdent {
		return "", false
	}
	for idx < len(tokens) {
		if tokens[idx].Type == lexer.TokenLBrace {
			return first.Value, true
		}
		idx++
	}
	return "", false
}

func (p *Parser) parseLegacyTopLevelBlock(keyword string, start diag.Position) ast.ExprStmt {
	_, _ = p.readTopLevelStatement()
	span := legacyKeywordSpan(p.file, start, keyword)
	hint := "rewrite it as one or more top-level assignments"
	if keyword == "param" {
		hint = "rewrite it as a top-level assignment that evaluates to a comb/table value"
	}
	if keyword == "let" {
		hint = "rewrite it as one or more top-level assignments or move shared values into a module used via `use`"
	}
	p.diags.AddError(
		diag.CodeE067,
		fmt.Sprintf("legacy top-level block '%s' is no longer supported", keyword),
		span,
		hint,
	)
	return ast.ExprStmt{Span: span}
}

func legacyKeywordSpan(file string, start diag.Position, keyword string) diag.Span {
	end := diag.NewPos(start.Offset+len(keyword), start.Line, start.Column+len(keyword))
	return diag.NewSpan(file, start, end)
}

func (p *Parser) peekTopLevelStatement() (string, diag.Position) {
	startPos := p.pos()
	startOff := p.off
	stmtEnd, _ := scanTopLevelStatementOffsets(p.src, startOff)
	return string(p.src[startOff:stmtEnd]), startPos
}

func (p *Parser) readTopLevelStatement() (string, diag.Position) {
	startPos := p.pos()
	startOff := p.off
	stmtEnd, nextOff := scanTopLevelStatementOffsets(p.src, startOff)
	for p.off < nextOff {
		p.advance()
	}
	return string(p.src[startOff:stmtEnd]), startPos
}
