package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/planutil"
)

type WithResolveOptions struct {
	AllowParam                bool
	AllowLet                  bool
	EnableMixedSourceFallback bool
	DetectAmbiguousSource     bool
}

type ExpandedWithVar struct {
	Visible   string
	SourceVar string
}

type ExpandedWithItem struct {
	Source string
	Kind   SourceKind
	Vars   []ExpandedWithVar
	Full   bool
	Span   diag.Span
}

type ResolveIssueKind int

const (
	IssueUnknownSource ResolveIssueKind = iota
	IssueUnknownVar
	IssueAmbiguousSource
	IssueDisallowedKind
)

type ResolveIssue struct {
	Kind     ResolveIssueKind
	Item     ast.WithItem
	Source   string
	Variable string
	Span     diag.Span
}

type WithResolver struct {
	Params  map[string]*Paramset
	Lets    map[string]*LetNamespace
	Sources map[string]*ImportSource
}

func (r WithResolver) ExpandWithItems(items []ast.WithItem, opts WithResolveOptions) ([]ExpandedWithItem, []ResolveIssue) {
	expanded := make([]ExpandedWithItem, 0, len(items))
	issues := make([]ResolveIssue, 0)

	for _, item := range items {
		if item.From == "" {
			src, issue := r.resolveNamedSource(item.Name, item, opts)
			if issue != nil {
				issues = append(issues, *issue)
				continue
			}
			expanded = append(expanded, expandFullSource(item, src))
			continue
		}

		src, issue := r.resolveNamedSource(item.From, item, opts)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if _, ok := src.Vars[item.Name]; ok {
			expanded = append(expanded, ExpandedWithItem{
				Source: src.Name,
				Kind:   src.Kind,
				Vars: []ExpandedWithVar{
					{
						Visible:   item.Name,
						SourceVar: item.Name,
					},
				},
				Full: false,
				Span: item.Span,
			})
			continue
		}

		if opts.EnableMixedSourceFallback {
			fallback, fallbackIssue := r.resolveFallbackSource(item.Name, item, opts)
			if fallbackIssue != nil {
				issues = append(issues, *fallbackIssue)
				continue
			}
			if fallback != nil {
				expanded = append(expanded, expandFullSource(item, fallback))
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

func (r WithResolver) resolveNamedSource(name string, item ast.WithItem, opts WithResolveOptions) (*ImportSource, *ResolveIssue) {
	if opts.DetectAmbiguousSource && r.Params[name] != nil && r.Lets[name] != nil {
		return nil, &ResolveIssue{
			Kind:   IssueAmbiguousSource,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	src := r.Sources[name]
	if src == nil {
		return nil, &ResolveIssue{
			Kind:   IssueUnknownSource,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	if !sourceKindAllowed(src.Kind, opts) {
		return nil, &ResolveIssue{
			Kind:   IssueDisallowedKind,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	return src, nil
}

func (r WithResolver) resolveFallbackSource(name string, item ast.WithItem, opts WithResolveOptions) (*ImportSource, *ResolveIssue) {
	if opts.DetectAmbiguousSource && r.Params[name] != nil && r.Lets[name] != nil {
		return nil, &ResolveIssue{
			Kind:   IssueAmbiguousSource,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	src := r.Sources[name]
	if src == nil {
		return nil, nil
	}
	if !sourceKindAllowed(src.Kind, opts) {
		return nil, &ResolveIssue{
			Kind:   IssueDisallowedKind,
			Item:   item,
			Source: name,
			Span:   item.Span,
		}
	}
	return src, nil
}

func sourceKindAllowed(kind SourceKind, opts WithResolveOptions) bool {
	switch kind {
	case SourceKindParam:
		return opts.AllowParam
	case SourceKindLet:
		return opts.AllowLet
	default:
		return false
	}
}

func expandFullSource(item ast.WithItem, src *ImportSource) ExpandedWithItem {
	vars := make([]ExpandedWithVar, 0, len(src.Order))
	for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
		vars = append(vars, ExpandedWithVar{
			Visible:   name,
			SourceVar: name,
		})
	}
	return ExpandedWithItem{
		Source: src.Name,
		Kind:   src.Kind,
		Vars:   vars,
		Full:   true,
		Span:   item.Span,
	}
}
