// parse `with`-clause item lists into `[]ast.WithItem`
//
// The canonical surface is intentionally small:
//   - `with source`
//   - `with source[col0, col1]`
package parser

import (
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func (p *Parser) parseWithItems() []ast.WithItem {
	items := make([]ast.WithItem, 0)
	for {
		p.skipTriviaInline()
		itemStart := p.pos()
		item, ok := p.parseWithItem()
		if ok {
			items = append(items, item)
		}
		if itemStart == p.pos() {
			break
		}
		p.skipTriviaInline()
		if p.peek() != ',' {
			break
		}
		p.advance()
	}
	return items
}

func (p *Parser) parseWithItem() (ast.WithItem, bool) {
	p.skipTriviaInline()
	source, sourceSpan := p.parseQualifiedName(diag.CodeE023, "expected source global binding name in with clause")
	if source == "" {
		return ast.WithItem{}, false
	}
	item := ast.WithItem{Source: source, Span: sourceSpan}

	p.skipTriviaInline()
	if p.peek() == '[' {
		selectors, sliceSpan, ok := p.parseWithSliceNames()
		if !ok {
			return ast.WithItem{}, false
		}
		item.Selectors = selectors
		item.Span = diag.Merge(item.Span, sliceSpan)
	}

	if p.rejectUnsupportedWithTail(item.Span) {
		return ast.WithItem{}, false
	}
	return item, item.Source != ""
}

func (p *Parser) rejectUnsupportedWithTail(itemSpan diag.Span) bool {
	p.skipTriviaInline()
	word, ok := p.peekWord()
	if !ok || (word != "from" && word != "in" && word != "as") {
		return false
	}

	tailStart := p.pos()
	for !p.eof() {
		r := p.peek()
		if r == ',' || r == '{' {
			break
		}
		p.advance()
	}
	tailSpan := diag.NewSpan(p.file, tailStart, p.pos())
	p.diags.AddError(
		diag.CodeE023,
		"invalid with-clause syntax",
		diag.Merge(itemSpan, tailSpan),
		"use `with source` or `with source[col0, col1]`",
	)
	return true
}

func (p *Parser) parseWithSliceNames() ([]string, diag.Span, bool) {
	start := p.pos()
	if p.peek() != '[' {
		p.diags.AddError(diag.CodeE023, "expected '[' in with slice syntax", diag.NewSpan(p.file, start, start), "use syntax: source[var0,var1]")
		return nil, diag.NewSpan(p.file, start, start), false
	}
	p.advance()
	names := make([]string, 0, 2)
	for {
		p.skipTriviaInline()
		if p.peek() == ']' {
			end := p.pos()
			p.advance()
			if len(names) == 0 {
				span := diag.NewSpan(p.file, start, end)
				p.diags.AddError(diag.CodeE023, "empty with-slice selector list", span, "add at least one variable name inside []")
				return nil, span, false
			}
			return names, diag.NewSpan(p.file, start, p.pos()), true
		}
		name, _ := p.parseQualifiedName(diag.CodeE023, "expected identifier in with slice selector")
		if name == "" {
			return nil, diag.NewSpan(p.file, start, p.pos()), false
		}
		names = append(names, name)
		p.skipTriviaInline()
		if p.peek() == ',' {
			p.advance()
			continue
		}
		if p.peek() == ']' {
			continue
		}
		span := diag.NewSpan(p.file, start, p.pos())
		p.diags.AddError(diag.CodeE023, "unterminated with slice selector list", span, "close selector list with ']'")
		return nil, span, false
	}
}

func (p *Parser) parseQualifiedName(code diag.Code, message string) (string, diag.Span) {
	name, span := p.parseRequiredIdent(code, message)
	if name == "" {
		return "", span
	}
	parts := []string{name}
	for {
		p.skipTriviaInline()
		if p.peek() != '.' {
			break
		}
		p.advance()
		p.skipTriviaInline()
		next, nextSpan := p.parseRequiredIdent(code, message)
		if next == "" {
			return strings.Join(parts, "."), span
		}
		parts = append(parts, next)
		span = diag.Merge(span, nextSpan)
	}
	return strings.Join(parts, "."), span
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
