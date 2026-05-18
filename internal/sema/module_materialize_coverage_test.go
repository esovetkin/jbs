package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestMaterializeModuleFunctionExportsNilAndNilGlobals(t *testing.T) {
	materializeModuleFunctionExports(nil)

	scope := emptyModuleScope()
	scope.Globals.Values["base"] = eval.Int(1)
	scope.Env["local"] = eval.Int(2)
	scope.ExportsByName["nil_export"] = nil
	scope.LocalExportsByName["nil_local"] = nil
	scope.GlobalVarByName["nil_global"] = nil

	materializeModuleFunctionExports(scope)
	if _, exists := scope.Env["nil_export"]; exists {
		t.Fatalf("nil export should not be written to env: %#v", scope.Env)
	}
}

func TestMaterializeCapturedFunctionRewritesCaptureDefaultsAndUsesMemos(t *testing.T) {
	span := diag.NewSpan("materialize.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	oldRoot := &eval.Frame{
		Values: map[string]*eval.Cell{
			"base": {Value: eval.Int(1), Origin: span, Assigned: true},
		},
		Resolve: func(string, diag.Span, *diag.Diagnostics) (eval.Value, bool) {
			return eval.Int(99), true
		},
	}
	captured := eval.NewChildFrame(oldRoot)
	captured.Values["local"] = &eval.Cell{
		Value: eval.List([]eval.Value{
			eval.Function(&eval.FunctionValue{Capture: oldRoot, Span: span}),
		}),
		Origin:   span,
		Assigned: true,
	}
	captured.Values["declared"] = &eval.Cell{Origin: span}
	root := eval.NewRootFrame(map[string]eval.Value{"base": eval.Int(10), "env_only": eval.String("ok")})
	unmaterializedDefault := eval.Function(&eval.FunctionValue{Capture: oldRoot, Span: span})
	fn := &eval.FunctionValue{
		Capture: captured,
		Span:    span,
		Defaults: map[int]eval.FunctionDefault{
			0: {
				Value: eval.Tuple([]eval.Value{
					eval.Function(&eval.FunctionValue{Capture: oldRoot, Span: span}),
				}),
				PreEvaluated: true,
			},
			1: {
				Value:        unmaterializedDefault,
				PreEvaluated: false,
			},
		},
	}
	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}

	got := materializeCapturedFunction(fn, root, frameMemo, cellMemo, fnMemo)
	if got == nil || got == fn {
		t.Fatalf("expected cloned function, got %#v", got)
	}
	if got.Capture == captured || got.Capture.Parent != root || got.Capture.Resolve != nil {
		t.Fatalf("unexpected materialized capture: %#v", got.Capture)
	}
	if got.Capture.Values["local"] == captured.Values["local"] {
		t.Fatalf("expected captured cell to be cloned")
	}
	localValues := got.Capture.Values["local"].Value.L
	if len(localValues) != 1 || localValues[0].Kind != eval.KindFunction || localValues[0].Fn.Capture != root {
		t.Fatalf("expected nested assigned function cell to be materialized, got %#v", localValues)
	}
	if got.Capture.Values["declared"].Assigned {
		t.Fatalf("declared-only cell should remain unassigned")
	}
	if defaultValue := got.Defaults[0].Value; len(defaultValue.L) != 1 || defaultValue.L[0].Fn.Capture != root {
		t.Fatalf("pre-evaluated default was not materialized: %#v", defaultValue)
	}
	if got.Defaults[1].Value.Fn != unmaterializedDefault.Fn {
		t.Fatalf("non-pre-evaluated default should be preserved: %#v", got.Defaults[1])
	}
	if again := materializeCapturedFunction(fn, root, frameMemo, cellMemo, fnMemo); again != got {
		t.Fatalf("expected memoized function clone, got %#v then %#v", got, again)
	}
	if materializeCapturedFunction(nil, root, frameMemo, cellMemo, fnMemo) != nil {
		t.Fatalf("nil function should materialize to nil")
	}
}

func TestMaterializeValueContainersAndDictEdges(t *testing.T) {
	span := diag.NewSpan("materialize.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	oldRoot := &eval.Frame{
		Values: map[string]*eval.Cell{},
		Resolve: func(string, diag.Span, *diag.Diagnostics) (eval.Value, bool) {
			return eval.Int(1), true
		},
	}
	root := eval.NewRootFrame(map[string]eval.Value{"x": eval.Int(2)})
	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}
	fnValue := eval.Function(&eval.FunctionValue{Capture: oldRoot, Span: span})

	gotFn := materializeValue(fnValue, root, frameMemo, cellMemo, fnMemo)
	if gotFn.Kind != eval.KindFunction || gotFn.Fn == fnValue.Fn || gotFn.Fn.Capture != root {
		t.Fatalf("expected function value to be materialized, got %#v", gotFn)
	}
	plainFn := eval.Function(&eval.FunctionValue{Capture: root, Span: span})
	if got := materializeValue(plainFn, root, frameMemo, cellMemo, fnMemo); got.Fn != plainFn.Fn {
		t.Fatalf("function without resolver-backed capture should be preserved")
	}
	if functionNeedsMaterialization(nil) {
		t.Fatalf("nil function should not need materialization")
	}

	nilDict := eval.Value{Kind: eval.KindDict}
	if got := materializeValue(nilDict, root, frameMemo, cellMemo, fnMemo); got.D != nil {
		t.Fatalf("nil dict should be returned unchanged, got %#v", got)
	}
	keep := eval.DictKey{Kind: eval.DictKeyString, S: "keep"}
	missing := eval.DictKey{Kind: eval.DictKeyString, S: "missing"}
	nested := eval.DictKey{Kind: eval.DictKeyString, S: "nested"}
	dict := eval.Value{
		Kind: eval.KindDict,
		D: &eval.Dict{
			Order: []eval.DictKey{keep, missing, nested},
			Entries: map[eval.DictKey]eval.Value{
				keep: eval.Int(3),
				nested: eval.List([]eval.Value{
					eval.Function(&eval.FunctionValue{Capture: oldRoot, Span: span}),
				}),
			},
		},
	}
	gotDict := materializeValue(dict, root, frameMemo, cellMemo, fnMemo)
	if !reflect.DeepEqual(gotDict.D.Order, []eval.DictKey{keep, nested}) {
		t.Fatalf("unexpected materialized dict order: %#v", gotDict.D.Order)
	}
	if gotDict.D.Entries[nested].L[0].Fn.Capture != root {
		t.Fatalf("nested dict function was not materialized: %#v", gotDict.D.Entries[nested])
	}
}

func TestMaterializeCapturedFrameAndCellMemoEdges(t *testing.T) {
	root := eval.NewRootFrame(map[string]eval.Value{"x": eval.Int(1)})
	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}

	if materializeCapturedFrame(nil, root, frameMemo, cellMemo, fnMemo) != root {
		t.Fatalf("nil frame should materialize to root")
	}
	if materializeCapturedCell(nil, root, frameMemo, cellMemo, fnMemo) != nil {
		t.Fatalf("nil cell should materialize to nil")
	}
	cell := &eval.Cell{Value: eval.Int(4), Assigned: true}
	gotCell := materializeCapturedCell(cell, root, frameMemo, cellMemo, fnMemo)
	if gotCell == nil || gotCell == cell || !eval.Equal(gotCell.Value, eval.Int(4)) {
		t.Fatalf("unexpected materialized cell: %#v", gotCell)
	}
	if again := materializeCapturedCell(cell, root, frameMemo, cellMemo, fnMemo); again != gotCell {
		t.Fatalf("expected memoized cell, got %#v then %#v", gotCell, again)
	}

	parent := eval.NewChildFrame(root)
	child := eval.NewChildFrame(parent)
	child.Values["cell"] = cell
	gotFrame := materializeCapturedFrame(child, root, frameMemo, cellMemo, fnMemo)
	if gotFrame == nil || gotFrame == child || gotFrame.Parent == child.Parent || gotFrame.Values["cell"] != gotCell {
		t.Fatalf("unexpected materialized frame: %#v", gotFrame)
	}
	if again := materializeCapturedFrame(child, root, frameMemo, cellMemo, fnMemo); again != gotFrame {
		t.Fatalf("expected memoized frame, got %#v then %#v", gotFrame, again)
	}
}
