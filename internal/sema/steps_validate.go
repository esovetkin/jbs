// perform semantic validation for step headers/import usage
//
// validate do uniqueness, dependency existence and cycle
// freedom, and `with`-clause source/variable correctness via resolver
// policies, including conflicting imported-name diagnostics.
package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func validateSteps(res *Result, diags *diag.Diagnostics) {
	nameToSpan := make(map[string]diag.Span)
	edges := make(map[string][]string)

	for _, b := range res.DoBlocks {
		validateDoNProc(b.Name, b.NProc, b.Span, diags)
		if prev, exists := nameToSpan[b.Name]; exists {
			diags.AddError(
				diag.CodeE211,
				fmt.Sprintf("duplicate step name '%s'", b.Name),
				b.Span,
				"use unique names for do blocks",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		nameToSpan[b.Name] = b.Span
		edges[b.Name] = append([]string(nil), b.After...)
	}
	for step, deps := range edges {
		for _, dep := range deps {
			if _, ok := nameToSpan[dep]; !ok {
				diags.AddError(
					diag.CodeE212,
					fmt.Sprintf("unknown dependency '%s' for step '%s'", dep, step),
					nameToSpan[step],
					"depend only on existing do block names",
				)
			}
		}
	}

	state := make(map[string]int)
	stack := make([]string, 0)
	var visit func(string)
	visit = func(node string) {
		state[node] = 1
		stack = append(stack, node)
		for _, dep := range edges[node] {
			if _, ok := edges[dep]; !ok {
				continue
			}
			if state[dep] == 0 {
				visit(dep)
				continue
			}
			if state[dep] == 1 {
				cycle := append(stack, dep)
				diags.AddError(
					diag.CodeE213,
					fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")),
					nameToSpan[node],
					"remove cyclic step dependencies",
					diag.RelatedSpan{Message: "cycle reference", Span: nameToSpan[dep]},
				)
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
	}

	for _, name := range slices.Sorted(maps.Keys(edges)) {
		if state[name] == 0 {
			visit(name)
		}
	}
}

func validateDoNProc(stepName string, nproc *int, at diag.Span, diags *diag.Diagnostics) {
	if nproc != nil && *nproc < 0 {
		diags.AddError(
			diag.CodeE219,
			fmt.Sprintf("do step '%s' has invalid nproc=%d (expected >= 0)", stepName, *nproc),
			at,
			"set nproc to 0 to use the available CPU count or to a positive integer",
		)
	}
}

func validateUseClauses(res *Result, diags *diag.Diagnostics) {
	for _, block := range res.DoBlocks {
		validateWithItems(block.WithItems, res, snapshotForDoBlock(res, block), diags)
	}
}

func validateWithItems(
	items []ast.WithItem,
	res *Result,
	snap *ScopeSnapshot,
	diags *diag.Diagnostics,
) {
	resolver := BindingResolver{
		Bindings:   snapshotBindings(res, snap),
		Globals:    snapshotGlobals(res, snap),
		Namespaces: snapshotNamespaces(res, snap),
	}
	expanded := resolver.ResolveDoWithItems(items, diags)

	seen := make(map[string]struct {
		Identity withImportIdentity
		Span     diag.Span
	})
	reported := make(map[string]struct{})
	for _, item := range expanded {
		for _, v := range item.Vars {
			sourceVar := v.SourceVar
			if sourceVar == "" {
				sourceVar = v.Visible
			}
			current := withImportIdentity{Source: item.Source, SourceKey: item.SourceKey, SourceVar: sourceVar}
			prev, exists := seen[v.Visible]
			if !exists {
				seen[v.Visible] = struct {
					Identity withImportIdentity
					Span     diag.Span
				}{Identity: current, Span: item.Span}
				continue
			}
			if sameImportIdentity(prev.Identity, current) {
				continue
			}
			key := conflictingImportKey(v.Visible, prev.Identity, current)
			if _, ok := reported[key]; ok {
				continue
			}
			reported[key] = struct{}{}
			diags.AddError(
				diag.CodeE214,
				fmt.Sprintf("conflicting variable '%s' imported via `with` from sources '%s' and '%s'", v.Visible, prev.Identity.Source, item.Source),
				item.Span,
				"import each visible name from only one source",
				diag.RelatedSpan{Message: "first conflicting import", Span: prev.Span},
			)
		}
	}
}

type withImportIdentity struct {
	Source    string
	SourceKey BindingVersionKey
	SourceVar string
}

func sameImportIdentity(a, b withImportIdentity) bool {
	if a.SourceKey != (BindingVersionKey{}) || b.SourceKey != (BindingVersionKey{}) {
		if a.SourceKey != b.SourceKey {
			return false
		}
	} else if a.Source != b.Source {
		return false
	}
	return a.SourceVar == b.SourceVar
}

func conflictingImportKey(name string, a, b withImportIdentity) string {
	left := a.Source + "\x00" + a.SourceKey.Public + "\x00" + a.SourceKey.Version + "\x00" + a.SourceVar
	right := b.Source + "\x00" + b.SourceKey.Public + "\x00" + b.SourceKey.Version + "\x00" + b.SourceVar
	if left > right {
		left, right = right, left
	}
	return name + "\x00" + left + "\x00" + right
}
