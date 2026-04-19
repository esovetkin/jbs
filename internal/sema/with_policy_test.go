package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestWithPolicyFormatHelpers(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))

	sourceFormat := unknownSourceFormat(func(ResolveIssue) string { return "custom source hint" })
	sourceIssue := ResolveIssue{Source: "p", Span: span}
	if sourceFormat.Code != diag.CodeE020 {
		t.Fatalf("expected E020, got %q", sourceFormat.Code)
	}
	if got := sourceFormat.Message(sourceIssue); got != "unknown global import source 'p' in with clause" {
		t.Fatalf("unexpected source-format message: %q", got)
	}
	if got := sourceFormat.Hint(sourceIssue); got != "custom source hint" {
		t.Fatalf("unexpected source-format hint: %q", got)
	}

	varFormat := unknownVarFormat(func(ResolveIssue) string { return "custom var hint" })
	varIssue := ResolveIssue{Source: "p", Variable: "x", Span: span}
	if varFormat.Code != diag.CodeE021 {
		t.Fatalf("expected E021, got %q", varFormat.Code)
	}
	if got := varFormat.Message(varIssue); got != "unknown variable 'x' in source 'p'" {
		t.Fatalf("unexpected var-format message: %q", got)
	}
	if got := varFormat.Hint(varIssue); got != "custom var hint" {
		t.Fatalf("unexpected var-format hint: %q", got)
	}

	disallowedFormat := analyseDisallowedBindingFormat()
	disallowedIssue := ResolveIssue{Source: "table", Span: span}
	if disallowedFormat.Code != diag.CodeE420 {
		t.Fatalf("expected E420, got %q", disallowedFormat.Code)
	}
	if got := disallowedFormat.Message(disallowedIssue); got != "analyse with-clause can only import scalar string data bindings; 'table' is not a data binding" {
		t.Fatalf("unexpected disallowed-binding message: %q", got)
	}
	if got := disallowedFormat.Hint(disallowedIssue); got != "use a scalar string data binding, not an expression-visible global such as a function" {
		t.Fatalf("unexpected disallowed-binding hint: %q", got)
	}

	stepDisallowed := stepDisallowedBindingFormat()
	if stepDisallowed.Code != diag.CodeE420 {
		t.Fatalf("expected step disallowed-binding code E420, got %q", stepDisallowed.Code)
	}
	if got := stepDisallowed.Message(ResolveIssue{Source: "fn", Span: span}); got != "with-clause can only import data bindings; 'fn' is not a data binding" {
		t.Fatalf("unexpected step disallowed-binding message: %q", got)
	}
	if got := stepDisallowed.Hint(ResolveIssue{Source: "fn", Span: span}); got != "use a scalar/table data binding, not an expression-visible global such as a function" {
		t.Fatalf("unexpected disallowed-binding hint: %q", got)
	}
}

func TestWithPolicyMappingsAndDefaults(t *testing.T) {
	if got := policyFormatForIssue(stepValidateWithDiagPolicy(), ResolveIssueKind(-1)); got.Code != "" || got.Message != nil || got.Hint != nil {
		t.Fatalf("expected zero format for unknown issue kind, got %#v", got)
	}

	base := baseWithDiagPolicy()
	if base.UnknownVar.Code != diag.CodeE021 {
		t.Fatalf("expected unknown-var code E021, got %q", base.UnknownVar.Code)
	}
	if base.UnknownSource.Code != "" || base.UnknownSource.Message != nil || base.UnknownSource.Hint != nil {
		t.Fatalf("expected base policy to leave unknown-source unset, got %#v", base.UnknownSource)
	}
	if base.DisallowedBinding.Code != "" || base.DisallowedBinding.Message != nil || base.DisallowedBinding.Hint != nil {
		t.Fatalf("expected base policy to leave disallowed-binding unset, got %#v", base.DisallowedBinding)
	}

	paramPolicy := paramWithDiagPolicy()
	if got := paramPolicy.UnknownSource.Hint(ResolveIssue{}); got != "define or import the global binding before using it" {
		t.Fatalf("unexpected param-policy unknown-source hint: %q", got)
	}
	if got := paramPolicy.UnknownVar.Hint(ResolveIssue{}); got != "import a variable that exists in the selected source" {
		t.Fatalf("unexpected param-policy unknown-var hint: %q", got)
	}

	stepPolicy := stepValidateWithDiagPolicy()
	if got := stepPolicy.UnknownSource.Hint(ResolveIssue{Item: ast.WithItem{}}); got != "import an existing global binding" {
		t.Fatalf("unexpected step-policy unknown-source hint without from: %q", got)
	}
	if got := stepPolicy.UnknownSource.Hint(ResolveIssue{Item: ast.WithItem{From: "src"}}); got != "import from an existing global binding" {
		t.Fatalf("unexpected step-policy unknown-source hint with from: %q", got)
	}
	if stepPolicy.DisallowedBinding.Code != diag.CodeE420 {
		t.Fatalf("expected step-policy disallowed-binding code E420, got %q", stepPolicy.DisallowedBinding.Code)
	}

	analysePolicy := analyseWithDiagPolicy()
	if got := analysePolicy.UnknownSource.Hint(ResolveIssue{}); got != "import from an existing scalar string global" {
		t.Fatalf("unexpected analyse-policy unknown-source hint: %q", got)
	}
	if got := analysePolicy.UnknownVar.Hint(ResolveIssue{}); got != "import a variable that exists in the selected global binding" {
		t.Fatalf("unexpected analyse-policy unknown-var hint: %q", got)
	}
	if analysePolicy.DisallowedBinding.Code != diag.CodeE420 {
		t.Fatalf("expected analyse-policy disallowed-binding code E420, got %q", analysePolicy.DisallowedBinding.Code)
	}
}

func TestEmitWithIssuesRoutesDiagnostics(t *testing.T) {
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
			Item:     ast.WithItem{Name: "x", From: "named", Span: span},
			Source:   "named",
			Variable: "x",
			Span:     span,
		},
		{
			Kind:   IssueDisallowedBinding,
			Item:   ast.WithItem{Name: "table", Span: span},
			Source: "table",
			Span:   span,
		},
	}

	diags := &diag.Diagnostics{}
	emitWithIssues(diags, analyseWithDiagPolicy(), issues)
	if len(diags.Items) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", len(diags.Items))
	}
	if diags.Items[0].Code != string(diag.CodeE020) {
		t.Fatalf("expected first code E020, got %s", diags.Items[0].Code)
	}
	if diags.Items[0].Hint != "import from an existing scalar string global" {
		t.Fatalf("unexpected unknown-source hint: %q", diags.Items[0].Hint)
	}
	if diags.Items[1].Code != string(diag.CodeE021) {
		t.Fatalf("expected second code E021, got %s", diags.Items[1].Code)
	}
	if diags.Items[1].Message != "unknown variable 'x' in source 'named'" {
		t.Fatalf("unexpected unknown-var message: %q", diags.Items[1].Message)
	}
	if diags.Items[2].Code != string(diag.CodeE420) {
		t.Fatalf("expected third code E420, got %s", diags.Items[2].Code)
	}
	if diags.Items[2].Message != "analyse with-clause can only import scalar string data bindings; 'table' is not a data binding" {
		t.Fatalf("unexpected disallowed-binding message: %q", diags.Items[2].Message)
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
