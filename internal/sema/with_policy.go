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
	UnknownSource  withIssueFormat
	UnknownVar     withIssueFormat
	Ambiguous      withIssueFormat
	DisallowedKind withIssueFormat
}

func unknownSourceFormat(hint func(ResolveIssue) string) withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE020,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("unknown parameterset '%s' in with clause", issue.Source)
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

func ambiguousSourceFormat() withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE218,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", issue.Source)
		},
		Hint: func(ResolveIssue) string {
			return "disambiguate by renaming the param or let namespace"
		},
	}
}

func analyseDisallowedKindFormat() withIssueFormat {
	return withIssueFormat{
		Code: diag.CodeE420,
		Message: func(issue ResolveIssue) string {
			return fmt.Sprintf("analyse with-clause can only import from let namespaces; '%s' is not a let namespace", issue.Source)
		},
		Hint: func(ResolveIssue) string {
			return "use `with <let_namespace>` or `with <variable> from <let_namespace>`"
		},
	}
}

func baseWithDiagPolicy() WithDiagPolicy {
	return WithDiagPolicy{
		UnknownVar: unknownVarFormat(func(ResolveIssue) string {
			return "import a variable that exists in the selected source"
		}),
		Ambiguous: ambiguousSourceFormat(),
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
	case IssueAmbiguousSource:
		return policy.Ambiguous
	case IssueDisallowedKind:
		return policy.DisallowedKind
	default:
		return withIssueFormat{}
	}
}

func paramWithDiagPolicy() WithDiagPolicy {
	policy := baseWithDiagPolicy()
	policy.UnknownSource = unknownSourceFormat(func(ResolveIssue) string {
		return "define/import the parameterset or let namespace before using it"
	})
	return policy
}

func stepValidateWithDiagPolicy() WithDiagPolicy {
	policy := baseWithDiagPolicy()
	policy.UnknownSource = unknownSourceFormat(func(issue ResolveIssue) string {
		if issue.Item.From == "" {
			return "import an existing parameterset or let namespace"
		}
		return "import from an existing parameterset or let namespace"
	})
	return policy
}

func analyseWithDiagPolicy() WithDiagPolicy {
	policy := baseWithDiagPolicy()
	policy.UnknownSource = unknownSourceFormat(func(ResolveIssue) string {
		return "import from an existing let namespace"
	})
	policy.UnknownVar = unknownVarFormat(func(ResolveIssue) string {
		return "import a variable that exists in the selected let namespace"
	})
	policy.DisallowedKind = analyseDisallowedKindFormat()
	return policy
}
