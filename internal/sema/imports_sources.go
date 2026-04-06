package sema

import (
	"maps"
	"slices"

	"jbs/internal/diag"
	"jbs/internal/eval"
)

func buildImportSources(res *Result) {
	res.ImportSourceByName = make(map[string]*ImportSource)
	for _, ps := range res.Paramsets {
		if ps == nil {
			continue
		}
		res.ImportSourceByName[ps.Name] = importSourceFromParam(ps)
	}
	for _, ls := range res.LetNamespaces {
		if ls == nil {
			continue
		}
		res.ImportSourceByName[ls.Name] = importSourceFromLet(ls)
	}
}

func importSourceFromParam(ps *Paramset) *ImportSource {
	return &ImportSource{
		Name:    ps.Name,
		Kind:    SourceKindParam,
		Vars:    cloneSeriesMap(ps.Vars),
		Origins: cloneSpanMap(ps.Origins),
		Modes:   cloneModeMap(ps.Modes),
		Order:   append([]string(nil), exposedVarNames(ps)...),
		Span:    ps.Block.Span,
	}
}

func importSourceFromLet(ns *LetNamespace) *ImportSource {
	vars := make(map[string][]eval.Value, len(ns.Vars))
	order := slices.Sorted(maps.Keys(ns.Vars))
	for _, name := range order {
		vars[name] = valueAsSeries(ns.Vars[name])
	}
	return &ImportSource{
		Name:    ns.Name,
		Kind:    SourceKindLet,
		Vars:    vars,
		Origins: cloneSpanMap(ns.Origins),
		Modes:   cloneModeMap(ns.Modes),
		Order:   order,
		Span:    ns.Span,
	}
}

func valueAsSeries(v eval.Value) []eval.Value {
	if v.Kind == eval.KindList {
		return slices.Clone(v.L)
	}
	return []eval.Value{v}
}

func cloneSeriesMap(src map[string][]eval.Value) map[string][]eval.Value {
	out := make(map[string][]eval.Value, len(src))
	for name, vals := range src {
		cp := make([]eval.Value, len(vals))
		copy(cp, vals)
		out[name] = cp
	}
	return out
}

func cloneSpanMap(src map[string]diag.Span) map[string]diag.Span {
	return maps.Clone(src)
}

func cloneModeMap(src map[string]string) map[string]string {
	return maps.Clone(src)
}
