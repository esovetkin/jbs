// tokenize JBS source into lexical tokens with source spans
//
// i.e. recognise keywords, literals, operators, statement
// separators. report any malformed lexical constructs
package lexer

import (
	"fmt"
	"unicode"

	"jbs/internal/diag"
)

type Lexer struct {
	file   string
	src    []rune
	off    int
	base   int
	line   int
	col    int
	tokens []Token
	diags  *diag.Diagnostics
}

func Lex(file, source string, diags *diag.Diagnostics) []Token {
	return LexFrom(file, source, diag.NewPos(0, 1, 1), diags)
}

func LexFrom(file, source string, start diag.Position, diags *diag.Diagnostics) []Token {
	l := &Lexer{
		file:  file,
		src:   []rune(source),
		base:  start.Offset,
		line:  start.Line,
		col:   start.Column,
		diags: diags,
	}
	l.run()
	return l.tokens
}

func (l *Lexer) run() {
	for !l.eof() {
		r := l.peek()
		if r == ' ' || r == '\t' || r == '\r' {
			l.advance()
			continue
		}
		if r == '\\' && l.peekN(1) == '\n' {
			l.advance()
			l.advance()
			continue
		}
		if r == '\n' {
			start := l.pos()
			l.advance()
			l.emit(TokenNewline, "\n", "", start, l.pos())
			continue
		}
		if r == '#' {
			l.lexComment()
			continue
		}
		if unicode.IsLetter(r) || r == '_' {
			l.lexIdent()
			continue
		}
		if unicode.IsDigit(r) || (r == '.' && unicode.IsDigit(l.peekN(1))) {
			l.lexNumber()
			continue
		}
		if r == '\'' || r == '"' {
			l.lexString()
			continue
		}
		l.lexSymbol()
	}
	p := l.pos()
	l.emit(TokenEOF, "", "", p, p)
}

func (l *Lexer) pos() diag.Position {
	return diag.NewPos(l.base+l.off, l.line, l.col)
}

func (l *Lexer) eof() bool {
	return l.off >= len(l.src)
}

func (l *Lexer) peek() rune {
	if l.eof() {
		return 0
	}
	return l.src[l.off]
}

func (l *Lexer) peekN(n int) rune {
	idx := l.off + n
	if idx < 0 || idx >= len(l.src) {
		return 0
	}
	return l.src[idx]
}

func (l *Lexer) advance() rune {
	if l.eof() {
		return 0
	}
	r := l.src[l.off]
	l.off++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *Lexer) emit(tt TokenType, text, value string, start, end diag.Position) {
	l.tokens = append(l.tokens, Token{Type: tt, Text: text, Value: value, Span: diag.NewSpan(l.file, start, end)})
}

func (l *Lexer) lexComment() {
	start := l.pos()
	l.advance()
	for !l.eof() && l.peek() != '\n' {
		l.advance()
	}
	end := l.pos()
	text := string(l.src[start.Offset-l.base : end.Offset-l.base])
	value := ""
	if len(text) > 0 {
		value = text[1:]
	}
	l.emit(TokenComment, text, value, start, end)
}

func (l *Lexer) lexIdent() {
	start := l.pos()
	buf := make([]rune, 0, 16)
	for !l.eof() {
		r := l.peek()
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			buf = append(buf, l.advance())
			continue
		}
		break
	}
	text := string(buf)
	if kw, ok := keywords[text]; ok {
		l.emit(kw, text, text, start, l.pos())
		return
	}
	l.emit(TokenIdent, text, text, start, l.pos())
}

func (l *Lexer) lexNumber() {
	start := l.pos()
	buf := make([]rune, 0, 24)

	if l.peek() == '.' {
		buf = append(buf, l.advance())
		for unicode.IsDigit(l.peek()) {
			buf = append(buf, l.advance())
		}
	} else {
		for unicode.IsDigit(l.peek()) {
			buf = append(buf, l.advance())
		}
		if l.peek() == '.' && unicode.IsDigit(l.peekN(1)) {
			buf = append(buf, l.advance())
			for unicode.IsDigit(l.peek()) {
				buf = append(buf, l.advance())
			}
		}
	}

	if l.peek() == 'e' || l.peek() == 'E' {
		first := l.peekN(1)
		second := l.peekN(2)
		if unicode.IsDigit(first) || ((first == '+' || first == '-') && unicode.IsDigit(second)) {
			buf = append(buf, l.advance())
			if l.peek() == '+' || l.peek() == '-' {
				buf = append(buf, l.advance())
			}
			for unicode.IsDigit(l.peek()) {
				buf = append(buf, l.advance())
			}
		}
	}
	text := string(buf)
	l.emit(TokenNumber, text, text, start, l.pos())
}

func (l *Lexer) lexString() {
	start := l.pos()
	quote := l.advance()
	buf := make([]rune, 0, 32)
	for !l.eof() {
		r := l.advance()
		if r == quote {
			raw := string(append([]rune{quote}, append(buf, quote)...))
			val := l.unescapeString(string(buf))
			l.emit(TokenString, raw, val, start, l.pos())
			return
		}
		if r == '\\' {
			if l.eof() {
				break
			}
			n := l.advance()
			buf = append(buf, '\\', n)
			continue
		}
		buf = append(buf, r)
	}
	l.diags.AddError(diag.CodeE001, "unterminated string literal", diag.NewSpan(l.file, start, l.pos()), "close the string with matching quote")
	l.emit(TokenString, string(buf), string(buf), start, l.pos())
}

func (l *Lexer) unescapeString(s string) string {
	r := []rune(s)
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); i++ {
		if r[i] != '\\' || i+1 >= len(r) {
			out = append(out, r[i])
			continue
		}
		n := r[i+1]
		switch n {
		case '\\', '"', '\'':
			out = append(out, n)
		default:
			// Keep unknown escapes literal (e.g. \n stays backslash+n).
			out = append(out, '\\', n)
		}
		i++
	}
	return string(out)
}

func (l *Lexer) lexSymbol() {
	start := l.pos()
	r := l.advance()
	var tt TokenType
	text := string(r)
	switch r {
	case ',':
		tt = TokenComma
	case ';':
		tt = TokenSemicolon
	case '.':
		tt = TokenDot
	case '=':
		if l.peek() == '=' {
			l.advance()
			tt = TokenEqEq
			text = "=="
		} else {
			tt = TokenEqual
		}
	case '!':
		if l.peek() == '=' {
			l.advance()
			tt = TokenNeq
			text = "!="
		} else {
			l.diags.AddError(diag.CodeE002, "unexpected '!'; only '!=' is allowed", diag.NewSpan(l.file, start, l.pos()), "use '!=' for inequality")
			return
		}
	case '<':
		if l.peek() == '=' {
			l.advance()
			tt = TokenLE
			text = "<="
		} else {
			tt = TokenLT
		}
	case '>':
		if l.peek() == '=' {
			l.advance()
			tt = TokenGE
			text = ">="
		} else {
			tt = TokenGT
		}
	case '+':
		if l.peek() == '=' {
			l.advance()
			tt = TokenPlusEqual
			text = "+="
		} else {
			tt = TokenPlus
		}
	case '-':
		if l.peek() == '=' {
			l.advance()
			tt = TokenMinusEqual
			text = "-="
		} else {
			tt = TokenMinus
		}
	case '*':
		if l.peek() == '=' {
			l.advance()
			tt = TokenStarEqual
			text = "*="
		} else {
			tt = TokenStar
		}
	case '/':
		if l.peek() == '=' {
			l.advance()
			tt = TokenSlashEqual
			text = "/="
		} else {
			tt = TokenSlash
		}
	case '%':
		if l.peek() == '=' {
			l.advance()
			tt = TokenPercentEqual
			text = "%="
		} else {
			tt = TokenPercent
		}
	case '(':
		tt = TokenLParen
	case ')':
		tt = TokenRParen
	case '[':
		tt = TokenLBracket
	case ']':
		tt = TokenRBracket
	case '{':
		tt = TokenLBrace
	case '}':
		tt = TokenRBrace
	default:
		l.diags.AddError(diag.CodeE003, fmt.Sprintf("unexpected character '%c'", r), diag.NewSpan(l.file, start, l.pos()), "remove or escape the character")
		return
	}
	l.emit(tt, text, text, start, l.pos())
}
