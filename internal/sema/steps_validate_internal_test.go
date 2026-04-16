package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
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

func TestValidateStepHeaderOptionsBounds(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 10, 1), diag.NewPos(1, 10, 8))
	negOne := -1
	zero := 0
	one := 1
	diags := &diag.Diagnostics{}

	validateStepHeaderOptions("do", "bad", &negOne, &negOne, &zero, span, diags)
	if countDiagCode(diags, string(diag.CodeE216)) != 1 {
		t.Fatalf("expected one E216 for max_async, got: %s", diags.String())
	}
	if countDiagCode(diags, string(diag.CodeE219)) != 1 {
		t.Fatalf("expected one E219 for procs, got: %s", diags.String())
	}
	if countDiagCode(diags, string(diag.CodeE217)) != 1 {
		t.Fatalf("expected one E217 for iterations, got: %s", diags.String())
	}

	okDiags := &diag.Diagnostics{}
	validateStepHeaderOptions("submit", "ok", &zero, &zero, &one, span, okDiags)
	if len(okDiags.Items) != 0 {
		t.Fatalf("expected no diagnostics for valid option bounds, got: %s", okDiags.String())
	}
}

func TestValidateWithItemsConflictsAcrossSources(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 20, 1), diag.NewPos(1, 20, 8))
	p0 := &Paramset{
		Name: "p0",
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1)},
		},
		Order: []string{"x"},
	}
	p1 := &Paramset{
		Name: "p1",
		Vars: map[string][]eval.Value{
			"x": {eval.Int(2)},
		},
		Order: []string{"x"},
	}
	items := []ast.WithItem{
		{Name: "x", From: "p0", Span: span},
		{Name: "x", From: "p1", Span: span},
	}
	params := map[string]*Paramset{"p0": p0, "p1": p1}
	sources := map[string]*ImportSource{
		"p0": {Name: "p0", Kind: SourceKindParam, Vars: p0.Vars, Order: p0.Order},
		"p1": {Name: "p1", Kind: SourceKindParam, Vars: p1.Vars, Order: p1.Order},
	}

	diags := &diag.Diagnostics{}
	validateWithItems(items, params, map[string]*LetNamespace{}, sources, diags)

	if countDiagCode(diags, string(diag.CodeE214)) != 1 {
		t.Fatalf("expected one E214 for cross-source with conflict, got: %s", diags.String())
	}
}
