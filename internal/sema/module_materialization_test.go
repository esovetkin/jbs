package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
)

func analyzeImportedModuleSource(t *testing.T, entry string, files map[string]string) (*Result, *diag.Diagnostics) {
	t.Helper()
	cwd := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", entry, cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImports(loadRes, map[string]eval.Value{
		"jbs_name": eval.String("bench"),
	}, diags)
	return res, diags
}

func TestImportedFunctionDefaultCapturesUseDefinitionModule(t *testing.T) {
	res, diags := analyzeImportedModuleSource(t, strings.Join([]string{
		`use make from "./lib.jbs"`,
		`f = make()`,
		`f(10)`,
		``,
	}, "\n"), map[string]string{
		"lib.jbs": strings.Join([]string{
			`base = 1`,
			`make = function(delta = base + 1) {`,
			`  function(x) { x + delta }`,
			`}`,
			``,
		}, "\n"),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(12)) {
		t.Fatalf("unexpected imported captured function result: %#v", res.TopLevelExprs)
	}
}

func TestImportedContainerFunctionsUseDefinitionModuleCaptures(t *testing.T) {
	res, diags := analyzeImportedModuleSource(t, strings.Join([]string{
		`use functions from "./lib.jbs"`,
		`functions[0](4)`,
		``,
	}, "\n"), map[string]string{
		"lib.jbs": strings.Join([]string{
			`base = 3`,
			`functions = [function(x) { x + base }]`,
			``,
		}, "\n"),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(res.TopLevelExprs) != 1 || !eval.Equal(res.TopLevelExprs[0].Value, eval.Int(7)) {
		t.Fatalf("unexpected imported container function result: %#v", res.TopLevelExprs)
	}
}

func TestFunctionNeedsMaterializationCaptureKinds(t *testing.T) {
	root := eval.NewRootFrame(nil)
	local := eval.NewChildFrame(root)
	resolverRoot := eval.NewRootFrame(nil)
	resolverRoot.Resolve = func(string, diag.Span, *diag.Diagnostics) (eval.Value, bool) {
		return eval.Int(1), true
	}
	resolverLocal := eval.NewChildFrame(root)
	resolverLocal.Resolve = func(string, diag.Span, *diag.Diagnostics) (eval.Value, bool) {
		return eval.Int(2), true
	}

	tests := []struct {
		name string
		fn   *eval.FunctionValue
		want bool
	}{
		{name: "nil", fn: nil, want: false},
		{name: "root only", fn: &eval.FunctionValue{Capture: root}, want: false},
		{name: "local only", fn: &eval.FunctionValue{Capture: local}, want: false},
		{name: "resolver root", fn: &eval.FunctionValue{Capture: resolverRoot}, want: true},
		{name: "resolver local", fn: &eval.FunctionValue{Capture: resolverLocal}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := functionNeedsMaterialization(tt.fn); got != tt.want {
				t.Fatalf("functionNeedsMaterialization() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaterializeCapturedFunctionRebindsFramesDefaultsAndMemos(t *testing.T) {
	replacementRoot := eval.NewRootFrame(map[string]eval.Value{"module_value": eval.Int(99)})
	originalRoot := eval.NewRootFrame(nil)
	originalRoot.Resolve = func(name string, _ diag.Span, _ *diag.Diagnostics) (eval.Value, bool) {
		if name == "module_value" {
			return eval.Int(1), true
		}
		return eval.Null(), false
	}
	capture := eval.NewChildFrame(originalRoot)
	capture.Values["local"] = &eval.Cell{Value: eval.Int(3), Assigned: true}
	defaultCapture := eval.NewChildFrame(originalRoot)
	defaultFn := &eval.FunctionValue{Capture: defaultCapture}
	lazyDefaultFn := &eval.FunctionValue{Capture: defaultCapture}
	fn := &eval.FunctionValue{
		Capture: capture,
		Defaults: map[int]eval.FunctionDefault{
			0: {Value: eval.Function(defaultFn), PreEvaluated: true},
			1: {Value: eval.Function(lazyDefaultFn), PreEvaluated: false},
		},
	}

	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}
	got := materializeCapturedFunction(fn, replacementRoot, frameMemo, cellMemo, fnMemo)

	if got == nil || got == fn {
		t.Fatalf("expected cloned function, got %#v", got)
	}
	if got.Capture == nil || got.Capture == capture {
		t.Fatalf("expected cloned capture frame, got %#v", got.Capture)
	}
	if got.Capture.Parent != replacementRoot {
		t.Fatalf("expected capture parent to be replacement root")
	}
	if cell := got.Capture.Values["local"]; cell == nil || cell == capture.Values["local"] || !eval.Equal(cell.Value, eval.Int(3)) {
		t.Fatalf("unexpected materialized local cell: %#v", cell)
	}
	materializedDefault := got.Defaults[0].Value.Fn
	if materializedDefault == nil || materializedDefault == defaultFn || materializedDefault.Capture.Parent != replacementRoot {
		t.Fatalf("expected pre-evaluated default function to be materialized, got %#v", got.Defaults[0])
	}
	if got.Defaults[1].Value.Fn != lazyDefaultFn {
		t.Fatalf("expected lazy default to remain unchanged")
	}
	if again := materializeCapturedFunction(fn, replacementRoot, frameMemo, cellMemo, fnMemo); again != got {
		t.Fatalf("expected function memo to return the same materialized pointer")
	}
	if materializeCapturedFunction(nil, replacementRoot, frameMemo, cellMemo, fnMemo) != nil {
		t.Fatalf("nil function should materialize to nil")
	}
}

func TestMaterializeCapturedFrameAndCellsPreserveSharingAndCycles(t *testing.T) {
	replacementRoot := eval.NewRootFrame(map[string]eval.Value{"base": eval.Int(10)})
	originalRoot := eval.NewRootFrame(nil)
	originalRoot.Resolve = func(string, diag.Span, *diag.Diagnostics) (eval.Value, bool) {
		return eval.Int(1), true
	}
	sharedCell := &eval.Cell{Value: eval.Int(4), Assigned: true}
	frame := eval.NewChildFrame(originalRoot)
	frame.Values["a"] = sharedCell
	frame.Values["b"] = sharedCell
	recursiveFn := &eval.FunctionValue{Capture: frame}
	frame.Values["self"] = &eval.Cell{Value: eval.Function(recursiveFn), Assigned: true}
	frame.Values["declared"] = &eval.Cell{Value: eval.Function(&eval.FunctionValue{Capture: frame}), Assigned: false}

	frameMemo := map[*eval.Frame]*eval.Frame{}
	cellMemo := map[*eval.Cell]*eval.Cell{}
	fnMemo := map[*eval.FunctionValue]*eval.FunctionValue{}
	got := materializeCapturedFrame(frame, replacementRoot, frameMemo, cellMemo, fnMemo)

	if got.Parent != replacementRoot {
		t.Fatalf("expected parent to be replacement root")
	}
	if got.Values["a"] == nil || got.Values["a"] != got.Values["b"] {
		t.Fatalf("expected shared source cells to remain shared after materialization")
	}
	selfFn := got.Values["self"].Value.Fn
	if selfFn == nil || selfFn.Capture != got {
		t.Fatalf("expected recursive function capture to point at materialized frame, got %#v", selfFn)
	}
	declared := got.Values["declared"]
	if declared == nil || declared.Assigned || declared.Value.Fn == nil || declared.Value.Fn.Capture != frame {
		t.Fatalf("expected unassigned cell value to remain unmaterialized, got %#v", declared)
	}
	if again := materializeCapturedFrame(frame, replacementRoot, frameMemo, cellMemo, fnMemo); again != got {
		t.Fatalf("expected frame memo to return same pointer")
	}
	if materializeCapturedFrame(nil, replacementRoot, frameMemo, cellMemo, fnMemo) != replacementRoot {
		t.Fatalf("nil frame should materialize to root")
	}
	if materializeCapturedFrame(originalRoot, replacementRoot, frameMemo, cellMemo, fnMemo) != replacementRoot {
		t.Fatalf("capture root should materialize to replacement root")
	}
	if materializeCapturedCell(nil, replacementRoot, frameMemo, cellMemo, fnMemo) != nil {
		t.Fatalf("nil cell should materialize to nil")
	}
}

func TestModuleProgramFallbacks(t *testing.T) {
	child := emptyModuleScope()
	child.Program = ast.Program{File: "child.jbs"}
	info := &imports.ModuleInfo{
		Uses: []imports.ResolvedUse{
			{Index: 2, Source: imports.ModuleRef{Label: "dep.jbs"}},
		},
	}

	if got := moduleProgram(child, info, 2); got.File != "child.jbs" {
		t.Fatalf("expected child program to win, got %#v", got)
	}
	if got := moduleProgram(nil, nil, 0); got.File != "" {
		t.Fatalf("expected empty program for nil info, got %#v", got)
	}
	if got := moduleProgram(nil, &imports.ModuleInfo{}, 0); got.File != "" {
		t.Fatalf("expected empty program for missing use, got %#v", got)
	}
	if got := moduleProgram(nil, info, 2); got.File != "dep.jbs" {
		t.Fatalf("expected source label fallback, got %#v", got)
	}
}
