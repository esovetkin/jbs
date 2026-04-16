// parse `with`-clause item lists into `[]ast.WithItem`
//
// support full-source imports, `name from source`, `(x, y) from
// source`, and qualified references, while preserving spans and
// reporting detailed syntax errors for malformed import item shapes.
package parser

import (
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func (p *Parser) parseWithItems() []ast.WithItem {
	items := make([]ast.WithItem, 0)
	currentFrom := ""
	for {
		p.skipTriviaInline()
		itemStart := p.pos()
		if p.peek() == '(' {
			names, ok := p.parseWithNames()
			if !ok || len(names) == 0 {
				break
			}
			src := ""
			srcSpan := diag.Span{}
			hasExplicitSource := false
			p.skipTriviaInline()
			if word, ok := p.peekWord(); ok && (word == "from" || word == "in") {
				p.consumeWord()
				srcName, fromSpan := p.parseQualifiedName(diag.CodeE024, "expected source parameterset name after with clause source keyword")
				src = srcName
				srcSpan = fromSpan
				currentFrom = srcName
				hasExplicitSource = true
			}
			if !hasExplicitSource && currentFrom != "" {
				src = currentFrom
			}
			p.skipTriviaInline()
			if word, ok := p.peekWord(); ok && word == "as" {
				asStart := p.pos()
				asEnd := p.consumeWord()
				_, parsedAliasSpan := p.parseRequiredIdent(diag.CodeE023, "expected alias identifier after 'as' in with clause")
				span := diag.Merge(diag.NewSpan(p.file, asStart, asEnd), parsedAliasSpan)
				p.diags.AddError(diag.CodeE023, "alias is only allowed for a single with-clause item", span, "use `name as alias` or split tuple imports into individual aliased items")
			}
			for _, name := range names {
				item := ast.WithItem{Name: name.Name, Span: name.Span, From: src}
				if src != "" && !srcSpan.IsZero() {
					item.Span = diag.Merge(item.Span, srcSpan)
				}
				items = append(items, item)
			}
		} else {
			name, nameSpan := p.parseQualifiedName(diag.CodeE023, "expected identifier in with clause")
			if name == "" {
				break
			}
			p.skipTriviaInline()
			if p.peek() == '[' {
				sliceNames, sliceSpan, ok := p.parseWithSliceNames()
				if !ok {
					break
				}
				combAlias := ""
				aliasSpan := diag.Span{}
				p.skipTriviaInline()
				if word, ok := p.peekWord(); ok && word == "as" {
					asStart := p.pos()
					asEnd := p.consumeWord()
					aliasName, parsedAliasSpan := p.parseRequiredIdent(diag.CodeE023, "expected alias identifier after 'as' in with clause")
					combAlias = aliasName
					aliasSpan = diag.Merge(diag.NewSpan(p.file, asStart, asEnd), parsedAliasSpan)
				}
				itemSpan := diag.Merge(nameSpan, sliceSpan)
				if !aliasSpan.IsZero() {
					itemSpan = diag.Merge(itemSpan, aliasSpan)
				}
				items = append(items, ast.WithItem{
					Name:        name,
					SourceExpr:  name,
					SourceSlice: sliceNames,
					CombAlias:   combAlias,
					Span:        itemSpan,
				})
			} else {
				src := ""
				srcSpan := diag.Span{}
				hasExplicitFrom := false
				p.skipTriviaInline()
				word, ok := p.peekWord()
				if ok && (word == "from" || word == "in") {
					p.consumeWord()
					srcName, fromSpan := p.parseQualifiedName(diag.CodeE024, "expected source parameterset name after with clause source keyword")
					src = srcName
					srcSpan = fromSpan
					currentFrom = srcName
					hasExplicitFrom = true
				}

				alias := ""
				aliasSpan := diag.Span{}
				p.skipTriviaInline()
				if word, ok := p.peekWord(); ok && word == "as" {
					asStart := p.pos()
					asEnd := p.consumeWord()
					aliasName, parsedAliasSpan := p.parseRequiredIdent(diag.CodeE023, "expected alias identifier after 'as' in with clause")
					alias = aliasName
					aliasSpan = diag.Merge(diag.NewSpan(p.file, asStart, asEnd), parsedAliasSpan)
				}
				if !hasExplicitFrom && currentFrom != "" && alias == "" {
					src = currentFrom
				}
				item := ast.WithItem{Name: name, Span: nameSpan, From: src}
				if src != "" && !srcSpan.IsZero() {
					item.Span = diag.Merge(item.Span, srcSpan)
				}
				if alias != "" {
					item.Alias = alias
					if !aliasSpan.IsZero() {
						item.Span = diag.Merge(item.Span, aliasSpan)
					}
				}
				items = append(items, item)
			}
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

func (p *Parser) parseWithNames() ([]withName, bool) {
	p.skipTriviaInline()
	if p.peek() != '(' {
		name, span := p.parseQualifiedName(diag.CodeE023, "expected identifier in with clause")
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
				p.diags.AddError(diag.CodeE023, "empty tuple in with clause", span, "add at least one identifier inside parentheses")
			} else {
				span := diag.NewSpan(p.file, tupleStart, p.pos())
				p.diags.AddError(diag.CodeE023, "trailing comma in with-clause tuple", span, "remove trailing comma or add another identifier")
			}
			p.advance()
			return names, len(names) > 0
		}

		name, span := p.parseQualifiedName(diag.CodeE023, "expected identifier in with clause")
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
			p.diags.AddError(diag.CodeE023, "unterminated tuple in with clause; missing ')'", span, "close tuple imports with ')'")
			return names, len(names) > 0
		}
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
