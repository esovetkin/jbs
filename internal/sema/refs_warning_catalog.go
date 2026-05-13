package sema

import (
	"maps"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
)

type warningSource struct {
	Key        BindingVersionKey
	Name       string
	Display    string
	Span       diag.Span
	Order      []string
	VarOrigins map[string]diag.Span
	DependsOn  []BindingVersionKey
	depNames   []string
}

type warningCatalog struct {
	byKey      map[BindingVersionKey]*warningSource
	keyByExact map[string]BindingVersionKey
	order      []BindingVersionKey
}

func newWarningCatalog() *warningCatalog {
	return &warningCatalog{
		byKey:      make(map[BindingVersionKey]*warningSource),
		keyByExact: make(map[string]BindingVersionKey),
		order:      make([]BindingVersionKey, 0),
	}
}

func buildWarningCatalog(res *Result) *warningCatalog {
	catalog := newWarningCatalog()
	if res == nil {
		return catalog
	}
	for _, binding := range res.Bindings {
		catalog.addBinding(binding, "")
	}
	for _, key := range slices.SortedFunc(maps.Keys(res.BindingsByKey), compareBindingVersionKey) {
		catalog.addBinding(res.BindingsByKey[key], key.Display())
	}
	for _, name := range slices.Sorted(maps.Keys(res.BindingsByName)) {
		catalog.addBinding(res.BindingsByName[name], name)
	}
	for _, index := range slices.Sorted(maps.Keys(res.ScopeSnapshotsByIndex)) {
		catalog.addSnapshot(res.ScopeSnapshotsByIndex[index])
	}
	for _, key := range slices.Sorted(maps.Keys(res.ScopeSnapshotsByBlock)) {
		catalog.addSnapshot(res.ScopeSnapshotsByBlock[key])
	}
	catalog.finalizeDeps()
	return catalog
}

func (c *warningCatalog) addSnapshot(snap *ScopeSnapshot) {
	if c == nil || snap == nil {
		return
	}
	for _, binding := range snap.Bindings {
		c.addBinding(binding, "")
	}
	for _, name := range slices.Sorted(maps.Keys(snap.BindingsByName)) {
		c.addBinding(snap.BindingsByName[name], name)
	}
}

func (c *warningCatalog) addBinding(binding *GlobalBinding, fallback string) {
	if c == nil {
		return
	}
	if binding == nil {
		return
	}
	key := BindingVersionKeyForBinding(binding, fallback)
	if key == (BindingVersionKey{}) {
		return
	}
	exact := binding.Name
	if exact == "" {
		exact = fallback
	}
	if exact != "" {
		if _, exists := c.keyByExact[exact]; !exists {
			c.keyByExact[exact] = key
		}
	}
	if _, exists := c.byKey[key]; exists {
		return
	}
	order := planutil.SourceVarNames(binding.Order, binding.Vars)
	if len(order) == 0 {
		return
	}
	c.byKey[key] = &warningSource{
		Key:        key,
		Name:       exact,
		Display:    key.Display(),
		Span:       binding.Span,
		Order:      order,
		VarOrigins: warningVarOrigins(binding, order),
		DependsOn:  append([]BindingVersionKey(nil), binding.DependsOnKeys...),
		depNames:   append([]string(nil), binding.DependsOn...),
	}
	c.order = append(c.order, key)
}

func (c *warningCatalog) finalizeDeps() {
	if c == nil {
		return
	}
	for _, key := range c.order {
		src := c.byKey[key]
		if src == nil || len(src.DependsOn) > 0 {
			continue
		}
		seen := make(map[BindingVersionKey]struct{}, len(src.depNames))
		for _, depName := range src.depNames {
			dep := c.keyForSource(nil, depName)
			if dep == (BindingVersionKey{}) || dep == key {
				continue
			}
			if _, exists := seen[dep]; exists {
				continue
			}
			seen[dep] = struct{}{}
			src.DependsOn = append(src.DependsOn, dep)
		}
		slices.SortFunc(src.DependsOn, compareBindingVersionKey)
	}
}

func (c *warningCatalog) sources() []warningSource {
	if c == nil {
		return nil
	}
	out := make([]warningSource, 0, len(c.order))
	for _, key := range c.order {
		src := c.byKey[key]
		if src == nil {
			continue
		}
		out = append(out, *src)
	}
	return out
}

func (c *warningCatalog) keyForSource(bindings map[string]*GlobalBinding, source string) BindingVersionKey {
	if source == "" {
		return BindingVersionKey{}
	}
	if binding := bindings[source]; binding != nil {
		return BindingVersionKeyForBinding(binding, source)
	}
	if c != nil {
		if key, ok := c.keyByExact[source]; ok {
			return key
		}
	}
	return BindingVersionKey{Public: source, Version: source}
}

func warningVarOrigins(binding *GlobalBinding, order []string) map[string]diag.Span {
	origins := make(map[string]diag.Span, len(order))
	for _, name := range order {
		if binding == nil {
			continue
		}
		origin := binding.Origins[name]
		if origin.IsZero() {
			origin = binding.Span
		}
		origins[name] = origin
	}
	return origins
}

func buildWarningSources(res *Result) []warningSource {
	return buildWarningCatalog(res).sources()
}

func buildGlobalSourceDeps(catalog *warningCatalog) map[BindingVersionKey][]BindingVersionKey {
	out := make(map[BindingVersionKey][]BindingVersionKey)
	seen := make(map[BindingVersionKey]map[BindingVersionKey]struct{})
	if catalog == nil {
		return out
	}
	for _, from := range catalog.order {
		src := catalog.byKey[from]
		if src == nil || len(src.DependsOn) == 0 {
			continue
		}
		for _, to := range src.DependsOn {
			if to == (BindingVersionKey{}) || to == from {
				continue
			}
			if catalog.byKey[to] == nil {
				continue
			}
			if _, ok := seen[from]; !ok {
				seen[from] = make(map[BindingVersionKey]struct{})
			}
			if _, ok := seen[from][to]; ok {
				continue
			}
			seen[from][to] = struct{}{}
			out[from] = append(out[from], to)
		}
	}
	for key := range out {
		slices.SortFunc(out[key], compareBindingVersionKey)
	}
	return out
}
