// track local variable dependencies in table-building expressions
package sema

import (
	"fmt"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type localAssignMeta struct {
	Expr ast.Expr
	Span diag.Span
}

type importedContribution struct {
	Source    string
	SourceVar string
	Span      diag.Span
}

func collectExprLocalIdentDeps(expr ast.Expr, out map[string]struct{}) {
	if expr == nil {
		return
	}
	var callbacks ast.WalkCallbacks
	callbacks.Expr = func(node ast.Expr) ast.WalkAction {
		switch n := node.(type) {
		case ast.IdentExpr:
			if n.Name != "" {
				out[n.Name] = struct{}{}
			}
		case ast.QualifiedIdentExpr:
			if n.Namespace != "" {
				out[n.Namespace] = struct{}{}
			}
		case ast.CallExpr:
			if isDeleteCallExpr(n) && deleteCallHasOnlyBareTargets(n) {
				ast.WalkExpr(n.Callee, callbacks)
				return ast.WalkSkipChildren
			}
		}
		return ast.WalkContinue
	}
	ast.WalkExpr(expr, callbacks)
}

type contributorKind uint8

const (
	contributorLocal contributorKind = iota
	contributorImported
)

type contributorID struct {
	Kind      contributorKind
	Visible   string
	Source    string
	SourceVar string
}

func makeLocalContributorID(name string) contributorID {
	return contributorID{
		Kind:    contributorLocal,
		Visible: name,
	}
}

func makeImportedContributorID(visible, source, sourceVar string) contributorID {
	return contributorID{
		Kind:      contributorImported,
		Visible:   visible,
		Source:    source,
		SourceVar: sourceVar,
	}
}

func compareContributorID(a, b contributorID) int {
	if a.Kind != b.Kind {
		if a.Kind < b.Kind {
			return -1
		}
		return 1
	}
	if a.Visible != b.Visible {
		if a.Visible < b.Visible {
			return -1
		}
		return 1
	}
	if a.Source != b.Source {
		if a.Source < b.Source {
			return -1
		}
		return 1
	}
	if a.SourceVar != b.SourceVar {
		if a.SourceVar < b.SourceVar {
			return -1
		}
		return 1
	}
	return 0
}

func warnUnusedGlobalContributors(assigns map[string]localAssignMeta, order []string, imported map[string]importedContribution, importedOrder []string, seed []string, diags *diag.Diagnostics) {
	if len(assigns) == 0 && len(imported) == 0 {
		return
	}

	localByVisible := make(map[string]contributorID, len(assigns))
	for _, name := range order {
		if _, ok := assigns[name]; !ok {
			continue
		}
		if _, exists := localByVisible[name]; exists {
			continue
		}
		localByVisible[name] = makeLocalContributorID(name)
	}
	for name := range assigns {
		if _, exists := localByVisible[name]; !exists {
			localByVisible[name] = makeLocalContributorID(name)
		}
	}

	importedByVisible := make(map[string][]contributorID, len(imported))
	for _, name := range importedOrder {
		meta, ok := imported[name]
		if !ok {
			continue
		}
		id := makeImportedContributorID(name, meta.Source, meta.SourceVar)
		importedByVisible[name] = append(importedByVisible[name], id)
	}
	for name, meta := range imported {
		if _, exists := importedByVisible[name]; exists {
			continue
		}
		id := makeImportedContributorID(name, meta.Source, meta.SourceVar)
		importedByVisible[name] = []contributorID{id}
	}

	depsByNode := make(map[contributorID][]contributorID, len(assigns))
	for name, meta := range assigns {
		node, ok := localByVisible[name]
		if !ok {
			continue
		}
		depsSet := make(map[string]struct{}, 4)
		collectExprLocalIdentDeps(meta.Expr, depsSet)
		nodeDeps := make(map[contributorID]struct{}, len(depsSet))
		for depName := range depsSet {
			if depName == name {
				for _, importedNode := range importedByVisible[depName] {
					nodeDeps[importedNode] = struct{}{}
				}
				continue
			}
			if localNode, exists := localByVisible[depName]; exists {
				nodeDeps[localNode] = struct{}{}
				continue
			}
			for _, importedNode := range importedByVisible[depName] {
				nodeDeps[importedNode] = struct{}{}
			}
		}
		deps := make([]contributorID, 0, len(nodeDeps))
		for depNode := range nodeDeps {
			deps = append(deps, depNode)
		}
		slices.SortFunc(deps, compareContributorID)
		depsByNode[node] = deps
	}

	reachable := make(map[contributorID]bool, len(assigns)+len(imported))
	var markReachable func(contributorID)
	markReachable = func(node contributorID) {
		if reachable[node] {
			return
		}
		reachable[node] = true
		if node.Kind == contributorImported {
			return
		}
		for _, dep := range depsByNode[node] {
			markReachable(dep)
		}
	}

	for _, root := range seed {
		if root == "" {
			continue
		}
		if localNode, ok := localByVisible[root]; ok {
			markReachable(localNode)
			continue
		}
		for _, importedNode := range importedByVisible[root] {
			markReachable(importedNode)
		}
	}

	for _, name := range order {
		localNode, ok := localByVisible[name]
		if !ok {
			continue
		}
		meta := assigns[name]
		if reachable[localNode] {
			continue
		}
		diags.AddWarning(
			diag.CodeW312,
			"global helper variable '"+name+"' is declared but does not contribute to the final table expression",
			meta.Span,
			"remove it, or reference it (directly or transitively) in the final table expression",
		)
	}
	for _, name := range importedOrder {
		meta, ok := imported[name]
		if !ok {
			continue
		}
		importedNode := makeImportedContributorID(name, meta.Source, meta.SourceVar)
		if reachable[importedNode] {
			continue
		}
		diags.AddWarning(
			diag.CodeW312,
			fmt.Sprintf("imported variable '%s' from source '%s' does not contribute to the final expression", name, meta.Source),
			meta.Span,
			"remove it from imports or reference it (directly or transitively) in the final table expression",
		)
	}
}
