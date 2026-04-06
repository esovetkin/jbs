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
		if p.peek().Type == lexer.TokenDot {
			p.next()
			memberTok := p.expect(lexer.TokenIdent, diag.CodeE064, "expected identifier after '.'")
			return ast.CombIdent{
				Name: nameTok.Value + "." + memberTok.Value,
				Span: diag.Merge(nameTok.Span, memberTok.Span),
			}
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
