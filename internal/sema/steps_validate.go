package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/planutil"
)

func validateSteps(res *Result, diags *diag.Diagnostics) {
	nameToSpan := make(map[string]diag.Span)
	edges := make(map[string][]string)

	for _, b := range res.DoBlocks {
		validateStepHeaderOptions("do", b.Name, b.MaxAsync, b.Iterations, b.Span, diags)
		if prev, exists := nameToSpan[b.Name]; exists {
			diags.AddError(
				diag.CodeE211,
				fmt.Sprintf("duplicate step name '%s'", b.Name),
				b.Span,
				"use unique names for do/submit blocks",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		nameToSpan[b.Name] = b.Span
		edges[b.Name] = append([]string(nil), b.After...)
	}
	for _, b := range res.Submits {
		validateStepHeaderOptions("submit", b.Name, b.MaxAsync, b.Iterations, b.Span, diags)
		if prev, exists := nameToSpan[b.Name]; exists {
			diags.AddError(
				diag.CodeE211,
				fmt.Sprintf("duplicate step name '%s'", b.Name),
				b.Span,
				"use unique names for do/submit blocks",
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
					"depend only on existing do/submit block names",
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

func validateStepHeaderOptions(kind, stepName string, maxAsync *int, iterations *int, at diag.Span, diags *diag.Diagnostics) {
	if maxAsync != nil && *maxAsync < 0 {
		diags.AddError(
			diag.CodeE216,
			fmt.Sprintf("%s step '%s' has invalid max_async=%d (expected >= 0)", kind, stepName, *maxAsync),
			at,
			"set max_async to an integer value >= 0",
		)
	}
	if iterations != nil && *iterations < 1 {
		diags.AddError(
			diag.CodeE217,
			fmt.Sprintf("%s step '%s' has invalid iterations=%d (expected >= 1)", kind, stepName, *iterations),
			at,
			"set iterations to an integer value >= 1",
		)
	}
}

func validateUseClauses(res *Result, diags *diag.Diagnostics) {
	for _, ps := range res.Paramsets {
		validateWithItems(ps.Block.WithItems, res.ParamByName, res.LetByName, res.ImportSourceByName, diags)
	}
	for _, block := range res.DoBlocks {
		validateWithItems(block.WithItems, res.ParamByName, res.LetByName, res.ImportSourceByName, diags)
	}
	for _, block := range res.Submits {
		validateWithItems(block.WithItems, res.ParamByName, res.LetByName, res.ImportSourceByName, diags)
	}
}

func validateWithItems(
	items []ast.WithItem,
	params map[string]*Paramset,
	lets map[string]*LetNamespace,
	sources map[string]*ImportSource,
	diags *diag.Diagnostics,
) {
	type importOrigin struct {
		source string
		span   diag.Span
	}
	seen := make(map[string]importOrigin)
	reported := make(map[string]struct{})

	addImported := func(name string, source string, span diag.Span) {
		if prev, ok := seen[name]; ok {
			if prev.source == source {
				return
			}
			left := prev.source
			right := source
			if left > right {
				left, right = right, left
			}
			key := name + "|" + left + "|" + right
			if _, exists := reported[key]; exists {
				return
			}
			reported[key] = struct{}{}
			diags.AddError(
				diag.CodeE214,
				fmt.Sprintf("conflicting variable '%s' imported from sources '%s' and '%s'", name, prev.source, source),
				span,
				"import each variable name from only one source",
				diag.RelatedSpan{Message: "first conflicting import", Span: prev.span},
			)
			return
		}
		seen[name] = importOrigin{source: source, span: span}
	}

	resolveSource := func(name string) (*ImportSource, bool) {
		_, hasParam := params[name]
		_, hasLet := lets[name]
		if hasParam && hasLet {
			return nil, true
		}
		return sources[name], false
	}

	for _, item := range items {
		if item.From == "" {
			src, ambiguous := resolveSource(item.Name)
			if ambiguous {
				diags.AddError(
					diag.CodeE218,
					fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
					item.Span,
					"disambiguate by renaming the param or let namespace",
				)
				continue
			}
			if src == nil {
				diags.AddError(
					diag.CodeE020,
					fmt.Sprintf("unknown parameterset '%s' in with clause", item.Name),
					item.Span,
					"import an existing parameterset or let namespace",
				)
			} else {
				for _, varName := range planutil.SourceVarNames(src.Order, src.Vars) {
					addImported(varName, src.Name, item.Span)
				}
			}
			continue
		}

		src, ambiguous := resolveSource(item.From)
		if ambiguous {
			diags.AddError(
				diag.CodeE218,
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.From),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if src == nil {
			diags.AddError(
				diag.CodeE020,
				fmt.Sprintf("unknown parameterset '%s' in with clause", item.From),
				item.Span,
				"import from an existing parameterset or let namespace",
			)
			continue
		}

		if _, ok := src.Vars[item.Name]; ok {
			addImported(item.Name, src.Name, item.Span)
			continue
		}
		fallback, fallbackAmbiguous := resolveSource(item.Name)
		if fallbackAmbiguous {
			diags.AddError(
				diag.CodeE218,
				fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", item.Name),
				item.Span,
				"disambiguate by renaming the param or let namespace",
			)
			continue
		}
		if fallback != nil {
			for _, varName := range planutil.SourceVarNames(fallback.Order, fallback.Vars) {
				addImported(varName, fallback.Name, item.Span)
			}
			continue
		}
		diags.AddError(
			diag.CodeE021,
			fmt.Sprintf("unknown variable '%s' in source '%s'", item.Name, item.From),
			item.Span,
			"import a variable that exists in the selected source",
		)
	}
}
