package sema

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"

func materializeModuleFunctionExports(scope *moduleScope) {
	if scope == nil {
		return
	}
	env := cloneValueMap(scope.Globals.Values)
	mergeIntoValueEnv(env, scope.Env)
	root := eval.NewRootFrame(env)
	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}

	rewriteValue := func(value eval.Value) eval.Value {
		return materializeValue(value, root, frameMemo, cellMemo, fnMemo)
	}
	rewriteGlobalVar := func(gv *GlobalVar) {
		if gv == nil {
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
			if defaultValue.PreEvaluated {
				defaultValue.Value = materializeValue(defaultValue.Value, root, frameMemo, cellMemo, fnMemo)
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

func materializeValue(value eval.Value, root *eval.Frame, frameMemo map[*eval.Frame]*eval.Frame, cellMemo map[*eval.Cell]*eval.Cell, fnMemo map[*eval.FunctionValue]*eval.FunctionValue) eval.Value {
	switch value.Kind {
	case eval.KindFunction:
		if value.Fn != nil && functionNeedsMaterialization(value.Fn) {
			return eval.Function(materializeCapturedFunction(value.Fn, root, frameMemo, cellMemo, fnMemo))
		}
		return value
	case eval.KindList:
		out := make([]eval.Value, len(value.L))
		for i, item := range value.L {
			out[i] = materializeValue(item, root, frameMemo, cellMemo, fnMemo)
		}
		return eval.List(out)
	case eval.KindTuple:
		out := make([]eval.Value, len(value.L))
		for i, item := range value.L {
			out[i] = materializeValue(item, root, frameMemo, cellMemo, fnMemo)
		}
		return eval.Tuple(out)
	case eval.KindDict:
		if value.D == nil {
			return value
		}
		out := eval.DictValue(nil)
		for _, key := range value.D.Order {
			item, ok := value.D.Entries[key]
			if !ok {
				continue
			}
			out.D.Set(key, materializeValue(item, root, frameMemo, cellMemo, fnMemo))
		}
		return out
	default:
		return value
	}
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
	if next.Assigned {
		next.Value = materializeValue(next.Value, root, frameMemo, cellMemo, fnMemo)
	}
	return next
}
