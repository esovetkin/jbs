package sema

import (
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestEmitWithIssuesStepValidatePolicy(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	issues := []ResolveIssue{
		{
			Kind:   IssueUnknownSource,
			Item:   ast.WithItem{Name: "missing", Span: span},
			Source: "missing",
			Span:   span,
		},
		{
			Kind:     IssueUnknownVar,
			Item:     ast.WithItem{Name: "x", From: "p", Span: span},
			Source:   "p",
			Variable: "x",
			Span:     span,
		},
		{
			Kind:   IssueAmbiguousSource,
			Item:   ast.WithItem{Name: "same", Span: span},
			Source: "same",
			Span:   span,
		},
	}

	diags := &diag.Diagnostics{}
	emitWithIssues(diags, stepValidateWithDiagPolicy(), issues)
	if len(diags.Items) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", len(diags.Items))
	}
	if diags.Items[0].Code != string(diag.CodeE020) {
		t.Fatalf("expected first code E020, got %s", diags.Items[0].Code)
	}
	if diags.Items[1].Code != string(diag.CodeE021) {
		t.Fatalf("expected second code E021, got %s", diags.Items[1].Code)
	}
	if diags.Items[2].Code != string(diag.CodeE218) {
		t.Fatalf("expected third code E218, got %s", diags.Items[2].Code)
	}
	if !strings.Contains(diags.Items[1].Message, "unknown variable 'x' in source 'p'") {
		t.Fatalf("unexpected unknown variable message: %s", diags.Items[1].Message)
	}
}

func TestEmitWithIssuesAnalyseDisallowedKind(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}
	emitWithIssues(diags, analyseWithDiagPolicy(), []ResolveIssue{
		{
			Kind:   IssueDisallowedKind,
			Item:   ast.WithItem{Name: "p", Span: span},
			Source: "p",
			Span:   span,
		},
	})
	if len(diags.Items) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(diags.Items))
	}
	if diags.Items[0].Code != string(diag.CodeE420) {
		t.Fatalf("expected E420, got %s", diags.Items[0].Code)
	}
	if !strings.Contains(diags.Items[0].Message, "can only import from let namespaces") {
		t.Fatalf("unexpected E420 message: %s", diags.Items[0].Message)
	}
}
