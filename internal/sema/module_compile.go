package sema

import (
	"maps"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

func compileModule(ref imports.ModuleRef, loadRes *imports.LoadResult, globals map[string]eval.Value, opts AnalyzeOptions, diags *diag.Diagnostics, cache map[string]*moduleScope, visiting map[string]bool) *moduleScope {
	if loadRes == nil || ref.ID == "" {
		return emptyModuleScope()
	}
	if cached := cache[ref.ID]; cached != nil {
		return cloneModuleScope(cached)
	}
	if visiting[ref.ID] {
		return emptyModuleScope()
	}
	info := loadRes.Modules[ref.ID]
	if info == nil {
		return emptyModuleScope()
	}

	visiting[ref.ID] = true
	defer delete(visiting, ref.ID)

	childByIndex := make(map[int]*moduleScope, len(info.Uses))
	prefixedByIndex := make(map[int]*moduleScope, len(info.Uses))
	for _, use := range info.Uses {
		child := compileModule(use.Source, loadRes, globals, AnalyzeOptions{ShellRunner: opts.ShellRunner, Environ: opts.Environ}, diags, cache, visiting)
		childByIndex[use.Index] = child
		if use.Kind == imports.UseNamespace {
			prefixedByIndex[use.Index] = prefixModuleScope(child, use.Alias)
		}
	}

	exec := execGlobalPlanWithOptions(
		buildModuleGlobalPlan(info, childByIndex, prefixedByIndex, globals, diags),
		globals,
		globals,
		globalExecOptions{CollectPrints: opts.CollectPrints, ShellRunner: opts.ShellRunner, Environ: opts.Environ},
		diags,
	)
	scope := emptyModuleScope()
	scope.Ref = ref
	scope.Program = info.Program
	if strings.TrimSpace(info.Program.File) != "" && strings.TrimSpace(info.BaseDir) != "" {
		scope.BaseDirByFile[info.Program.File] = info.BaseDir
	}
	scope.Globals = GlobalState{
		Values: cloneValueMap(exec.ScalarGlobals.Values),
		Spans:  maps.Clone(exec.ScalarGlobals.Spans),
	}
	scope.GlobalVarByName, scope.GlobalVarOrder = globalVarsFromExec(exec)
	for _, name := range scope.GlobalVarOrder {
		gv := scope.GlobalVarByName[name]
		registerModuleExport(scope, name, gv, gv != nil && gv.Namespace == "" && !isBuiltinGlobalName(name))
		binding := bindingFromGlobalVar(name, gv)
		if binding == nil || isBuiltinGlobalName(name) {
			continue
		}
		registerModuleBinding(scope, binding, gv != nil && gv.Namespace == "")
	}
	registerSnapshotBindings(scope, exec.SnapshotBindings)
	scope.TopLevelExprs = cloneTopLevelExprResults(exec.TopLevelExprs)
	scope.PrintEvents = clonePrintEvents(exec.PrintEvents)
	scope.ScopeSnapshotsByIndex = cloneScopeSnapshotsByIndex(exec.ScopeSnapshotsByIndex)
	scope.ScopeSnapshotsByBlock = cloneScopeSnapshotsByBlock(exec.ScopeSnapshotsByBlock)

	for _, use := range info.Uses {
		if use.Kind != imports.UseNamespace {
			continue
		}
		mergeModuleScope(scope, prefixedByIndex[use.Index])
	}

	for _, stmt := range info.Program.Stmts {
		switch n := stmt.(type) {
		case ast.DoBlock:
			scope.DoBlocks = append(scope.DoBlocks, n)
			scope.StepOrder = appendUniqueString(scope.StepOrder, n.Name)
		case ast.AnalyseBlock:
			scope.AnalyseBlocks = append(scope.AnalyseBlocks, n)
		}
	}

	materializeModuleFunctionExports(scope)
	mergeGlobalVarsIntoState(&scope.Globals, scope.ExportsByName)
	cache[ref.ID] = cloneModuleScope(scope)
	return cloneModuleScope(scope)
}

func moduleProgram(child *moduleScope, info *imports.ModuleInfo, index int) ast.Program {
	if child != nil {
		return child.Program
	}
	if info == nil {
		return ast.Program{}
	}
	useByIndex := make(map[int]imports.ResolvedUse, len(info.Uses))
	for _, use := range info.Uses {
		useByIndex[use.Index] = use
	}
	use := useByIndex[index]
	return ast.Program{File: use.Source.Label}
}
