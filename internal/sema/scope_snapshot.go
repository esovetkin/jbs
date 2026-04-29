package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func snapshotForDoBlock(res *Result, block ast.DoBlock) *ScopeSnapshot {
	if res == nil {
		return nil
	}
	if snap := res.ScopeSnapshotsByBlock[doBlockSnapshotKey(block)]; snap != nil {
		return snap
	}
	return res.ScopeSnapshotsByIndex[stmtIndexBySpan(res.Program, block.Span)]
}

func snapshotForSubmitBlock(res *Result, block ast.SubmitBlock) *ScopeSnapshot {
	if res == nil {
		return nil
	}
	if snap := res.ScopeSnapshotsByBlock[submitBlockSnapshotKey(block)]; snap != nil {
		return snap
	}
	return res.ScopeSnapshotsByIndex[stmtIndexBySpan(res.Program, block.Span)]
}

func snapshotForAnalyseBlock(res *Result, block ast.AnalyseBlock) *ScopeSnapshot {
	if res == nil {
		return nil
	}
	if snap := res.ScopeSnapshotsByBlock[analyseBlockSnapshotKey(block)]; snap != nil {
		return snap
	}
	return res.ScopeSnapshotsByIndex[stmtIndexBySpan(res.Program, block.Span)]
}

func stmtIndexBySpan(prog ast.Program, span diag.Span) int {
	for index, stmt := range prog.Stmts {
		if stmt.GetSpan() == span {
			return index
		}
	}
	return -1
}

func snapshotBindings(res *Result, snap *ScopeSnapshot) map[string]*GlobalBinding {
	if snap != nil && snap.BindingsByName != nil {
		return snap.BindingsByName
	}
	if res == nil {
		return nil
	}
	return res.BindingsByName
}

func snapshotBindingsWithResult(res *Result, snap *ScopeSnapshot) map[string]*GlobalBinding {
	if res == nil {
		return snapshotBindings(res, snap)
	}
	if snap == nil || snap.BindingsByName == nil {
		return res.BindingsByName
	}
	out := make(map[string]*GlobalBinding, len(res.BindingsByName)+len(snap.BindingsByName))
	for name, binding := range res.BindingsByName {
		out[name] = binding
	}
	for name, binding := range snap.BindingsByName {
		out[name] = binding
	}
	return out
}

func snapshotGlobals(res *Result, snap *ScopeSnapshot) map[string]eval.Value {
	if snap != nil && snap.Globals.Values != nil {
		return snap.Globals.Values
	}
	if res == nil {
		return nil
	}
	return res.Globals.Values
}

func snapshotNamespaces(res *Result, snap *ScopeSnapshot) map[string]*Namespace {
	if snap != nil && snap.Namespaces != nil {
		return snap.Namespaces
	}
	if res == nil {
		return nil
	}
	return res.Namespaces
}
