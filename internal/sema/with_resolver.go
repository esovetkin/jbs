package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

type ResolveOptions struct {
	Context                   ImportContext
	EnableMixedSourceFallback bool
}

type ExpandedWithVar struct {
	Visible   string
	SourceVar string
}

type ExpandedWithItem struct {
	Source     string
	Vars       []ExpandedWithVar
	Full       bool
	SourceExpr string
	CombAlias  string
	SliceOrder []string
	Span       diag.Span
}

type ResolveIssueKind int

const (
	IssueUnknownSource ResolveIssueKind = iota
	IssueUnknownVar
	IssueDisallowedBinding
)

type ResolveIssue struct {
	Kind     ResolveIssueKind
	Item     ast.WithItem
	Source   string
	Variable string
	Span     diag.Span
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
		if item.Rejected {
			continue
		}
		if item.SourceExpr != "" && len(item.SourceSlice) > 0 {
			src, issue := r.resolveBinding(item.SourceExpr, item, opts)
			if issue != nil {
				issues = append(issues, *issue)
				continue
			}
			vars := make([]ExpandedWithVar, 0, len(item.SourceSlice))
			ok := true
			for _, sel := range item.SourceSlice {
				if _, exists := src.Vars[sel]; !exists {
					issues = append(issues, ResolveIssue{
						Kind:     IssueUnknownVar,
						Item:     item,
						Source:   item.SourceExpr,
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
				Source:     src.Name,
				Vars:       vars,
				SourceExpr: item.SourceExpr,
				CombAlias:  item.CombAlias,
				SliceOrder: append([]string(nil), item.SourceSlice...),
				Span:       item.Span,
			})
			continue
		}
		if item.From == "" {
			src, issue := r.resolveBinding(item.Name, item, opts)
			if issue != nil {
				issues = append(issues, *issue)
				continue
			}
			expanded = append(expanded, expandFullBinding(item, src))
			continue
		}

		src, issue := r.resolveBinding(item.From, item, opts)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if _, ok := src.Vars[item.Name]; ok {
			visible := item.Name
			if item.Alias != "" {
				visible = item.Alias
			}
			expanded = append(expanded, ExpandedWithItem{
				Source: src.Name,
				Vars: []ExpandedWithVar{{
					Visible:   visible,
					SourceVar: item.Name,
				}},
				Span: item.Span,
			})
			continue
		}
		if opts.EnableMixedSourceFallback {
			fallback, fallbackIssue := r.resolveBinding(item.Name, item, opts)
			if fallbackIssue != nil {
				if fallbackIssue.Kind != IssueUnknownSource {
					issues = append(issues, *fallbackIssue)
				}
			} else if fallback != nil {
				expanded = append(expanded, expandFullBinding(item, fallback))
				continue
			}
		}
		issues = append(issues, ResolveIssue{
			Kind:     IssueUnknownVar,
			Item:     item,
			Source:   item.From,
			Variable: item.Name,
			Span:     item.Span,
		})
	}
	return expanded, issues
}

func (r BindingResolver) resolveBinding(name string, item ast.WithItem, opts ResolveOptions) (*GlobalBinding, *ResolveIssue) {
	src := r.Bindings[name]
	if src == nil {
		if r.isExpressionVisibleOnly(name) {
			return nil, &ResolveIssue{
				Kind:   IssueDisallowedBinding,
				Item:   item,
				Source: name,
				Span:   item.Span,
			}
		}
		return nil, &ResolveIssue{
			Kind:   IssueUnknownSource,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	if !src.Supports(opts.Context) {
		return nil, &ResolveIssue{
			Kind:   IssueDisallowedBinding,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
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
	sourceExpr := item.Name
	if item.Alias != "" {
		sourceExpr = item.Alias
	}
	return ExpandedWithItem{
		Source:     binding.Name,
		Vars:       vars,
		Full:       true,
		SourceExpr: sourceExpr,
		Span:       item.Span,
	}
}
