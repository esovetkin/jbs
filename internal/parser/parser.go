package parser

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

type Parser struct {
	file  string
	src   []rune
	off   int
	line  int
	col   int
	diags *diag.Diagnostics
}

func Parse(file, source string, diags *diag.Diagnostics) ast.Program {
	p := &Parser{
		file:  file,
		src:   []rune(source),
		line:  1,
		col:   1,
		diags: diags,
	}
	return p.parseProgram()
}

func (p *Parser) parseProgram() ast.Program {
	stmts := make([]ast.Stmt, 0)
	for {
		p.skipTrivia()
		if p.eof() {
			break
		}
		start := p.pos()
		if p.isTopLevelAssignmentStart() {
			stmts = append(stmts, p.parseGlobalAssign(start))
			continue
		}
		word, ok := p.peekWord()
		if !ok {
			p.diags.AddError(
				"E010",
				"expected block keyword (param/do/submit/let/analyse)",
				diag.NewSpan(p.file, start, start),
				"start a block with param, do, submit, let, or analyse",
			)
			p.advance()
			continue
		}

		switch word {
		case "param":
			p.consumeWord()
			stmts = append(stmts, p.parseParamBlock(start))
		case "do":
			p.consumeWord()
			stmts = append(stmts, p.parseDoBlock(start))
		case "submit":
			p.consumeWord()
			stmts = append(stmts, p.parseSubmitBlock(start))
		case "let":
			p.consumeWord()
			stmts = append(stmts, p.parseLetBlock(start))
		case "analyse":
			p.consumeWord()
			stmts = append(stmts, p.parseAnalyseBlock(start))
		default:
			end := p.consumeWord()
			p.diags.AddError(
				"E011",
				fmt.Sprintf("unknown block keyword '%s'", word),
				diag.NewSpan(p.file, start, end),
				"valid keywords are param, do, submit, let, analyse",
			)
		}
	}

	prog := ast.Program{
		File:  p.file,
		Stmts: stmts,
	}
	if len(stmts) > 0 {
		prog.Span = diag.Merge(stmts[0].GetSpan(), stmts[len(stmts)-1].GetSpan())
	}
	return prog
}

func (p *Parser) isTopLevelAssignmentStart() bool {
	word, ok := p.peekWord()
	if !ok || word == "param" || word == "do" || word == "submit" || word == "let" || word == "analyse" {
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
		return r == '='
	}
	return false
}

func (p *Parser) parseGlobalAssign(start diag.Position) ast.GlobalAssign {
	stmt, stmtStart := p.readTopLevelStatement()
	tokens := lexer.LexFrom(p.file, stmt, stmtStart, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenIdent || tp.peekN(1).Type != lexer.TokenEqual {
		tok := tp.peek()
		p.diags.AddError(
			"E012",
			"expected top-level global assignment",
			tok.Span,
			"use syntax: name = expression",
		)
		return ast.GlobalAssign{
			Span: diag.NewSpan(p.file, start, start),
		}
	}
	asn := tp.parseAssignment()
	return ast.GlobalAssign{
		Name: asn.Name,
		Expr: asn.Expr,
		Span: asn.Span,
	}
}

func (p *Parser) readTopLevelStatement() (string, diag.Position) {
	startPos := p.pos()
	startOff := p.off
	mode := blockScanCode
	escaped := false
	for !p.eof() {
		r := p.peek()
		switch mode {
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
					stmt := string(p.src[startOff:p.off])
					for !p.eof() && p.peek() != '\n' {
						p.advance()
					}
					if !p.eof() && p.peek() == '\n' {
						p.advance()
					}
					return stmt, startPos
				case '\n':
					if p.off > startOff && p.src[p.off-1] == '\\' {
						p.advance()
						continue
					}
					stmt := string(p.src[startOff:p.off])
					p.advance()
					return stmt, startPos
				case ';':
					stmt := string(p.src[startOff:p.off])
					p.advance()
					return stmt, startPos
				case '\'':
					mode = blockScanSingleQuote
				p.advance()
			case '"':
				mode = blockScanDoubleQuote
				p.advance()
			default:
				p.advance()
			}
		}
	}
	return string(p.src[startOff:p.off]), startPos
}

func (p *Parser) parseParamBlock(blockStart diag.Position) ast.ParamBlock {
	name, nameSpan := p.parseRequiredIdent("E020", "expected param block name")
	withItems := p.parseOptionalWithClause()
	p.skipTrivia()
	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(
			"E021",
			"expected '{' to start param block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after param header",
		)
		return ast.ParamBlock{
			Name:      name,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.ParamBlock{
			Name:      name,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	assignments, final := parseParamBody(p.file, body, innerStart, p.diags)
	return ast.ParamBlock{
		Name:        name,
		WithItems:   withItems,
		Assignments: assignments,
		Final:       final,
		BodyRaw:     body,
		Span:        diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseDoBlock(blockStart diag.Position) ast.DoBlock {
	name, nameSpan := p.parseRequiredIdent("E030", "expected do block name")
	after, withItems := p.parseOptionalAfterAndWith()
	p.skipTrivia()

	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(
			"E031",
			"expected '{' to start do block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' before do script body",
		)
		return ast.DoBlock{
			Name:      name,
			After:     after,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.DoBlock{
			Name:      name,
			After:     after,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	return ast.DoBlock{
		Name:      name,
		After:     after,
		WithItems: withItems,
		Body:      body,
		BodyStart: innerStart,
		Span:      diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseSubmitBlock(blockStart diag.Position) ast.SubmitBlock {
	name, nameSpan := p.parseRequiredIdent("E040", "expected submit block name")
	after, withItems := p.parseOptionalAfterAndWith()
	p.skipTrivia()

	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(
			"E041",
			"expected '{' to start submit block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after submit header",
		)
		return ast.SubmitBlock{
			Name:      name,
			After:     after,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.SubmitBlock{
			Name:      name,
			After:     after,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	fields := parseSubmitFields(p.file, body, innerStart, p.diags)

	return ast.SubmitBlock{
		Name:      name,
		After:     after,
		WithItems: withItems,
		Fields:    fields,
		BodyRaw:   body,
		Span:      diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseLetBlock(blockStart diag.Position) ast.LetBlock {
	name, nameSpan := p.parseRequiredIdent("E080", "expected let block name")
	p.skipTrivia()
	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(
			"E081",
			"expected '{' to start let block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after let header",
		)
		return ast.LetBlock{
			Name: name,
			Span: diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}
	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.LetBlock{
			Name: name,
			Span: diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}
	assignments := parseLetBody(p.file, body, innerStart, p.diags)
	return ast.LetBlock{
		Name:        name,
		Assignments: assignments,
		BodyRaw:     body,
		Span:        diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseAnalyseBlock(blockStart diag.Position) ast.AnalyseBlock {
	stepName, stepSpan := p.parseRequiredIdent("E416", "expected analyse target step name")
	after, withItems := p.parseOptionalAfterAndWith()
	if len(after) > 0 {
		span := diag.NewSpan(p.file, blockStart, p.pos())
		p.diags.AddError(
			"E416",
			"analyse block does not support an after-clause",
			span,
			"use syntax: analyse <step_name> [with ...] { ... }",
		)
	}
	p.skipTrivia()
	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(
			"E416",
			"expected '{' to start analyse block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after analyse header",
		)
		return ast.AnalyseBlock{
			StepName:  stepName,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, stepSpan.End),
		}
	}
	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.AnalyseBlock{
			StepName:  stepName,
			WithItems: withItems,
			Span:      diag.NewSpan(p.file, blockStart, stepSpan.End),
		}
	}
	assignments, columns := parseAnalyseBody(p.file, body, innerStart, p.diags)
	return ast.AnalyseBlock{
		StepName:    stepName,
		WithItems:   withItems,
		Assignments: assignments,
		Columns:     columns,
		BodyRaw:     body,
		Span:        diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseOptionalWithClause() []ast.WithItem {
	p.skipTriviaInline()
	word, ok := p.peekWord()
	if !ok || word != "with" {
		return nil
	}
	p.consumeWord()
	return p.parseWithItems()
}

func (p *Parser) parseOptionalAfterAndWith() ([]string, []ast.WithItem) {
	after := make([]string, 0)
	withItems := make([]ast.WithItem, 0)
	for {
		p.skipTriviaInline()
		word, ok := p.peekWord()
		if !ok {
			break
		}
		if word == "after" {
			p.consumeWord()
			after = append(after, p.parseNameList()...)
			continue
		}
		if word == "with" {
			p.consumeWord()
			withItems = append(withItems, p.parseWithItems()...)
			continue
		}
		break
	}
	return after, withItems
}

func (p *Parser) parseWithItems() []ast.WithItem {
	items := make([]ast.WithItem, 0)
	currentFrom := ""
	for {
		names, ok := p.parseWithNames()
		if !ok || len(names) == 0 {
			break
		}

		src := ""
		srcSpan := diag.Span{}
		p.skipTriviaInline()
		word, ok := p.peekWord()
		if ok && word == "from" {
			p.consumeWord()
			srcName, fromSpan := p.parseRequiredIdent("E024", "expected source parameterset name after 'from'")
			src = srcName
			srcSpan = fromSpan
			currentFrom = srcName
		} else if currentFrom != "" {
			src = currentFrom
		}

		for _, name := range names {
			item := ast.WithItem{Name: name.Name, Span: name.Span, From: src}
			if src != "" && !srcSpan.IsZero() {
				item.Span = diag.Merge(item.Span, srcSpan)
			}
			items = append(items, item)
		}
		p.skipTriviaInline()
		if p.peek() != ',' {
			break
		}
		p.advance()
	}
	return items
}

type withName struct {
	Name string
	Span diag.Span
}

func (p *Parser) parseWithNames() ([]withName, bool) {
	p.skipTriviaInline()
	if p.peek() != '(' {
		name, span := p.parseRequiredIdent("E023", "expected identifier in with clause")
		if name == "" {
			return nil, false
		}
		return []withName{{Name: name, Span: span}}, true
	}

	tupleStart := p.pos()
	p.advance()
	names := make([]withName, 0)

	for {
		p.skipTriviaInline()
		if p.peek() == ')' {
			if len(names) == 0 {
				span := diag.NewSpan(p.file, tupleStart, p.pos())
				p.diags.AddError("E023", "empty tuple in with clause", span, "add at least one identifier inside parentheses")
			} else {
				span := diag.NewSpan(p.file, tupleStart, p.pos())
				p.diags.AddError("E023", "trailing comma in with-clause tuple", span, "remove trailing comma or add another identifier")
			}
			p.advance()
			return names, len(names) > 0
		}

		name, span := p.parseRequiredIdent("E023", "expected identifier in with clause")
		if name == "" {
			return names, len(names) > 0
		}
		names = append(names, withName{Name: name, Span: span})

		p.skipTriviaInline()
		switch p.peek() {
		case ',':
			p.advance()
		case ')':
			p.advance()
			return names, true
		default:
			span := diag.NewSpan(p.file, tupleStart, p.pos())
			p.diags.AddError("E023", "unterminated tuple in with clause; missing ')'", span, "close tuple imports with ')'")
			return names, len(names) > 0
		}
	}
}

func (p *Parser) parseNameList() []string {
	out := make([]string, 0)
	for {
		name, _ := p.parseRequiredIdent("E022", "expected identifier in dependency list")
		if name != "" {
			out = append(out, name)
		}
		p.skipTriviaInline()
		if p.peek() != ',' {
			break
		}
		p.advance()
	}
	return out
}

func (p *Parser) parseRequiredIdent(code, message string) (string, diag.Span) {
	p.skipTriviaInline()
	start := p.pos()
	word, ok := p.peekWord()
	if !ok {
		p.diags.AddError(code, message, diag.NewSpan(p.file, start, start), "use a valid identifier")
		return "", diag.NewSpan(p.file, start, start)
	}
	end := p.consumeWord()
	return word, diag.NewSpan(p.file, start, end)
}

func (p *Parser) readBalancedBlock() (content string, innerStart diag.Position, blockEnd diag.Position, ok bool) {
	content, innerStart, blockEnd, ok = readBalancedBlockShared(
		p.src,
		func() rune { return p.peek() },
		func() rune { return p.advance() },
		func() bool { return p.eof() },
		func() diag.Position { return p.pos() },
		func() int { return p.off },
	)
	if ok {
		return content, innerStart, blockEnd, true
	}
	span := diag.NewSpan(p.file, innerStart, p.pos())
	p.diags.AddError("E025", "unterminated block; missing closing '}'", span, "close the block with '}'")
	return "", innerStart, p.pos(), false
}

func (p *Parser) skipTrivia() {
	for !p.eof() {
		r := p.peek()
		if unicode.IsSpace(r) {
			p.advance()
			continue
		}
		if r == ';' {
			p.advance()
			continue
		}
		if r == '#' {
			for !p.eof() && p.peek() != '\n' {
				p.advance()
			}
			continue
		}
		break
	}
}

func (p *Parser) skipTriviaInline() {
	for !p.eof() {
		r := p.peek()
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			p.advance()
			continue
		}
		if r == '#' {
			for !p.eof() && p.peek() != '\n' {
				p.advance()
			}
			continue
		}
		break
	}
}

func (p *Parser) peekWord() (string, bool) {
	if p.eof() {
		return "", false
	}
	r := p.peek()
	if !(unicode.IsLetter(r) || r == '_') {
		return "", false
	}
	i := p.off
	buf := make([]rune, 0, 16)
	for i < len(p.src) {
		r = p.src[i]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			buf = append(buf, r)
			i++
			continue
		}
		break
	}
	return string(buf), true
}

func (p *Parser) consumeWord() diag.Position {
	for !p.eof() {
		r := p.peek()
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			p.advance()
			continue
		}
		break
	}
	return p.pos()
}

func (p *Parser) eof() bool {
	return p.off >= len(p.src)
}

func (p *Parser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.src[p.off]
}

func (p *Parser) advance() rune {
	if p.eof() {
		return 0
	}
	r := p.src[p.off]
	p.off++
	if r == '\n' {
		p.line++
		p.col = 1
	} else {
		p.col++
	}
	return r
}

func (p *Parser) pos() diag.Position {
	return diag.NewPos(p.off, p.line, p.col)
}

type tokenParser struct {
	tokens []lexer.Token
	idx    int
	diags  *diag.Diagnostics
}

func parseParamBody(file, body string, start diag.Position, diags *diag.Diagnostics) ([]ast.Assignment, ast.CombExpr) {
	tokens := lexer.LexFrom(file, body, start, diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	assignments := make([]ast.Assignment, 0)
	var final ast.CombExpr

	for {
		tp.skipStmtSeparators()
		if tp.peek().Type == lexer.TokenEOF {
			break
		}
		if tp.peek().Type == lexer.TokenIdent && tp.peekN(1).Type == lexer.TokenEqual {
			assignments = append(assignments, tp.parseAssignment())
			continue
		}
		final = tp.parseCombExpr()
		tp.skipStmtSeparators()
		if tp.peek().Type != lexer.TokenEOF {
			tok := tp.peek()
			diags.AddError(
				"E026",
				"unexpected tokens after final combination expression",
				tok.Span,
				"final expression must be the last statement in param block",
			)
		}
		break
	}

	if final == nil {
		diags.AddError(
			"E027",
			"param block missing final combination expression",
			diag.NewSpan(file, start, start),
			"add a final expression like '(a+b)*c'",
		)
	}
	return assignments, final
}

func parseLetBody(file, body string, start diag.Position, diags *diag.Diagnostics) []ast.Assignment {
	tokens := lexer.LexFrom(file, body, start, diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	out := make([]ast.Assignment, 0)

	for {
		tp.skipStmtSeparators()
		if tp.peek().Type == lexer.TokenEOF {
			break
		}
		if tp.peek().Type != lexer.TokenIdent || tp.peekN(1).Type != lexer.TokenEqual {
			tok := tp.peek()
			diags.AddError(
				"E418",
				"malformed let statement; expected 'name = expression'",
				tok.Span,
				"use syntax: variable = expression",
			)
			tp.consumeUntilStmtEnd()
			continue
		}
		out = append(out, tp.parseAssignment())
	}
	return out
}

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
				diags.AddError(
					"E417",
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
		diags.AddError(
			"E417",
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
		diags.AddError(
			"E416",
			"malformed analyse statement; expected 'name = expression' or 'name = expression in \"file\"'",
			stmtStart.Span,
			"use syntax: name = expression [in \"filename\"]",
		)
		tp.consumeUntilStmtEnd()
		return ast.AnalyseAssign{}
	}
	nameTok := tp.next()

	if tp.peek().Type != lexer.TokenEqual {
		diags.AddError(
			"E416",
			"malformed analyse statement; expected '=' after variable name",
			nameTok.Span,
			"use syntax: name = expression [in \"filename\"]",
		)
		tp.consumeUntilStmtEnd()
		return ast.AnalyseAssign{}
	}
	tp.next()

	expr := tp.parseExpr()
	if expr == nil {
		tp.consumeUntilStmtEnd()
		return ast.AnalyseAssign{}
	}

	fileName := ""
	span := diag.Merge(nameTok.Span, expr.GetSpan())
	if tp.peek().Type == lexer.TokenIn {
		tp.next()
		fileTok := tp.peek()
		if fileTok.Type != lexer.TokenString {
			diags.AddError(
				"E416",
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

	if tp.peek().Type != lexer.TokenEOF && tp.peek().Type != lexer.TokenNewline && tp.peek().Type != lexer.TokenSemicolon {
		diags.AddError(
			"E416",
			"unexpected trailing tokens in analyse statement",
			tp.peek().Span,
			"separate statements with newline or ';'",
		)
	}
	tp.consumeUntilStmtEnd()

	return ast.AnalyseAssign{
		Name: nameTok.Value,
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
			diags.AddError(
				"E417",
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
			diags.AddError(
				"E417",
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
			memberTok := tp.expect(lexer.TokenIdent, "E417", "expected identifier after '.' in analyse result tuple")
			name = name + "." + memberTok.Value
			span = diag.Merge(span, memberTok.Span)
		}
		title := ""
		if tp.peek().Type == lexer.TokenAs {
			tp.next()
			titleTok := tp.peek()
			if titleTok.Type != lexer.TokenString {
				diags.AddError(
					"E417",
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

		diags.AddError(
			"E417",
			"expected ',' or ')' in analyse result tuple",
			tp.peek().Span,
			"separate tuple items with commas",
		)
		tp.consumeUntilNewline()
		return columns
	}
}

func parseSubmitFields(file, body string, start diag.Position, diags *diag.Diagnostics) []ast.SubmitField {
	sp := &submitFieldParser{
		file:  file,
		src:   []rune(body),
		base:  start.Offset,
		line:  start.Line,
		col:   start.Column,
		diags: diags,
	}
	return sp.parse()
}

type submitFieldParser struct {
	file  string
	src   []rune
	off   int
	base  int
	line  int
	col   int
	diags *diag.Diagnostics
}

func (p *submitFieldParser) parse() []ast.SubmitField {
	fields := make([]ast.SubmitField, 0)
	for {
		p.skipTrivia()
		if p.eof() {
			break
		}

		stmtStart := p.pos()
		name, nameSpan, ok := p.parseIdent()
		if !ok {
			p.diags.AddError(
				"E076",
				"malformed submit statement; expected 'name = value'",
				diag.NewSpan(p.file, stmtStart, stmtStart),
				"use syntax: key = expression or preprocess/postprocess = { ... }",
			)
			p.recoverLine()
			continue
		}

		p.skipInlineTrivia()
		if p.peek() != '=' {
			p.diags.AddError(
				"E076",
				"malformed submit statement; expected '=' after key",
				nameSpan,
				"use syntax: key = expression or preprocess/postprocess = { ... }",
			)
			p.recoverLine()
			continue
		}
		p.advance()
		p.skipInlineTrivia()

		if p.peek() == '{' {
			raw, rawStart, blockEnd, ok := p.readBalancedBlock()
			if !ok {
				break
			}
			field := ast.SubmitField{
				Name:     name,
				Raw:      raw,
				RawStart: rawStart,
				IsRaw:    true,
				Span:     diag.NewSpan(p.file, stmtStart, blockEnd),
			}
			fields = append(fields, field)
			if p.hasUnexpectedTrailingTextAfterRawBlock() {
				p.diags.AddError(
					"E076",
					"unexpected trailing text after submit raw block",
					field.Span,
					"separate statements with newline or ';'",
				)
				p.recoverLine()
			}
			continue
		}

		exprStart := p.pos()
		exprText := p.scanExprUntilStmtEnd()
		expr := parseSubmitExpr(p.file, exprText, exprStart, p.diags)
		fieldSpan := diag.NewSpan(p.file, stmtStart, p.pos())
		if expr != nil {
			fieldSpan = diag.Merge(diag.NewSpan(p.file, stmtStart, stmtStart), expr.GetSpan())
		}
		fields = append(fields, ast.SubmitField{
			Name: name,
			Expr: expr,
			Span: fieldSpan,
		})
	}
	return fields
}

func parseSubmitExpr(file, expr string, start diag.Position, diags *diag.Diagnostics) ast.Expr {
	tokens := lexer.LexFrom(file, expr, start, diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	tp.skipNewlines()
	if tp.peek().Type == lexer.TokenEOF {
		diags.AddError(
			"E076",
			"malformed submit statement; expected expression after '='",
			diag.NewSpan(file, start, start),
			"use syntax: key = expression",
		)
		return nil
	}
	exprNode := tp.parseExpr()
	tp.skipNewlines()
	if tp.peek().Type != lexer.TokenEOF {
		tok := tp.peek()
		diags.AddError(
			"E076",
			"unexpected trailing tokens in submit expression",
			tok.Span,
			"use one expression per submit assignment",
		)
	}
	return exprNode
}

func (p *submitFieldParser) eof() bool {
	return p.off >= len(p.src)
}

func (p *submitFieldParser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.src[p.off]
}

func (p *submitFieldParser) peekN(n int) rune {
	idx := p.off + n
	if idx < 0 || idx >= len(p.src) {
		return 0
	}
	return p.src[idx]
}

func (p *submitFieldParser) advance() rune {
	if p.eof() {
		return 0
	}
	r := p.src[p.off]
	p.off++
	if r == '\n' {
		p.line++
		p.col = 1
	} else {
		p.col++
	}
	return r
}

func (p *submitFieldParser) pos() diag.Position {
	return diag.NewPos(p.base+p.off, p.line, p.col)
}

func (p *submitFieldParser) skipTrivia() {
	for !p.eof() {
		r := p.peek()
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == ';' {
			p.advance()
			continue
		}
		if r == '#' {
			for !p.eof() && p.peek() != '\n' {
				p.advance()
			}
			continue
		}
		break
	}
}

func (p *submitFieldParser) skipInlineTrivia() {
	for !p.eof() {
		r := p.peek()
		if r == ' ' || r == '\t' || r == '\r' {
			p.advance()
			continue
		}
		break
	}
}

func (p *submitFieldParser) parseIdent() (string, diag.Span, bool) {
	start := p.pos()
	if p.eof() {
		return "", diag.NewSpan(p.file, start, start), false
	}
	r := p.peek()
	if !(unicode.IsLetter(r) || r == '_') {
		return "", diag.NewSpan(p.file, start, start), false
	}
	for !p.eof() {
		r = p.peek()
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			p.advance()
			continue
		}
		break
	}
	end := p.pos()
	return string(p.src[start.Offset-p.base : end.Offset-p.base]), diag.NewSpan(p.file, start, end), true
}

func (p *submitFieldParser) readBalancedBlock() (content string, innerStart diag.Position, blockEnd diag.Position, ok bool) {
	content, innerStart, blockEnd, ok = readBalancedBlockShared(
		p.src,
		func() rune { return p.peek() },
		func() rune { return p.advance() },
		func() bool { return p.eof() },
		func() diag.Position { return p.pos() },
		func() int { return p.off },
	)
	if ok {
		return content, innerStart, blockEnd, true
	}
	span := diag.NewSpan(p.file, innerStart, p.pos())
	p.diags.AddError("E025", "unterminated block; missing closing '}'", span, "close the block with '}'")
	return "", innerStart, p.pos(), false
}

type blockScannerMode uint8

const (
	blockScanCode blockScannerMode = iota
	blockScanSingleQuote
	blockScanDoubleQuote
	blockScanLineComment
)

func readBalancedBlockShared(
	src []rune,
	peek func() rune,
	advance func() rune,
	eof func() bool,
	pos func() diag.Position,
	off func() int,
) (content string, innerStart diag.Position, blockEnd diag.Position, ok bool) {
	if peek() != '{' {
		p := pos()
		return "", p, p, false
	}
	advance()
	innerStart = pos()
	startIdx := off()
	if !scanBalancedBlock(advance, eof) {
		return "", innerStart, pos(), false
	}
	endIdx := off() - 1
	return string(src[startIdx:endIdx]), innerStart, pos(), true
}

func scanBalancedBlock(advance func() rune, eof func() bool) bool {
	depth := 1
	mode := blockScanCode
	escaped := false
	for !eof() {
		r := advance()
		switch mode {
		case blockScanLineComment:
			if r == '\n' {
				mode = blockScanCode
			}
		case blockScanSingleQuote:
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
			case '\'':
				mode = blockScanSingleQuote
			case '"':
				mode = blockScanDoubleQuote
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return true
				}
			}
		}
	}
	return false
}

func (p *submitFieldParser) recoverLine() {
	for !p.eof() && p.peek() != '\n' {
		p.advance()
	}
	if !p.eof() && p.peek() == '\n' {
		p.advance()
	}
}

func (p *submitFieldParser) scanExprUntilStmtEnd() string {
	start := p.off
	mode := blockScanCode
	escaped := false
	for !p.eof() {
		r := p.peek()
		switch mode {
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
				case '\\':
					if p.peekN(1) == '\n' {
						p.advance()
						p.advance()
						continue
					}
					p.advance()
				case '\n', ';', '#':
					return string(p.src[start:p.off])
				case '\'':
					mode = blockScanSingleQuote
					p.advance()
			case '"':
				mode = blockScanDoubleQuote
				p.advance()
			default:
				p.advance()
			}
		}
	}
	return string(p.src[start:p.off])
}

func (p *submitFieldParser) hasUnexpectedTrailingTextAfterRawBlock() bool {
	p.skipInlineTrivia()
	if p.eof() {
		return false
	}
	switch p.peek() {
	case ';':
		p.advance()
		return false
	case '\n':
		return false
	case '#':
		p.recoverLine()
		return false
	default:
		return true
	}
}

func (p *tokenParser) parseAssignment() ast.Assignment {
	name := p.expect(lexer.TokenIdent, "E050", "expected assignment identifier")
	p.expect(lexer.TokenEqual, "E051", "expected '=' in assignment")
	expr := p.parseExpr()
	span := name.Span
	if expr != nil {
		span = diag.Merge(span, expr.GetSpan())
	}
	if p.peek().Type != lexer.TokenEOF && p.peek().Type != lexer.TokenNewline && p.peek().Type != lexer.TokenSemicolon {
		tok := p.peek()
		p.diags.AddError(
			"E061",
			"unexpected trailing tokens after assignment expression",
			tok.Span,
			"remove unsupported trailing syntax after the expression",
		)
	}
	p.consumeUntilStmtEnd()
	return ast.Assignment{
		Name: name.Value,
		Expr: expr,
		Span: span,
	}
}

func (p *tokenParser) parseExpr() ast.Expr {
	return p.parseConditional()
}

func (p *tokenParser) parseConditional() ast.Expr {
	thenExpr := p.parseOr()
	if p.peek().Type == lexer.TokenIf {
		ifTok := p.next()
		cond := p.parseOr()
		p.expect(lexer.TokenElse, "E052", "expected 'else' in conditional expression")
		elseExpr := p.parseConditional()
		span := diag.Merge(thenExpr.GetSpan(), elseExpr.GetSpan())
		span = diag.Merge(span, ifTok.Span)
		return ast.ConditionalExpr{
			Then: thenExpr,
			Cond: cond,
			Else: elseExpr,
			Span: span,
		}
	}
	return thenExpr
}

func (p *tokenParser) parseOr() ast.Expr {
	left := p.parseAnd()
	for p.peek().Type == lexer.TokenOr {
		op := p.next()
		right := p.parseAnd()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseAnd() ast.Expr {
	left := p.parseCompare()
	for p.peek().Type == lexer.TokenAnd {
		op := p.next()
		right := p.parseCompare()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseCompare() ast.Expr {
	left := p.parseAdd()
	t := p.peek().Type
	if t == lexer.TokenEqEq || t == lexer.TokenNeq || t == lexer.TokenLT || t == lexer.TokenGT || t == lexer.TokenLE || t == lexer.TokenGE {
		op := p.next()
		right := p.parseAdd()
		return ast.CompareExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseAdd() ast.Expr {
	left := p.parseMul()
	for {
		t := p.peek().Type
		if t != lexer.TokenPlus && t != lexer.TokenMinus {
			break
		}
		op := p.next()
		right := p.parseMul()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseMul() ast.Expr {
	left := p.parseUnary()
	for {
		t := p.peek().Type
		if t != lexer.TokenStar && t != lexer.TokenSlash && t != lexer.TokenPercent {
			break
		}
		op := p.next()
		right := p.parseUnary()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseUnary() ast.Expr {
	t := p.peek().Type
	if t == lexer.TokenPlus || t == lexer.TokenMinus {
		op := p.next()
		expr := p.parseUnary()
		return ast.UnaryExpr{
			Op:   op.Text,
			Expr: expr,
			Span: diag.Merge(op.Span, expr.GetSpan()),
		}
	}
	return p.parsePrimary()
}

func (p *tokenParser) parsePrimary() ast.Expr {
	tok := p.peek()
	switch tok.Type {
	case lexer.TokenIdent:
		if tok.Value == "true" || tok.Value == "True" {
			p.next()
			return ast.BoolExpr{Value: true, Span: tok.Span}
		}
		if tok.Value == "false" || tok.Value == "False" {
			p.next()
			return ast.BoolExpr{Value: false, Span: tok.Span}
		}
		if (tok.Value == "shell" || tok.Value == "python") && p.peekN(1).Type == lexer.TokenLParen {
			modeTok := p.next()
			p.expect(lexer.TokenLParen, "E062", "expected '(' after mode expression")
			arg := p.parseExpr()
			close := p.expect(lexer.TokenRParen, "E063", "expected ')' to close mode expression")
			return ast.ModeExpr{
				Mode: modeTok.Value,
				Expr: arg,
				Span: diag.Merge(modeTok.Span, close.Span),
			}
		}
		nameTok := p.next()
		if p.peek().Type == lexer.TokenDot {
			p.next()
			memberTok := p.expect(lexer.TokenIdent, "E064", "expected identifier after '.'")
			return ast.QualifiedIdentExpr{
				Namespace: nameTok.Value,
				Name:      memberTok.Value,
				Span:      diag.Merge(nameTok.Span, memberTok.Span),
			}
		}
		return ast.IdentExpr{Name: nameTok.Value, Span: nameTok.Span}
	case lexer.TokenString:
		p.next()
		return ast.StringExpr{Value: tok.Value, Span: tok.Span}
	case lexer.TokenNumber:
		p.next()
		value, _ := strconv.ParseFloat(tok.Value, 64)
		return ast.NumberExpr{
			Raw:   tok.Value,
			Value: value,
			Int:   !strings.Contains(tok.Value, "."),
			Span:  tok.Span,
		}
	case lexer.TokenLParen:
		open := p.next()
		p.skipNewlines()
		if p.peek().Type == lexer.TokenRParen {
			close := p.next()
			return ast.TupleExpr{Items: nil, Span: diag.Merge(open.Span, close.Span)}
		}
		first := p.parseExpr()
		p.skipNewlines()
		if p.peek().Type == lexer.TokenComma {
			items := []ast.Expr{first}
			for p.peek().Type == lexer.TokenComma {
				p.next()
				p.skipNewlines()
				if p.peek().Type == lexer.TokenRParen {
					break
				}
				items = append(items, p.parseExpr())
				p.skipNewlines()
			}
			close := p.expect(lexer.TokenRParen, "E053", "expected ')' to close tuple")
			return ast.TupleExpr{
				Items: items,
				Span:  diag.Merge(open.Span, close.Span),
			}
		}
		p.skipNewlines()
		p.expect(lexer.TokenRParen, "E054", "expected ')' to close expression")
		return first
	case lexer.TokenLBracket:
		open := p.next()
		p.skipNewlines()
		items := make([]ast.Expr, 0)
		if p.peek().Type != lexer.TokenRBracket {
			for {
				items = append(items, p.parseExpr())
				p.skipNewlines()
				if p.peek().Type != lexer.TokenComma {
					break
				}
				p.next()
				p.skipNewlines()
				if p.peek().Type == lexer.TokenRBracket {
					break
				}
			}
		}
		p.skipNewlines()
		close := p.expect(lexer.TokenRBracket, "E055", "expected ']' to close list")
		return ast.ListExpr{
			Items: items,
			Span:  diag.Merge(open.Span, close.Span),
		}
	default:
		p.diags.AddError(
			"E058",
			fmt.Sprintf("unexpected token '%s' in expression", tok.Text),
			tok.Span,
			"use a valid expression term",
		)
		p.next()
		return ast.StringExpr{Value: "", Span: tok.Span}
	}
}

func (p *tokenParser) parseCombExpr() ast.CombExpr {
	return p.parseCombAdd()
}

func (p *tokenParser) parseCombAdd() ast.CombExpr {
	left := p.parseCombMul()
	for p.peek().Type == lexer.TokenPlus {
		op := p.next()
		right := p.parseCombMul()
		left = ast.CombBinary{
			Left:   left,
			Op:     op.Text,
			OpSpan: op.Span,
			Right:  right,
			Span:   diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseCombMul() ast.CombExpr {
	left := p.parseCombPrimary()
	for p.peek().Type == lexer.TokenStar {
		op := p.next()
		right := p.parseCombPrimary()
		left = ast.CombBinary{
			Left:   left,
			Op:     op.Text,
			OpSpan: op.Span,
			Right:  right,
			Span:   diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseCombPrimary() ast.CombExpr {
	tok := p.peek()
	if tok.Type == lexer.TokenIdent {
		nameTok := p.next()
		if p.peek().Type == lexer.TokenDot {
			p.next()
			memberTok := p.expect(lexer.TokenIdent, "E064", "expected identifier after '.'")
			return ast.CombIdent{
				Name: nameTok.Value + "." + memberTok.Value,
				Span: diag.Merge(nameTok.Span, memberTok.Span),
			}
		}
		return ast.CombIdent{Name: nameTok.Value, Span: nameTok.Span}
	}
	if tok.Type == lexer.TokenLParen {
		p.next()
		expr := p.parseCombExpr()
		p.expect(lexer.TokenRParen, "E059", "expected ')' in combination expression")
		return expr
	}
	p.diags.AddError(
		"E060",
		fmt.Sprintf("unexpected token '%s' in combination expression", tok.Text),
		tok.Span,
		"combination expression allows identifiers, +, *, and parentheses",
	)
	p.next()
	return ast.CombIdent{Name: "", Span: tok.Span}
}

func (p *tokenParser) skipNewlines() {
	for p.peek().Type == lexer.TokenNewline {
		p.next()
	}
}

func (p *tokenParser) skipStmtSeparators() {
	for {
		t := p.peek().Type
		if t != lexer.TokenNewline && t != lexer.TokenSemicolon {
			break
		}
		p.next()
	}
}

func isStmtTerminator(t lexer.TokenType) bool {
	return t == lexer.TokenEOF || t == lexer.TokenNewline || t == lexer.TokenSemicolon
}

func (p *tokenParser) consumeUntilStmtEnd() {
	for !isStmtTerminator(p.peek().Type) {
		p.next()
	}
	p.skipStmtSeparators()
}

func (p *tokenParser) consumeUntilNewline() {
	for {
		t := p.peek().Type
		if t == lexer.TokenEOF || t == lexer.TokenNewline {
			break
		}
		p.next()
	}
	p.skipNewlines()
}

func (p *tokenParser) expect(tt lexer.TokenType, code, message string) lexer.Token {
	tok := p.peek()
	if tok.Type != tt {
		p.diags.AddError(code, message, tok.Span, "check token ordering and delimiters")
		return tok
	}
	return p.next()
}

func (p *tokenParser) peek() lexer.Token {
	if p.idx >= len(p.tokens) {
		if len(p.tokens) == 0 {
			return lexer.Token{Type: lexer.TokenEOF}
		}
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.idx]
}

func (p *tokenParser) peekN(n int) lexer.Token {
	i := p.idx + n
	if i >= len(p.tokens) {
		if len(p.tokens) == 0 {
			return lexer.Token{Type: lexer.TokenEOF}
		}
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[i]
}

func (p *tokenParser) next() lexer.Token {
	tok := p.peek()
	if p.idx < len(p.tokens) {
		p.idx++
	}
	return tok
}
