package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestWithResolverExpandWithItems(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	p := &Paramset{
		Name:  "p",
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}, "b": {eval.String("x")}},
		Order: []string{"a", "b"},
	}
	p1 := &Paramset{
		Name:  "p1",
		Vars:  map[string][]eval.Value{"x": {eval.Int(1)}},
		Order: []string{"x"},
	}
	p2 := &Paramset{
		Name:  "p2",
		Vars:  map[string][]eval.Value{"y": {eval.Int(2)}},
		Order: []string{"y"},
	}
	l := &LetNamespace{
		Name: "l",
		Vars: map[string]eval.Value{"s": eval.String("v")},
	}

	params := map[string]*Paramset{
		"p":  p,
		"p1": p1,
		"p2": p2,
	}
	lets := map[string]*LetNamespace{
		"l": l,
	}
	sources := map[string]*ImportSource{
		"p":  importSourceFromParam(p),
		"p1": importSourceFromParam(p1),
		"p2": importSourceFromParam(p2),
		"l":  importSourceFromLet(l),
	}
	resolver := WithResolver{
		Params:  params,
		Lets:    lets,
		Sources: sources,
	}

	tests := []struct {
		name       string
		items      []ast.WithItem
		opts       WithResolveOptions
		wantItems  int
		wantIssues []ResolveIssueKind
		wantSource string
		wantVars   []string
	}{
		{
			name: "rejected item is skipped",
			items: []ast.WithItem{
				{Name: "p", Rejected: true, Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems: 0,
		},
		{
			name: "full source import",
			items: []ast.WithItem{
				{Name: "p", Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems:  1,
			wantSource: "p",
			wantVars:   []string{"a", "b"},
		},
		{
			name: "variable import",
			items: []ast.WithItem{
				{Name: "a", From: "p", Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems:  1,
			wantSource: "p",
			wantVars:   []string{"a"},
		},
		{
			name: "mixed fallback import",
			items: []ast.WithItem{
				{Name: "x", From: "p1", Span: span},
				{Name: "p2", From: "p1", Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems:  2,
			wantSource: "p2",
			wantVars:   []string{"y"},
		},
		{
			name: "unknown fallback yields unknown variable issue",
			items: []ast.WithItem{
				{Name: "unknown", From: "p1", Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems:  0,
			wantIssues: []ResolveIssueKind{IssueUnknownVar},
		},
		{
			name: "disallowed kind",
			items: []ast.WithItem{
				{Name: "p", Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                false,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems:  0,
			wantIssues: []ResolveIssueKind{IssueDisallowedKind},
		},
		{
			name: "fallback disallowed kind",
			items: []ast.WithItem{
				{Name: "p", From: "l", Span: span},
			},
			opts: WithResolveOptions{
				AllowParam:                false,
				AllowLet:                  true,
				EnableMixedSourceFallback: true,
				DetectAmbiguousSource:     true,
			},
			wantItems:  0,
			wantIssues: []ResolveIssueKind{IssueDisallowedKind},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotItems, gotIssues := resolver.ExpandWithItems(tt.items, tt.opts)
			if len(gotItems) != tt.wantItems {
				t.Fatalf("expanded item count mismatch: got %d want %d", len(gotItems), tt.wantItems)
			}
			if len(tt.wantIssues) != len(gotIssues) {
				t.Fatalf("issue count mismatch: got %d want %d", len(gotIssues), len(tt.wantIssues))
			}
			for i, issue := range tt.wantIssues {
				if gotIssues[i].Kind != issue {
					t.Fatalf("issue %d kind mismatch: got %v want %v", i, gotIssues[i].Kind, issue)
				}
			}
			if tt.wantSource != "" {
				found := false
				for _, item := range gotItems {
					if item.Source != tt.wantSource {
						continue
					}
					found = true
					gotVars := make([]string, 0, len(item.Vars))
					for _, v := range item.Vars {
						gotVars = append(gotVars, v.Visible)
					}
					if len(gotVars) != len(tt.wantVars) {
						t.Fatalf("visible var count mismatch: got %d want %d", len(gotVars), len(tt.wantVars))
					}
					for j := range gotVars {
						if gotVars[j] != tt.wantVars[j] {
							t.Fatalf("visible var %d mismatch: got %q want %q", j, gotVars[j], tt.wantVars[j])
						}
					}
				}
				if !found {
					t.Fatalf("expected expanded source %q, got %#v", tt.wantSource, gotItems)
				}
			}
		})
	}
}

func TestWithResolverRejectedAndValidItemsMixed(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	p := &Paramset{
		Name:  "p",
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}},
		Order: []string{"a"},
	}
	resolver := WithResolver{
		Params: map[string]*Paramset{"p": p},
		Lets:   map[string]*LetNamespace{},
		Sources: map[string]*ImportSource{
			"p": importSourceFromParam(p),
		},
	}
	items := []ast.WithItem{
		{Name: "missing.p", Rejected: true, Span: span},
		{Name: "p", Span: span},
	}
	expanded, issues := resolver.ExpandWithItems(items, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	})
	if len(issues) != 0 {
		t.Fatalf("expected no issues for mixed rejected+valid items, got %#v", issues)
	}
	if len(expanded) != 1 {
		t.Fatalf("expected one expanded item, got %#v", expanded)
	}
	if expanded[0].Source != "p" {
		t.Fatalf("expected valid source p to be resolved, got %#v", expanded[0])
	}
}

func TestWithResolverAmbiguousSource(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	param := &Paramset{
		Name:  "same",
		Vars:  map[string][]eval.Value{"x": {eval.Int(1)}},
		Order: []string{"x"},
	}
	let := &LetNamespace{
		Name: "same",
		Vars: map[string]eval.Value{"x": eval.String("a")},
	}
	resolver := WithResolver{
		Params: map[string]*Paramset{
			"same": param,
		},
		Lets: map[string]*LetNamespace{
			"same": let,
		},
		Sources: map[string]*ImportSource{
			"same": importSourceFromParam(param),
		},
	}
	_, issues := resolver.ExpandWithItems([]ast.WithItem{{Name: "same", Span: span}}, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	})
	if len(issues) != 1 || issues[0].Kind != IssueAmbiguousSource {
		t.Fatalf("expected one ambiguous source issue, got %#v", issues)
	}
}

func TestWithResolverAppliesAliasToVisibleAndSourceExpr(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	p := &Paramset{
		Name:  "p",
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}},
		Order: []string{"a"},
	}
	resolver := WithResolver{
		Params: map[string]*Paramset{"p": p},
		Lets:   map[string]*LetNamespace{},
		Sources: map[string]*ImportSource{
			"p": importSourceFromParam(p),
		},
	}
	items := []ast.WithItem{
		{Name: "a", From: "p", Alias: "a_0", Span: span},
		{Name: "p", Alias: "p_0", Span: span},
	}
	expanded, issues := resolver.ExpandWithItems(items, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  false,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
	if len(expanded) != 2 {
		t.Fatalf("expected 2 expanded items, got %d", len(expanded))
	}
	if expanded[0].Vars[0].Visible != "a_0" || expanded[0].Vars[0].SourceVar != "a" {
		t.Fatalf("unexpected variable alias expansion: %#v", expanded[0])
	}
	if !expanded[1].Full || expanded[1].SourceExpr != "p_0" {
		t.Fatalf("unexpected full-source alias expansion: %#v", expanded[1])
	}
}

func TestWithResolverResolveNamedSourceUnknown(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 2, 1), diag.NewPos(1, 2, 2))
	item := ast.WithItem{Name: "missing", Span: span}
	resolver := WithResolver{
		Params:  map[string]*Paramset{},
		Lets:    map[string]*LetNamespace{},
		Sources: map[string]*ImportSource{},
	}

	src, issue := resolver.resolveNamedSource("missing", item, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     false,
	})
	if src != nil {
		t.Fatalf("expected nil source for unknown source, got %#v", src)
	}
	if issue == nil {
		t.Fatalf("expected unknown-source issue")
	}
	if issue.Kind != IssueUnknownSource {
		t.Fatalf("expected IssueUnknownSource, got %v", issue.Kind)
	}
	if issue.Source != "missing" || issue.Span != span {
		t.Fatalf("unexpected unknown-source issue payload: %#v", issue)
	}
}

func TestWithResolverResolveFallbackSourceAmbiguous(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 3, 1), diag.NewPos(1, 3, 2))
	item := ast.WithItem{Name: "x", From: "same", Span: span}
	param := &Paramset{
		Name:  "same",
		Vars:  map[string][]eval.Value{"x": {eval.Int(1)}},
		Order: []string{"x"},
	}
	let := &LetNamespace{
		Name: "same",
		Vars: map[string]eval.Value{"x": eval.String("a")},
	}
	resolver := WithResolver{
		Params: map[string]*Paramset{
			"same": param,
		},
		Lets: map[string]*LetNamespace{
			"same": let,
		},
		Sources: map[string]*ImportSource{
			"same": importSourceFromParam(param),
		},
	}

	src, issue := resolver.resolveFallbackSource("same", item, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	})
	if src != nil {
		t.Fatalf("expected nil source for ambiguous fallback, got %#v", src)
	}
	if issue == nil {
		t.Fatalf("expected ambiguous-source issue")
	}
	if issue.Kind != IssueAmbiguousSource {
		t.Fatalf("expected IssueAmbiguousSource, got %v", issue.Kind)
	}
	if issue.Source != "same" || issue.Span != span {
		t.Fatalf("unexpected ambiguous issue payload: %#v", issue)
	}
}

func TestSourceKindAllowedDefault(t *testing.T) {
	opts := WithResolveOptions{AllowParam: true, AllowLet: true}
	if sourceKindAllowed(SourceKind("custom"), opts) {
		t.Fatalf("expected custom source kind to be disallowed")
	}
}

func TestWithResolverSourceSliceExpansionSuccess(t *testing.T) {
	span := diag.NewSpan("slice.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	p := &Paramset{
		Name:  "p",
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}, "b": {eval.Int(2)}, "c": {eval.Int(3)}},
		Order: []string{"a", "b", "c"},
	}
	resolver := WithResolver{
		Params: map[string]*Paramset{"p": p},
		Lets:   map[string]*LetNamespace{},
		Sources: map[string]*ImportSource{
			"p": importSourceFromParam(p),
		},
	}

	items := []ast.WithItem{
		{
			SourceExpr: "p",
			SourceSlice: []string{
				"b",
				"a",
			},
			CombAlias: "slice_alias",
			Span:      span,
		},
	}
	expanded, issues := resolver.ExpandWithItems(items, WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  false,
		EnableMixedSourceFallback: false,
		DetectAmbiguousSource:     true,
	})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
	if len(expanded) != 1 {
		t.Fatalf("expected one expanded item, got %#v", expanded)
	}
	got := expanded[0]
	if got.Source != "p" || got.Kind != SourceKindParam {
		t.Fatalf("unexpected expanded source/kind: %#v", got)
	}
	if got.Full {
		t.Fatalf("expected source-slice expansion to be non-full import")
	}
	if got.CombAlias != "slice_alias" {
		t.Fatalf("expected comb alias to propagate, got %#v", got.CombAlias)
	}
	if got.SourceExpr != "p" {
		t.Fatalf("expected source expression p, got %q", got.SourceExpr)
	}
	if len(got.SliceOrder) != 2 || got.SliceOrder[0] != "b" || got.SliceOrder[1] != "a" {
		t.Fatalf("unexpected slice order: %#v", got.SliceOrder)
	}
	if len(got.Vars) != 2 {
		t.Fatalf("expected 2 mapped vars, got %#v", got.Vars)
	}
	if got.Vars[0].Visible != "b" || got.Vars[0].SourceVar != "b" || got.Vars[1].Visible != "a" || got.Vars[1].SourceVar != "a" {
		t.Fatalf("unexpected source-slice var mapping: %#v", got.Vars)
	}
}

func TestWithResolverSourceSliceIssues(t *testing.T) {
	span := diag.NewSpan("slice_issues.jbs", diag.NewPos(0, 2, 1), diag.NewPos(1, 2, 2))
	paramSame := &Paramset{
		Name:  "same",
		Vars:  map[string][]eval.Value{"x": {eval.Int(1)}},
		Order: []string{"x"},
	}
	letSame := &LetNamespace{
		Name: "same",
		Vars: map[string]eval.Value{"x": eval.String("x")},
	}
	letOnly := &LetNamespace{
		Name: "l",
		Vars: map[string]eval.Value{"s": eval.String("v")},
	}
	p := &Paramset{
		Name:  "p",
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}},
		Order: []string{"a"},
	}
	resolver := WithResolver{
		Params: map[string]*Paramset{
			"p":    p,
			"same": paramSame,
		},
		Lets: map[string]*LetNamespace{
			"same": letSame,
			"l":    letOnly,
		},
		Sources: map[string]*ImportSource{
			"p":    importSourceFromParam(p),
			"same": importSourceFromParam(paramSame),
			"l":    importSourceFromLet(letOnly),
		},
	}

	tests := []struct {
		name     string
		item     ast.WithItem
		opts     WithResolveOptions
		wantKind ResolveIssueKind
	}{
		{
			name: "unknown source",
			item: ast.WithItem{
				SourceExpr:  "missing",
				SourceSlice: []string{"a"},
				Span:        span,
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  false,
				EnableMixedSourceFallback: false,
				DetectAmbiguousSource:     true,
			},
			wantKind: IssueUnknownSource,
		},
		{
			name: "unknown projected var",
			item: ast.WithItem{
				SourceExpr:  "p",
				SourceSlice: []string{"missing"},
				Span:        span,
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  false,
				EnableMixedSourceFallback: false,
				DetectAmbiguousSource:     true,
			},
			wantKind: IssueUnknownVar,
		},
		{
			name: "disallowed source kind",
			item: ast.WithItem{
				SourceExpr:  "l",
				SourceSlice: []string{"s"},
				Span:        span,
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  false,
				EnableMixedSourceFallback: false,
				DetectAmbiguousSource:     true,
			},
			wantKind: IssueDisallowedKind,
		},
		{
			name: "ambiguous source",
			item: ast.WithItem{
				SourceExpr:  "same",
				SourceSlice: []string{"x"},
				Span:        span,
			},
			opts: WithResolveOptions{
				AllowParam:                true,
				AllowLet:                  true,
				EnableMixedSourceFallback: false,
				DetectAmbiguousSource:     true,
			},
			wantKind: IssueAmbiguousSource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, issues := resolver.ExpandWithItems([]ast.WithItem{tt.item}, tt.opts)
			if len(expanded) != 0 {
				t.Fatalf("expected no expanded items on error, got %#v", expanded)
			}
			if len(issues) != 1 {
				t.Fatalf("expected one issue, got %#v", issues)
			}
			if issues[0].Kind != tt.wantKind {
				t.Fatalf("expected issue %v, got %#v", tt.wantKind, issues[0])
			}
		})
	}
}
