package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func mkImportSource(name string, kind SourceKind, order []string, vars ...string) *ImportSource {
	m := make(map[string][]eval.Value, len(vars))
	for _, v := range vars {
		m[v] = []eval.Value{eval.String(v)}
	}
	return &ImportSource{Name: name, Kind: kind, Vars: m, Order: order}
}

func TestWithResolverExpandWithItems(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	resolver := WithResolver{
		Params: map[string]*Paramset{
			"p":    {Name: "p"},
			"same": {Name: "same"},
		},
		Lets: map[string]*LetNamespace{
			"l":    {Name: "l"},
			"same": {Name: "same"},
		},
		Sources: map[string]*ImportSource{
			"p":    mkImportSource("p", SourceKindParam, []string{"x", "y"}, "x", "y"),
			"l":    mkImportSource("l", SourceKindLet, []string{"a"}, "a"),
			"same": mkImportSource("same", SourceKindParam, []string{"s"}, "s"),
		},
	}
	items := []ast.WithItem{
		{Rejected: true, Name: "p", Span: span},
		{Name: "p", Span: span},
		{Name: "p", Alias: "pp", Span: span},
		{Name: "x", From: "p", Alias: "xx", Span: span},
		{Name: "l", From: "p", Span: span}, // fallback to full source l
		{SourceExpr: "p", SourceSlice: []string{"x"}, CombAlias: "slice_alias", Span: span},
		{SourceExpr: "p", SourceSlice: []string{"missing"}, Span: span},
		{Name: "missing", Span: span},
		{Name: "same", Span: span},
		{Name: "nope", From: "p", Span: span},
	}
	opts := WithResolveOptions{
		AllowParam:                true,
		AllowLet:                  true,
		EnableMixedSourceFallback: true,
		DetectAmbiguousSource:     true,
	}

	expanded, issues := resolver.ExpandWithItems(items, opts)
	if len(expanded) != 5 {
		t.Fatalf("expected 5 expanded items, got %d: %#v", len(expanded), expanded)
	}
	if !expanded[0].Full || expanded[0].Source != "p" || expanded[0].SourceExpr != "p" {
		t.Fatalf("unexpected full import expansion: %#v", expanded[0])
	}
	if !expanded[1].Full || expanded[1].SourceExpr != "pp" {
		t.Fatalf("expected aliased full import source expression, got %#v", expanded[1])
	}
	if expanded[2].Full || expanded[2].Vars[0].Visible != "xx" || expanded[2].Vars[0].SourceVar != "x" {
		t.Fatalf("unexpected from-import with alias expansion: %#v", expanded[2])
	}
	if !expanded[3].Full || expanded[3].Source != "l" {
		t.Fatalf("expected mixed-source fallback full import from let source l, got %#v", expanded[3])
	}
	if expanded[4].SourceExpr != "p" || expanded[4].CombAlias != "slice_alias" || expanded[4].Full {
		t.Fatalf("unexpected source-slice expansion metadata: %#v", expanded[4])
	}
	if !reflect.DeepEqual(expanded[4].SliceOrder, []string{"x"}) {
		t.Fatalf("unexpected source-slice order: %#v", expanded[4].SliceOrder)
	}

	issueKinds := make([]ResolveIssueKind, 0, len(issues))
	for _, issue := range issues {
		issueKinds = append(issueKinds, issue.Kind)
	}
	wantKinds := []ResolveIssueKind{
		IssueUnknownVar,      // source-slice unknown var
		IssueUnknownSource,   // missing full source
		IssueAmbiguousSource, // same matches param+let
		IssueUnknownVar,      // unknown var from p with no fallback
	}
	if !reflect.DeepEqual(issueKinds, wantKinds) {
		t.Fatalf("unexpected resolve issues: got=%#v want=%#v", issueKinds, wantKinds)
	}
}

func TestWithResolverNamedAndFallbackResolutionBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 10, 1), diag.NewPos(1, 10, 2))
	item := ast.WithItem{Name: "x", Span: span}

	resolver := WithResolver{
		Params: map[string]*Paramset{"same": {Name: "same"}},
		Lets:   map[string]*LetNamespace{"same": {Name: "same"}},
		Sources: map[string]*ImportSource{
			"p": mkImportSource("p", SourceKindParam, []string{"x"}, "x"),
			"l": mkImportSource("l", SourceKindLet, []string{"x"}, "x"),
		},
	}

	if src, issue := resolver.resolveNamedSource("same", item, WithResolveOptions{
		AllowParam:            true,
		AllowLet:              true,
		DetectAmbiguousSource: true,
	}); src != nil || issue == nil || issue.Kind != IssueAmbiguousSource {
		t.Fatalf("expected ambiguous-source issue, got src=%#v issue=%#v", src, issue)
	}
	if src, issue := resolver.resolveNamedSource("missing", item, WithResolveOptions{
		AllowParam: true,
		AllowLet:   true,
	}); src != nil || issue == nil || issue.Kind != IssueUnknownSource {
		t.Fatalf("expected unknown-source issue, got src=%#v issue=%#v", src, issue)
	}
	if src, issue := resolver.resolveNamedSource("p", item, WithResolveOptions{
		AllowParam: false,
		AllowLet:   true,
	}); src != nil || issue == nil || issue.Kind != IssueDisallowedKind {
		t.Fatalf("expected disallowed-kind issue for param source, got src=%#v issue=%#v", src, issue)
	}
	if src, issue := resolver.resolveNamedSource("l", item, WithResolveOptions{
		AllowParam: false,
		AllowLet:   true,
	}); src == nil || issue != nil || src.Name != "l" {
		t.Fatalf("expected let source to resolve, got src=%#v issue=%#v", src, issue)
	}

	if src, issue := resolver.resolveFallbackSource("same", item, WithResolveOptions{
		AllowParam:            true,
		AllowLet:              true,
		DetectAmbiguousSource: true,
	}); src != nil || issue == nil || issue.Kind != IssueAmbiguousSource {
		t.Fatalf("expected ambiguous issue in fallback resolve, got src=%#v issue=%#v", src, issue)
	}
	if src, issue := resolver.resolveFallbackSource("missing", item, WithResolveOptions{
		AllowParam: true,
		AllowLet:   true,
	}); src != nil || issue != nil {
		t.Fatalf("expected missing fallback source to return nil,nil, got src=%#v issue=%#v", src, issue)
	}
	if src, issue := resolver.resolveFallbackSource("p", item, WithResolveOptions{
		AllowParam: false,
		AllowLet:   true,
	}); src != nil || issue == nil || issue.Kind != IssueDisallowedKind {
		t.Fatalf("expected disallowed issue in fallback resolve, got src=%#v issue=%#v", src, issue)
	}
	if src, issue := resolver.resolveFallbackSource("l", item, WithResolveOptions{
		AllowParam: false,
		AllowLet:   true,
	}); src == nil || issue != nil || src.Name != "l" {
		t.Fatalf("expected let fallback source resolve, got src=%#v issue=%#v", src, issue)
	}
}

func TestSourceKindAllowedAndExpandFullSource(t *testing.T) {
	if !sourceKindAllowed(SourceKindParam, WithResolveOptions{AllowParam: true}) {
		t.Fatalf("expected param kind allowed")
	}
	if sourceKindAllowed(SourceKindParam, WithResolveOptions{AllowParam: false}) {
		t.Fatalf("expected param kind disallowed")
	}
	if !sourceKindAllowed(SourceKindLet, WithResolveOptions{AllowLet: true}) {
		t.Fatalf("expected let kind allowed")
	}
	if sourceKindAllowed(SourceKindLet, WithResolveOptions{AllowLet: false}) {
		t.Fatalf("expected let kind disallowed")
	}
	if sourceKindAllowed(SourceKind("unknown"), WithResolveOptions{AllowParam: true, AllowLet: true}) {
		t.Fatalf("expected unknown source kind disallowed")
	}

	src := mkImportSource("p", SourceKindParam, []string{"b", "a"}, "a", "b")
	item := ast.WithItem{
		Name:  "p",
		Alias: "alias_name",
		Span:  diag.NewSpan("in.jbs", diag.NewPos(0, 20, 1), diag.NewPos(1, 20, 2)),
	}
	got := expandFullSource(item, src)
	if !got.Full || got.Source != "p" || got.Kind != SourceKindParam || got.SourceExpr != "alias_name" {
		t.Fatalf("unexpected expanded full source metadata: %#v", got)
	}
	if len(got.Vars) != 2 || got.Vars[0].Visible != "b" || got.Vars[1].Visible != "a" {
		t.Fatalf("expected vars in source order [b,a], got %#v", got.Vars)
	}
}
