// parse `with`-clause item lists into expression-backed `[]ast.WithItem`
package parser

import (
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/shellvar"
)

func (p *Parser) parseWithItems() []ast.WithItem {
	items := make([]ast.WithItem, 0)
	for {
		raw, span, ok := p.readWithItemExprText()
		if !ok {
			break
		}
		exprText, alias, aliasSpan, ok := splitWithAlias(raw, span, p.diags)
		if !ok {
			break
		}
		before := len(p.diags.Items)
		expr, parsed := ParseStandaloneExpr(p.file, exprText, span.Start, p.diags)
		if !parsed || expr == nil || parserAddedErrorSince(p.diags, before) {
			p.diags.AddError(diag.CodeE023, "invalid with-clause expression", span, `use a variable name or table projection such as cases["x"]`)
			break
		}
		items = append(items, ast.WithItem{Expr: expr, Alias: alias, AliasSpan: aliasSpan, Span: span})

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
	aliasStarted := false
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
			if r == ',' || r == '{' || r == '#' {
				break
			}
			word, isBoundaryWord := p.nextWithItemBoundaryWord(startOff)
			if isBoundaryWord {
				if isWithItemBoundaryWord(word) {
					break
				}
				if word == "as" {
					aliasStarted = true
				} else if !aliasStarted {
					break
				}
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

func (p *Parser) nextWithItemBoundaryWord(itemStartOff int) (string, bool) {
	if p.off <= itemStartOff {
		return "", false
	}
	if p.off > 0 && !unicode.IsSpace(p.src[p.off-1]) {
		return "", false
	}
	word, ok := p.peekWord()
	if !ok {
		return "", false
	}
	if !p.headerWordFollowedBySpace(word) {
		return "", false
	}
	return word, true
}

func isWithItemBoundaryWord(word string) bool {
	switch word {
	case "after", "with", "nproc", "fsub":
		return true
	default:
		return false
	}
}

func splitWithAlias(raw string, span diag.Span, diags *diag.Diagnostics) (string, string, diag.Span, bool) {
	exprText := strings.TrimSpace(raw)
	asStart := findTopLevelWithAlias(raw)
	if asStart < 0 {
		return exprText, "", diag.Span{}, true
	}
	exprText = strings.TrimSpace(raw[:asStart])
	if exprText == "" {
		diags.AddError(diag.CodeE023, "expected expression before with-clause alias", spanForRawRange(span, raw, 0, asStart), `use syntax such as with x as y`)
		return "", "", diag.Span{}, false
	}
	aliasTailStart := asStart + len("as")
	tail := raw[aliasTailStart:]
	leading := leadingSpaceLen(tail)
	aliasTailStart += leading
	aliasTail := strings.TrimSpace(tail)
	fields := strings.Fields(aliasTail)
	if len(fields) != 1 {
		diags.AddError(diag.CodeE023, "expected one alias identifier after 'as' in with clause", spanForRawRange(span, raw, asStart, len(raw)), `use syntax such as with x as y`)
		return "", "", diag.Span{}, false
	}
	alias := fields[0]
	aliasSpan := spanForRawRange(span, raw, aliasTailStart, aliasTailStart+len(alias))
	if !shellvar.ValidName(alias) {
		diags.AddError(diag.CodeE023, "invalid with-clause alias", aliasSpan, "use a shell variable name such as x, system_name, or _tmp")
		return "", "", diag.Span{}, false
	}
	return exprText, alias, aliasSpan, true
}

func findTopLevelWithAlias(raw string) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := rune(0)
	escaped := false
	for idx, r := range raw {
		if quote != 0 {
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
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && strings.HasPrefix(raw[idx:], "as") && withAliasBoundary(raw, idx) {
			return idx
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
	}
	return -1
}

func withAliasBoundary(raw string, idx int) bool {
	if idx <= 0 || idx+2 >= len(raw) {
		return false
	}
	return unicode.IsSpace(rune(raw[idx-1])) && unicode.IsSpace(rune(raw[idx+2]))
}

func leadingSpaceLen(text string) int {
	for idx, r := range text {
		if !unicode.IsSpace(r) {
			return idx
		}
	}
	return len(text)
}

func spanForRawRange(span diag.Span, raw string, start, end int) diag.Span {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > len(raw) {
		end = len(raw)
	}
	startPos := advancePosition(span.Start, raw[:start])
	endPos := advancePosition(startPos, raw[start:end])
	return diag.NewSpan(span.File, startPos, endPos)
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
