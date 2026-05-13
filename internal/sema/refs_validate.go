// validate variable-reference usage across steps and emits warnings
//
// scan do raw text and relevant string/expression payloads for
// `$var`/ `${var}` and identifier refs, compares references with
// effective imports, accounts usage per source variable, emits W311
// for referenced-but-not-imported vars and W310 for
// exposed-but-never-used vars, and includes shell-like
// scanners/parsers (escape, quote/comment awareness, ...) plus small
// naming helpers.
package sema

import (
	"fmt"
	"maps"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/shellref"
)

type varRef struct {
	Name string
	Span diag.Span
}

type stepUnusedImport struct {
	StepName   string
	Visible    string
	Source     string
	SourceVar  string
	SourceKey  BindingVersionKey
	Display    string
	ImportSpan diag.Span
}

type sourceCandidate struct {
	SourceKey BindingVersionKey
	Source    string
	Display   string
	SourceVar string
	Origin    diag.Span
}

func validateStepVarReferences(res *Result, diags *diag.Diagnostics) {
	catalog := buildWarningCatalog(res)
	warningSources := catalog.sources()
	used := make(usedBySource)
	stepUnused := make(map[string]stepUnusedImport)

	markUsedSourceVar := func(key BindingVersionKey, sourceVar string) {
		used.mark(key, sourceVar)
		src := catalog.byKey[key]
		if src == nil || slices.Contains(src.Order, sourceVar) || len(src.Order) != 1 {
			return
		}
		used.mark(key, src.Order[0])
	}

	markUsedExact := func(bindings map[string]*GlobalBinding, source string, sourceVar string) {
		markUsedSourceVar(catalog.keyForSource(bindings, source), sourceVar)
	}

	markUsedByImports := func(imports []importedVar) {
		for _, imp := range imports {
			sourceVar := imp.SourceVar
			if sourceVar == "" {
				sourceVar = imp.Name
			}
			key := imp.SourceKey
			if key == (BindingVersionKey{}) {
				key = catalog.keyForSource(nil, imp.Source)
			}
			markUsedSourceVar(key, sourceVar)
		}
	}

	markUsedCandidates := func(candidates []sourceCandidate) {
		for _, cand := range candidates {
			used.mark(cand.SourceKey, cand.SourceVar)
		}
	}

	warnMissing := func(stepName string, ref varRef, candidates []sourceCandidate) {
		if len(candidates) == 0 {
			return
		}
		source := candidates[0]
		related := []diag.RelatedSpan{}
		if !source.Origin.IsZero() {
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", source.Display),
				Span:    source.Origin,
			})
		}
		diags.AddWarning(
			diag.CodeW311,
			fmt.Sprintf("variable '%s' is referenced in step '%s' but is not visible through `with` or predecessor inheritance", ref.Name, stepName),
			ref.Span,
			"add `with <source>` or `with <source>[<variable>]`, or inherit it from a predecessor via `after`",
			related...,
		)
	}

	processStepWithImports := func(stepName string, imports map[string][]importedVar, refs []varRef, candidatesByVar map[string][]sourceCandidate) {
		if imports == nil {
			imports = map[string][]importedVar{}
		}
		warned := make(map[string]struct{})
		for _, ref := range refs {
			origins := imports[ref.Name]
			if len(origins) > 0 {
				markUsedByImports(origins)
				continue
			}
			candidates := candidatesByVar[ref.Name]
			if len(candidates) == 0 {
				continue
			}
			markUsedCandidates(candidates)
			key := stepName + "::" + ref.Name
			if _, exists := warned[key]; exists {
				continue
			}
			warned[key] = struct{}{}
			warnMissing(stepName, ref, candidates)
		}
	}
	resolveEffectiveImports := func(stepName string, bindings map[string]*GlobalBinding) map[string][]importedVar {
		return versionImports(importsFromStepPlan(res.StepScopeByName[stepName]), catalog, bindings)
	}
	resolveExplicitImports := func(stepName string, bindings map[string]*GlobalBinding) map[string][]importedVar {
		return versionImports(explicitImportsFromStepPlan(res.StepScopeByName[stepName], bindings), catalog, bindings)
	}
	collectStepUnusedImports := func(stepName string, imports map[string][]importedVar, refs []varRef) {
		if len(imports) == 0 {
			return
		}
		referenced := make(map[string]struct{}, len(refs))
		for _, ref := range refs {
			referenced[ref.Name] = struct{}{}
		}
		for visible, origins := range imports {
			if _, ok := referenced[visible]; ok {
				continue
			}
			for _, origin := range origins {
				sourceVar := origin.SourceVar
				if sourceVar == "" {
					sourceVar = origin.Name
				}
				sourceKey := origin.SourceKey
				if sourceKey == (BindingVersionKey{}) {
					sourceKey = catalog.keyForSource(nil, origin.Source)
				}
				key := stepName + "::" + visible + "::" + sourceKey.Public + "::" + sourceKey.Version + "::" + sourceVar
				if _, ok := stepUnused[key]; ok {
					continue
				}
				stepUnused[key] = stepUnusedImport{
					StepName:   stepName,
					Visible:    visible,
					Source:     origin.Source,
					SourceVar:  sourceVar,
					SourceKey:  sourceKey,
					Display:    origin.Display,
					ImportSpan: origin.Span,
				}
			}
		}
	}

	for _, block := range res.DoBlocks {
		snap := snapshotForDoBlock(res, block)
		bindings := snapshotBindingsWithResult(res, snap)
		base := block.BodyStart
		if base.Line == 0 {
			base = block.Span.Start
		}
		refs := shellRefsToVarRefs(shellref.Collect(block.Body, base, block.Span.File))
		for _, fsub := range block.FSubs {
			for _, rule := range fsub.Rules {
				refs = append(refs, collectExprIdentRefs(rule.Expr)...)
			}
		}
		effectiveImports := resolveEffectiveImports(block.Name, bindings)
		candidatesByVar := stepWarningCandidates(res, catalog, block.Name, snap)
		processStepWithImports(block.Name, effectiveImports, refs, candidatesByVar)
		explicitImports := resolveExplicitImports(block.Name, bindings)
		collectStepUnusedImports(block.Name, explicitImports, refs)
	}
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.AnalyseBlock)
		if !ok {
			continue
		}
		snap := snapshotForAnalyseBlock(res, block)
		bindings := snapshotBindings(res, snap)
		imports := resolveAnalyseImportsCanonical(block.WithItems, bindings, snapshotGlobals(res, snap), snapshotNamespaces(res, snap), nil, analyseImportOptions{
			EmitDiagnostics: false,
		})
		for _, origin := range imports {
			markUsedExact(bindings, origin.Source, origin.SourceVar)
		}
	}
	usedForW310 := cloneUsedBySource(used)
	propagateUsedByGlobalDeps(usedForW310, catalog, buildGlobalSourceDeps(catalog))

	for _, src := range warningSources {
		for _, varName := range src.Order {
			if usedForW310.has(src.Key, varName) {
				continue
			}
			message := fmt.Sprintf("exposed variable '%s' from global '%s' is never used in any do/analyse block", varName, src.Display)
			hint := fmt.Sprintf("remove it from the global binding or reference it with %s via imports", varName)
			diags.AddWarning(
				diag.CodeW310,
				message,
				src.VarOrigins[varName],
				hint,
			)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(stepUnused)) {
		item := stepUnused[key]
		if item.SourceKey == (BindingVersionKey{}) || item.SourceVar == "" {
			continue
		}
		if !used.has(item.SourceKey, item.SourceVar) {
			continue
		}
		sourceSpan := diag.Span{}
		if src := catalog.byKey[item.SourceKey]; src != nil {
			sourceSpan = src.VarOrigins[item.SourceVar]
		}
		span := item.ImportSpan
		if span.IsZero() {
			span = sourceSpan
		}
		related := []diag.RelatedSpan{}
		if !sourceSpan.IsZero() {
			display := item.Display
			if display == "" {
				display = item.SourceKey.Display()
			}
			related = append(related, diag.RelatedSpan{
				Message: fmt.Sprintf("source '%s'", display),
				Span:    sourceSpan,
			})
		}
		diags.AddWarning(
			diag.CodeW313,
			fmt.Sprintf("variable '%s' is imported in step '%s' but not referenced in this step", item.Visible, item.StepName),
			span,
			"remove it from the with-clause or reference it in this step",
			related...,
		)
	}
}

func shellRefsToVarRefs(refs []shellref.Ref) []varRef {
	out := make([]varRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, varRef{Name: ref.Name, Span: ref.Span})
	}
	return out
}
