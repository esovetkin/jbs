// parse header clauses attached to blocks
//
// handle `with`, `after`, submit-header `use`, and step options,
// including repetition/ordering rules, value parsing, and
// clause-specific syntax diagnostics.
package parser

import (
	"fmt"
	"strconv"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func (p *Parser) parseOptionalWithClause() []ast.WithItem {
	p.skipTriviaInline()
	word, ok := p.peekWord()
	if !ok || word != "with" {
		return nil
	}
	p.consumeWord()
	return p.parseWithItems()
}

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

type stepHeaderOptions struct {
	MaxAsync   *int
	Procs      *int
	Iterations *int
	seen       map[string]diag.Span
}

func (o *stepHeaderOptions) set(key string, value int, at diag.Span) bool {
	if o.seen == nil {
		o.seen = map[string]diag.Span{}
	}
	if _, exists := o.seen[key]; exists {
		return false
	}
	o.seen[key] = at
	switch key {
	case "max_async":
		v := value
		o.MaxAsync = &v
	case "procs":
		v := value
		o.Procs = &v
	case "iterations":
		v := value
		o.Iterations = &v
	}
	return true
}

func (p *Parser) parseOptionalDoHeaderClauses() ([]string, []ast.WithItem, stepHeaderOptions) {
	after := make([]string, 0)
	withItems := make([]ast.WithItem, 0)
	opts := stepHeaderOptions{}
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
		default:
			if p.parseStepHeaderOption("do", &opts) {
				continue
			}
			return after, withItems, opts
		}
	}
	return after, withItems, opts
}

func (p *Parser) parseOptionalSubmitHeaderClauses() ([]string, []string, []ast.WithItem, stepHeaderOptions) {
	after := make([]string, 0)
	useNames := make([]string, 0)
	withItems := make([]ast.WithItem, 0)
	opts := stepHeaderOptions{}
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
		case "use":
			p.consumeWord()
			useNames = append(useNames, p.parseNameList()...)
		default:
			if p.parseStepHeaderOption("submit", &opts) {
				continue
			}
			return after, useNames, withItems, opts
		}
	}
	return after, useNames, withItems, opts
}

func (p *Parser) parseStepHeaderOption(kind string, opts *stepHeaderOptions) bool {
	p.skipTriviaInline()
	word, ok := p.peekWord()
	if !ok {
		return false
	}
	if !p.looksLikeStepHeaderAssignment() && !isAllowedStepOptionKey(word) {
		return false
	}

	keyStart := p.pos()
	keyEnd := p.consumeWord()
	keySpan := diag.NewSpan(p.file, keyStart, keyEnd)
	key := word

	p.skipTriviaInline()
	if p.peek() != '=' {
		p.diags.AddError(diag.CodeE035,
			fmt.Sprintf("expected '=' after %s header option '%s'", kind, key),
			keySpan,
			"use syntax: "+key+"=<integer>",
		)
		return true
	}
	p.advance()

	valueText, valueSpan, valueOK := p.readStepHeaderOptionValue()
	if !valueOK {
		p.diags.AddError(diag.CodeE034,
			fmt.Sprintf("%s header option '%s' expects an integer value", kind, key),
			keySpan,
			"use syntax: "+key+"=<integer>",
		)
		return true
	}

	if !isAllowedStepOptionKey(key) {
		p.diags.AddError(diag.CodeE032,
			fmt.Sprintf("unknown %s header option '%s'", kind, key),
			keySpan,
			"allowed options are "+allowedStepOptionKeysHint(),
		)
		return true
	}

	parsed, err := strconv.Atoi(valueText)
	if err != nil {
		p.diags.AddError(diag.CodeE034,
			fmt.Sprintf("%s header option '%s' expects an integer value", kind, key),
			valueSpan,
			"use syntax: "+key+"=<integer>",
		)
		return true
	}

	if !opts.set(key, parsed, keySpan) {
		p.diags.AddError(diag.CodeE033,
			fmt.Sprintf("duplicate %s header option '%s'", kind, key),
			keySpan,
			"set this option at most once per block",
		)
		return true
	}
	return true
}

func (p *Parser) looksLikeStepHeaderAssignment() bool {
	i := p.off
	if i >= len(p.src) {
		return false
	}
	r := p.src[i]
	if !(unicode.IsLetter(r) || r == '_') {
		return false
	}
	for i < len(p.src) {
		r = p.src[i]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			i++
			continue
		}
		break
	}
	for i < len(p.src) {
		r = p.src[i]
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			i++
			continue
		}
		if r == '#' {
			for i < len(p.src) && p.src[i] != '\n' {
				i++
			}
			continue
		}
		return r == '='
	}
	return false
}

func (p *Parser) readStepHeaderOptionValue() (string, diag.Span, bool) {
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
