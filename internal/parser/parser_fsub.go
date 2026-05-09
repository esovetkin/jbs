package parser

import (
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
)

func (p *Parser) parseFileSubstitution(start diag.Position) ast.FileSubstitution {
	path, pathSpan := p.parseFSubPath()
	p.skipTriviaInline()
	open := p.pos()
	if p.eof() || p.peek() != '{' {
		p.diags.AddError(
			diag.CodeE035,
			"expected '{' to start fsub substitution map",
			diag.NewSpan(p.file, open, open),
			`use syntax: fsub "template" { "pattern": value }`,
		)
		return ast.FileSubstitution{Path: path, PathSpan: pathSpan, Span: diag.NewSpan(p.file, start, pathSpan.End)}
	}

	rawStart := p.pos()
	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	rawEnd := p.pos()
	if !ok {
		return ast.FileSubstitution{Path: path, PathSpan: pathSpan, Span: diag.NewSpan(p.file, start, blockEnd)}
	}
	rules := parseFSubRules(p.file, string(p.src[rawStart.Offset:rawEnd.Offset]), rawStart, p.diags)
	return ast.FileSubstitution{
		Path:      path,
		PathSpan:  pathSpan,
		Rules:     rules,
		BodyRaw:   body,
		BodyStart: innerStart,
		Span:      diag.NewSpan(p.file, start, blockEnd),
	}
}

func (p *Parser) parseFSubPath() (string, diag.Span) {
	p.skipTriviaInline()
	start := p.pos()
	startOff := p.off
	if !p.advanceUntilFSubMapOpen() {
		span := diag.NewSpan(p.file, start, p.pos())
		if strings.TrimSpace(string(p.src[startOff:p.off])) == "" {
			p.diags.AddError(diag.CodeE035, "fsub path must be a string literal", span, `use fsub "template" { ... }`)
		}
		return "", span
	}
	text := strings.TrimSpace(string(p.src[startOff:p.off]))
	if text == "" {
		span := diag.NewSpan(p.file, start, p.pos())
		p.diags.AddError(diag.CodeE035, "fsub path must be a string literal", span, `use fsub "template" { ... }`)
		return "", span
	}

	tokens := lexer.LexFrom(p.file, text, start, p.diags)
	tp := &tokenParser{tokens: tokens, diags: p.diags}
	tp.skipStmtSeparators()
	expr := tp.parseExpr()
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		p.diags.AddError(diag.CodeE035, "unexpected tokens after fsub path", tp.peek().Span, "use one quoted path")
	}
	if str, ok := expr.(ast.StringExpr); ok {
		return str.Value, str.Span
	}
	span := diag.NewSpan(p.file, start, p.pos())
	if expr != nil {
		span = expr.GetSpan()
	}
	p.diags.AddError(diag.CodeE035, "fsub path must be a string literal", span, `use fsub "template" { ... }`)
	return "", span
}

func (p *Parser) advanceUntilFSubMapOpen() bool {
	inSingle := false
	inDouble := false
	escaped := false
	for !p.eof() {
		r := p.peek()
		if escaped {
			escaped = false
			p.advance()
			continue
		}
		if inSingle {
			if r == '\\' {
				escaped = true
			} else if r == '\'' {
				inSingle = false
			}
			p.advance()
			continue
		}
		if inDouble {
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				inDouble = false
			}
			p.advance()
			continue
		}
		switch r {
		case '\'':
			inSingle = true
			p.advance()
		case '"':
			inDouble = true
			p.advance()
		case '{':
			return true
		default:
			p.advance()
		}
	}
	return false
}

func parseFSubRules(file, raw string, start diag.Position, diags *diag.Diagnostics) []ast.FileSubstitutionRule {
	tokens := lexer.LexFrom(file, raw, start, diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	tp.skipStmtSeparators()
	expr := tp.parseExpr()
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		diags.AddError(diag.CodeE035, "unexpected tokens after fsub substitution map", tp.peek().Span, "remove trailing syntax")
	}
	dict, ok := expr.(ast.DictExpr)
	if !ok {
		span := diag.NewSpan(file, start, start)
		if expr != nil {
			span = expr.GetSpan()
		}
		diags.AddError(diag.CodeE035, "fsub substitution map must use dictionary syntax", span, `use { "pattern": value }`)
		return nil
	}

	rules := make([]ast.FileSubstitutionRule, 0, len(dict.Entries))
	for _, entry := range dict.Entries {
		key, ok := entry.Key.(ast.StringExpr)
		if !ok {
			span := entry.Span
			if entry.Key != nil {
				span = entry.Key.GetSpan()
			}
			diags.AddError(diag.CodeE035, "fsub pattern must be a string literal", span, `use "regex": value`)
			continue
		}
		rules = append(rules, ast.FileSubstitutionRule{
			Pattern:     key.Value,
			PatternSpan: key.Span,
			Expr:        entry.Value,
			Span:        entry.Span,
		})
	}
	return rules
}
