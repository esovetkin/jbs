package sema

import (
	"maps"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func materializeModuleFunctionExports(scope *moduleScope) {
	if scope == nil {
		return
	}
	env := maps.Clone(scope.Globals.Values)
	mergeIntoValueEnv(env, scope.Env)
	root := eval.NewRootFrame(env)
	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}

	rewriteValue := func(value eval.Value) eval.Value {
		if value.Kind != eval.KindFunction || value.Fn == nil || !functionNeedsMaterialization(value.Fn) {
			return value
		}
		return eval.Function(materializeCapturedFunction(value.Fn, root, frameMemo, cellMemo, fnMemo))
	}
	rewriteGlobalVar := func(gv *GlobalVar) {
		if gv == nil || gv.Value.Kind != eval.KindFunction || gv.Value.Fn == nil {
			return
		}
		gv.Value = rewriteValue(gv.Value)
		gv.Order, gv.Vars = globalVarSeries(gv.Name, gv.Value)
	}

	for name, gv := range scope.ExportsByName {
		rewriteGlobalVar(gv)
		if gv != nil {
			scope.Env[name] = gv.Value
			root.AssignLocal(name, gv.Value, gv.Span)
		}
	}
	for _, gv := range scope.LocalExportsByName {
		rewriteGlobalVar(gv)
	}
	for _, gv := range scope.GlobalVarByName {
		rewriteGlobalVar(gv)
	}
}

func materializeCapturedFunction(fn *eval.FunctionValue, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) *eval.FunctionValue {
	if fn == nil {
		return nil
	}
	if next := fnMemo[fn]; next != nil {
		return next
	}
	next := *fn
	fnMemo[fn] = &next
	next.Capture = materializeCapturedFrame(fn.Capture, root, frameMemo, cellMemo, fnMemo)
	if len(fn.Defaults) > 0 {
		next.Defaults = make(map[int]eval.FunctionDefault, len(fn.Defaults))
		for index, defaultValue := range fn.Defaults {
			if defaultValue.PreEvaluated && defaultValue.Value.Kind == eval.KindFunction && defaultValue.Value.Fn != nil && functionNeedsMaterialization(defaultValue.Value.Fn) {
				defaultValue.Value = eval.Function(materializeCapturedFunction(defaultValue.Value.Fn, root, frameMemo, cellMemo, fnMemo))
			}
			next.Defaults[index] = defaultValue
		}
	}
	return &next
}

func functionNeedsMaterialization(fn *eval.FunctionValue) bool {
	if fn == nil {
		return false
	}
	for frame := fn.Capture; frame != nil; frame = frame.Parent {
		if frame.Resolve != nil {
			return true
		}
	}
	return false
}

func materializeCapturedFrame(frame *eval.Frame, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) *eval.Frame {
	if frame == nil || frame.Parent == nil {
		return root
	}
	if next := frameMemo[frame]; next != nil {
		return next
	}
	next := &eval.Frame{
		Parent: materializeCapturedFrame(frame.Parent, root, frameMemo, cellMemo, fnMemo),
		Values: make(map[string]*eval.Cell, len(frame.Values)),
	}
	frameMemo[frame] = next
	for name, cell := range frame.Values {
		next.Values[name] = materializeCapturedCell(cell, root, frameMemo, cellMemo, fnMemo)
	}
	return next
}

func materializeCapturedCell(cell *eval.Cell, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) *eval.Cell {
	if cell == nil {
		return nil
	}
	if next := cellMemo[cell]; next != nil {
		return next
	}
	next := &eval.Cell{
		Value:    cell.Value,
		Origin:   cell.Origin,
		Assigned: cell.Assigned,
	}
	cellMemo[cell] = next
	if next.Assigned && next.Value.Kind == eval.KindFunction && next.Value.Fn != nil && functionNeedsMaterialization(next.Value.Fn) {
		next.Value = eval.Function(materializeCapturedFunction(next.Value.Fn, root, frameMemo, cellMemo, fnMemo))
	}
	return next
}
