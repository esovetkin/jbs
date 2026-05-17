// parse assignment-level expressions into AST nodes
//
// implement expression grammar with precedence/associativity
// (logical, compare, arithmetic, unary),
// literals/identifiers/qualified identifiers, lists/tuples,
// conditional expressions and call expressions, emitting syntax diagnostics for
// malformed expression constructs.
package parser

import (
	"fmt"
	"strconv"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
)

type tokenParser struct {
	tokens []lexer.Token
	idx    int
	diags  *diag.Diagnostics
}

type functionParseContext struct {
	LoopDepth int
}

func (ctx functionParseContext) nestedLoop() functionParseContext {
	ctx.LoopDepth++
	return ctx
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

func exprWithSpan(expr ast.Expr, span diag.Span) ast.Expr {
	switch e := expr.(type) {
	case ast.IdentExpr:
		e.Span = span
		return e
	case ast.QualifiedIdentExpr:
		e.Span = span
		return e
	case ast.MemberExpr:
		e.Span = span
		return e
	case ast.IndexExpr:
		e.Span = span
		return e
	case ast.StringExpr:
		e.Span = span
		return e
	case ast.NumberExpr:
		e.Span = span
		return e
	case ast.BoolExpr:
		e.Span = span
		return e
	case ast.ListExpr:
		e.Span = span
		return e
	case ast.TupleExpr:
		e.Span = span
		return e
	case ast.DictExpr:
		e.Span = span
		return e
	case ast.RangeExpr:
		e.Span = span
		return e
	case ast.CallExpr:
		e.Span = span
		return e
	case ast.FunctionExpr:
		e.Span = span
		return e
	case ast.AliasExpr:
		e.Span = span
		return e
	case ast.UnaryExpr:
		e.Span = span
		return e
	case ast.BinaryExpr:
		e.Span = span
		return e
	case ast.CompareExpr:
		e.Span = span
		return e
	case ast.ConditionalExpr:
		e.Span = span
		return e
	default:
		return expr
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
	thenExpr := p.parseRange()
	if p.peek().Type == lexer.TokenIf {
		ifTok := p.next()
		cond := p.parseRange()
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

func (p *tokenParser) parseConditionalNoRange() ast.Expr {
	thenExpr := p.parsePipe()
	if p.peek().Type == lexer.TokenIf {
		ifTok := p.next()
		cond := p.parsePipe()
		p.expect(lexer.TokenElse, diag.CodeE052, "expected 'else' in conditional expression")
		elseExpr := p.parseConditionalNoRange()
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

func (p *tokenParser) parseExprNoRange() ast.Expr {
	return p.parseConditionalNoRange()
}

func (p *tokenParser) parseRange() ast.Expr {
	start := p.parsePipe()
	if p.peek().Type != lexer.TokenColon {
		return start
	}

	firstColon := p.next()
	stop := p.parsePipe()
	if stop == nil {
		p.diags.AddError(diag.CodeE058, "expected range stop after ':'", firstColon.Span, "use start:stop or start:stop:step")
		return start
	}

	var step ast.Expr
	end := stop.GetSpan()
	if p.peek().Type == lexer.TokenColon {
		secondColon := p.next()
		step = p.parsePipe()
		if step == nil {
			p.diags.AddError(diag.CodeE058, "expected range step after ':'", secondColon.Span, "use start:stop:step")
			return ast.RangeExpr{Start: start, Stop: stop, Span: diag.Merge(start.GetSpan(), end)}
		}
		end = step.GetSpan()
	}

	return ast.RangeExpr{
		Start: start,
		Stop:  stop,
		Step:  step,
		Span:  diag.Merge(start.GetSpan(), end),
	}
}

func canonicalLogicalOp(tt lexer.TokenType) (string, bool) {
	switch tt {
	case lexer.TokenAmp, lexer.TokenAnd:
		return "&", true
	case lexer.TokenPipe, lexer.TokenOr:
		return "|", true
	default:
		return "", false
	}
}

func (p *tokenParser) parsePipe() ast.Expr {
	left := p.parseAmp()
	for {
		tt := p.peek().Type
		if tt != lexer.TokenPipe && tt != lexer.TokenOr {
			break
		}
		op := p.next()
		opText, _ := canonicalLogicalOp(op.Type)
		right := p.parseAmp()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    opText,
			Right: right,
			Span:  diag.Merge(left.GetSpan(), right.GetSpan()),
		}
	}
	return left
}

func (p *tokenParser) parseAmp() ast.Expr {
	left := p.parseCompare()
	for {
		tt := p.peek().Type
		if tt != lexer.TokenAmp && tt != lexer.TokenAnd {
			break
		}
		op := p.next()
		opText, _ := canonicalLogicalOp(op.Type)
		right := p.parseCompare()
		left = ast.BinaryExpr{
			Left:  left,
			Op:    opText,
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
	if t == lexer.TokenPlus || t == lexer.TokenMinus || t == lexer.TokenBang {
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
	expr := p.parsePrimaryAtom()
	return p.parsePostfix(expr)
}

func (p *tokenParser) parsePrimaryAtom() ast.Expr {
	tok := p.peek()
	switch tok.Type {
	case lexer.TokenFunction:
		return p.parseFunctionExpr()
	case lexer.TokenReturn:
		p.diags.AddError(
			diag.CodeE058,
			"'return' is only allowed inside function bodies",
			tok.Span,
			"use 'return expr' inside function(...) { ... }",
		)
		p.next()
		return ast.StringExpr{Value: "", Span: tok.Span}
	case lexer.TokenIdent:
		if tok.Value == "true" || tok.Value == "True" || tok.Value == "TRUE" {
			p.next()
			return ast.BoolExpr{Value: true, Span: tok.Span}
		}
		if tok.Value == "false" || tok.Value == "False" || tok.Value == "FALSE" {
			p.next()
			return ast.BoolExpr{Value: false, Span: tok.Span}
		}
		nameTok := p.next()
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
		close := p.expect(lexer.TokenRParen, diag.CodeE054, "expected ')' to close expression")
		return exprWithSpan(first, diag.Merge(open.Span, close.Span))
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
	case lexer.TokenLBrace:
		return p.parseDictExpr()
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

func (p *tokenParser) parseDictExpr() ast.Expr {
	open := p.expect(lexer.TokenLBrace, diag.CodeE058, "expected '{' to start dictionary")
	p.skipNewlines()
	entries := make([]ast.DictEntryExpr, 0)
	if p.peek().Type != lexer.TokenRBrace {
		for {
			key := p.parseExprNoRange()
			p.skipNewlines()
			colon := p.expect(lexer.TokenColon, diag.CodeE058, "expected ':' between dictionary key and value")
			p.skipNewlines()
			value := p.parseExpr()
			span := diag.Span{}
			if key != nil {
				span = key.GetSpan()
			}
			if value != nil {
				if span.IsZero() {
					span = value.GetSpan()
				} else {
					span = diag.Merge(span, value.GetSpan())
				}
			}
			if key != nil && value != nil && !colon.Span.IsZero() {
				span = diag.Merge(key.GetSpan(), diag.Merge(colon.Span, value.GetSpan()))
			}
			entries = append(entries, ast.DictEntryExpr{Key: key, Value: value, Span: span})
			p.skipNewlines()
			if p.peek().Type != lexer.TokenComma {
				break
			}
			p.next()
			p.skipNewlines()
			if p.peek().Type == lexer.TokenRBrace {
				break
			}
		}
	}
	close := p.expect(lexer.TokenRBrace, diag.CodeE055, "expected '}' to close dictionary")
	return ast.DictExpr{Entries: entries, Span: diag.Merge(open.Span, close.Span)}
}

func (p *tokenParser) parsePostfix(base ast.Expr) ast.Expr {
	expr := base
	for {
		switch p.peek().Type {
		case lexer.TokenDot:
			dotTok := p.next()
			memberTok := p.expect(lexer.TokenIdent, diag.CodeE064, "expected identifier after '.'")
			switch n := expr.(type) {
			case ast.IdentExpr:
				expr = ast.QualifiedIdentExpr{
					Namespace: n.Name,
					Name:      memberTok.Value,
					Span:      diag.Merge(n.Span, memberTok.Span),
				}
			case ast.QualifiedIdentExpr:
				ns := n.Namespace + "." + n.Name
				expr = ast.QualifiedIdentExpr{
					Namespace: ns,
					Name:      memberTok.Value,
					Span:      diag.Merge(n.Span, memberTok.Span),
				}
			default:
				expr = ast.MemberExpr{
					Base: expr,
					Name: memberTok.Value,
					Span: diag.Merge(expr.GetSpan(), diag.Merge(dotTok.Span, memberTok.Span)),
				}
			}
		case lexer.TokenLParen:
			expr = p.parseCallExpr(expr)
		case lexer.TokenLBracket:
			expr = p.parseIndexExpr(expr)
		case lexer.TokenAs:
			asTok := p.next()
			aliasTok := p.peek()
			if aliasTok.Type != lexer.TokenIdent {
				p.diags.AddError(
					diag.CodeE058,
					"expected alias identifier after 'as'",
					aliasTok.Span,
					"use syntax: expression as identifier",
				)
				if aliasTok.Type != lexer.TokenEOF {
					aliasTok = p.next()
					expr = ast.AliasExpr{
						Expr:  expr,
						Alias: "",
						Span:  diag.Merge(expr.GetSpan(), aliasTok.Span),
					}
				} else {
					expr = ast.AliasExpr{
						Expr:  expr,
						Alias: "",
						Span:  diag.Merge(expr.GetSpan(), asTok.Span),
					}
				}
				continue
			}
			aliasTok = p.next()
			expr = ast.AliasExpr{
				Expr:  expr,
				Alias: aliasTok.Value,
				Span:  diag.Merge(expr.GetSpan(), aliasTok.Span),
			}
		default:
			return expr
		}
	}
}

func (p *tokenParser) parseIndexExpr(base ast.Expr) ast.Expr {
	open := p.expect(lexer.TokenLBracket, diag.CodeE055, "expected '[' after expression")
	p.skipNewlines()
	items := make([]ast.Expr, 0, 2)
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
	close := p.expect(lexer.TokenRBracket, diag.CodeE055, "expected ']' to close index expression")
	return ast.IndexExpr{
		Base:  base,
		Items: items,
		Span:  diag.Merge(base.GetSpan(), diag.Merge(open.Span, close.Span)),
	}
}

func (p *tokenParser) parseCallExpr(callee ast.Expr) ast.Expr {
	p.expect(lexer.TokenLParen, diag.CodeE062, "expected '(' after function name")
	args := p.parseCallArgs()
	close := p.expect(lexer.TokenRParen, diag.CodeE063, "expected ')' to close function call")
	return ast.CallExpr{
		Callee: callee,
		Args:   args,
		Span:   diag.Merge(callee.GetSpan(), close.Span),
	}
}

func (p *tokenParser) parseCallArgs() []ast.CallArg {
	p.skipNewlines()
	if p.peek().Type == lexer.TokenRParen {
		return nil
	}
	args := make([]ast.CallArg, 0, 2)
	seenNamed := make(map[string]diag.Span)
	sawNamed := false
	for {
		arg := p.parseCallArg()
		switch arg.EffectiveKind() {
		case ast.CallArgNamed:
			sawNamed = true
			if prev, exists := seenNamed[arg.Name]; exists {
				p.diags.AddError(
					diag.CodeE058,
					fmt.Sprintf("duplicate named argument '%s'", arg.Name),
					arg.Span,
					fmt.Sprintf("remove the duplicate named argument; first declaration was at %d:%d", prev.Start.Line, prev.Start.Column),
				)
			} else {
				seenNamed[arg.Name] = arg.Span
			}
		case ast.CallArgPositional:
			if sawNamed {
				p.diags.AddError(
					diag.CodeE058,
					"positional argument cannot appear after named arguments",
					arg.Span,
					"move positional arguments before the first named argument",
				)
			}
		}
		if arg.EffectiveKind() == ast.CallArgKeywordSpread {
			sawNamed = true
		}
		args = append(args, arg)
		p.skipNewlines()
		if p.peek().Type != lexer.TokenComma {
			break
		}
		p.next()
		p.skipNewlines()
		if p.peek().Type == lexer.TokenRParen {
			break
		}
	}
	return args
}

func (p *tokenParser) parseCallArg() ast.CallArg {
	if p.peek().Type == lexer.TokenStarStar {
		start := p.next()
		expr := p.parseExpr()
		span := start.Span
		if expr != nil {
			span = diag.Merge(start.Span, expr.GetSpan())
		}
		return ast.CallArg{Kind: ast.CallArgKeywordSpread, Expr: expr, Span: span}
	}
	if p.peek().Type == lexer.TokenStar {
		start := p.next()
		expr := p.parseExpr()
		span := start.Span
		if expr != nil {
			span = diag.Merge(start.Span, expr.GetSpan())
		}
		return ast.CallArg{Kind: ast.CallArgPositionalSpread, Expr: expr, Span: span}
	}
	if p.peek().Type == lexer.TokenIdent && p.peekN(1).Type == lexer.TokenEqual {
		nameTok := p.next()
		eqTok := p.next()
		p.skipNewlines()
		expr := p.parseExpr()
		span := diag.Merge(nameTok.Span, eqTok.Span)
		if expr != nil {
			span = diag.Merge(nameTok.Span, expr.GetSpan())
		}
		return ast.CallArg{
			Kind: ast.CallArgNamed,
			Name: nameTok.Value,
			Expr: expr,
			Span: span,
		}
	}
	expr := p.parseExpr()
	span := diag.Span{}
	if expr != nil {
		span = expr.GetSpan()
	}
	return ast.CallArg{Kind: ast.CallArgPositional, Expr: expr, Span: span}
}

func (p *tokenParser) parseFunctionExpr() ast.Expr {
	fnTok := p.expect(lexer.TokenFunction, diag.CodeE058, "expected 'function'")
	p.skipNewlines()
	openTok := p.expect(lexer.TokenLParen, diag.CodeE062, "expected '(' after 'function'")
	params := p.parseFunctionParams()
	closeParen := p.expect(lexer.TokenRParen, diag.CodeE063, "expected ')' to close function parameter list")
	p.skipNewlines()
	openBrace := p.expect(lexer.TokenLBrace, diag.CodeE025, "expected '{' to start function body")
	body := []ast.FuncBodyStmt(nil)
	closeBrace := openBrace
	if openBrace.Type == lexer.TokenLBrace {
		body = p.parseFunctionBody(functionParseContext{})
		closeBrace = p.expect(lexer.TokenRBrace, diag.CodeE025, "expected '}' to close function body")
	}
	endSpan := closeBrace.Span
	if endSpan.IsZero() {
		endSpan = closeParen.Span
	}
	if endSpan.IsZero() {
		endSpan = openTok.Span
	}
	return ast.FunctionExpr{
		Params: params,
		Body:   body,
		Span:   diag.Merge(fnTok.Span, endSpan),
	}
}

func (p *tokenParser) parseFunctionParams() []ast.FuncParam {
	p.skipNewlines()
	if p.peek().Type == lexer.TokenRParen {
		return nil
	}
	params := make([]ast.FuncParam, 0, 2)
	seen := make(map[string]diag.Span)
	sawDefault := false
	sawRest := false
	seenArgs := false
	seenKwargs := false
	for {
		tok := p.peek()
		var param ast.FuncParam
		if tok.Type == lexer.TokenStar || tok.Type == lexer.TokenStarStar {
			starTok := p.next()
			nameTok := p.expect(lexer.TokenIdent, diag.CodeE050, "expected parameter name after rest marker")
			kind := ast.FuncParamArgs
			if starTok.Type == lexer.TokenStarStar {
				kind = ast.FuncParamKwargs
			}
			param = ast.FuncParam{
				Kind: kind,
				Name: nameTok.Value,
				Span: diag.Merge(starTok.Span, nameTok.Span),
			}
			if kind == ast.FuncParamArgs {
				if seenArgs {
					p.diags.AddError(diag.CodeE058, "duplicate *args parameter", starTok.Span, "declare at most one *args parameter")
				}
				if seenKwargs {
					p.diags.AddError(diag.CodeE058, "*args parameter cannot follow **kwargs", starTok.Span, "place *args before **kwargs")
				}
				seenArgs = true
			} else {
				if seenKwargs {
					p.diags.AddError(diag.CodeE058, "duplicate **kwargs parameter", starTok.Span, "declare at most one **kwargs parameter")
				}
				seenKwargs = true
			}
			sawRest = true
			p.skipNewlines()
			if p.peek().Type == lexer.TokenEqual {
				p.diags.AddError(diag.CodeE058, "rest parameters cannot have defaults", p.peek().Span, "remove the default value from the rest parameter")
				p.next()
				p.skipNewlines()
				defaultExpr := p.parseExpr()
				if defaultExpr != nil {
					param.Span = diag.Merge(param.Span, defaultExpr.GetSpan())
				}
			}
		} else if tok.Type == lexer.TokenIdent {
			nameTok := p.next()
			param = ast.FuncParam{Kind: ast.FuncParamValue, Name: nameTok.Value, Span: nameTok.Span}
			if sawRest {
				p.diags.AddError(
					diag.CodeE058,
					fmt.Sprintf("parameter '%s' cannot follow *args or **kwargs", param.Name),
					nameTok.Span,
					"place ordinary parameters before rest parameters",
				)
			}
			p.skipNewlines()
			if p.peek().Type == lexer.TokenEqual {
				p.next()
				p.skipNewlines()
				param.Default = p.parseExpr()
				if param.Default != nil {
					param.Span = diag.Merge(nameTok.Span, param.Default.GetSpan())
				}
				sawDefault = true
			} else if sawDefault {
				p.diags.AddError(
					diag.CodeE058,
					fmt.Sprintf("parameter '%s' without default follows a defaulted parameter", param.Name),
					nameTok.Span,
					"move required parameters before defaulted parameters",
				)
			}
		} else {
			p.diags.AddError(
				diag.CodeE050,
				"expected parameter name in function parameter list",
				tok.Span,
				"use syntax: function(arg, other = expr, *args, **kwargs) { ... }",
			)
			if tok.Type != lexer.TokenRParen && tok.Type != lexer.TokenEOF {
				p.next()
			}
			break
		}
		if prev, exists := seen[param.Name]; exists {
			p.diags.AddError(
				diag.CodeE058,
				fmt.Sprintf("duplicate parameter '%s'", param.Name),
				param.Span,
				fmt.Sprintf("remove the duplicate parameter; first declaration was at %d:%d", prev.Start.Line, prev.Start.Column),
			)
		} else {
			if param.Name != "" {
				seen[param.Name] = param.Span
			}
		}
		params = append(params, param)

		p.skipNewlines()
		if p.peek().Type != lexer.TokenComma {
			break
		}
		p.next()
		p.skipNewlines()
		if p.peek().Type == lexer.TokenRParen {
			break
		}
	}
	return params
}

func (p *tokenParser) parseFunctionBody(ctx functionParseContext) []ast.FuncBodyStmt {
	body := make([]ast.FuncBodyStmt, 0, 4)
	for {
		p.skipStmtSeparators()
		if p.peek().Type == lexer.TokenRBrace || p.peek().Type == lexer.TokenEOF {
			return body
		}
		body = append(body, p.parseFunctionBodyStmt(ctx))
	}
}

func (p *tokenParser) parseFunctionBodyStmt(ctx functionParseContext) ast.FuncBodyStmt {
	switch p.peek().Type {
	case lexer.TokenIf:
		return p.parseFuncIfStmt(ctx)
	case lexer.TokenElif, lexer.TokenElse:
		tok := p.next()
		p.diags.AddError(
			diag.CodeE080,
			fmt.Sprintf("'%s' without matching if", tok.Text),
			tok.Span,
			"attach the branch to a preceding `if` block",
		)
		p.consumeUntilFunctionBodyStmtEnd()
		return ast.ExprStmt{Span: tok.Span}
	case lexer.TokenFor:
		return p.parseFuncForStmt(ctx)
	case lexer.TokenWhile:
		return p.parseFuncWhileStmt(ctx)
	case lexer.TokenBreak:
		return p.parseFuncBreakStmt(ctx)
	case lexer.TokenContinue:
		return p.parseFuncContinueStmt(ctx)
	case lexer.TokenReturn:
		return p.parseReturnStmt()
	case lexer.TokenDo, lexer.TokenAnalyse, lexer.TokenUse:
		tok := p.next()
		p.diags.AddError(
			diag.CodeE058,
			fmt.Sprintf("'%s' is not allowed inside function bodies", tok.Text),
			tok.Span,
			"use assignments, return statements, expressions, or control-flow statements inside function bodies",
		)
		p.consumeUntilFunctionBodyStmtEnd()
		return ast.ExprStmt{
			Expr: ast.StringExpr{Value: "", Span: tok.Span},
			Span: tok.Span,
		}
	case lexer.TokenIdent:
		if isAssignToken(p.peekN(1).Type) {
			return p.parseLocalAssignStmt()
		}
	}
	return p.parseFunctionExprStmt()
}

func (p *tokenParser) parseFuncIfStmt(ctx functionParseContext) ast.FuncBodyStmt {
	ifTok := p.expect(lexer.TokenIf, diag.CodeE080, "expected 'if'")
	p.skipNewlines()
	cond := p.parseExpr()
	open := p.expect(lexer.TokenLBrace, diag.CodeE080, "expected '{' after if condition")
	thenBody := []ast.FuncBodyStmt(nil)
	end := open.Span
	if open.Type == lexer.TokenLBrace {
		thenBody = p.parseFunctionBody(ctx)
		closeTok := p.expect(lexer.TokenRBrace, diag.CodeE025, "expected '}' to close if body")
		end = closeTok.Span
	}

	elifs := []ast.FuncElifBranch(nil)
	for {
		p.skipStmtSeparators()
		if p.peek().Type != lexer.TokenElif {
			break
		}
		branch := p.parseFuncElifBranch(ctx)
		elifs = append(elifs, branch)
		end = branch.Span
	}

	elseBody := []ast.FuncBodyStmt(nil)
	p.skipStmtSeparators()
	if p.peek().Type == lexer.TokenElse {
		elseBody, end = p.parseFuncElseBody(ctx, end)
	}
	p.skipStmtSeparators()
	return ast.FuncIfStmt{
		Cond:  cond,
		Then:  thenBody,
		Elifs: elifs,
		Else:  elseBody,
		Span:  diag.Merge(ifTok.Span, end),
	}
}

func (p *tokenParser) parseFuncElifBranch(ctx functionParseContext) ast.FuncElifBranch {
	elifTok := p.expect(lexer.TokenElif, diag.CodeE080, "expected 'elif'")
	p.skipNewlines()
	cond := p.parseExpr()
	open := p.expect(lexer.TokenLBrace, diag.CodeE080, "expected '{' after elif condition")
	body := []ast.FuncBodyStmt(nil)
	end := open.Span
	if open.Type == lexer.TokenLBrace {
		body = p.parseFunctionBody(ctx)
		closeTok := p.expect(lexer.TokenRBrace, diag.CodeE025, "expected '}' to close elif body")
		end = closeTok.Span
	}
	return ast.FuncElifBranch{
		Cond: cond,
		Body: body,
		Span: diag.Merge(elifTok.Span, end),
	}
}

func (p *tokenParser) parseFuncElseBody(ctx functionParseContext, previousEnd diag.Span) ([]ast.FuncBodyStmt, diag.Span) {
	elseTok := p.next()
	p.skipNewlines()
	if p.peek().Type != lexer.TokenLBrace {
		p.diags.AddError(
			diag.CodeE080,
			"expected '{' after 'else'",
			p.peek().Span,
			"use `elif condition { ... }` instead of `else if condition { ... }`",
		)
		if p.peek().Type == lexer.TokenIf {
			discard := p.parseFuncIfStmt(ctx)
			return nil, discard.GetSpan()
		}
		p.consumeUntilFunctionBodyStmtEnd()
		return nil, elseTok.Span
	}
	openElse := p.next()
	elseBody := p.parseFunctionBody(ctx)
	closeElse := p.expect(lexer.TokenRBrace, diag.CodeE025, "expected '}' to close else body")
	if openElse.Type != lexer.TokenLBrace {
		return elseBody, elseTok.Span
	}
	if closeElse.Type != lexer.TokenRBrace {
		return elseBody, previousEnd
	}
	return elseBody, closeElse.Span
}

func (p *tokenParser) parseFuncForStmt(ctx functionParseContext) ast.FuncBodyStmt {
	forTok := p.expect(lexer.TokenFor, diag.CodeE080, "expected 'for'")
	p.skipNewlines()
	nameTok := p.expect(lexer.TokenIdent, diag.CodeE080, "expected loop variable after 'for'")
	p.expect(lexer.TokenIn, diag.CodeE080, "expected 'in' after loop variable")
	iterable := p.parseExpr()
	open := p.expect(lexer.TokenLBrace, diag.CodeE080, "expected '{' after for header")
	body := []ast.FuncBodyStmt(nil)
	end := open.Span
	if open.Type == lexer.TokenLBrace {
		body = p.parseFunctionBody(ctx.nestedLoop())
		closeTok := p.expect(lexer.TokenRBrace, diag.CodeE025, "expected '}' to close for body")
		end = closeTok.Span
	}
	target := ""
	if nameTok.Type == lexer.TokenIdent {
		target = nameTok.Value
	}
	p.skipStmtSeparators()
	return ast.FuncForStmt{
		Target:   target,
		Iterable: iterable,
		Body:     body,
		Span:     diag.Merge(forTok.Span, end),
	}
}

func (p *tokenParser) parseFuncWhileStmt(ctx functionParseContext) ast.FuncBodyStmt {
	whileTok := p.expect(lexer.TokenWhile, diag.CodeE080, "expected 'while'")
	p.skipNewlines()
	cond := p.parseExpr()
	open := p.expect(lexer.TokenLBrace, diag.CodeE080, "expected '{' after while condition")
	body := []ast.FuncBodyStmt(nil)
	end := open.Span
	if open.Type == lexer.TokenLBrace {
		body = p.parseFunctionBody(ctx.nestedLoop())
		closeTok := p.expect(lexer.TokenRBrace, diag.CodeE025, "expected '}' to close while body")
		end = closeTok.Span
	}
	p.skipStmtSeparators()
	return ast.FuncWhileStmt{
		Cond: cond,
		Body: body,
		Span: diag.Merge(whileTok.Span, end),
	}
}

func (p *tokenParser) parseFuncBreakStmt(ctx functionParseContext) ast.FuncBodyStmt {
	tok := p.expect(lexer.TokenBreak, diag.CodeE080, "expected 'break'")
	p.validateFuncLoopControlTail(tok, "break", ctx)
	return ast.BreakStmt{Span: tok.Span}
}

func (p *tokenParser) parseFuncContinueStmt(ctx functionParseContext) ast.FuncBodyStmt {
	tok := p.expect(lexer.TokenContinue, diag.CodeE080, "expected 'continue'")
	p.validateFuncLoopControlTail(tok, "continue", ctx)
	return ast.ContinueStmt{Span: tok.Span}
}

func (p *tokenParser) validateFuncLoopControlTail(tok lexer.Token, keyword string, ctx functionParseContext) {
	if ctx.LoopDepth == 0 {
		p.diags.AddError(
			diag.CodeE080,
			fmt.Sprintf("'%s' is only allowed inside loops", keyword),
			tok.Span,
			fmt.Sprintf("move %s into a for/while body", keyword),
		)
	}
	if !isFunctionBodyStmtTerminator(p.peek().Type) {
		p.diags.AddError(
			diag.CodeE061,
			fmt.Sprintf("unexpected trailing tokens after %s", keyword),
			p.peek().Span,
			fmt.Sprintf("use `%s` without arguments", keyword),
		)
		p.consumeUntilFunctionBodyStmtEnd()
		return
	}
	p.skipStmtSeparators()
}

func (p *tokenParser) parseLocalAssignStmt() ast.FuncBodyStmt {
	nameTok := p.expect(lexer.TokenIdent, diag.CodeE050, "expected local assignment identifier")
	op, _, ok := p.parseAssignOp()
	if !ok {
		p.diags.AddError(
			diag.CodeE051,
			"expected assignment operator in function body assignment",
			p.peek().Span,
			"use one of: =, +=, -=, *=, /=, %=",
		)
		p.consumeUntilFunctionBodyStmtEnd()
		return ast.LocalAssignStmt{
			Name: nameTok.Value,
			Op:   ast.AssignEq,
			Span: nameTok.Span,
		}
	}
	expr := p.parseExpr()
	span := nameTok.Span
	if expr != nil {
		span = diag.Merge(span, expr.GetSpan())
	}
	if !isFunctionBodyStmtTerminator(p.peek().Type) {
		p.diags.AddError(
			diag.CodeE061,
			"unexpected trailing tokens after function body assignment",
			p.peek().Span,
			"remove unsupported trailing syntax after the expression",
		)
		p.consumeUntilFunctionBodyStmtEnd()
	} else {
		p.skipStmtSeparators()
	}
	return ast.LocalAssignStmt{
		Name: nameTok.Value,
		Op:   op,
		Expr: expr,
		Span: span,
	}
}

func (p *tokenParser) parseReturnStmt() ast.FuncBodyStmt {
	retTok := p.expect(lexer.TokenReturn, diag.CodeE058, "expected 'return'")
	expr := p.parseExpr()
	span := retTok.Span
	if expr != nil {
		span = diag.Merge(span, expr.GetSpan())
	}
	if !isFunctionBodyStmtTerminator(p.peek().Type) {
		p.diags.AddError(
			diag.CodeE061,
			"unexpected trailing tokens after return expression",
			p.peek().Span,
			"remove unsupported trailing syntax after the return expression",
		)
		p.consumeUntilFunctionBodyStmtEnd()
	} else {
		p.skipStmtSeparators()
	}
	return ast.ReturnStmt{
		Expr: expr,
		Span: span,
	}
}

func (p *tokenParser) parseFunctionExprStmt() ast.FuncBodyStmt {
	expr := p.parseExpr()
	span := diag.Span{}
	if expr != nil {
		span = expr.GetSpan()
	}
	if !isFunctionBodyStmtTerminator(p.peek().Type) {
		p.diags.AddError(
			diag.CodeE061,
			"unexpected trailing tokens after function body expression",
			p.peek().Span,
			"remove unsupported trailing syntax after the expression",
		)
		p.consumeUntilFunctionBodyStmtEnd()
	} else {
		p.skipStmtSeparators()
	}
	return ast.ExprStmt{
		Expr: expr,
		Span: span,
	}
}

func isFunctionBodyStmtTerminator(t lexer.TokenType) bool {
	return t == lexer.TokenEOF ||
		t == lexer.TokenNewline ||
		t == lexer.TokenSemicolon ||
		t == lexer.TokenComment ||
		t == lexer.TokenRBrace
}

func (p *tokenParser) consumeUntilFunctionBodyStmtEnd() {
	for !isFunctionBodyStmtTerminator(p.peek().Type) {
		p.next()
	}
	p.skipStmtSeparators()
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
