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
			srcName, fromSpan := p.parseQualifiedName(diag.CodeE024, "expected source parameterset name after 'from'")
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
