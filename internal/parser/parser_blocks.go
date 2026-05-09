// parse top-level block statements
//
// parse `do` and `analyse` blocks and capture their
// structural boundaries/spans. Coordinate header parsing, balanced
// raw-body extraction, block-specific body parsers, and block-level
// syntax diagnostics for malformed or unterminated blocks.
package parser

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func (p *Parser) parseDoBlock(blockStart diag.Position) ast.DoBlock {
	name, nameSpan := p.parseRequiredIdent(diag.CodeE030, "expected do block name")
	headerStart := nameSpan.End
	after, withItems, opts := p.parseOptionalDoHeaderClauses()
	headerEnd := p.pos()
	headerRaw := string(p.src[headerStart.Offset:headerEnd.Offset])
	headerElems := parseHeaderElements(p.file, headerRaw, headerStart)
	p.skipTrivia()

	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(diag.CodeE031,
			"expected '{' to start do block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' before do script body",
		)
		return ast.DoBlock{
			Name:      name,
			After:     after,
			WithItems: withItems,
			NProc:     opts.NProc,
			FSubs:     opts.FSubs,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.DoBlock{
			Name:      name,
			After:     after,
			WithItems: withItems,
			NProc:     opts.NProc,
			FSubs:     opts.FSubs,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	return ast.DoBlock{
		Name:      name,
		After:     after,
		WithItems: withItems,
		NProc:     opts.NProc,
		FSubs:     opts.FSubs,
		HeaderRaw: headerRaw,
		Header:    headerElems,
		Body:      body,
		BodyStart: innerStart,
		Span:      diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseAnalyseBlock(blockStart diag.Position) ast.AnalyseBlock {
	stepName, stepSpan := p.parseRequiredIdent(diag.CodeE416, "expected analyse target step name")
	headerStart := stepSpan.End
	after, withItems := p.parseOptionalAfterAndWith()
	headerEnd := p.pos()
	headerRaw := string(p.src[headerStart.Offset:headerEnd.Offset])
	headerElems := parseHeaderElements(p.file, headerRaw, headerStart)
	if len(after) > 0 {
		span := diag.NewSpan(p.file, blockStart, p.pos())
		p.diags.AddError(diag.CodeE416,
			"analyse block does not support an after-clause",
			span,
			"use syntax: analyse <step_name> [with ...] { ... }",
		)
	}
	p.skipTrivia()
	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(diag.CodeE416,
			"expected '{' to start analyse block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after analyse header",
		)
		return ast.AnalyseBlock{
			StepName:  stepName,
			WithItems: withItems,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, stepSpan.End),
		}
	}
	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.AnalyseBlock{
			StepName:  stepName,
			WithItems: withItems,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, stepSpan.End),
		}
	}
	assignments, columns := parseAnalyseBody(p.file, body, innerStart, p.diags)
	return ast.AnalyseBlock{
		StepName:    stepName,
		WithItems:   withItems,
		Assignments: assignments,
		Columns:     columns,
		HeaderRaw:   headerRaw,
		Header:      headerElems,
		BodyRaw:     body,
		Span:        diag.NewSpan(p.file, blockStart, blockEnd),
	}
}
