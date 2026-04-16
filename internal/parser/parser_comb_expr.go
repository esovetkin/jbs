// parses `param` final combination expressions into `ast.CombExpr`
//
// handle the dedicated combination algebra grammar (`+` zip / `*`
// product), precedence (`*` before `+`), parentheses, identifier
// leaves, and emits combination-specific syntax diagnostics when
// final-expression structure is invalid.
package parser

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

func (p *tokenParser) parseCombExpr() ast.CombExpr {
	return p.parseCombAdd()
}

func (p *tokenParser) parseCombAdd() ast.CombExpr {
	left := p.parseCombMul()
	for p.peek().Type == lexer.TokenPlus {
		op := p.next()
		right := p.parseCombMul()
		left = ast.CombBinary{
			Left:   left,
			Op:     op.Text,
			OpSpan: op.Span,
			Right:  right,
			Span:   diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseCombMul() ast.CombExpr {
	left := p.parseCombPrimary()
	for p.peek().Type == lexer.TokenStar {
		op := p.next()
		right := p.parseCombPrimary()
		left = ast.CombBinary{
			Left:   left,
			Op:     op.Text,
			OpSpan: op.Span,
			Right:  right,
			Span:   diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseCombPrimary() ast.CombExpr {
	tok := p.peek()
	if tok.Type == lexer.TokenIdent {
		nameTok := p.next()
		span := nameTok.Span
		if p.peek().Type == lexer.TokenDot {
			p.next()
			memberTok := p.expect(lexer.TokenIdent, diag.CodeE064, "expected identifier after '.'")
			span = diag.Merge(nameTok.Span, memberTok.Span)
			if p.peek().Type == lexer.TokenLParen {
				callSpan := p.consumeCombCallTail()
				if !callSpan.IsZero() {
					span = diag.Merge(span, callSpan)
				}
				p.diags.AddError(
					diag.CodeE060,
					"function call is not allowed in final combination expression",
					span,
					"assign the call result to a variable, then use the variable in the final expression",
				)
				return ast.CombIdent{Name: "", Span: span}
			}
			return ast.CombIdent{
				Name: nameTok.Value + "." + memberTok.Value,
				Span: span,
			}
		}
		if p.peek().Type == lexer.TokenLParen {
			callSpan := p.consumeCombCallTail()
			if !callSpan.IsZero() {
				span = diag.Merge(span, callSpan)
			}
			p.diags.AddError(
				diag.CodeE060,
				"function call is not allowed in final combination expression",
				span,
				"assign the call result to a variable, then use the variable in the final expression",
			)
			return ast.CombIdent{Name: "", Span: span}
		}
		return ast.CombIdent{Name: nameTok.Value, Span: nameTok.Span}
	}
	if tok.Type == lexer.TokenLParen {
		p.next()
		expr := p.parseCombExpr()
		p.expect(lexer.TokenRParen, diag.CodeE059, "expected ')' in combination expression")
		return expr
	}
	p.diags.AddError(diag.CodeE060,
		fmt.Sprintf("unexpected token '%s' in combination expression", tok.Text),
		tok.Span,
		"combination expression allows identifiers, +, *, and parentheses",
	)
	p.next()
	return ast.CombIdent{Name: "", Span: tok.Span}
}

func (p *tokenParser) consumeCombCallTail() diag.Span {
	open := p.expect(lexer.TokenLParen, diag.CodeE062, "expected '(' after identifier")
	if open.Span.IsZero() {
		return diag.Span{}
	}
	span := open.Span
	depth := 1
	for depth > 0 {
		tok := p.peek()
		if tok.Type == lexer.TokenEOF {
			p.diags.AddError(diag.CodeE059, "expected ')' in combination expression", span, "close the function call with ')'")
			return span
		}
		tok = p.next()
		span = diag.Merge(span, tok.Span)
		switch tok.Type {
		case lexer.TokenLParen:
			depth++
		case lexer.TokenRParen:
			depth--
		}
	}
	return span
}
