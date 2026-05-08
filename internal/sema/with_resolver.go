package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
)

type ResolveOptions struct {
	Context ImportContext
}

type ExpandedWithVar struct {
	Visible   string
	SourceVar string
}

type ExpandedWithItem struct {
	Source string
	Vars   []ExpandedWithVar
	Full   bool
	Span   diag.Span
}

type ResolveIssueKind int

const (
	IssueUnknownSource ResolveIssueKind = iota
	IssueUnknownVar
	IssueDisallowedBinding
)

type DisallowedBindingReason int

const (
	DisallowedBindingNone DisallowedBindingReason = iota
	DisallowedBindingNotData
	DisallowedBindingAnalyseTable
	DisallowedBindingAnalyseMultiColumn
	DisallowedBindingAnalyseNonString
)

type ResolveIssue struct {
	Kind                ResolveIssueKind
	Item                ast.WithItem
	Source              string
	Variable            string
	Span                diag.Span
	DisallowedReason    DisallowedBindingReason
	DisallowedShape     BindingShape
	DisallowedColumns   int
	DisallowedValueKind eval.Kind
}

type BindingResolver struct {
	Bindings   map[string]*GlobalBinding
	Globals    map[string]eval.Value
	Namespaces map[string]*Namespace
}

func (r BindingResolver) ExpandWithItems(items []ast.WithItem, opts ResolveOptions) ([]ExpandedWithItem, []ResolveIssue) {
	expanded := make([]ExpandedWithItem, 0, len(items))
	issues := make([]ResolveIssue, 0)

	for _, item := range items {
		src, issue := r.resolveBinding(item.Source, item, opts)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if len(item.Selectors) == 0 {
			expanded = append(expanded, expandFullBinding(item, src))
			continue
		}
		vars := make([]ExpandedWithVar, 0, len(item.Selectors))
		ok := true
		for _, sel := range item.Selectors {
			if _, exists := src.Vars[sel]; !exists {
				issues = append(issues, ResolveIssue{
					Kind:     IssueUnknownVar,
					Item:     item,
					Source:   item.Source,
					Variable: sel,
					Span:     item.Span,
				})
				ok = false
				continue
			}
			vars = append(vars, ExpandedWithVar{Visible: sel, SourceVar: sel})
		}
		if !ok {
			continue
		}
		expanded = append(expanded, ExpandedWithItem{
			Source: src.Name,
			Vars:   vars,
			Span:   item.Span,
		})
	}
	return expanded, issues
}

func (r BindingResolver) resolveBinding(name string, item ast.WithItem, opts ResolveOptions) (*GlobalBinding, *ResolveIssue) {
	src := r.Bindings[name]
	if src == nil {
		if r.isExpressionVisibleOnly(name) {
			return nil, &ResolveIssue{
				Kind:             IssueDisallowedBinding,
				Item:             item,
				Source:           name,
				Span:             item.Span,
				DisallowedReason: DisallowedBindingNotData,
			}
		}
		return nil, &ResolveIssue{
			Kind:   IssueUnknownSource,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	if reason := src.SupportIssue(opts.Context); reason != DisallowedBindingNone {
		issue := &ResolveIssue{
			Kind:              IssueDisallowedBinding,
			Item:              item,
			Source:            name,
			Span:              item.Span,
			DisallowedReason:  reason,
			DisallowedShape:   src.Shape,
			DisallowedColumns: len(src.Order),
		}
		if len(src.Order) == 1 {
			vals := src.Vars[src.Order[0]]
			if len(vals) > 0 {
				issue.DisallowedValueKind = vals[0].Kind
			}
		}
		return nil, issue
	}
	return src, nil
}

func (r BindingResolver) isExpressionVisibleOnly(name string) bool {
	if name == "" {
		return false
	}
	if _, exists := r.Bindings[name]; exists {
		return false
	}
	if _, exists := r.Globals[name]; exists {
		return true
	}
	return r.Namespaces[name] != nil
}

func expandFullBinding(item ast.WithItem, binding *GlobalBinding) ExpandedWithItem {
	vars := make([]ExpandedWithVar, 0, len(binding.Order))
	for _, name := range planutil.SourceVarNames(binding.Order, binding.Vars) {
		vars = append(vars, ExpandedWithVar{
			Visible:   name,
			SourceVar: name,
		})
	}
	return ExpandedWithItem{
		Source: binding.Name,
		Vars:   vars,
		Full:   true,
		Span:   item.Span,
	}
}
