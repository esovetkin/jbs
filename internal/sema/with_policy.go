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
	return WithDiagPolicy{
		UnknownSource: withIssueFormat{
			Code: diag.CodeE020,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("unknown parameterset '%s' in with clause", issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "define/import the parameterset or let namespace before using it"
			},
		},
		UnknownVar: withIssueFormat{
			Code: diag.CodeE021,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("unknown variable '%s' in source '%s'", issue.Variable, issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "import a variable that exists in the selected source"
			},
		},
		Ambiguous: withIssueFormat{
			Code: diag.CodeE218,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "disambiguate by renaming the param or let namespace"
			},
		},
	}
}

func stepValidateWithDiagPolicy() WithDiagPolicy {
	return WithDiagPolicy{
		UnknownSource: withIssueFormat{
			Code: diag.CodeE020,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("unknown parameterset '%s' in with clause", issue.Source)
			},
			Hint: func(issue ResolveIssue) string {
				if issue.Item.From == "" {
					return "import an existing parameterset or let namespace"
				}
				return "import from an existing parameterset or let namespace"
			},
		},
		UnknownVar: withIssueFormat{
			Code: diag.CodeE021,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("unknown variable '%s' in source '%s'", issue.Variable, issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "import a variable that exists in the selected source"
			},
		},
		Ambiguous: withIssueFormat{
			Code: diag.CodeE218,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "disambiguate by renaming the param or let namespace"
			},
		},
	}
}

func analyseWithDiagPolicy() WithDiagPolicy {
	return WithDiagPolicy{
		UnknownSource: withIssueFormat{
			Code: diag.CodeE020,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("unknown parameterset '%s' in with clause", issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "import from an existing let namespace"
			},
		},
		UnknownVar: withIssueFormat{
			Code: diag.CodeE021,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("unknown variable '%s' in source '%s'", issue.Variable, issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "import a variable that exists in the selected let namespace"
			},
		},
		Ambiguous: withIssueFormat{
			Code: diag.CodeE218,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("ambiguous with source '%s': matches both param and let namespace", issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "disambiguate by renaming the param or let namespace"
			},
		},
		DisallowedKind: withIssueFormat{
			Code: diag.CodeE420,
			Message: func(issue ResolveIssue) string {
				return fmt.Sprintf("analyse with-clause can only import from let namespaces; '%s' is not a let namespace", issue.Source)
			},
			Hint: func(ResolveIssue) string {
				return "use `with <let_namespace>` or `with <variable> from <let_namespace>`"
			},
		},
	}
}
