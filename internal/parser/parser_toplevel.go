// handle top-level statement scanning/parsing helpers
package parser

import (
	"fmt"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

type topLevelParseContext struct {
	InControlBody bool
	LoopDepth     int
}

func (ctx topLevelParseContext) nestedControl() topLevelParseContext {
	ctx.InControlBody = true
	return ctx
}

func (ctx topLevelParseContext) nestedLoop() topLevelParseContext {
	ctx.InControlBody = true
	ctx.LoopDepth++
	return ctx
}

func (p *Parser) parseStmtList(ctx topLevelParseContext, stopAtRBrace bool) []ast.Stmt {
	stmts := make([]ast.Stmt, 0)
	for {
		p.skipTrivia()
		if p.eof() {
			break
		}
		if stopAtRBrace && p.peek() == '}' {
			break
		}
		stmts = append(stmts, p.parseTopLevelStmt(ctx))
	}
	return stmts
}

func (p *Parser) parseTopLevelStmt(ctx topLevelParseContext) ast.Stmt {
	start := p.pos()
	if p.isTopLevelAssignmentStart() {
		return p.parseGlobalAssign(start)
	}
	word, ok := p.peekWord()
	if ok {
		switch word {
		case "if":
			p.consumeWord()
			return p.parseIfStmt(start, ctx)
		case "for":
			p.consumeWord()
			return p.parseForStmt(start, ctx)
		case "while":
			p.consumeWord()
			return p.parseWhileStmt(start, ctx)
		case "break":
			p.consumeWord()
			return p.parseBreakStmt(start, ctx)
		case "continue":
			p.consumeWord()
			return p.parseContinueStmt(start, ctx)
		case "do":
			p.consumeWord()
			stmt := p.parseDoBlock(start)
			p.rejectControlBodyDeclaration(ctx, "do", stmt.Span)
			return stmt
		case "analyse":
			p.consumeWord()
			stmt := p.parseAnalyseBlock(start)
			p.rejectControlBodyDeclaration(ctx, "analyse", stmt.Span)
			return stmt
		case "use":
			p.consumeWord()
			stmt := p.parseUseStmt(start)
			p.rejectControlBodyDeclaration(ctx, "use", stmt.Span)
			return stmt
		}
	}
	return p.parseTopLevelExprStmt(start)
}

func (p *Parser) rejectControlBodyDeclaration(ctx topLevelParseContext, kind string, span diag.Span) {
	if !ctx.InControlBody {
		return
	}
	code := diag.CodeE080
	if kind == "use" {
		code = diag.CodeE430
	}
	p.diags.AddError(
		code,
		fmt.Sprintf("'%s' is not allowed inside control-flow bodies", kind),
		span,
		"move declarations and imports to module top level; use control flow only for assignments and expressions",
	)
}

func (p *Parser) isTopLevelAssignmentStart() bool {
	word, ok := p.peekWord()
	if !ok ||
		word == "if" ||
		word == "else" ||
		word == "for" ||
		word == "while" ||
		word == "break" ||
		word == "continue" ||
		word == "do" ||
		word == "analyse" ||
		word == "use" {
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

func (p *Parser) parseIfStmt(start diag.Position, ctx topLevelParseContext) ast.IfStmt {
	cond, ok := p.parseControlCondition("if")
	if !ok {
		return ast.IfStmt{
			Cond: cond,
			Span: diag.NewSpan(p.file, start, p.pos()),
		}
	}
	p.advance()
	thenBody := p.parseStmtList(ctx.nestedControl(), true)
	closeThen := p.expectTopLevelRBrace(diag.CodeE025, "expected '}' to close if body")

	elseBody := []ast.Stmt(nil)
	end := closeThen
	p.skipTrivia()
	if word, ok := p.peekWord(); ok && word == "else" {
		p.consumeWord()
		p.skipTrivia()
		if p.peek() != '{' {
			at := p.pos()
			p.diags.AddError(
				diag.CodeE080,
				"expected '{' to start else body",
				diag.NewSpan(p.file, at, at),
				"use `else { ... }`; nested `else if` is not supported yet",
			)
			if word, ok := p.peekWord(); ok && word == "if" {
				p.consumeWord()
				discard := p.parseIfStmt(at, ctx)
				end = discard.Span.End
			}
			return ast.IfStmt{
				Cond: cond,
				Then: thenBody,
				Span: diag.NewSpan(p.file, start, end),
			}
		}
		p.advance()
		elseBody = p.parseStmtList(ctx.nestedControl(), true)
		end = p.expectTopLevelRBrace(diag.CodeE025, "expected '}' to close else body")
	}

	return ast.IfStmt{
		Cond: cond,
		Then: thenBody,
		Else: elseBody,
		Span: diag.NewSpan(p.file, start, end),
	}
}

func (p *Parser) parseForStmt(start diag.Position, ctx topLevelParseContext) ast.ForStmt {
	header, headerStart, ok := p.readUntilControlBodyBrace()
	target, iterable := p.parseForHeader(header, headerStart)
	if !ok {
		at := p.pos()
		p.diags.AddError(
			diag.CodeE080,
			"expected '{' after for header",
			diag.NewSpan(p.file, at, at),
			"use syntax: for x in values { ... }",
		)
		return ast.ForStmt{
			Target:   target,
			Iterable: iterable,
			Span:     diag.NewSpan(p.file, start, at),
		}
	}
	p.advance()
	body := p.parseStmtList(ctx.nestedLoop(), true)
	end := p.expectTopLevelRBrace(diag.CodeE025, "expected '}' to close for body")
	return ast.ForStmt{
		Target:   target,
		Iterable: iterable,
		Body:     body,
		Span:     diag.NewSpan(p.file, start, end),
	}
}

func (p *Parser) parseForHeader(src string, start diag.Position) (string, ast.Expr) {
	tokens := lexer.LexFrom(p.file, src, start, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	nameTok := tp.expect(lexer.TokenIdent, diag.CodeE080, "expected loop variable after 'for'")
	inTok := tp.expect(lexer.TokenIn, diag.CodeE080, "expected 'in' after loop variable")
	if tp.peek().Type == lexer.TokenEOF {
		p.diags.AddError(
			diag.CodeE080,
			"expected iterable expression after 'in'",
			tp.peek().Span,
			"use syntax: for x in values { ... }",
		)
		if nameTok.Type != lexer.TokenIdent || inTok.Type != lexer.TokenIn {
			return "", nil
		}
		return nameTok.Value, nil
	}
	expr := tp.parseExpr()
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		p.diags.AddError(
			diag.CodeE061,
			"unexpected trailing tokens after for iterable",
			tp.peek().Span,
			"remove unsupported syntax before `{`",
		)
	}
	if nameTok.Type != lexer.TokenIdent || inTok.Type != lexer.TokenIn {
		return "", expr
	}
	return nameTok.Value, expr
}

func (p *Parser) parseWhileStmt(start diag.Position, ctx topLevelParseContext) ast.WhileStmt {
	cond, ok := p.parseControlCondition("while")
	if !ok {
		return ast.WhileStmt{
			Cond: cond,
			Span: diag.NewSpan(p.file, start, p.pos()),
		}
	}
	p.advance()
	body := p.parseStmtList(ctx.nestedLoop(), true)
	end := p.expectTopLevelRBrace(diag.CodeE025, "expected '}' to close while body")
	return ast.WhileStmt{
		Cond: cond,
		Body: body,
		Span: diag.NewSpan(p.file, start, end),
	}
}

func (p *Parser) parseBreakStmt(start diag.Position, ctx topLevelParseContext) ast.BreakStmt {
	span := p.parseLoopControlTail(start, "break")
	if ctx.LoopDepth == 0 {
		p.diags.AddError(
			diag.CodeE080,
			"'break' is only allowed inside loops",
			span,
			"move break into a for/while body",
		)
	}
	return ast.BreakStmt{Span: span}
}

func (p *Parser) parseContinueStmt(start diag.Position, ctx topLevelParseContext) ast.ContinueStmt {
	span := p.parseLoopControlTail(start, "continue")
	if ctx.LoopDepth == 0 {
		p.diags.AddError(
			diag.CodeE080,
			"'continue' is only allowed inside loops",
			span,
			"move continue into a for/while body",
		)
	}
	return ast.ContinueStmt{Span: span}
}

func (p *Parser) parseLoopControlTail(start diag.Position, keyword string) diag.Span {
	stmt, stmtStart := p.readTopLevelStatement()
	tokens := lexer.LexFrom(p.file, stmt, stmtStart, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		p.diags.AddError(
			diag.CodeE061,
			fmt.Sprintf("unexpected trailing tokens after %s", keyword),
			tp.peek().Span,
			fmt.Sprintf("use `%s` without arguments", keyword),
		)
		tp.consumeUntilStmtEnd()
	}
	return diag.NewSpan(p.file, start, p.pos())
}

func (p *Parser) expectTopLevelRBrace(code diag.Code, message string) diag.Position {
	if p.peek() == '}' {
		p.advance()
		return p.pos()
	}
	at := p.pos()
	p.diags.AddError(code, message, diag.NewSpan(p.file, at, at), "close the block with '}'")
	return at
}

func (p *Parser) parseControlCondition(kind string) (ast.Expr, bool) {
	p.skipTriviaInline()
	condText, condStart, ok := p.readUntilControlBodyBrace()
	if strings.TrimSpace(condText) == "" {
		at := condStart
		p.diags.AddError(
			diag.CodeE080,
			fmt.Sprintf("expected %s condition before '{'", kind),
			diag.NewSpan(p.file, at, at),
			fmt.Sprintf("use syntax: %s condition { ... }", kind),
		)
	}
	tokens := lexer.LexFrom(p.file, condText, condStart, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	cond := tp.parseExpr()
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		p.diags.AddError(
			diag.CodeE061,
			fmt.Sprintf("unexpected trailing tokens after %s condition", kind),
			tp.peek().Span,
			"remove unsupported syntax before `{`",
		)
	}
	if !ok {
		at := p.pos()
		p.diags.AddError(
			diag.CodeE080,
			fmt.Sprintf("expected '{' after %s condition", kind),
			diag.NewSpan(p.file, at, at),
			fmt.Sprintf("use syntax: %s condition { ... }", kind),
		)
	}
	return cond, ok
}

func (p *Parser) readUntilControlBodyBrace() (string, diag.Position, bool) {
	start := p.pos()
	startOff := p.off
	mode := blockScanCode
	escaped := false
	parenDepth := 0
	bracketDepth := 0
	for !p.eof() {
		r := p.peek()
		switch mode {
		case blockScanLineComment:
			if r == '\n' {
				mode = blockScanCode
			}
			p.advance()
		case blockScanSingleQuote:
			p.advance()
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				mode = blockScanCode
			}
		case blockScanDoubleQuote:
			p.advance()
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				mode = blockScanCode
			}
		default:
			switch r {
			case '#':
				mode = blockScanLineComment
				p.advance()
			case '\'':
				mode = blockScanSingleQuote
				escaped = false
				p.advance()
			case '"':
				mode = blockScanDoubleQuote
				escaped = false
				p.advance()
			case '(':
				parenDepth++
				p.advance()
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
				p.advance()
			case '[':
				bracketDepth++
				p.advance()
			case ']':
				if bracketDepth > 0 {
					bracketDepth--
				}
				p.advance()
			case '{':
				if parenDepth == 0 && bracketDepth == 0 {
					return string(p.src[startOff:p.off]), start, true
				}
				p.advance()
			default:
				p.advance()
			}
		}
	}
	return string(p.src[startOff:p.off]), start, false
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
