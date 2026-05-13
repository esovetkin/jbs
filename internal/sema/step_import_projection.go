package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
)

type importedVar struct {
	Name      string
	SourceVar string
	Source    string
	SourceKey BindingVersionKey
	Display   string
	Span      diag.Span
}

func importsFromStepPlan(plan *StepScopePlan) map[string][]importedVar {
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
			Source:    origin.Source,
			SourceKey: origin.SourceKey,
			Span:      origin.Span,
		})
	}
	return out
}

func explicitImportsFromStepPlan(plan *StepScopePlan, bindingsByKey map[BindingVersionKey]*GlobalBinding, bindings map[string]*GlobalBinding) map[string][]importedVar {
	if plan == nil {
		return map[string][]importedVar{}
	}
	out := make(map[string][]importedVar, len(plan.ExplicitDelta))
	expansions := expansionsByItemID(plan.Expansions)
	for _, imp := range plan.ExplicitDelta {
		if imp.Full {
			if expansion, ok := expansions[imp.ItemID]; ok {
				for _, v := range expansion.Vars {
					sourceVar := v.SourceVar
					if sourceVar == "" {
						sourceVar = v.Visible
					}
					out[v.Visible] = append(out[v.Visible], importedVar{
						Name:      v.Visible,
						SourceVar: sourceVar,
						Source:    imp.Source,
						SourceKey: imp.SourceKey,
						Span:      imp.Span,
					})
				}
				continue
			}
			src := bindingByKey(bindingsByKey, imp.SourceKey)
			if src == nil {
				src = bindings[imp.Source]
			}
			if src == nil {
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				out[name] = append(out[name], importedVar{
					Name:      name,
					SourceVar: name,
					Source:    imp.Source,
					SourceKey: imp.SourceKey,
					Span:      imp.Span,
				})
			}
			continue
		}
		sourceVar := imp.SourceVar
		if sourceVar == "" {
			sourceVar = imp.Visible
		}
		out[imp.Visible] = append(out[imp.Visible], importedVar{
			Name:      imp.Visible,
			SourceVar: sourceVar,
			Source:    imp.Source,
			SourceKey: imp.SourceKey,
			Span:      imp.Span,
		})
	}
	return out
}

func visibleSpansFromStepPlan(plan *StepScopePlan, bindingsByKey map[BindingVersionKey]*GlobalBinding, bindings map[string]*GlobalBinding) map[string]diag.Span {
	if plan == nil {
		return map[string]diag.Span{}
	}
	out := make(map[string]diag.Span, len(plan.Effective))
	for name, origin := range plan.Effective {
		src := bindingByKey(bindingsByKey, origin.SourceKey)
		if src == nil {
			src = bindings[origin.Source]
		}
		if src != nil {
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

func addEnvFromStepPlan(env map[string]eval.Value, plan *StepScopePlan, bindingsByKey map[BindingVersionKey]*GlobalBinding, bindings map[string]*GlobalBinding) {
	if plan == nil {
		return
	}
	for name, values := range plan.EffectiveValues {
		env[name] = seriesAsValue(values)
	}
	for name, origin := range plan.Effective {
		if _, exists := env[name]; exists {
			continue
		}
		src := bindingByKey(bindingsByKey, origin.SourceKey)
		if src == nil {
			src = bindings[origin.Source]
		}
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

func expansionsByItemID(expansions []WithExpansion) map[int]WithExpansion {
	out := make(map[int]WithExpansion, len(expansions))
	for _, expansion := range expansions {
		out[expansion.ItemID] = expansion
	}
	return out
}
