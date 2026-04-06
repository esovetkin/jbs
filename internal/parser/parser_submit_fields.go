package parser

import (
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

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
			p.diags.AddError(diag.CodeE077,
				"malformed submit statement; expected 'name = value'",
				diag.NewSpan(p.file, stmtStart, stmtStart),
				"use syntax: key = expression or preprocess/postprocess = { ... }",
			)
			p.recoverLine()
			continue
		}

		p.skipInlineTrivia()
		if p.peek() != '=' {
			p.diags.AddError(diag.CodeE077,
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
				p.diags.AddError(diag.CodeE077,
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
		diags.AddError(diag.CodeE077,
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
		diags.AddError(diag.CodeE077,
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
	p.diags.AddError(diag.CodeE025, "unterminated block; missing closing '}'", span, "close the block with '}'")
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
