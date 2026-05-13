package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

type moduleScope struct {
	Ref                   imports.ModuleRef
	Program               ast.Program
	BaseDirByFile         map[string]string
	Globals               GlobalState
	GlobalVarByName       map[string]*GlobalVar
	GlobalVarOrder        []string
	TopLevelExprs         []TopLevelExprResult
	LocalExportsByName    map[string]*GlobalVar
	ExportsByName         map[string]*GlobalVar
	LocalBindings         []*GlobalBinding
	LocalBindingsByName   map[string]*GlobalBinding
	Bindings              []*GlobalBinding
	BindingsByName        map[string]*GlobalBinding
	BindingsByKey         map[BindingVersionKey]*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
	DoBlocks              []ast.DoBlock
	AnalyseBlocks         []ast.AnalyseBlock
	StepOrder             []string
	Namespaces            map[string]*Namespace
	Env                   map[string]eval.Value
	PrintEvents           []PrintEvent
}

func buildEntryModuleScope(loadRes *imports.LoadResult, globals map[string]eval.Value, opts AnalyzeOptions, diags *diag.Diagnostics) *moduleScope {
	if loadRes == nil {
		return emptyModuleScope()
	}
	return compileModule(loadRes.Entry, loadRes, globals, opts, diags, map[string]*moduleScope{}, map[string]bool{})
}
