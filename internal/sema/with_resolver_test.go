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
