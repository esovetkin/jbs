// parse top-level block statements
//
// `param`, `let`, `do`, `submit`, `analyse` and capture their
// structural boundaries/spans. Coordinate header parsing, balanced
// raw-body extraction, block-specific body, parsers, and block-level
// syntax diagnostics for malformed or unterminated blocks.
package parser

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
)

func (p *Parser) parseParamBlock(blockStart diag.Position) ast.ParamBlock {
	name, nameSpan := p.parseRequiredIdent(diag.CodeE082, "expected param block name")
	headerStart := nameSpan.End
	withItems := p.parseOptionalParamWithClauses()
	headerEnd := p.pos()
	headerRaw := string(p.src[headerStart.Offset:headerEnd.Offset])
	headerElems := parseHeaderElements(p.file, headerRaw, headerStart)
	p.skipTrivia()
	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(diag.CodeE083,
			"expected '{' to start param block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after param header",
		)
		return ast.ParamBlock{
			Name:      name,
			WithItems: withItems,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.ParamBlock{
			Name:      name,
			WithItems: withItems,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	assignments, final, finalExpr := parseParamBody(p.file, body, innerStart, p.diags)
	return ast.ParamBlock{
		Name:        name,
		WithItems:   withItems,
		Assignments: assignments,
		Final:       final,
		FinalExpr:   finalExpr,
		HeaderRaw:   headerRaw,
		Header:      headerElems,
		BodyRaw:     body,
		Span:        diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

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
			Name:       name,
			After:      after,
			WithItems:  withItems,
			MaxAsync:   opts.MaxAsync,
			Procs:      opts.Procs,
			Iterations: opts.Iterations,
			HeaderRaw:  headerRaw,
			Header:     headerElems,
			Span:       diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.DoBlock{
			Name:       name,
			After:      after,
			WithItems:  withItems,
			MaxAsync:   opts.MaxAsync,
			Procs:      opts.Procs,
			Iterations: opts.Iterations,
			HeaderRaw:  headerRaw,
			Header:     headerElems,
			Span:       diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	return ast.DoBlock{
		Name:       name,
		After:      after,
		WithItems:  withItems,
		MaxAsync:   opts.MaxAsync,
		Procs:      opts.Procs,
		Iterations: opts.Iterations,
		HeaderRaw:  headerRaw,
		Header:     headerElems,
		Body:       body,
		BodyStart:  innerStart,
		Span:       diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseSubmitBlock(blockStart diag.Position) ast.SubmitBlock {
	name, nameSpan := p.parseRequiredIdent(diag.CodeE040, "expected submit block name")
	headerStart := nameSpan.End
	after, useNames, withItems, opts := p.parseOptionalSubmitHeaderClauses()
	headerEnd := p.pos()
	headerRaw := string(p.src[headerStart.Offset:headerEnd.Offset])
	headerElems := parseHeaderElements(p.file, headerRaw, headerStart)
	p.skipTrivia()

	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(diag.CodeE041,
			"expected '{' to start submit block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after submit header",
		)
		return ast.SubmitBlock{
			Name:       name,
			After:      after,
			UseNames:   useNames,
			WithItems:  withItems,
			MaxAsync:   opts.MaxAsync,
			Procs:      opts.Procs,
			Iterations: opts.Iterations,
			HeaderRaw:  headerRaw,
			Header:     headerElems,
			Span:       diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.SubmitBlock{
			Name:       name,
			After:      after,
			UseNames:   useNames,
			WithItems:  withItems,
			MaxAsync:   opts.MaxAsync,
			Procs:      opts.Procs,
			Iterations: opts.Iterations,
			HeaderRaw:  headerRaw,
			Header:     headerElems,
			Span:       diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}

	fields := parseSubmitFields(p.file, body, innerStart, p.diags)

	return ast.SubmitBlock{
		Name:       name,
		After:      after,
		UseNames:   useNames,
		WithItems:  withItems,
		MaxAsync:   opts.MaxAsync,
		Procs:      opts.Procs,
		Iterations: opts.Iterations,
		HeaderRaw:  headerRaw,
		Header:     headerElems,
		Fields:     fields,
		BodyRaw:    body,
		Span:       diag.NewSpan(p.file, blockStart, blockEnd),
	}
}

func (p *Parser) parseLetBlock(blockStart diag.Position) ast.LetBlock {
	name, nameSpan := p.parseRequiredIdent(diag.CodeE080, "expected let block name")
	headerStart := nameSpan.End
	p.skipTrivia()
	headerEnd := p.pos()
	headerRaw := string(p.src[headerStart.Offset:headerEnd.Offset])
	headerElems := parseHeaderElements(p.file, headerRaw, headerStart)
	if p.peek() != '{' {
		pos := p.pos()
		p.diags.AddError(diag.CodeE081,
			"expected '{' to start let block body",
			diag.NewSpan(p.file, pos, pos),
			"add '{' after let header",
		)
		return ast.LetBlock{
			Name:      name,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}
	body, innerStart, blockEnd, ok := p.readBalancedBlock()
	if !ok {
		return ast.LetBlock{
			Name:      name,
			HeaderRaw: headerRaw,
			Header:    headerElems,
			Span:      diag.NewSpan(p.file, blockStart, nameSpan.End),
		}
	}
	assignments := parseLetBody(p.file, body, innerStart, p.diags)
	return ast.LetBlock{
		Name:        name,
		Assignments: assignments,
		HeaderRaw:   headerRaw,
		Header:      headerElems,
		BodyRaw:     body,
		Span:        diag.NewSpan(p.file, blockStart, blockEnd),
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
