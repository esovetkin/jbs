// entry point for the parser and core parser state
//
// take lexer output/source, parse a full JBS file into an abstract
// syntax tree `ast.Program`, dispatch top-level statements
// (do/submit/analyse/use/global assignment) and do syntax diagnostics
package parser

import (
	"fmt"

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
		word, ok := p.peekWord()
		if !ok {
			p.diags.AddError(diag.CodeE010,
				"expected block keyword (do/submit/analyse/use)",
				diag.NewSpan(p.file, start, start),
				"start a block with do, submit, analyse, or use",
			)
			p.advance()
			continue
		}

		switch word {
		case "do":
			p.consumeWord()
			stmts = append(stmts, p.parseDoBlock(start))
		case "submit":
			p.consumeWord()
			stmts = append(stmts, p.parseSubmitBlock(start))
		case "analyse":
			p.consumeWord()
			stmts = append(stmts, p.parseAnalyseBlock(start))
		case "use":
			p.consumeWord()
			stmts = append(stmts, p.parseUseStmt(start))
		default:
			end := p.consumeWord()
			p.diags.AddError(diag.CodeE011,
				fmt.Sprintf("unknown block keyword '%s'", word),
				diag.NewSpan(p.file, start, end),
				"valid keywords are do, submit, analyse, use",
			)
		}
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
