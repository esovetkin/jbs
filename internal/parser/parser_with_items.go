// parse `with`-clause item lists into `[]ast.WithItem`
//
// The canonical surface is intentionally small:
//   - `with source`
//   - `with source[col0, col1]`
//
// The parser still recognizes legacy `with a from p` / `with (a, b)
// from p` shapes only to emit targeted migration diagnostics and keep
// header recovery local.
package parser

import (
	"fmt"
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
	if p.peek() == '(' {
		return p.parseLegacyGroupedWithItem()
	}

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

	p.skipTriviaInline()
	if keyword, ok := p.peekWord(); ok && (keyword == "from" || keyword == "in") {
		kwStart := p.pos()
		kwEnd := p.consumeWord()
		actualSource, sourceRefSpan := p.parseQualifiedName(diag.CodeE024, "expected source global binding name after with clause source keyword")
		legacySpan := diag.Merge(item.Span, diag.NewSpan(p.file, kwStart, kwEnd))
		if !sourceRefSpan.IsZero() {
			legacySpan = diag.Merge(legacySpan, sourceRefSpan)
		}
		if actualSource != "" {
			item = ast.WithItem{
				Source:    actualSource,
				Selectors: []string{source},
				Span:      diag.Merge(sourceSpan, sourceRefSpan),
			}
		}
		p.diags.AddError(
			diag.CodeE023,
			fmt.Sprintf("legacy with-clause syntax `%s %s %s` is no longer supported", source, keyword, actualSource),
			legacySpan,
			rewriteLegacyWithHint(source, []string{source}, actualSource, keyword),
		)
	}

	item.Span = p.rejectWithAlias(item.Span)
	return item, item.Source != ""
}

func (p *Parser) parseLegacyGroupedWithItem() (ast.WithItem, bool) {
	names, tupleSpan, ok := p.parseWithNames()
	if !ok || len(names) == 0 {
		return ast.WithItem{}, false
	}

	selectors := make([]string, 0, len(names))
	for _, name := range names {
		selectors = append(selectors, name.Name)
	}

	p.skipTriviaInline()
	keyword, hasKeyword := p.peekWord()
	if !hasKeyword || (keyword != "from" && keyword != "in") {
		span := tupleSpan
		p.diags.AddError(
			diag.CodeE023,
			"grouped with-clause syntax is no longer supported",
			span,
			"use `with source[col0, col1]`",
		)
		span = p.rejectWithAlias(span)
		return ast.WithItem{}, false
	}

	kwStart := p.pos()
	kwEnd := p.consumeWord()
	source, sourceSpan := p.parseQualifiedName(diag.CodeE024, "expected source global binding name after with clause source keyword")
	span := diag.Merge(tupleSpan, diag.NewSpan(p.file, kwStart, kwEnd))
	if !sourceSpan.IsZero() {
		span = diag.Merge(span, sourceSpan)
	}
	p.diags.AddError(
		diag.CodeE023,
		fmt.Sprintf("legacy grouped with-clause syntax `%s %s %s` is no longer supported", legacyGroupedWithText(selectors), keyword, source),
		span,
		rewriteLegacyWithHint(legacyGroupedWithText(selectors), selectors, source, keyword),
	)
	span = p.rejectWithAlias(span)
	if source == "" {
		return ast.WithItem{}, false
	}
	return ast.WithItem{
		Source:    source,
		Selectors: selectors,
		Span:      span,
	}, true
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

type withName struct {
	Name string
	Span diag.Span
}

func (p *Parser) parseWithNames() ([]withName, diag.Span, bool) {
	p.skipTriviaInline()
	if p.peek() != '(' {
		name, span := p.parseQualifiedName(diag.CodeE023, "expected identifier in with clause")
		if name == "" {
			return nil, span, false
		}
		return []withName{{Name: name, Span: span}}, span, true
	}

	tupleStart := p.pos()
	p.advance()
	names := make([]withName, 0)

	for {
		p.skipTriviaInline()
		if p.peek() == ')' {
			if len(names) == 0 {
				span := diag.NewSpan(p.file, tupleStart, p.pos())
				p.diags.AddError(diag.CodeE023, "empty tuple in with clause", span, "add at least one identifier inside parentheses")
			} else {
				span := diag.NewSpan(p.file, tupleStart, p.pos())
				p.diags.AddError(diag.CodeE023, "trailing comma in with-clause tuple", span, "remove trailing comma or add another identifier")
			}
			p.advance()
			return names, diag.NewSpan(p.file, tupleStart, p.pos()), len(names) > 0
		}

		name, span := p.parseQualifiedName(diag.CodeE023, "expected identifier in with clause")
		if name == "" {
			return names, diag.NewSpan(p.file, tupleStart, p.pos()), len(names) > 0
		}
		names = append(names, withName{Name: name, Span: span})

		p.skipTriviaInline()
		switch p.peek() {
		case ',':
			p.advance()
		case ')':
			p.advance()
			return names, diag.NewSpan(p.file, tupleStart, p.pos()), true
		default:
			span := diag.NewSpan(p.file, tupleStart, p.pos())
			p.diags.AddError(diag.CodeE023, "unterminated tuple in with clause; missing ')'", span, "close tuple imports with ')'")
			return names, span, len(names) > 0
		}
	}
}

func (p *Parser) rejectWithAlias(itemSpan diag.Span) diag.Span {
	p.skipTriviaInline()
	word, ok := p.peekWord()
	if !ok || word != "as" {
		return itemSpan
	}

	asStart := p.pos()
	asEnd := p.consumeWord()
	_, aliasSpan := p.parseRequiredIdent(diag.CodeE023, "expected alias identifier after 'as' in with clause")
	span := itemSpan
	extraSpan := diag.Merge(diag.NewSpan(p.file, asStart, asEnd), aliasSpan)
	if !extraSpan.IsZero() {
		span = diag.Merge(span, extraSpan)
	}
	p.diags.AddError(
		diag.CodeE023,
		"with-clause aliasing is no longer supported",
		span,
		"import the source directly with `with source` or select columns with `with source[col]`",
	)
	return span
}

func legacyGroupedWithText(selectors []string) string {
	return "(" + strings.Join(selectors, ", ") + ")"
}

func rewriteLegacyWithHint(oldHead string, selectors []string, source string, keyword string) string {
	if source == "" {
		return "use syntax: with source[col0, col1]"
	}
	return fmt.Sprintf("rewrite `with %s %s %s` as `with %s[%s]`", oldHead, keyword, source, source, strings.Join(selectors, ", "))
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
