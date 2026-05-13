// parse `with`-clause item lists into expression-backed `[]ast.WithItem`
package parser

import (
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func (p *Parser) parseWithItems() []ast.WithItem {
	items := make([]ast.WithItem, 0)
	for {
		raw, span, ok := p.readWithItemExprText()
		if !ok {
			break
		}
		before := len(p.diags.Items)
		expr, parsed := ParseStandaloneExpr(p.file, raw, span.Start, p.diags)
		if !parsed || expr == nil || parserAddedErrorSince(p.diags, before) {
			p.diags.AddError(diag.CodeE023, "invalid with-clause expression", span, `use a variable name or table projection such as cases["x"]`)
			break
		}
		items = append(items, ast.WithItem{Expr: expr, Span: span})

		p.skipTriviaInline()
		if p.peek() != ',' {
			break
		}
		p.advance()
	}
	return items
}

func parserAddedErrorSince(diags *diag.Diagnostics, start int) bool {
	if diags == nil || start >= len(diags.Items) {
		return false
	}
	for _, item := range diags.Items[start:] {
		if item.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func (p *Parser) readWithItemExprText() (string, diag.Span, bool) {
	p.skipTriviaInline()
	start := p.pos()
	startOff := p.off
	if p.eof() || p.peek() == ',' || p.peek() == '{' {
		span := diag.NewSpan(p.file, start, start)
		p.diags.AddError(diag.CodeE023, "expected expression in with clause", span, `use a variable name or table projection such as cases["x"]`)
		return "", span, false
	}

	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := rune(0)
	escaped := false
	for !p.eof() {
		r := p.peek()
		if quote != 0 {
			p.advance()
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}

		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			if r == ',' || r == '{' || r == '#' || p.startsNextWithBoundaryWord(startOff) {
				break
			}
		}

		switch r {
		case '\'', '"':
			quote = r
			escaped = false
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		}
		p.advance()
	}

	end := p.pos()
	raw := strings.TrimSpace(string(p.src[startOff:p.off]))
	if raw == "" {
		span := diag.NewSpan(p.file, start, end)
		p.diags.AddError(diag.CodeE023, "expected expression in with clause", span, `use a variable name or table projection such as cases["x"]`)
		return "", span, false
	}
	return raw, diag.NewSpan(p.file, start, end), true
}

func (p *Parser) startsNextWithBoundaryWord(itemStartOff int) bool {
	if p.off <= itemStartOff {
		return false
	}
	if p.off > 0 && !unicode.IsSpace(p.src[p.off-1]) {
		return false
	}
	word, ok := p.peekWord()
	if !ok {
		return false
	}
	return p.headerWordFollowedBySpace(word)
}

func (p *Parser) parseNameList() []string {
	out := make([]string, 0)
	for {
		name, _ := p.parseRequiredIdent(diag.CodeE028, "expected identifier in dependency list")
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

func (p *Parser) parseRequiredIdent(code diag.Code, message string) (string, diag.Span) {
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
