package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func (e *globalSeqEngine) recordDeclarationSnapshot(step globalInputStep) {
	snap := e.cloneSnapshot(step.Index)
	if snap == nil {
		return
	}
	e.res.ScopeSnapshotsByIndex[step.Index] = snap
	if key := globalStepBlockKey(step); key != "" {
		e.res.ScopeSnapshotsByBlock[key] = snap
	}
	for _, binding := range snap.Bindings {
		e.res.SnapshotBindings = append(e.res.SnapshotBindings, cloneBinding(binding))
	}
}

func (e *globalSeqEngine) cloneSnapshot(index int) *ScopeSnapshot {
	snap := &ScopeSnapshot{
		Index: index,
		Globals: GlobalState{
			Values: maps.Clone(e.values),
			Spans:  maps.Clone(e.spans),
		},
		Bindings:       make([]*GlobalBinding, 0, len(e.currentBindings)),
		BindingsByName: make(map[string]*GlobalBinding, len(e.currentBindings)*2),
		Namespaces:     cloneVisibleNamespaces(e.namespaces),
	}
	snap.GlobalVarByName, snap.GlobalVarOrder = cloneGlobalVars(e.globalVars, e.globalOrder)
	for _, public := range e.currentBindingOrder {
		binding := e.currentBindings[public]
		if binding == nil {
			continue
		}
		next := cloneBinding(binding)
		next.PublicName = bindingDisplayName(next)
		next.Name = e.snapshotBindingName(next.PublicName, index)
		next.SyntheticGlobal = true
		snap.Bindings = append(snap.Bindings, next)
		snap.BindingsByName[next.Name] = next
		if next.PublicName != "" {
			snap.BindingsByName[next.PublicName] = next
		}
	}
	return snap
}

func (e *globalSeqEngine) snapshotBindingName(public string, index int) string {
	base := "_js__" + fmt.Sprint(index) + "__" + sanitizeSnapshotName(public)
	name := base
	for i := 1; ; i++ {
		if _, exists := e.snapshotNames[name]; !exists {
			if _, collides := e.currentBindings[name]; !collides {
				e.snapshotNames[name] = struct{}{}
				return name
			}
		}
		name = fmt.Sprintf("%s_%d", base, i)
	}
}

func bindingVersionID(step globalInputStep) string {
	span := globalStepSpan(step)
	if !span.IsZero() {
		return fmt.Sprintf("%s:%d:%d", span.File, span.Start.Offset, span.End.Offset)
	}
	return fmt.Sprintf("%s:%d:%s", step.Kind, step.ID, step.Name)
}

func sanitizeSnapshotName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "binding"
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "binding"
	}
	return b.String()
}

func (e *globalSeqEngine) acceptGlobalVar(gv *GlobalVar) bool {
	if gv == nil {
		return false
	}
	if gv.Name == "jbs_name" {
		if gv.Value.Kind != eval.KindString {
			e.diags.AddError(
				diag.CodeE301,
				gv.Name+" must be a simple string literal",
				gv.Span,
				"assign a plain quoted string",
			)
			return false
		}
	}
	if _, ok := e.scalarSeed[gv.Name]; ok && !isScalarGlobalValue(gv.Value) {
		e.diags.AddError(
			diag.CodeE304,
			"global variable '"+gv.Name+"' must be scalar; tuples/lists are not allowed",
			gv.Span,
			"use string/int/float/bool scalar values",
		)
		return false
	}
	return true
}

func (e *globalSeqEngine) expandGlobalDeps(deps []string, self string) []string {
	if len(deps) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(deps))
	queue := append([]string(nil), deps...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if name == "" || name == self {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if gv := e.globalVars[name]; gv != nil {
			queue = append(queue, gv.DependsOn...)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(seen))
}

func (e *globalSeqEngine) expandGlobalDepKeys(deps []string, self string) []BindingVersionKey {
	if len(deps) == 0 {
		return nil
	}
	seen := make(map[BindingVersionKey]struct{}, len(deps))
	seenNames := make(map[string]struct{}, len(deps))
	addKey := func(key BindingVersionKey) {
		if key == (BindingVersionKey{}) || key.Public == self {
			return
		}
		seen[key] = struct{}{}
	}
	var addName func(string)
	addName = func(name string) {
		if name == "" || name == self {
			return
		}
		if _, exists := seenNames[name]; exists {
			return
		}
		seenNames[name] = struct{}{}
		if key, ok := e.bindingKeyForCurrentName(name); ok {
			addKey(key)
		}
		if gv := e.globalVars[name]; gv != nil {
			if len(gv.DependsOnKeys) > 0 {
				for _, dep := range gv.DependsOnKeys {
					addKey(dep)
				}
				return
			}
			for _, depName := range gv.DependsOn {
				addName(depName)
			}
		}
	}
	for _, dep := range deps {
		addName(dep)
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]BindingVersionKey, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	slices.SortFunc(out, compareBindingVersionKey)
	return out
}

func (e *globalSeqEngine) bindingKeyForCurrentName(name string) (BindingVersionKey, bool) {
	if e == nil || name == "" {
		return BindingVersionKey{}, false
	}
	if binding := e.currentBindings[name]; binding != nil {
		return BindingVersionKeyForBinding(binding, name), true
	}
	if gv := e.globalVars[name]; gv != nil {
		return BindingVersionKeyForGlobalVar(gv, name), true
	}
	return BindingVersionKey{}, false
}

func (e *globalSeqEngine) publishGlobalVar(gv *GlobalVar) {
	if e == nil || gv == nil || gv.Name == "" {
		return
	}
	e.values[gv.Name] = gv.Value
	e.spans[gv.Name] = gv.Span
	e.rootFrame.AssignLocal(gv.Name, gv.Value, gv.Span)

	if _, ok := e.scalarSeed[gv.Name]; ok {
		e.scalarSeed[gv.Name] = gv.Value
		e.scalarSpans[gv.Name] = gv.Span
	}

	e.globalVars[gv.Name] = cloneGlobalVar(gv)
	if _, seen := e.globalOrderSeen[gv.Name]; !seen {
		e.globalOrderSeen[gv.Name] = struct{}{}
		e.globalOrder = append(e.globalOrder, gv.Name)
	}

	binding := bindingFromGlobalVar(gv.Name, gv)
	if binding == nil || isBuiltinGlobalName(gv.Name) {
		delete(e.currentBindings, gv.Name)
		return
	}
	binding.PublicName = gv.Name
	e.publishBinding(binding)
}

func (e *globalSeqEngine) publishBinding(binding *GlobalBinding) {
	if e == nil || binding == nil || binding.Name == "" {
		return
	}
	if binding.PublicName == "" {
		binding.PublicName = binding.Name
	}
	e.currentBindings[binding.Name] = cloneBinding(binding)
	if _, seen := e.currentBindingSeen[binding.Name]; !seen {
		e.currentBindingSeen[binding.Name] = struct{}{}
		e.currentBindingOrder = append(e.currentBindingOrder, binding.Name)
	}
}

func (e *globalSeqEngine) currentNameCatalog() *eval.NameCatalog {
	if e == nil {
		return nil
	}
	return scopeNameCatalog(visibleNamesFromEnv(e.values), e.namespaces)
}

func globalVarsFromExec(exec *globalExecResult) (map[string]*GlobalVar, []string) {
	if exec == nil {
		return map[string]*GlobalVar{}, nil
	}
	return cloneGlobalVars(exec.UserGlobalVarByName, exec.UserGlobalOrder)
}
