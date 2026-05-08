package sema

import (
	"maps"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func prefixModuleScope(scope *moduleScope, prefix string) *moduleScope {
	if scope == nil || strings.TrimSpace(prefix) == "" {
		return cloneModuleScope(scope)
	}
	out := emptyModuleScope()
	out.BaseDirByFile = maps.Clone(scope.BaseDirByFile)
	for name, exported := range scope.ExportsByName {
		if exported == nil {
			continue
		}
		prefixedName := prefix + "." + name
		next := cloneGlobalVar(exported)
		next.Name = prefixedName
		next.DependsOn = prefixNames(prefix, exported.DependsOn)
		next.DependsOnKeys = prefixBindingVersionKeys(prefix, exported.DependsOnKeys)
		out.ExportsByName[prefixedName] = next
		out.Env[prefixedName] = next.Value
	}
	for name, exported := range scope.LocalExportsByName {
		if exported == nil {
			continue
		}
		prefixedName := prefix + "." + name
		next := cloneGlobalVar(exported)
		next.Name = prefixedName
		next.DependsOn = prefixNames(prefix, exported.DependsOn)
		next.DependsOnKeys = prefixBindingVersionKeys(prefix, exported.DependsOnKeys)
		out.LocalExportsByName[prefixedName] = next
	}
	for _, binding := range scope.Bindings {
		if binding == nil {
			continue
		}
		prefixedName := prefix + "." + binding.Name
		next := cloneBinding(binding)
		next.Name = prefixedName
		next.PublicName = prefix + "." + bindingDisplayName(binding)
		next.DependsOn = prefixNames(prefix, binding.DependsOn)
		next.DependsOnKeys = prefixBindingVersionKeys(prefix, binding.DependsOnKeys)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[prefixedName] = next
	}
	for _, block := range scope.DoBlocks {
		out.DoBlocks = append(out.DoBlocks, prefixDoBlock(block, prefix))
	}
	for index, snap := range scope.ScopeSnapshotsByIndex {
		out.ScopeSnapshotsByIndex[index] = prefixScopeSnapshot(snap, prefix)
	}
	for key, snap := range scope.ScopeSnapshotsByBlock {
		out.ScopeSnapshotsByBlock[prefixSnapshotBlockKey(key, prefix)] = prefixScopeSnapshot(snap, prefix)
	}
	for _, stepName := range scope.StepOrder {
		out.StepOrder = append(out.StepOrder, prefix+"."+stepName)
	}
	out.Namespaces[prefix] = &Namespace{Name: prefix}
	for name, ns := range scope.Namespaces {
		q := prefix + "." + name
		out.Namespaces[q] = &Namespace{
			Name:     q,
			Members:  prefixNames(prefix, ns.Members),
			Bindings: prefixNames(prefix, ns.Bindings),
			Steps:    prefixNames(prefix, ns.Steps),
		}
	}
	for name := range out.ExportsByName {
		head, _, ok := strings.Cut(name, ".")
		if !ok {
			continue
		}
		ns := out.Namespaces[head]
		if ns == nil {
			ns = &Namespace{Name: head}
			out.Namespaces[head] = ns
		}
		ns.Members = appendUniqueString(ns.Members, name)
	}
	for _, binding := range out.Bindings {
		head, _, ok := strings.Cut(binding.Name, ".")
		if !ok {
			continue
		}
		ns := out.Namespaces[head]
		if ns == nil {
			ns = &Namespace{Name: head}
			out.Namespaces[head] = ns
		}
		ns.Bindings = appendUniqueString(ns.Bindings, binding.Name)
	}
	for _, stepName := range out.StepOrder {
		head, _, ok := strings.Cut(stepName, ".")
		if !ok {
			continue
		}
		ns := out.Namespaces[head]
		if ns == nil {
			ns = &Namespace{Name: head}
			out.Namespaces[head] = ns
		}
		ns.Steps = appendUniqueString(ns.Steps, stepName)
	}
	return out
}

func prefixDoBlock(block ast.DoBlock, prefix string) ast.DoBlock {
	block.Name = prefix + "." + block.Name
	block.After = prefixNames(prefix, block.After)
	block.WithItems = prefixWithItems(block.WithItems, prefix)
	return block
}

func prefixWithItems(items []ast.WithItem, prefix string) []ast.WithItem {
	out := make([]ast.WithItem, len(items))
	for i, item := range items {
		next := item
		if next.Source != "" {
			next.Source = prefix + "." + next.Source
		}
		out[i] = next
	}
	return out
}

func prefixNames(prefix string, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, prefix+"."+name)
	}
	return out
}

func prefixBindingVersionKeys(prefix string, keys []BindingVersionKey) []BindingVersionKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]BindingVersionKey, 0, len(keys))
	for _, key := range keys {
		if key == (BindingVersionKey{}) {
			continue
		}
		if strings.TrimSpace(key.Public) != "" {
			key.Public = prefix + "." + key.Public
		}
		out = append(out, key)
	}
	return out
}

func prefixScopeSnapshot(snap *ScopeSnapshot, prefix string) *ScopeSnapshot {
	if snap == nil || strings.TrimSpace(prefix) == "" {
		return cloneScopeSnapshot(snap)
	}
	out := cloneScopeSnapshot(snap)
	out.Globals.Values = prefixValueMap(prefix, out.Globals.Values)
	out.Globals.Spans = prefixSpanMap(prefix, out.Globals.Spans)
	out.GlobalVarByName = make(map[string]*GlobalVar, len(snap.GlobalVarByName))
	out.GlobalVarOrder = prefixNames(prefix, snap.GlobalVarOrder)
	for name, gv := range snap.GlobalVarByName {
		next := cloneGlobalVar(gv)
		if next == nil {
			continue
		}
		next.Name = prefix + "." + name
		next.DependsOn = prefixNames(prefix, next.DependsOn)
		next.DependsOnKeys = prefixBindingVersionKeys(prefix, next.DependsOnKeys)
		out.GlobalVarByName[next.Name] = next
	}
	out.Bindings = make([]*GlobalBinding, 0, len(snap.Bindings))
	out.BindingsByName = make(map[string]*GlobalBinding, len(snap.BindingsByName))
	for _, binding := range snap.Bindings {
		next := cloneBinding(binding)
		if next == nil {
			continue
		}
		next.Name = prefix + "." + next.Name
		next.PublicName = prefix + "." + bindingDisplayName(binding)
		next.DependsOn = prefixNames(prefix, next.DependsOn)
		next.DependsOnKeys = prefixBindingVersionKeys(prefix, next.DependsOnKeys)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[next.Name] = next
		out.BindingsByName[next.PublicName] = next
	}
	out.Namespaces = make(map[string]*Namespace, len(snap.Namespaces)+1)
	for name, ns := range snap.Namespaces {
		if ns == nil {
			continue
		}
		q := prefix + "." + name
		out.Namespaces[q] = &Namespace{
			Name:     q,
			Members:  prefixNames(prefix, ns.Members),
			Bindings: prefixNames(prefix, ns.Bindings),
			Steps:    prefixNames(prefix, ns.Steps),
		}
	}
	out.Namespaces[prefix] = &Namespace{Name: prefix}
	for _, binding := range out.Bindings {
		out.Namespaces[prefix].Bindings = appendUniqueString(out.Namespaces[prefix].Bindings, binding.PublicName)
	}
	for name := range out.GlobalVarByName {
		out.Namespaces[prefix].Members = appendUniqueString(out.Namespaces[prefix].Members, name)
	}
	return out
}

func prefixSnapshotBlockKey(key string, prefix string) string {
	parts := strings.Split(key, "|")
	if len(parts) < 4 || strings.TrimSpace(prefix) == "" {
		return key
	}
	parts[1] = prefix + "." + parts[1]
	return strings.Join(parts, "|")
}

func prefixValueMap(prefix string, in map[string]eval.Value) map[string]eval.Value {
	out := make(map[string]eval.Value, len(in))
	for name, value := range in {
		out[prefix+"."+name] = eval.CloneValue(value)
	}
	return out
}

func prefixSpanMap(prefix string, in map[string]diag.Span) map[string]diag.Span {
	out := make(map[string]diag.Span, len(in))
	for name, value := range in {
		out[prefix+"."+name] = value
	}
	return out
}
