package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestValidateStepsReportsDuplicatesUnknownDepsAndCycles(t *testing.T) {
	span := diag.NewSpan("steps.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		DoBlocks: []ast.DoBlock{
			{Name: "build", Span: span},
			{Name: "a", After: []string{"b"}, Span: span},
			{Name: "b", After: []string{"a"}, Span: span},
		},
		Submits: []ast.SubmitBlock{
			{Name: "build", Span: span},
			{Name: "submit", After: []string{"missing"}, Span: span},
		},
	}

	diags := &diag.Diagnostics{}
	validateSteps(res, diags)

	if countDiagCode(diags, "E211") != 1 {
		t.Fatalf("expected one duplicate-step diagnostic, got %d: %s", countDiagCode(diags, "E211"), diags.String())
	}
	if countDiagCode(diags, "E212") != 1 {
		t.Fatalf("expected one unknown-dependency diagnostic, got %d: %s", countDiagCode(diags, "E212"), diags.String())
	}
	if countDiagCode(diags, "E213") != 1 {
		t.Fatalf("expected one dependency-cycle diagnostic, got %d: %s", countDiagCode(diags, "E213"), diags.String())
	}
}

func TestValidateStepHeaderOptionsBounds(t *testing.T) {
	span := diag.NewSpan("steps.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	maxAsync := -1
	procs := -2
	iterations := 0
	diags := &diag.Diagnostics{}

	validateStepHeaderOptions("do", "run", &maxAsync, &procs, &iterations, span, diags)

	if countDiagCode(diags, "E216") != 1 {
		t.Fatalf("expected one invalid max_async diagnostic, got %d: %s", countDiagCode(diags, "E216"), diags.String())
	}
	if countDiagCode(diags, "E219") != 1 {
		t.Fatalf("expected one invalid procs diagnostic, got %d: %s", countDiagCode(diags, "E219"), diags.String())
	}
	if countDiagCode(diags, "E217") != 1 {
		t.Fatalf("expected one invalid iterations diagnostic, got %d: %s", countDiagCode(diags, "E217"), diags.String())
	}
}

func TestValidateUseClausesAndWithItemsConflicts(t *testing.T) {
	span := diag.NewSpan("steps.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	bindings := map[string]*GlobalBinding{
		"srcA": {
			Name:  "srcA",
			Shape: BindingTable,
			Order: []string{"a"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(1)},
			},
		},
		"srcB": {
			Name:  "srcB",
			Shape: BindingTable,
			Order: []string{"a"},
			Vars: map[string][]eval.Value{
				"a": {eval.Int(2)},
			},
		},
	}
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{
				"fn": eval.Function(&eval.FunctionValue{}),
			},
		},
		BindingsByName: bindings,
		DoBlocks: []ast.DoBlock{
			{
				Name: "run",
				WithItems: []ast.WithItem{
					{Name: "a", From: "srcA", Span: span},
					{Name: "a", From: "srcB", Span: span},
					{Name: "missing_var", From: "srcA", Span: span},
				},
				Span: span,
			},
			{
				Name: "fn_step",
				WithItems: []ast.WithItem{
					{Name: "fn", Span: span},
				},
				Span: span,
			},
		},
		Submits: []ast.SubmitBlock{
			{
				Name: "submit",
				WithItems: []ast.WithItem{
					{Name: "missing_source", Span: span},
				},
				Span: span,
			},
		},
	}

	diags := &diag.Diagnostics{}
	validateUseClauses(res, diags)

	if countDiagCode(diags, "E214") != 1 {
		t.Fatalf("expected one conflicting-import diagnostic, got %d: %s", countDiagCode(diags, "E214"), diags.String())
	}
	if countDiagCode(diags, "E021") != 1 {
		t.Fatalf("expected one unknown-variable diagnostic, got %d: %s", countDiagCode(diags, "E021"), diags.String())
	}
	if countDiagCode(diags, "E020") != 1 {
		t.Fatalf("expected one unknown-source diagnostic, got %d: %s", countDiagCode(diags, "E020"), diags.String())
	}
	if countDiagCode(diags, "E420") != 1 {
		t.Fatalf("expected one disallowed-binding diagnostic, got %d: %s", countDiagCode(diags, "E420"), diags.String())
	}
}
