// define diagnostic policy mappings for with-resolution issues
package sema

import (
	"fmt"

	"jbs/internal/diag"
)

type withIssueFormat struct {
	Code    diag.Code
	Message func(ResolveIssue) string
	Hint    func(ResolveIssue) string
}

type WithDiagPolicy struct {
	UnknownSource     withIssueFormat
	UnknownVar        withIssueFormat
	DisallowedBinding withIssueFormat
}

func unknownSourceFormat(hint func(ResolveIssue) string) withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE020,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("unknown global import source '%s' in with clause", issue.Source)
		},
		Hint: hint,
	}
}

func unknownVarFormat(hint func(ResolveIssue) string) withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE021,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("unknown variable '%s' in source '%s'", issue.Variable, issue.Source)
		},
		Hint: hint,
	}
}

func analyseDisallowedBindingFormat() withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE420,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("analyse with-clause can only import scalar string data bindings; '%s' is not a data binding", issue.Source)
		},
		Hint: func(ResolveIssue) string {
			return "use a scalar string data binding, not an expression-visible global such as a function"
		},
	}
}

func stepDisallowedBindingFormat() withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE420,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("with-clause can only import data bindings; '%s' is not a data binding", issue.Source)
		},
		Hint: func(ResolveIssue) string {
			return "use a scalar/table data binding, not an expression-visible global such as a function"
		},
	}
}

func baseWithDiagPolicy() WithDiagPolicy {
	return WithDiagPolicy{
		UnknownVar: unknownVarFormat(func(ResolveIssue) string {
			return "import a variable that exists in the selected source"
		}),
	}
}

func emitWithIssues(diags *diag.Diagnostics, policy WithDiagPolicy, issues []ResolveIssue) {
	for _, issue := range issues {
		format := policyFormatForIssue(policy, issue.Kind)
		if format.Message == nil {
			continue
		}
		msg := format.Message(issue)
		hint := ""
		if format.Hint != nil {
			hint = format.Hint(issue)
		}
		diags.AddError(format.Code, msg, issue.Span, hint)
	}
}

func policyFormatForIssue(policy WithDiagPolicy, kind ResolveIssueKind) withIssueFormat {
	switch kind {
	case IssueUnknownSource:
		return policy.UnknownSource
	case IssueUnknownVar:
		return policy.UnknownVar
	case IssueDisallowedBinding:
		return policy.DisallowedBinding
	default:
		return withIssueFormat{}
	}
}

func paramWithDiagPolicy() WithDiagPolicy {
	policy := baseWithDiagPolicy()
	policy.UnknownSource = unknownSourceFormat(func(ResolveIssue) string {
		return "define or import the global binding before using it"
	})
	return policy
}

func stepValidateWithDiagPolicy() WithDiagPolicy {
	policy := baseWithDiagPolicy()
	policy.UnknownSource = unknownSourceFormat(func(issue ResolveIssue) string {
		if issue.Item.From == "" {
			return "import an existing global binding"
		}
		return "import from an existing global binding"
	})
	policy.DisallowedBinding = stepDisallowedBindingFormat()
	return policy
}

func analyseWithDiagPolicy() WithDiagPolicy {
	policy := baseWithDiagPolicy()
	policy.UnknownSource = unknownSourceFormat(func(ResolveIssue) string {
		return "import from an existing scalar string global"
	})
	policy.UnknownVar = unknownVarFormat(func(ResolveIssue) string {
		return "import a variable that exists in the selected global binding"
	})
	policy.DisallowedBinding = analyseDisallowedBindingFormat()
	return policy
}
