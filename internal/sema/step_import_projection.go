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
			Span:      origin.Span,
		})
	}
	return out
}

func explicitImportsFromStepPlan(plan *StepScopePlan, bindings map[string]*GlobalBinding) map[string][]importedVar {
	if plan == nil {
		return map[string][]importedVar{}
	}
	out := make(map[string][]importedVar, len(plan.ExplicitDelta))
	for _, imp := range plan.ExplicitDelta {
		if imp.Full {
			src := bindings[imp.Source]
			if src == nil {
				continue
			}
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				out[name] = append(out[name], importedVar{
					Name:      name,
					SourceVar: name,
					Source:    imp.Source,
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
			Span:      imp.Span,
		})
	}
	return out
}

func visibleSpansFromStepPlan(plan *StepScopePlan, bindings map[string]*GlobalBinding) map[string]diag.Span {
	if plan == nil {
		return map[string]diag.Span{}
	}
	out := make(map[string]diag.Span, len(plan.Effective))
	for name, origin := range plan.Effective {
		if src := bindings[origin.Source]; src != nil {
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

func addEnvFromStepPlan(env map[string]eval.Value, plan *StepScopePlan, bindings map[string]*GlobalBinding) {
	if plan == nil {
		return
	}
	for name, origin := range plan.Effective {
		src := bindings[origin.Source]
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
