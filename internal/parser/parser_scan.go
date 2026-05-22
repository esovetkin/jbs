// low-level parser scanning/navigation utilities
package parser

import (
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
)

type blockScanMode uint8

const (
	blockScanCode blockScanMode = iota
	blockScanLineComment
	blockScanSingleQuote
	blockScanDoubleQuote
)

type hereDocSpec struct {
	Delimiter string
	StripTabs bool
}

func parseHereDocRedirect(src []rune, start int) (hereDocSpec, bool) {
	if start > 0 && src[start-1] == '<' {
		return hereDocSpec{}, false
	}
	if start+1 >= len(src) || src[start] != '<' || src[start+1] != '<' {
		return hereDocSpec{}, false
	}
	if start+2 < len(src) && src[start+2] == '<' {
		return hereDocSpec{}, false
	}
	i := start + 2
	spec := hereDocSpec{}
	if i < len(src) && src[i] == '-' {
		spec.StripTabs = true
		i++
	}
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	delimiter, ok := parseHereDocWord(src, i)
	if !ok {
		return hereDocSpec{}, false
	}
	spec.Delimiter = delimiter
	return spec, true
}

func parseHereDocWord(src []rune, start int) (string, bool) {
	var b strings.Builder
	for i := start; i < len(src); i++ {
		r := src[i]
		if isHereDocWordBoundary(r) {
			break
		}
		switch r {
		case '\'':
			i++
			for i < len(src) && src[i] != '\'' {
				b.WriteRune(src[i])
				i++
			}
			if i >= len(src) {
				return "", false
			}
		case '"':
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				b.WriteRune(src[i])
				i++
			}
			if i >= len(src) {
				return "", false
			}
		case '\\':
			if i+1 >= len(src) {
				return "", false
			}
			i++
			b.WriteRune(src[i])
		default:
			b.WriteRune(r)
		}
	}
	return b.String(), b.Len() > 0
}

func isHereDocWordBoundary(r rune) bool {
	switch r {
	case 0, ' ', '\t', '\r', '\n', ';', '|', '&', '(', ')':
		return true
	default:
		return false
	}
}

func nextLineEnd(src []rune, start int) int {
	i := start
	for i < len(src) {
		i++
		if src[i-1] == '\n' {
			break
		}
	}
	return i
}

func trimLineEnding(line []rune) []rune {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}

func hereDocLineMatches(line []rune, spec hereDocSpec) bool {
	line = trimLineEnding(line)
	if spec.StripTabs {
		for len(line) > 0 && line[0] == '\t' {
			line = line[1:]
		}
	}
	return string(line) == spec.Delimiter
}

func popHereDoc(queue []hereDocSpec) (*hereDocSpec, []hereDocSpec) {
	if len(queue) == 0 {
		return nil, queue
	}
	spec := queue[0]
	return &spec, queue[1:]
}

func readBalancedBlockShared(
	src []rune,
	peek func() rune,
	advance func() rune,
	eof func() bool,
	pos func() diag.Position,
	offset func() int,
) (content string, innerStart diag.Position, blockEnd diag.Position, ok bool) {
	if eof() || peek() != '{' {
		p := pos()
		return "", p, p, false
	}
	advance()
	innerStart = pos()
	startOff := offset()
	depth := 1
	mode := blockScanCode
	escaped := false
	pendingHereDocs := make([]hereDocSpec, 0)
	var activeHereDoc *hereDocSpec
	for !eof() {
		if activeHereDoc != nil {
			lineStart := offset()
			lineEnd := nextLineEnd(src, lineStart)
			line := src[lineStart:lineEnd]
			for offset() < lineEnd {
				advance()
			}
			if hereDocLineMatches(line, *activeHereDoc) {
				activeHereDoc, pendingHereDocs = popHereDoc(pendingHereDocs)
			}
			continue
		}
		r := peek()
		switch mode {
		case blockScanLineComment:
			if r == '\n' {
				mode = blockScanCode
				if activeHereDoc == nil {
					activeHereDoc, pendingHereDocs = popHereDoc(pendingHereDocs)
				}
			}
			advance()
		case blockScanSingleQuote:
			advance()
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
			advance()
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
				advance()
			case '<':
				if spec, ok := parseHereDocRedirect(src, offset()); ok {
					pendingHereDocs = append(pendingHereDocs, spec)
				}
				advance()
			case '\'':
				mode = blockScanSingleQuote
				escaped = false
				advance()
			case '"':
				mode = blockScanDoubleQuote
				escaped = false
				advance()
			case '{':
				depth++
				advance()
			case '}':
				depth--
				if depth == 0 {
					endOff := offset()
					blockEnd = pos()
					advance()
					if startOff < 0 || endOff < startOff || startOff > len(src) {
						return "", innerStart, blockEnd, false
					}
					if endOff > len(src) {
						endOff = len(src)
					}
					return strings.TrimRight(string(src[startOff:endOff]), "\r\n"), innerStart, blockEnd, true
				}
				advance()
			case '\n':
				advance()
				if activeHereDoc == nil {
					activeHereDoc, pendingHereDocs = popHereDoc(pendingHereDocs)
				}
			default:
				advance()
			}
		}
	}
	return string(src[startOff:]), innerStart, pos(), false
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
	p.diags.AddError(diag.CodeE025, "unterminated block; missing closing '}'", span, "close the block with '}'")
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

func (p *Parser) seekTo(pos diag.Position) {
	if pos.Offset < 0 {
		return
	}
	if pos.Offset < p.off {
		p.off = 0
		p.line = 1
		p.col = 1
	}
	for p.off < pos.Offset && !p.eof() {
		p.advance()
	}
}

func (p *tokenParser) skipNewlines() {
	for {
		t := p.peek().Type
		if t != lexer.TokenNewline && t != lexer.TokenComment {
			break
		}
		p.next()
	}
}

func (p *tokenParser) skipStmtSeparators() {
	for {
		t := p.peek().Type
		if t != lexer.TokenNewline && t != lexer.TokenSemicolon && t != lexer.TokenComment {
			break
		}
		p.next()
	}
}

func isStmtTerminator(t lexer.TokenType) bool {
	return t == lexer.TokenEOF || t == lexer.TokenNewline || t == lexer.TokenSemicolon || t == lexer.TokenComment
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
		if t == lexer.TokenEOF || t == lexer.TokenNewline || t == lexer.TokenComment {
			break
		}
		p.next()
	}
	p.skipStmtSeparators()
}

func (p *tokenParser) expect(tt lexer.TokenType, code diag.Code, message string) lexer.Token {
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
