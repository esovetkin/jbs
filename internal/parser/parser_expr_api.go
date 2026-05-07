package parser

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/lexer"
)

// ParseStandaloneExpr parses one standalone expression from source.
//
// It returns (expr, true) when the input is expression-shaped for REPL
// evaluation. It returns (nil, false) when the input looks like a
// top-level statement (for example assignment or block keyword),
// allowing the caller to fall back to statement parsing.
func ParseStandaloneExpr(file, source string, start diag.Position, diags *diag.Diagnostics) (ast.Expr, bool) {
	tokens := lexer.LexFrom(file, source, start, diags)
	tp := &tokenParser{tokens: tokens, diags: diags}
	tp.skipStmtSeparators()
	first := tp.peek()
	if first.Type == lexer.TokenEOF {
		return nil, false
	}
	if looksLikeTopLevelStatementStart(tp) {
		return nil, false
	}
	expr := tp.parseExpr()
	tp.skipStmtSeparators()
	if tp.peek().Type != lexer.TokenEOF {
		trailing := tp.peek()
		diags.AddError(
			diag.CodeE061,
			"unexpected trailing tokens after expression",
			trailing.Span,
			"remove unsupported trailing syntax after the expression",
		)
		tp.consumeUntilStmtEnd()
		return expr, true
	}
	return expr, true
}

func looksLikeTopLevelStatementStart(tp *tokenParser) bool {
	first := tp.peek()
	if first.Type == lexer.TokenDo || first.Type == lexer.TokenAnalyse || first.Type == lexer.TokenUse {
		return true
	}
	if first.Type != lexer.TokenIdent {
		return false
	}
	if first.Value == "do" || first.Value == "analyse" || first.Value == "use" {
		return true
	}
	return isAssignToken(tp.peekN(1).Type)
}
