// parse assignment-level expressions into AST nodes
//
// implement expression grammar with precedence/associativity
// (logical, compare, arithmetic, unary),
// literals/identifiers/qualified identifiers, lists/tuples,
// conditional expressions, and mode/conversion forms (`shell`,
// `python`, `tuple`, `list`), emitting syntax diagnostics for
// malformed expression constructs.
package parser

import (
	"fmt"
	"strconv"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/lexer"
)

type tokenParser struct {
	tokens []lexer.Token
	idx    int
	diags  *diag.Diagnostics
}

func isAssignToken(tt lexer.TokenType) bool {
	return tt == lexer.TokenEqual ||
		tt == lexer.TokenPlusEqual ||
		tt == lexer.TokenMinusEqual ||
		tt == lexer.TokenStarEqual ||
		tt == lexer.TokenSlashEqual ||
		tt == lexer.TokenPercentEqual
}

func tokenToAssignOp(tt lexer.TokenType) ast.AssignOp {
	switch tt {
	case lexer.TokenPlusEqual:
		return ast.AssignPlusEq
	case lexer.TokenMinusEqual:
		return ast.AssignMinusEq
	case lexer.TokenStarEqual:
		return ast.AssignStarEq
	case lexer.TokenSlashEqual:
		return ast.AssignSlashEq
	case lexer.TokenPercentEqual:
		return ast.AssignPctEq
	default:
		return ast.AssignEq
	}
}

func (p *tokenParser) parseAssignOp() (ast.AssignOp, diag.Span, bool) {
	tok := p.peek()
	if !isAssignToken(tok.Type) {
		return ast.AssignEq, tok.Span, false
	}
	p.next()
	return tokenToAssignOp(tok.Type), tok.Span, true
}

func (p *tokenParser) parseAssignment() ast.Assignment {
	name := p.expect(lexer.TokenIdent, diag.CodeE050, "expected assignment identifier")
	op, _, ok := p.parseAssignOp()
	if !ok {
		tok := p.peek()
		p.diags.AddError(diag.CodeE051, "expected assignment operator in assignment", tok.Span, "use one of: =, +=, -=, *=, /=, %=")
		p.consumeUntilStmtEnd()
		return ast.Assignment{
			Name: name.Value,
			Op:   ast.AssignEq,
			Span: name.Span,
		}
	}
	expr := p.parseExpr()
	span := name.Span
	if expr != nil {
		span = diag.Merge(span, expr.GetSpan())
	}
	if p.peek().Type != lexer.TokenEOF &&
		p.peek().Type != lexer.TokenNewline &&
		p.peek().Type != lexer.TokenSemicolon &&
		p.peek().Type != lexer.TokenComment {
		tok := p.peek()
		p.diags.AddError(diag.CodeE061,
			"unexpected trailing tokens after assignment expression",
			tok.Span,
			"remove unsupported trailing syntax after the expression",
		)
	}
	p.consumeUntilStmtEnd()
	return ast.Assignment{
		Name: name.Value,
		Op:   op,
		Expr: expr,
		Span: span,
	}
}

func (p *tokenParser) parseExpr() ast.Expr {
	return p.parseConditional()
}

func (p *tokenParser) parseConditional() ast.Expr {
	thenExpr := p.parseOr()
	if p.peek().Type == lexer.TokenIf {
		ifTok := p.next()
		cond := p.parseOr()
		p.expect(lexer.TokenElse, diag.CodeE052, "expected 'else' in conditional expression")
		elseExpr := p.parseConditional()
		span := diag.Merge(thenExpr.GetSpan(), elseExpr.GetSpan())
		span = diag.Merge(span, ifTok.Span)
		return ast.ConditionalExpr{
			Then: thenExpr,
			Cond: cond,
			Else: elseExpr,
			Span: span,
		}
	}
	return thenExpr
}

func (p *tokenParser) parseOr() ast.Expr {
	left := p.parseAnd()
	for p.peek().Type == lexer.TokenOr {
		op := p.next()
		right := p.parseAnd()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseAnd() ast.Expr {
	left := p.parseCompare()
	for p.peek().Type == lexer.TokenAnd {
		op := p.next()
		right := p.parseCompare()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseCompare() ast.Expr {
	left := p.parseAdd()
	t := p.peek().Type
	if t == lexer.TokenEqEq || t == lexer.TokenNeq || t == lexer.TokenLT || t == lexer.TokenGT || t == lexer.TokenLE || t == lexer.TokenGE {
		op := p.next()
		right := p.parseAdd()
		return ast.CompareExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseAdd() ast.Expr {
	left := p.parseMul()
	for {
		t := p.peek().Type
		if t != lexer.TokenPlus && t != lexer.TokenMinus {
			break
		}
		op := p.next()
		right := p.parseMul()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseMul() ast.Expr {
	left := p.parseUnary()
	for {
		t := p.peek().Type
		if t != lexer.TokenStar && t != lexer.TokenSlash && t != lexer.TokenPercent {
			break
		}
		op := p.next()
		right := p.parseUnary()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    op.Text,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseUnary() ast.Expr {
	t := p.peek().Type
	if t == lexer.TokenPlus || t == lexer.TokenMinus {
		op := p.next()
		expr := p.parseUnary()
		return ast.UnaryExpr{
			Op:   op.Text,
			Expr: expr,
			Span: diag.Merge(op.Span, expr.GetSpan()),
		}
	}
	return p.parsePrimary()
}

func (p *tokenParser) parsePrimary() ast.Expr {
	tok := p.peek()
	switch tok.Type {
	case lexer.TokenIdent:
		if tok.Value == "true" || tok.Value == "True" || tok.Value == "TRUE" {
			p.next()
			return ast.BoolExpr{Value: true, Span: tok.Span}
		}
		if tok.Value == "false" || tok.Value == "False" || tok.Value == "FALSE" {
			p.next()
			return ast.BoolExpr{Value: false, Span: tok.Span}
		}
		if (tok.Value == "shell" || tok.Value == "python") && p.peekN(1).Type == lexer.TokenLParen {
			modeTok := p.next()
			p.expect(lexer.TokenLParen, diag.CodeE062, "expected '(' after mode expression")
			arg := p.parseExpr()
			close := p.expect(lexer.TokenRParen, diag.CodeE063, "expected ')' to close mode expression")
			return ast.ModeExpr{
				Mode: modeTok.Value,
				Expr: arg,
				Span: diag.Merge(modeTok.Span, close.Span),
			}
		}
		if (tok.Value == "tuple" || tok.Value == "list") && p.peekN(1).Type == lexer.TokenLParen {
			targetTok := p.next()
			p.expect(lexer.TokenLParen, diag.CodeE062, "expected '(' after conversion expression")
			arg := p.parseExpr()
			close := p.expect(lexer.TokenRParen, diag.CodeE063, "expected ')' to close conversion expression")
			return ast.ConvertExpr{
				Target: targetTok.Value,
				Expr:   arg,
				Span:   diag.Merge(targetTok.Span, close.Span),
			}
		}
		nameTok := p.next()
		if p.peek().Type == lexer.TokenDot {
			p.next()
			memberTok := p.expect(lexer.TokenIdent, diag.CodeE064, "expected identifier after '.'")
			return ast.QualifiedIdentExpr{
				Namespace: nameTok.Value,
				Name:      memberTok.Value,
				Span:      diag.Merge(nameTok.Span, memberTok.Span),
			}
		}
		return ast.IdentExpr{Name: nameTok.Value, Span: nameTok.Span}
	case lexer.TokenString:
		p.next()
		return ast.StringExpr{Value: tok.Value, Span: tok.Span}
	case lexer.TokenNumber:
		p.next()
		if isDecimalIntegerLiteral(tok.Value) {
			intValue, err := strconv.ParseInt(tok.Value, 10, 64)
			if err != nil {
				p.diags.AddError(diag.CodeE065, "invalid integer literal", tok.Span, "use a valid 64-bit signed integer literal")
				intValue = 0
			}
			return ast.NumberExpr{
				Raw:      tok.Value,
				Int:      true,
				IntValue: intValue,
				Span:     tok.Span,
			}
		}
		floatValue, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			p.diags.AddError(diag.CodeE066, "invalid floating-point literal", tok.Span, "use a valid floating-point literal")
		}
		return ast.NumberExpr{
			Raw:        tok.Value,
			Int:        false,
			FloatValue: floatValue,
			Span:       tok.Span,
		}
	case lexer.TokenLParen:
		open := p.next()
		p.skipNewlines()
		if p.peek().Type == lexer.TokenRParen {
			close := p.next()
			return ast.TupleExpr{Items: nil, Span: diag.Merge(open.Span, close.Span)}
		}
		first := p.parseExpr()
		p.skipNewlines()
		if p.peek().Type == lexer.TokenComma {
			items := []ast.Expr{first}
			for p.peek().Type == lexer.TokenComma {
				p.next()
				p.skipNewlines()
				if p.peek().Type == lexer.TokenRParen {
					break
				}
				items = append(items, p.parseExpr())
				p.skipNewlines()
			}
			close := p.expect(lexer.TokenRParen, diag.CodeE053, "expected ')' to close tuple")
			return ast.TupleExpr{
				Items: items,
				Span:  diag.Merge(open.Span, close.Span),
			}
		}
		p.skipNewlines()
		p.expect(lexer.TokenRParen, diag.CodeE054, "expected ')' to close expression")
		return first
	case lexer.TokenLBracket:
		open := p.next()
		p.skipNewlines()
		items := make([]ast.Expr, 0)
		if p.peek().Type != lexer.TokenRBracket {
			for {
				items = append(items, p.parseExpr())
				p.skipNewlines()
				if p.peek().Type != lexer.TokenComma {
					break
				}
				p.next()
				p.skipNewlines()
				if p.peek().Type == lexer.TokenRBracket {
					break
				}
			}
		}
		p.skipNewlines()
		close := p.expect(lexer.TokenRBracket, diag.CodeE055, "expected ']' to close list")
		return ast.ListExpr{
			Items: items,
			Span:  diag.Merge(open.Span, close.Span),
		}
	default:
		p.diags.AddError(diag.CodeE058,
			fmt.Sprintf("unexpected token '%s' in expression", tok.Text),
			tok.Span,
			"use a valid expression term",
		)
		p.next()
		return ast.StringExpr{Value: "", Span: tok.Span}
	}
}

func isDecimalIntegerLiteral(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
