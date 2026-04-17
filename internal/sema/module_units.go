package sema

import (
	"maps"
	"slices"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/imports"
)

type moduleUnit struct {
	Bindings       []*GlobalBinding
	BindingsByName map[string]*GlobalBinding
	DoBlocks       []ast.DoBlock
	Submits        []ast.SubmitBlock
	StepOrder      []string
	Namespaces     map[string]*Namespace
	Env            map[string]eval.Value
}

func buildEntryNamespaceUnit(loadRes *imports.LoadResult, globals map[string]eval.Value, diags *diag.Diagnostics) (*moduleUnit, map[string]eval.Value) {
	unit := &moduleUnit{
		Bindings:       make([]*GlobalBinding, 0),
		BindingsByName: make(map[string]*GlobalBinding),
		DoBlocks:       make([]ast.DoBlock, 0),
		Submits:        make([]ast.SubmitBlock, 0),
		StepOrder:      make([]string, 0),
		Namespaces:     make(map[string]*Namespace),
		Env:            make(map[string]eval.Value),
	}
	if loadRes == nil {
		return unit, unit.Env
	}
	for _, alias := range slices.Sorted(maps.Keys(loadRes.Aliases)) {
		ref := loadRes.Aliases[alias]
		root := buildModuleRootUnit(ref, loadRes, globals, diags, map[string]*moduleUnit{})
		prefixed := prefixModuleUnit(root, alias)
		mergeModuleUnit(unit, prefixed)
	}
	return unit, maps.Clone(unit.Env)
}

func buildModuleRootUnit(ref imports.ModuleRef, loadRes *imports.LoadResult, globals map[string]eval.Value, diags *diag.Diagnostics, cache map[string]*moduleUnit) *moduleUnit {
	if loadRes == nil {
		return &moduleUnit{
			Bindings:       make([]*GlobalBinding, 0),
			BindingsByName: make(map[string]*GlobalBinding),
			DoBlocks:       make([]ast.DoBlock, 0),
			Submits:        make([]ast.SubmitBlock, 0),
			StepOrder:      make([]string, 0),
			Namespaces:     make(map[string]*Namespace),
			Env:            make(map[string]eval.Value),
		}
	}
	if cached := cache[ref.ID]; cached != nil {
		return cloneModuleUnit(cached)
	}
	info := loadRes.Modules[ref.ID]
	unit := &moduleUnit{
		Bindings:       make([]*GlobalBinding, 0),
		BindingsByName: make(map[string]*GlobalBinding),
		DoBlocks:       make([]ast.DoBlock, 0),
		Submits:        make([]ast.SubmitBlock, 0),
		StepOrder:      make([]string, 0),
		Namespaces:     make(map[string]*Namespace),
		Env:            make(map[string]eval.Value),
	}
	cache[ref.ID] = unit
	if info == nil {
		return cloneModuleUnit(unit)
	}

	for _, alias := range slices.Sorted(maps.Keys(info.Aliases)) {
		childRef := info.Aliases[alias]
		child := buildModuleRootUnit(childRef, loadRes, globals, diags, cache)
		mergeModuleUnit(unit, prefixModuleUnit(child, alias))
	}

	seedEnv := mergeValueEnv(globals, unit.Env)
	compiledGlobals, order := compileUserGlobals(info.Program, seedEnv, diags)
	for _, name := range order {
		gv := compiledGlobals[name]
		if gv == nil {
			continue
		}
		binding := bindingFromGlobalVar(name, gv)
		if binding == nil {
			continue
		}
		unit.Bindings = append(unit.Bindings, binding)
		unit.BindingsByName[name] = binding
		unit.Env[name] = binding.Value
	}

	for _, stmt := range info.Program.Stmts {
		switch n := stmt.(type) {
		case ast.DoBlock:
			unit.DoBlocks = append(unit.DoBlocks, n)
			unit.StepOrder = append(unit.StepOrder, n.Name)
		case ast.SubmitBlock:
			unit.Submits = append(unit.Submits, n)
			unit.StepOrder = append(unit.StepOrder, n.Name)
		}
	}
	return cloneModuleUnit(unit)
}

func prefixModuleUnit(unit *moduleUnit, prefix string) *moduleUnit {
	if unit == nil || strings.TrimSpace(prefix) == "" {
		return cloneModuleUnit(unit)
	}
	out := &moduleUnit{
		Bindings:       make([]*GlobalBinding, 0, len(unit.Bindings)),
		BindingsByName: make(map[string]*GlobalBinding, len(unit.BindingsByName)),
		DoBlocks:       make([]ast.DoBlock, 0, len(unit.DoBlocks)),
		Submits:        make([]ast.SubmitBlock, 0, len(unit.Submits)),
		StepOrder:      make([]string, 0, len(unit.StepOrder)),
		Namespaces:     make(map[string]*Namespace, len(unit.Namespaces)+1),
		Env:            make(map[string]eval.Value, len(unit.Env)),
	}
	for _, binding := range unit.Bindings {
		if binding == nil {
			continue
		}
		name := prefix + "." + binding.Name
		next := *binding
		next.Name = name
		next.Order = append([]string(nil), binding.Order...)
		next.Origins = maps.Clone(binding.Origins)
		next.Modes = maps.Clone(binding.Modes)
		next.Vars = cloneSeriesMap(binding.Vars)
		next.BaseVars = cloneSeriesMap(binding.BaseVars)
		next.DependsOn = prefixNames(prefix, binding.DependsOn)
		next.Rows = cloneCombRows(binding.Rows, binding.Span)
		out.Bindings = append(out.Bindings, &next)
		out.BindingsByName[name] = &next
		out.Env[name] = next.Value
	}
	for _, block := range unit.DoBlocks {
		out.DoBlocks = append(out.DoBlocks, prefixDoBlock(block, prefix))
	}
	for _, block := range unit.Submits {
		out.Submits = append(out.Submits, prefixSubmitBlock(block, prefix))
	}
	for _, stepName := range unit.StepOrder {
		out.StepOrder = append(out.StepOrder, prefix+"."+stepName)
	}
	out.Namespaces[prefix] = &Namespace{Name: prefix}
	for name, ns := range unit.Namespaces {
		q := prefix + "." + name
		next := &Namespace{
			Name:     q,
			Bindings: prefixNames(prefix, ns.Bindings),
			Steps:    prefixNames(prefix, ns.Steps),
		}
		out.Namespaces[q] = next
	}
	for _, binding := range out.Bindings {
		head, _, ok := strings.Cut(binding.Name, ".")
		if ok {
			ns := out.Namespaces[head]
			if ns == nil {
				ns = &Namespace{Name: head}
				out.Namespaces[head] = ns
			}
			ns.Bindings = appendUniqueString(ns.Bindings, binding.Name)
		}
	}
	for _, stepName := range out.StepOrder {
		head, _, ok := strings.Cut(stepName, ".")
		if ok {
			ns := out.Namespaces[head]
			if ns == nil {
				ns = &Namespace{Name: head}
				out.Namespaces[head] = ns
			}
			ns.Steps = appendUniqueString(ns.Steps, stepName)
		}
	}
	return out
}

func prefixDoBlock(block ast.DoBlock, prefix string) ast.DoBlock {
	block.Name = prefix + "." + block.Name
	block.After = prefixNames(prefix, block.After)
	block.WithItems = prefixWithItems(block.WithItems, prefix)
	return block
}

func prefixSubmitBlock(block ast.SubmitBlock, prefix string) ast.SubmitBlock {
	block.Name = prefix + "." + block.Name
	block.After = prefixNames(prefix, block.After)
	block.UseNames = prefixNames(prefix, block.UseNames)
	block.WithItems = prefixWithItems(block.WithItems, prefix)
	return block
}

func prefixWithItems(items []ast.WithItem, prefix string) []ast.WithItem {
	out := make([]ast.WithItem, len(items))
	for i, item := range items {
		next := item
		if next.From != "" {
			next.From = prefix + "." + next.From
		}
		if next.SourceExpr != "" {
			next.SourceExpr = prefix + "." + next.SourceExpr
		}
		if next.From == "" && next.SourceExpr == "" {
			next.Name = prefix + "." + next.Name
		}
		out[i] = next
	}
	return out
}

func mergeModuleUnit(dst *moduleUnit, src *moduleUnit) {
	if dst == nil || src == nil {
		return
	}
	for _, binding := range src.Bindings {
		if binding == nil {
			continue
		}
		if _, exists := dst.BindingsByName[binding.Name]; exists {
			continue
		}
		dst.Bindings = append(dst.Bindings, binding)
		dst.BindingsByName[binding.Name] = binding
		dst.Env[binding.Name] = binding.Value
		head, _, ok := strings.Cut(binding.Name, ".")
		if ok {
			ns := dst.Namespaces[head]
			if ns == nil {
				ns = &Namespace{Name: head}
				dst.Namespaces[head] = ns
			}
			ns.Bindings = appendUniqueString(ns.Bindings, binding.Name)
		}
	}
	for _, block := range src.DoBlocks {
		if containsStepName(dst.DoBlocks, block.Name) {
			continue
		}
		dst.DoBlocks = append(dst.DoBlocks, block)
	}
	for _, block := range src.Submits {
		if containsSubmitName(dst.Submits, block.Name) {
			continue
		}
		dst.Submits = append(dst.Submits, block)
	}
	for _, stepName := range src.StepOrder {
		dst.StepOrder = appendUniqueString(dst.StepOrder, stepName)
		head, _, ok := strings.Cut(stepName, ".")
		if ok {
			ns := dst.Namespaces[head]
			if ns == nil {
				ns = &Namespace{Name: head}
				dst.Namespaces[head] = ns
			}
			ns.Steps = appendUniqueString(ns.Steps, stepName)
		}
	}
	for name, ns := range src.Namespaces {
		current := dst.Namespaces[name]
		if current == nil {
			current = &Namespace{Name: name}
			dst.Namespaces[name] = current
		}
		current.Bindings = mergeUniqueStrings(current.Bindings, ns.Bindings)
		current.Steps = mergeUniqueStrings(current.Steps, ns.Steps)
	}
}

func integrateModuleUnit(res *Result, unit *moduleUnit) {
	if res == nil || unit == nil {
		return
	}
	for _, binding := range unit.Bindings {
		if binding == nil {
			continue
		}
		if _, exists := res.BindingsByName[binding.Name]; exists {
			continue
		}
		res.Bindings = append(res.Bindings, binding)
		res.BindingsByName[binding.Name] = binding
	}
	res.DoBlocks = append(res.DoBlocks, unit.DoBlocks...)
	res.Submits = append(res.Submits, unit.Submits...)
	for _, stepName := range unit.StepOrder {
		res.StepOrder = appendUniqueString(res.StepOrder, stepName)
	}
	for name, ns := range unit.Namespaces {
		current := res.Namespaces[name]
		if current == nil {
			current = &Namespace{Name: name}
			res.Namespaces[name] = current
		}
		current.Bindings = mergeUniqueStrings(current.Bindings, ns.Bindings)
		current.Steps = mergeUniqueStrings(current.Steps, ns.Steps)
	}
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

func cloneModuleUnit(unit *moduleUnit) *moduleUnit {
	if unit == nil {
		return &moduleUnit{
			Bindings:       make([]*GlobalBinding, 0),
			BindingsByName: make(map[string]*GlobalBinding),
			DoBlocks:       make([]ast.DoBlock, 0),
			Submits:        make([]ast.SubmitBlock, 0),
			StepOrder:      make([]string, 0),
			Namespaces:     make(map[string]*Namespace),
			Env:            make(map[string]eval.Value),
		}
	}
	out := &moduleUnit{
		Bindings:       make([]*GlobalBinding, 0, len(unit.Bindings)),
		BindingsByName: make(map[string]*GlobalBinding, len(unit.BindingsByName)),
		DoBlocks:       append([]ast.DoBlock(nil), unit.DoBlocks...),
		Submits:        append([]ast.SubmitBlock(nil), unit.Submits...),
		StepOrder:      append([]string(nil), unit.StepOrder...),
		Namespaces:     make(map[string]*Namespace, len(unit.Namespaces)),
		Env:            maps.Clone(unit.Env),
	}
	for _, binding := range unit.Bindings {
		if binding == nil {
			continue
		}
		next := *binding
		next.Order = append([]string(nil), binding.Order...)
		next.Origins = maps.Clone(binding.Origins)
		next.Modes = maps.Clone(binding.Modes)
		next.Vars = cloneSeriesMap(binding.Vars)
		next.BaseVars = cloneSeriesMap(binding.BaseVars)
		next.Rows = cloneCombRows(binding.Rows, binding.Span)
		next.DependsOn = append([]string(nil), binding.DependsOn...)
		out.Bindings = append(out.Bindings, &next)
		out.BindingsByName[next.Name] = &next
	}
	for name, ns := range unit.Namespaces {
		out.Namespaces[name] = &Namespace{
			Name:     ns.Name,
			Bindings: append([]string(nil), ns.Bindings...),
			Steps:    append([]string(nil), ns.Steps...),
		}
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

func containsStepName(blocks []ast.DoBlock, name string) bool {
	for _, block := range blocks {
		if block.Name == name {
			return true
		}
	}
	return false
}

func containsSubmitName(blocks []ast.SubmitBlock, name string) bool {
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
