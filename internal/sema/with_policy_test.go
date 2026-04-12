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

func TestEmitWithIssuesSkipsUnknownIssueKind(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}
	emitWithIssues(diags, stepValidateWithDiagPolicy(), []ResolveIssue{
		{
			Kind:   ResolveIssueKind(999),
			Item:   ast.WithItem{Name: "x", Span: span},
			Source: "x",
			Span:   span,
		},
	})
	if len(diags.Items) != 0 {
		t.Fatalf("expected unknown issue kind to be ignored, got %d diagnostics: %s", len(diags.Items), diags.String())
	}
}

func TestStepValidateUnknownSourceHintWithFrom(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}
	emitWithIssues(diags, stepValidateWithDiagPolicy(), []ResolveIssue{
		{
			Kind:   IssueUnknownSource,
			Item:   ast.WithItem{Name: "x", From: "missing", Span: span},
			Source: "missing",
			Span:   span,
		},
	})
	if len(diags.Items) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(diags.Items))
	}
	if got := diags.Items[0].Hint; got != "import from an existing parameterset or let namespace" {
		t.Fatalf("unexpected hint for with-from unknown source: %q", got)
	}
}

func TestParamWithDiagPolicyMappings(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	tests := []struct {
		name        string
		issue       ResolveIssue
		wantCode    string
		wantMessage string
		wantHint    string
	}{
		{
			name: "unknown source",
			issue: ResolveIssue{
				Kind:   IssueUnknownSource,
				Item:   ast.WithItem{Name: "p", Span: span},
				Source: "p",
				Span:   span,
			},
			wantCode:    "E020",
			wantMessage: "unknown parameterset 'p' in with clause",
			wantHint:    "define/import the parameterset or let namespace before using it",
		},
		{
			name: "unknown variable",
			issue: ResolveIssue{
				Kind:     IssueUnknownVar,
				Item:     ast.WithItem{Name: "x", From: "p", Span: span},
				Source:   "p",
				Variable: "x",
				Span:     span,
			},
			wantCode:    "E021",
			wantMessage: "unknown variable 'x' in source 'p'",
			wantHint:    "import a variable that exists in the selected source",
		},
		{
			name: "ambiguous source",
			issue: ResolveIssue{
				Kind:   IssueAmbiguousSource,
				Item:   ast.WithItem{Name: "same", Span: span},
				Source: "same",
				Span:   span,
			},
			wantCode:    "E218",
			wantMessage: "ambiguous with source 'same': matches both param and let namespace",
			wantHint:    "disambiguate by renaming the param or let namespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			emitWithIssues(diags, paramWithDiagPolicy(), []ResolveIssue{tc.issue})
			if len(diags.Items) != 1 {
				t.Fatalf("expected one diagnostic, got %d", len(diags.Items))
			}
			got := diags.Items[0]
			if got.Code != tc.wantCode {
				t.Fatalf("unexpected code: got=%s want=%s", got.Code, tc.wantCode)
			}
			if got.Message != tc.wantMessage {
				t.Fatalf("unexpected message: got=%q want=%q", got.Message, tc.wantMessage)
			}
			if got.Hint != tc.wantHint {
				t.Fatalf("unexpected hint: got=%q want=%q", got.Hint, tc.wantHint)
			}
		})
	}
}

func TestAnalyseWithDiagPolicyMappings(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	tests := []struct {
		name        string
		issue       ResolveIssue
		wantCode    string
		wantMessage string
		wantHint    string
	}{
		{
			name: "unknown source",
			issue: ResolveIssue{
				Kind:   IssueUnknownSource,
				Item:   ast.WithItem{Name: "l", Span: span},
				Source: "l",
				Span:   span,
			},
			wantCode:    "E020",
			wantMessage: "unknown parameterset 'l' in with clause",
			wantHint:    "import from an existing let namespace",
		},
		{
			name: "unknown variable",
			issue: ResolveIssue{
				Kind:     IssueUnknownVar,
				Item:     ast.WithItem{Name: "x", From: "l", Span: span},
				Source:   "l",
				Variable: "x",
				Span:     span,
			},
			wantCode:    "E021",
			wantMessage: "unknown variable 'x' in source 'l'",
			wantHint:    "import a variable that exists in the selected let namespace",
		},
		{
			name: "ambiguous source",
			issue: ResolveIssue{
				Kind:   IssueAmbiguousSource,
				Item:   ast.WithItem{Name: "same", Span: span},
				Source: "same",
				Span:   span,
			},
			wantCode:    "E218",
			wantMessage: "ambiguous with source 'same': matches both param and let namespace",
			wantHint:    "disambiguate by renaming the param or let namespace",
		},
		{
			name: "disallowed kind",
			issue: ResolveIssue{
				Kind:   IssueDisallowedKind,
				Item:   ast.WithItem{Name: "p", Span: span},
				Source: "p",
				Span:   span,
			},
			wantCode:    "E420",
			wantMessage: "analyse with-clause can only import from let namespaces; 'p' is not a let namespace",
			wantHint:    "use `with <let_namespace>` or `with <variable> from <let_namespace>`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			emitWithIssues(diags, analyseWithDiagPolicy(), []ResolveIssue{tc.issue})
			if len(diags.Items) != 1 {
				t.Fatalf("expected one diagnostic, got %d", len(diags.Items))
			}
			got := diags.Items[0]
			if got.Code != tc.wantCode {
				t.Fatalf("unexpected code: got=%s want=%s", got.Code, tc.wantCode)
			}
			if got.Message != tc.wantMessage {
				t.Fatalf("unexpected message: got=%q want=%q", got.Message, tc.wantMessage)
			}
			if got.Hint != tc.wantHint {
				t.Fatalf("unexpected hint: got=%q want=%q", got.Hint, tc.wantHint)
			}
		})
	}
}

func TestPolicyFormatForIssueDefault(t *testing.T) {
	policy := stepValidateWithDiagPolicy()
	got := policyFormatForIssue(policy, ResolveIssueKind(-1))
	if got.Code != "" {
		t.Fatalf("expected zero code for unknown issue kind, got %q", got.Code)
	}
	if got.Message != nil || got.Hint != nil {
		t.Fatalf("expected nil message/hint for unknown issue kind, got %#v", got)
	}
}
