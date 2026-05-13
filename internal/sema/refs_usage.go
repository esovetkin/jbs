package sema

import (
	"maps"
	"slices"
)

type usedBySource map[BindingVersionKey]map[string]bool

func (u usedBySource) mark(key BindingVersionKey, sourceVar string) {
	if key == (BindingVersionKey{}) || sourceVar == "" {
		return
	}
	if _, ok := u[key]; !ok {
		u[key] = make(map[string]bool)
	}
	u[key][sourceVar] = true
}

func (u usedBySource) has(key BindingVersionKey, sourceVar string) bool {
	if key == (BindingVersionKey{}) || sourceVar == "" {
		return false
	}
	return u[key][sourceVar]
}

func cloneUsedBySource(used usedBySource) usedBySource {
	out := make(usedBySource, len(used))
	for src, vars := range used {
		if len(vars) == 0 {
			out[src] = map[string]bool{}
			continue
		}
		cp := make(map[string]bool, len(vars))
		for name, mark := range vars {
			cp[name] = mark
		}
		out[src] = cp
	}
	return out
}

func propagateUsedByGlobalDeps(used usedBySource, catalog *warningCatalog, deps map[BindingVersionKey][]BindingVersionKey) {
	if len(used) == 0 || len(deps) == 0 {
		return
	}
	queue := make([]BindingVersionKey, 0, len(used))
	seen := make(map[BindingVersionKey]bool, len(used))
	for src, vars := range used {
		if len(vars) == 0 {
			continue
		}
		if seen[src] {
			continue
		}
		seen[src] = true
		queue = append(queue, src)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dep := range deps[current] {
			src := catalog.byKey[dep]
			if src == nil || len(src.Order) == 0 {
				continue
			}
			for _, varName := range src.Order {
				used.mark(dep, varName)
			}
			if !seen[dep] {
				seen[dep] = true
				queue = append(queue, dep)
			}
		}
	}
}

func versionImports(imports map[string][]importedVar, catalog *warningCatalog, bindings map[string]*GlobalBinding) map[string][]importedVar {
	if len(imports) == 0 {
		return map[string][]importedVar{}
	}
	out := make(map[string][]importedVar, len(imports))
	for visible, origins := range imports {
		for _, origin := range origins {
			sourceVar := origin.SourceVar
			if sourceVar == "" {
				sourceVar = origin.Name
			}
			key := origin.SourceKey
			if key == (BindingVersionKey{}) {
				key = catalog.keyForSource(bindings, origin.Source)
			}
			display := origin.Display
			if display == "" {
				display = key.Display()
			}
			origin.SourceVar = sourceVar
			origin.SourceKey = key
			origin.Display = display
			out[visible] = append(out[visible], origin)
		}
	}
	return out
}

func stepWarningCandidates(res *Result, catalog *warningCatalog, stepName string, snap *ScopeSnapshot) map[string][]sourceCandidate {
	candidates := make(map[string][]sourceCandidate)
	seen := make(map[string]struct{})
	addKey := func(key BindingVersionKey) {
		src := catalog.byKey[key]
		if src == nil {
			return
		}
		for _, name := range src.Order {
			dedupe := key.Public + "\x00" + key.Version + "\x00" + name
			if _, exists := seen[dedupe]; exists {
				continue
			}
			seen[dedupe] = struct{}{}
			candidates[name] = append(candidates[name], sourceCandidate{
				SourceKey: key,
				Source:    src.Name,
				Display:   src.Display,
				SourceVar: name,
				Origin:    src.VarOrigins[name],
			})
		}
	}
	addBinding := func(binding *GlobalBinding) {
		if binding == nil {
			return
		}
		addKey(BindingVersionKeyForBinding(binding, binding.Name))
	}
	addSource := func(bindings map[string]*GlobalBinding, source string) {
		addKey(catalog.keyForSource(bindings, source))
	}

	if snap != nil {
		for _, binding := range snap.Bindings {
			addBinding(binding)
		}
		if len(snap.Bindings) == 0 {
			for _, name := range slices.Sorted(maps.Keys(snap.BindingsByName)) {
				addSource(snap.BindingsByName, name)
			}
		}
	} else {
		for _, key := range catalog.order {
			addKey(key)
		}
	}
	if res != nil {
		if plan := res.StepScopeByName[stepName]; plan != nil {
			for _, name := range slices.Sorted(maps.Keys(plan.Inherited)) {
				origin := plan.Inherited[name]
				if origin.SourceKey != (BindingVersionKey{}) {
					addKey(origin.SourceKey)
					continue
				}
				addSource(res.BindingsByName, origin.Source)
			}
		}
	}
	return candidates
}
