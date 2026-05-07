// entry point for the parser and core parser state
//
// take lexer output/source, parse a full JBS file into an abstract
// syntax tree `ast.Program`, dispatch top-level statements
// (do/analyse/use/global assignment/expression) and do syntax diagnostics
package parser

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
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
	stmts := p.parseStmtList(topLevelParseContext{}, false)

	prog := ast.Program{
		File:  p.file,
		Stmts: stmts,
	}
	if len(stmts) > 0 {
		prog.Span = diag.Merge(stmts[0].GetSpan(), stmts[len(stmts)-1].GetSpan())
	}
	return prog
}
