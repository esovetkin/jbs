package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

func stepVisibleVariables(items []ast.WithItem, sources map[string]*ImportSource) map[string]diag.Span {
	out := make(map[string]diag.Span)
	imports := resolveImportedVars(items, sources)
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		origin := origins[0]
		src := sources[origin.Paramset]
		if src == nil {
			out[name] = origin.Span
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if s, ok := src.Origins[sourceVar]; ok {
			out[name] = s
		} else {
			out[name] = origin.Span
		}
	}
	return out
}

func stepVisibleVariablesFromPlan(plan *StepImportPlan, sources map[string]*ImportSource) map[string]diag.Span {
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

func addStepValuesToEnvFromPlan(env map[string]eval.Value, plan *StepImportPlan, sources map[string]*ImportSource) {
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

func addStepValuesToEnvFromWithItems(env map[string]eval.Value, items []ast.WithItem, sources map[string]*ImportSource) {
	imports := resolveImportedVars(items, sources)
	for name, origins := range imports {
		if len(origins) == 0 {
			continue
		}
		origin := origins[0]
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

type importedVar struct {
	Name      string
	SourceVar string
	Paramset  string
	Kind      SourceKind
	Span      diag.Span
}

func exposedVarNames(ps *Paramset) []string {
	return planutil.SourceVarNames(ps.Order, ps.Vars)
}

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
			add(item.Name, item.Name, src.Name, src.Kind, item.Span)
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

func resolveImportedVarsFromPlan(plan *StepImportPlan) map[string][]importedVar {
	out := make(map[string][]importedVar, len(plan.Effective))
	for name, origin := range plan.Effective {
		out[name] = append(out[name], importedVar{
			Name:      name,
			SourceVar: origin.SourceVar,
			Paramset:  origin.Paramset,
			Kind:      origin.Kind,
			Span:      origin.Span,
		})
	}
	return out
}
