// entry point for the parser and core parser state
//
// take lexer output/source, parse a full JBS file into an abstract
// syntax tree `ast.Program`, dispatch top-level statements
// (do/submit/analyse/use/global assignment/expression) and do syntax diagnostics
package parser

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
)

type Parser struct {
	file  string
	src   []rune
	off   int
	line  int
	col   int
	diags *diag.Diagnostics
}

func Parse(file, source string, diags *diag.Diagnostics) ast.Program {
	p := &Parser{
		file:  file,
		src:   []rune(source),
		line:  1,
		col:   1,
		diags: diags,
	}
	return p.parseProgram()
}

func (p *Parser) parseProgram() ast.Program {
	stmts := make([]ast.Stmt, 0)
	for {
		p.skipTrivia()
		if p.eof() {
			break
		}
		start := p.pos()
		if p.isTopLevelAssignmentStart() {
			stmts = append(stmts, p.parseGlobalAssign(start))
			continue
		}
		if keyword, ok := p.legacyTopLevelBlockKeyword(); ok {
			stmts = append(stmts, p.parseLegacyTopLevelBlock(keyword, start))
			continue
		}
		word, ok := p.peekWord()
		if ok {
			switch word {
			case "do":
				p.consumeWord()
				stmts = append(stmts, p.parseDoBlock(start))
				continue
			case "submit":
				p.consumeWord()
				stmts = append(stmts, p.parseSubmitBlock(start))
				continue
			case "analyse":
				p.consumeWord()
				stmts = append(stmts, p.parseAnalyseBlock(start))
				continue
			case "use":
				p.consumeWord()
				stmts = append(stmts, p.parseUseStmt(start))
				continue
			}
		}
		stmts = append(stmts, p.parseTopLevelExprStmt(start))
	}

	prog := ast.Program{
		File:  p.file,
		Stmts: stmts,
	}
	if len(stmts) > 0 {
		prog.Span = diag.Merge(stmts[0].GetSpan(), stmts[len(stmts)-1].GetSpan())
	}
	return prog
}
