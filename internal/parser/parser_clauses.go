// parse header clauses attached to blocks
//
// handle `with`, `after`, and do-header options.
package parser

import (
	"fmt"
	"strconv"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

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

type doHeaderOptions struct {
	NProc *int
	seen  map[string]diag.Span
}

func (p *Parser) parseOptionalDoHeaderClauses() ([]string, []ast.WithItem, doHeaderOptions) {
	after := make([]string, 0)
	withItems := make([]ast.WithItem, 0)
	opts := doHeaderOptions{}
	for {
		p.skipTriviaInline()
		word, ok := p.peekWord()
		if !ok {
			break
		}
		switch word {
		case "after":
			p.consumeWord()
			after = append(after, p.parseNameList()...)
		case "with":
			p.consumeWord()
			withItems = append(withItems, p.parseWithItems()...)
		case "nproc":
			if !p.headerWordFollowedBySpace(word) {
				return after, withItems, opts
			}
			p.consumeWord()
			opts.setNProc(p.parseNProcValue("do"), diag.NewSpan(p.file, p.pos(), p.pos()), p.diags)
		default:
			return after, withItems, opts
		}
	}
	return after, withItems, opts
}

func (p *Parser) headerWordFollowedBySpace(word string) bool {
	i := p.off + len(word)
	return i >= len(p.src) || unicode.IsSpace(p.src[i])
}

func (o *doHeaderOptions) setNProc(value *int, at diag.Span, diags *diag.Diagnostics) {
	if value == nil {
		return
	}
	if o.seen == nil {
		o.seen = map[string]diag.Span{}
	}
	if _, exists := o.seen["nproc"]; exists {
		diags.AddError(diag.CodeE033,
			"duplicate do header option 'nproc'",
			at,
			"set this option at most once per block",
		)
		return
	}
	o.seen["nproc"] = at
	o.NProc = value
}

func (p *Parser) parseNProcValue(kind string) *int {
	p.skipTriviaInline()
	valueText, valueSpan, valueOK := p.readHeaderIntegerValue()
	if !valueOK {
		p.diags.AddError(diag.CodeE034,
			fmt.Sprintf("%s header option 'nproc' expects an integer value", kind),
			valueSpan,
			"use syntax: nproc <integer>",
		)
		return nil
	}
	parsed, err := strconv.Atoi(valueText)
	if err != nil {
		p.diags.AddError(diag.CodeE034,
			fmt.Sprintf("%s header option 'nproc' expects an integer value", kind),
			valueSpan,
			"use syntax: nproc <integer>",
		)
		return nil
	}
	return &parsed
}

func (p *Parser) readHeaderIntegerValue() (string, diag.Span, bool) {
	p.skipTriviaInline()
	start := p.pos()
	if p.eof() {
		return "", diag.NewSpan(p.file, start, start), false
	}
	startOff := p.off
	if p.peek() == '+' || p.peek() == '-' {
		p.advance()
	}
	digits := 0
	for !p.eof() && unicode.IsDigit(p.peek()) {
		p.advance()
		digits++
	}
	end := p.pos()
	if digits == 0 {
		return "", diag.NewSpan(p.file, start, end), false
	}
	if !p.eof() {
		r := p.peek()
		if !(unicode.IsSpace(r) || r == ',' || r == ';' || r == '{' || r == '}') {
			for !p.eof() {
				r = p.peek()
				if unicode.IsSpace(r) || r == ',' || r == ';' || r == '{' || r == '}' {
					break
				}
				p.advance()
			}
			return string(p.src[startOff:p.off]), diag.NewSpan(p.file, start, p.pos()), false
		}
	}
	return string(p.src[startOff:p.off]), diag.NewSpan(p.file, start, end), true
}
