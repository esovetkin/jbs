package sema

import (
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

type projectedImportDecisionKey struct {
	Index int
	Name  string
}

type moduleBindingPrep struct {
	AcceptedImports   map[projectedImportDecisionKey]*projectedImport
	LocalVisibleNames []string
}

func prepareModuleBindings(info *imports.ModuleInfo, childByIndex map[int]*moduleScope, diags *diag.Diagnostics) moduleBindingPrep {
	out := moduleBindingPrep{
		AcceptedImports:   make(map[projectedImportDecisionKey]*projectedImport),
		LocalVisibleNames: make([]string, 0),
	}
	if info == nil {
		return out
	}
	useByIndex := make(map[int]imports.ResolvedUse, len(info.Uses))
	aliasSpans := make(map[string]diag.Span)
	for _, use := range info.Uses {
		useByIndex[use.Index] = use
		if use.Kind == imports.UseNamespace && strings.TrimSpace(use.Alias) != "" {
			if _, exists := aliasSpans[use.Alias]; !exists {
				aliasSpans[use.Alias] = use.Span
			}
		}
	}
	nonGlobalSymbols := collectModuleNonGlobalSymbols(info.Program)
	for index, stmt := range info.Program.Stmts {
		if use, ok := useByIndex[index]; ok {
			if use.Kind == imports.UseNamespace {
				continue
			}
			for _, name := range use.Names {
				if span, exists := aliasSpans[name]; exists {
					diags.AddError(
						diag.CodeE534,
						"import name collision: projected global '"+name+"' conflicts with module alias",
						use.Span,
						"rename the alias or imported global",
						diag.RelatedSpan{Message: "conflicting alias", Span: span},
					)
					continue
				}
				if span, exists := nonGlobalSymbols[name]; exists {
					diags.AddError(
						diag.CodeE534,
						"import name collision: projected global '"+name+"' conflicts with local step symbol",
						use.Span,
						"rename the imported global or conflicting symbol",
						diag.RelatedSpan{Message: "conflicting symbol", Span: span},
					)
					continue
				}
				child := childByIndex[index]
				exported := (*GlobalVar)(nil)
				if child != nil {
					exported = child.LocalExportsByName[name]
				}
				if exported == nil {
					switch moduleLocalSymbolKind(moduleProgram(child, info, index), name) {
					case localSymbolDo, localSymbolAnalyse:
						diags.AddError(
							diag.CodeE533,
							"symbol '"+name+"' in module '"+use.Source.Label+"' is not importable",
							use.Span,
							"only globals are selectively importable",
						)
					default:
						if moduleLocalSymbolKind(moduleProgram(child, info, index), name) != localSymbolGlobal {
							diags.AddError(
								diag.CodeE532,
								"unknown symbol '"+name+"' in module '"+use.Source.Label+"'",
								use.Span,
								"import a global that exists in the source module",
							)
						}
					}
					continue
				}
				out.AcceptedImports[projectedImportDecisionKey{Index: index, Name: name}] = &projectedImport{
					LocalName:    name,
					SourceName:   name,
					SourceGlobal: exported,
					Span:         use.Span,
				}
			}
			continue
		}
		collectModuleVisibleNameStmt(stmt, &out)
	}
	return out
}

func collectModuleVisibleNameStmt(stmt ast.Stmt, out *moduleBindingPrep) {
	switch n := stmt.(type) {
	case ast.GlobalAssign:
		appendModuleVisibleName(&out.LocalVisibleNames, n.Name)
	case ast.IfStmt:
		for _, child := range n.Then {
			collectModuleVisibleNameStmt(child, out)
		}
		for _, branch := range n.Elifs {
			for _, child := range branch.Body {
				collectModuleVisibleNameStmt(child, out)
			}
		}
		for _, child := range n.Else {
			collectModuleVisibleNameStmt(child, out)
		}
	case ast.ForStmt:
		appendModuleVisibleName(&out.LocalVisibleNames, n.Target)
		for _, child := range n.Body {
			collectModuleVisibleNameStmt(child, out)
		}
	case ast.WhileStmt:
		for _, child := range n.Body {
			collectModuleVisibleNameStmt(child, out)
		}
	}
}

func appendModuleVisibleName(names *[]string, name string) {
	if name == "" || slices.Contains(*names, name) {
		return
	}
	*names = append(*names, name)
}

type localSymbolKind int

const (
	localSymbolNone localSymbolKind = iota
	localSymbolGlobal
	localSymbolDo
	localSymbolAnalyse
)

func moduleLocalSymbolKind(prog ast.Program, name string) localSymbolKind {
	if strings.TrimSpace(name) == "" {
		return localSymbolNone
	}
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.GlobalAssign:
			if n.Name == name {
				return localSymbolGlobal
			}
		case ast.DoBlock:
			if n.Name == name {
				return localSymbolDo
			}
		case ast.AnalyseBlock:
			if n.StepName == name {
				return localSymbolAnalyse
			}
		case ast.IfStmt:
			if kind := moduleLocalSymbolKind(ast.Program{Stmts: n.Then}, name); kind != localSymbolNone {
				return kind
			}
			for _, branch := range n.Elifs {
				if kind := moduleLocalSymbolKind(ast.Program{Stmts: branch.Body}, name); kind != localSymbolNone {
					return kind
				}
			}
			if kind := moduleLocalSymbolKind(ast.Program{Stmts: n.Else}, name); kind != localSymbolNone {
				return kind
			}
		case ast.ForStmt:
			if n.Target == name {
				return localSymbolGlobal
			}
			if kind := moduleLocalSymbolKind(ast.Program{Stmts: n.Body}, name); kind != localSymbolNone {
				return kind
			}
		case ast.WhileStmt:
			if kind := moduleLocalSymbolKind(ast.Program{Stmts: n.Body}, name); kind != localSymbolNone {
				return kind
			}
		}
	}
	return localSymbolNone
}

func collectModuleNonGlobalSymbols(prog ast.Program) map[string]diag.Span {
	out := make(map[string]diag.Span)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.DoBlock:
			if _, exists := out[n.Name]; !exists {
				out[n.Name] = n.Span
			}
		}
	}
	return out
}
