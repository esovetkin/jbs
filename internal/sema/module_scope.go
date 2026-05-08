package sema

import (
	"maps"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func mergeModuleScope(dst *moduleScope, src *moduleScope) {
	if dst == nil || src == nil {
		return
	}
	for file, baseDir := range src.BaseDirByFile {
		if strings.TrimSpace(file) == "" || strings.TrimSpace(baseDir) == "" {
			continue
		}
		if _, exists := dst.BaseDirByFile[file]; !exists {
			dst.BaseDirByFile[file] = baseDir
		}
	}
	for name, exported := range src.ExportsByName {
		if exported == nil {
			continue
		}
		if _, exists := dst.ExportsByName[name]; exists {
			continue
		}
		next := cloneGlobalVar(exported)
		dst.ExportsByName[name] = next
		dst.Env[name] = next.Value
	}
	for _, binding := range src.Bindings {
		if binding == nil {
			continue
		}
		if _, exists := dst.BindingsByName[binding.Name]; exists {
			continue
		}
		next := cloneBinding(binding)
		dst.Bindings = append(dst.Bindings, next)
		dst.BindingsByName[next.Name] = next
		dst.Env[next.Name] = next.Value
	}
	for _, block := range src.DoBlocks {
		if containsStepName(dst.DoBlocks, block.Name) {
			continue
		}
		dst.DoBlocks = append(dst.DoBlocks, block)
	}
	for _, stepName := range src.StepOrder {
		dst.StepOrder = appendUniqueString(dst.StepOrder, stepName)
	}
	for name, ns := range src.Namespaces {
		current := dst.Namespaces[name]
		if current == nil {
			current = &Namespace{Name: name}
			dst.Namespaces[name] = current
		}
		current.Members = mergeUniqueStrings(current.Members, ns.Members)
		current.Bindings = mergeUniqueStrings(current.Bindings, ns.Bindings)
		current.Steps = mergeUniqueStrings(current.Steps, ns.Steps)
	}
	for index, snap := range src.ScopeSnapshotsByIndex {
		if _, exists := dst.ScopeSnapshotsByIndex[index]; !exists {
			dst.ScopeSnapshotsByIndex[index] = cloneScopeSnapshot(snap)
		}
	}
	for key, snap := range src.ScopeSnapshotsByBlock {
		if _, exists := dst.ScopeSnapshotsByBlock[key]; !exists {
			dst.ScopeSnapshotsByBlock[key] = cloneScopeSnapshot(snap)
		}
	}
}

func emptyModuleScope() *moduleScope {
	return &moduleScope{
		Globals:               GlobalState{Values: map[string]eval.Value{}, Spans: map[string]diag.Span{}},
		GlobalVarByName:       make(map[string]*GlobalVar),
		GlobalVarOrder:        make([]string, 0),
		TopLevelExprs:         make([]TopLevelExprResult, 0),
		PrintEvents:           make([]PrintEvent, 0),
		LocalExportsByName:    make(map[string]*GlobalVar),
		ExportsByName:         make(map[string]*GlobalVar),
		LocalBindings:         make([]*GlobalBinding, 0),
		LocalBindingsByName:   make(map[string]*GlobalBinding),
		Bindings:              make([]*GlobalBinding, 0),
		BindingsByName:        make(map[string]*GlobalBinding),
		ScopeSnapshotsByIndex: make(map[int]*ScopeSnapshot),
		ScopeSnapshotsByBlock: make(map[string]*ScopeSnapshot),
		BaseDirByFile:         make(map[string]string),
		DoBlocks:              make([]ast.DoBlock, 0),
		AnalyseBlocks:         make([]ast.AnalyseBlock, 0),
		StepOrder:             make([]string, 0),
		Namespaces:            make(map[string]*Namespace),
		Env:                   make(map[string]eval.Value),
	}
}

func cloneModuleScope(scope *moduleScope) *moduleScope {
	if scope == nil {
		return emptyModuleScope()
	}
	out := emptyModuleScope()
	out.Ref = scope.Ref
	out.Program = scope.Program
	out.BaseDirByFile = maps.Clone(scope.BaseDirByFile)
	out.Globals = GlobalState{
		Values: maps.Clone(scope.Globals.Values),
		Spans:  maps.Clone(scope.Globals.Spans),
	}
	out.GlobalVarByName, out.GlobalVarOrder = cloneGlobalVars(scope.GlobalVarByName, scope.GlobalVarOrder)
	out.TopLevelExprs = cloneTopLevelExprResults(scope.TopLevelExprs)
	out.PrintEvents = clonePrintEvents(scope.PrintEvents)
	out.DoBlocks = append([]ast.DoBlock(nil), scope.DoBlocks...)
	out.AnalyseBlocks = append([]ast.AnalyseBlock(nil), scope.AnalyseBlocks...)
	out.StepOrder = append([]string(nil), scope.StepOrder...)
	out.Env = maps.Clone(scope.Env)
	for name, exported := range scope.LocalExportsByName {
		out.LocalExportsByName[name] = cloneGlobalVar(exported)
	}
	for name, exported := range scope.ExportsByName {
		out.ExportsByName[name] = cloneGlobalVar(exported)
	}
	for _, binding := range scope.LocalBindings {
		next := cloneBinding(binding)
		out.LocalBindings = append(out.LocalBindings, next)
		out.LocalBindingsByName[next.Name] = next
	}
	for _, binding := range scope.Bindings {
		next := cloneBinding(binding)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[next.Name] = next
	}
	out.ScopeSnapshotsByIndex = cloneScopeSnapshotsByIndex(scope.ScopeSnapshotsByIndex)
	out.ScopeSnapshotsByBlock = cloneScopeSnapshotsByBlock(scope.ScopeSnapshotsByBlock)
	for name, ns := range scope.Namespaces {
		out.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Members:  append([]string(nil), ns.Members...),
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	return out
}

func cloneGlobalVars(byName map[string]*GlobalVar, order []string) (map[string]*GlobalVar, []string) {
	out := make(map[string]*GlobalVar, len(byName))
	for name, gv := range byName {
		if gv == nil {
			continue
		}
		next := *gv
		next.Order = append([]string(nil), gv.Order...)
		next.Vars = cloneSeriesMap(gv.Vars)
		next.DependsOn = append([]string(nil), gv.DependsOn...)
		next.DependsOnKeys = append([]BindingVersionKey(nil), gv.DependsOnKeys...)
		out[name] = &next
	}
	return out, slices.Clone(order)
}

func cloneBinding(binding *GlobalBinding) *GlobalBinding {
	if binding == nil {
		return nil
	}
	next := *binding
	next.Order = append([]string(nil), binding.Order...)
	next.Origins = maps.Clone(binding.Origins)
	next.Vars = cloneSeriesMap(binding.Vars)
	next.BaseVars = cloneSeriesMap(binding.BaseVars)
	next.Rows = cloneCombRows(binding.Rows, binding.Span)
	next.DependsOn = append([]string(nil), binding.DependsOn...)
	next.DependsOnKeys = append([]BindingVersionKey(nil), binding.DependsOnKeys...)
	return &next
}

func cloneScopeSnapshot(snap *ScopeSnapshot) *ScopeSnapshot {
	if snap == nil {
		return nil
	}
	out := &ScopeSnapshot{
		Index: snap.Index,
		Globals: GlobalState{
			Values: maps.Clone(snap.Globals.Values),
			Spans:  maps.Clone(snap.Globals.Spans),
		},
		Bindings:       make([]*GlobalBinding, 0, len(snap.Bindings)),
		BindingsByName: make(map[string]*GlobalBinding, len(snap.BindingsByName)),
		Namespaces:     make(map[string]*Namespace, len(snap.Namespaces)),
	}
	out.GlobalVarByName, out.GlobalVarOrder = cloneGlobalVars(snap.GlobalVarByName, snap.GlobalVarOrder)
	for _, binding := range snap.Bindings {
		next := cloneBinding(binding)
		out.Bindings = append(out.Bindings, next)
		out.BindingsByName[next.Name] = next
		if next.PublicName != "" {
			out.BindingsByName[next.PublicName] = next
		}
	}
	for name, binding := range snap.BindingsByName {
		if binding == nil {
			continue
		}
		if existing := out.BindingsByName[binding.Name]; existing != nil {
			out.BindingsByName[name] = existing
			continue
		}
		out.BindingsByName[name] = cloneBinding(binding)
	}
	for name, ns := range snap.Namespaces {
		if ns == nil {
			continue
		}
		out.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Members:  append([]string(nil), ns.Members...),
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
	}
	return out
}

func cloneScopeSnapshotsByIndex(in map[int]*ScopeSnapshot) map[int]*ScopeSnapshot {
	out := make(map[int]*ScopeSnapshot, len(in))
	for index, snap := range in {
		out[index] = cloneScopeSnapshot(snap)
	}
	return out
}

func cloneScopeSnapshotsByBlock(in map[string]*ScopeSnapshot) map[string]*ScopeSnapshot {
	out := make(map[string]*ScopeSnapshot, len(in))
	for key, snap := range in {
		out[key] = cloneScopeSnapshot(snap)
	}
	return out
}

func cloneGlobalVar(gv *GlobalVar) *GlobalVar {
	if gv == nil {
		return nil
	}
	next := *gv
	next.Order = append([]string(nil), gv.Order...)
	next.Vars = cloneSeriesMap(gv.Vars)
	next.DependsOn = append([]string(nil), gv.DependsOn...)
	next.DependsOnKeys = append([]BindingVersionKey(nil), gv.DependsOnKeys...)
	return &next
}

func cloneTopLevelExprResults(in []TopLevelExprResult) []TopLevelExprResult {
	if len(in) == 0 {
		return []TopLevelExprResult{}
	}
	out := make([]TopLevelExprResult, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Value = eval.CloneValue(item.Value)
	}
	return out
}

func clonePrintEvents(in []PrintEvent) []PrintEvent {
	if len(in) == 0 {
		return []PrintEvent{}
	}
	out := make([]PrintEvent, len(in))
	for i, event := range in {
		out[i] = event
		out[i].Values = eval.CloneValues(event.Values)
	}
	return out
}

func mergeValueEnv(base map[string]eval.Value, extras map[string]eval.Value) map[string]eval.Value {
	out := make(map[string]eval.Value, len(base)+len(extras))
	for name, value := range base {
		out[name] = value
	}
	for name, value := range extras {
		out[name] = value
	}
	return out
}

func registerModuleExport(scope *moduleScope, name string, gv *GlobalVar, local bool) {
	if scope == nil || gv == nil || strings.TrimSpace(name) == "" {
		return
	}
	next := cloneGlobalVar(gv)
	next.Name = name
	if local {
		scope.LocalExportsByName[name] = next
	}
	if _, exists := scope.ExportsByName[name]; !exists {
		scope.ExportsByName[name] = next
	}
	scope.Env[name] = next.Value
}

func registerModuleBinding(scope *moduleScope, binding *GlobalBinding, local bool) {
	if scope == nil || binding == nil || strings.TrimSpace(binding.Name) == "" {
		return
	}
	next := cloneBinding(binding)
	if local {
		scope.LocalBindings = append(scope.LocalBindings, next)
		scope.LocalBindingsByName[next.Name] = next
	}
	if _, exists := scope.BindingsByName[next.Name]; exists {
		return
	}
	scope.Bindings = append(scope.Bindings, next)
	scope.BindingsByName[next.Name] = next
}

func registerSnapshotBindings(scope *moduleScope, bindings []*GlobalBinding) {
	if scope == nil {
		return
	}
	for _, binding := range bindings {
		if binding == nil || strings.TrimSpace(binding.Name) == "" {
			continue
		}
		next := cloneBinding(binding)
		next.SyntheticGlobal = true
		scope.Bindings = append(scope.Bindings, next)
		scope.BindingsByName[next.Name] = next
	}
}

func mergeIntoValueEnv(dst map[string]eval.Value, src map[string]eval.Value) {
	if dst == nil {
		return
	}
	for name, value := range src {
		dst[name] = value
	}
}

func mergeBindingValues(env map[string]eval.Value, bindings map[string]*GlobalBinding) {
	if env == nil {
		return
	}
	for name, binding := range bindings {
		if binding == nil {
			continue
		}
		env[name] = binding.Value
	}
}

func containsStepName(blocks []ast.DoBlock, name string) bool {
	for _, block := range blocks {
		if block.Name == name {
			return true
		}
	}
	return false
}

func mergeUniqueStrings(dst []string, src []string) []string {
	for _, value := range src {
		dst = appendUniqueString(dst, value)
	}
	return dst
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
