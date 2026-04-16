package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

type importedVar struct {
	Name      string
	SourceVar string
	Paramset  string
	Kind      SourceKind
	Span      diag.Span
}

// importsFromStepPlan projects effective step imports into visible-name origins.
func importsFromStepPlan(plan *StepImportPlan) map[string][]importedVar {
	if plan == nil {
		return map[string][]importedVar{}
	}
	out := make(map[string][]importedVar, len(plan.Effective))
	for name, origin := range plan.Effective {
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		out[name] = append(out[name], importedVar{
			Name:      name,
			SourceVar: sourceVar,
			Paramset:  origin.Paramset,
			Kind:      origin.Kind,
			Span:      origin.Span,
		})
	}
	return out
}

// explicitImportsFromStepPlan projects explicit delta imports for W313 accounting.
func explicitImportsFromStepPlan(plan *StepImportPlan, sources map[string]*ImportSource) map[string][]importedVar {
	if plan == nil {
		return map[string][]importedVar{}
	}
	out := make(map[string][]importedVar, len(plan.ExplicitDelta))
	for _, imp := range plan.ExplicitDelta {
		if imp.Full {
			src := sources[imp.Source]
			if src == nil {
				continue
			}
			kind := imp.Kind
			if kind == "" {
				kind = src.Kind
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				out[name] = append(out[name], importedVar{
					Name:      name,
					SourceVar: name,
					Paramset:  imp.Source,
					Kind:      kind,
					Span:      imp.Span,
				})
			}
			continue
		}
		sourceVar := imp.SourceVar
		if sourceVar == "" {
			sourceVar = imp.Visible
		}
		kind := imp.Kind
		if kind == "" {
			if src := sources[imp.Source]; src != nil {
				kind = src.Kind
			}
		}
		out[imp.Visible] = append(out[imp.Visible], importedVar{
			Name:      imp.Visible,
			SourceVar: sourceVar,
			Paramset:  imp.Source,
			Kind:      kind,
			Span:      imp.Span,
		})
	}
	return out
}

// visibleSpansFromStepPlan maps visible step names to source-origin spans.
func visibleSpansFromStepPlan(plan *StepImportPlan, sources map[string]*ImportSource) map[string]diag.Span {
	if plan == nil {
		return map[string]diag.Span{}
	}
	out := make(map[string]diag.Span, len(plan.Effective))
	for name, origin := range plan.Effective {
		if src := sources[origin.Paramset]; src != nil {
			sourceVar := origin.SourceVar
			if sourceVar == "" {
				sourceVar = name
			}
			if span, ok := src.Origins[sourceVar]; ok {
				out[name] = span
				continue
			}
		}
		out[name] = origin.Span
	}
	return out
}

// addEnvFromStepPlan seeds step-visible values for expression evaluation.
func addEnvFromStepPlan(env map[string]eval.Value, plan *StepImportPlan, sources map[string]*ImportSource) {
	if plan == nil {
		return
	}
	for name, origin := range plan.Effective {
		src := sources[origin.Paramset]
		if src == nil {
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if vals, ok := src.Vars[sourceVar]; ok {
			env[name] = seriesAsValue(vals)
		}
	}
}

// resolveImportedVars expands raw with-items for non-step contexts (for example analyse-with).
func resolveImportedVars(items []ast.WithItem, sources map[string]*ImportSource) map[string][]importedVar {
	out := make(map[string][]importedVar)
	seen := make(map[string]struct{})
	add := func(name, sourceVar, source string, kind SourceKind, span diag.Span) {
		key := source + "::" + sourceVar + "::" + name
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out[name] = append(out[name], importedVar{
			Name:      name,
			SourceVar: sourceVar,
			Paramset:  source,
			Kind:      kind,
			Span:      span,
		})
	}

	for _, item := range items {
		if item.SourceExpr != "" && len(item.SourceSlice) > 0 {
			src := sources[item.SourceExpr]
			if src == nil {
				continue
			}
			for _, sel := range item.SourceSlice {
				if _, ok := src.Vars[sel]; !ok {
					continue
				}
				add(sel, sel, src.Name, src.Kind, item.Span)
			}
			continue
		}
		if item.From == "" {
			src := sources[item.Name]
			if src == nil {
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				add(name, name, src.Name, src.Kind, item.Span)
			}
			continue
		}

		src := sources[item.From]
		if src == nil {
			continue
		}
		if _, ok := src.Vars[item.Name]; ok {
			visible := item.Name
			if item.Alias != "" {
				visible = item.Alias
			}
			add(visible, item.Name, src.Name, src.Kind, item.Span)
			continue
		}
		if fallback := sources[item.Name]; fallback != nil {
			for _, name := range planutil.SourceVarNames(fallback.Order, fallback.Vars) {
				add(name, name, fallback.Name, fallback.Kind, item.Span)
			}
		}
	}
	return out
}
