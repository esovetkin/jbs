package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestValidateStepsDuplicateAndUnknownDependencyBranches(t *testing.T) {
	doDupFirst := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 5))
	doDupSecond := diag.NewSpan("in.jbs", diag.NewPos(0, 2, 1), diag.NewPos(1, 2, 5))
	doA := diag.NewSpan("in.jbs", diag.NewPos(0, 3, 1), diag.NewPos(1, 3, 5))
	doB := diag.NewSpan("in.jbs", diag.NewPos(0, 4, 1), diag.NewPos(1, 4, 5))
	subDup := diag.NewSpan("in.jbs", diag.NewPos(0, 5, 1), diag.NewPos(1, 5, 8))
	sub := diag.NewSpan("in.jbs", diag.NewPos(0, 6, 1), diag.NewPos(1, 6, 8))

	res := &Result{
		DoBlocks: []ast.DoBlock{
			{Name: "dup", Span: doDupFirst},
			{Name: "dup", Span: doDupSecond},
			{Name: "a", After: []string{"missing_dep"}, Span: doA},
			{Name: "b", After: []string{"a"}, Span: doB},
		},
		Submits: []ast.SubmitBlock{
			{Name: "dup", Span: subDup},
			{Name: "sub", After: []string{"missing_submit_dep"}, Span: sub},
		},
	}
	diags := &diag.Diagnostics{}
	validateSteps(res, diags)

	e211 := 0
	e212 := 0
	e213 := 0
	seenDoDupSecond := false
	seenSubDup := false
	seenDoUnknown := false
	seenSubmitUnknown := false

	for _, item := range diags.Items {
		switch item.Code {
		case string(diag.CodeE211):
			e211++
			if item.Span == doDupSecond {
				seenDoDupSecond = true
			}
			if item.Span == subDup {
				seenSubDup = true
			}
			if len(item.Related) == 0 || item.Related[0].Span != doDupFirst {
				t.Fatalf("expected duplicate-step diagnostic to include first definition span, got %#v", item)
			}
		case string(diag.CodeE212):
			e212++
			if item.Span == doA {
				seenDoUnknown = true
			}
			if item.Span == sub {
				seenSubmitUnknown = true
			}
		case string(diag.CodeE213):
			e213++
		}
	}

	if e211 != 2 {
		t.Fatalf("expected 2 duplicate step diagnostics, got %d: %s", e211, diags.String())
	}
	if !seenDoDupSecond || !seenSubDup {
		t.Fatalf("expected duplicate diagnostics at second do and duplicate submit spans, got %#v", diags.Items)
	}
	if e212 != 2 {
		t.Fatalf("expected 2 unknown dependency diagnostics, got %d: %s", e212, diags.String())
	}
	if !seenDoUnknown || !seenSubmitUnknown {
		t.Fatalf("expected unknown dependency diagnostics on do and submit steps, got %#v", diags.Items)
	}
	if e213 != 0 {
		t.Fatalf("did not expect cycle diagnostics, got %d: %s", e213, diags.String())
	}
}
