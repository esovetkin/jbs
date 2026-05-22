package parser

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestParseWithItemsEarlyBreakOnInvalidStart(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(")", diags)

	items := p.parseWithItems()
	if len(items) != 0 {
		t.Fatalf("expected no with items for invalid start token, got %#v", items)
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected E023 for missing source in with clause, got: %s", diags.String())
	}
}

func TestParseWithItemsCanonicalSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`cases["id","label"], env`, diags)
	items := p.parseWithItems()
	if len(items) != 2 {
		t.Fatalf("expected two with items, got %#v", items)
	}
	assertWithIndexStringColumns(t, items[0], "cases", []string{"id", "label"})
	assertWithIdent(t, items[1], "env")
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseWithItemsRejectsUnsupportedSyntax(t *testing.T) {
	tests := []string{
		"p[",
		"x +",
		"(x, y",
	}

	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			p := newTopLevelParser(src, diags)
			items := p.parseWithItems()
			if len(items) != 0 {
				t.Fatalf("expected unsupported with syntax to produce no items, got %#v", items)
			}
			if !hasDiag(diags, "E023") {
				t.Fatalf("expected E023, got: %s", diags.String())
			}
			got := diags.String()
			if strings.Contains(got, "rewrite") || strings.Contains(got, "old with") || strings.Contains(got, "aliasing") {
				t.Fatalf("expected generic invalid syntax diagnostic, got: %s", got)
			}
		})
	}
}

func TestParseWithItemsExpressionSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`p["a,b", sel], q with r`, diags)
	items := p.parseWithItems()
	if len(items) != 2 {
		t.Fatalf("expected two comma-separated with items, got %#v", items)
	}
	assertWithIndexMixedColumns(t, items[0], "p", []string{"a,b", "sel"})
	assertWithIdent(t, items[1], "q")
	word, ok := p.peekWord()
	if !ok || word != "with" {
		t.Fatalf("expected parser to stop before next with clause, got word=%q ok=%v", word, ok)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestParseWithItemsAliases(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`x as y, cases["very_long_column"] as short with z as q`, diags)
	items := p.parseWithItems()
	if len(items) != 2 {
		t.Fatalf("expected two comma-separated with items, got %#v", items)
	}
	assertWithIdent(t, items[0], "x")
	if items[0].Alias != "y" {
		t.Fatalf("expected alias y, got %#v", items[0])
	}
	assertWithIndexStringColumns(t, items[1], "cases", []string{"very_long_column"})
	if items[1].Alias != "short" {
		t.Fatalf("expected alias short, got %#v", items[1])
	}
	word, ok := p.peekWord()
	if !ok || word != "with" {
		t.Fatalf("expected parser to stop before next with clause, got word=%q ok=%v", word, ok)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestParseWithItemsAliasIgnoresAsInsideSelector(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser(`cases["as"] as alias`, diags)
	items := p.parseWithItems()
	if len(items) != 1 {
		t.Fatalf("expected one with item, got %#v", items)
	}
	assertWithIndexStringColumns(t, items[0], "cases", []string{"as"})
	if items[0].Alias != "alias" {
		t.Fatalf("expected alias, got %#v", items[0])
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestParseWithItemsRejectsMalformedAliases(t *testing.T) {
	tests := []string{
		"x as y z",
		`x as "y"`,
		"x as 1x",
	}
	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			p := newTopLevelParser(src, diags)
			items := p.parseWithItems()
			if len(items) != 0 {
				t.Fatalf("expected malformed alias to produce no items, got %#v", items)
			}
			if !hasDiag(diags, "E023") {
				t.Fatalf("expected E023 for malformed alias, got: %s", diags.String())
			}
		})
	}
}

func assertWithIdent(t *testing.T, item ast.WithItem, name string) {
	t.Helper()
	ident, ok := item.Expr.(ast.IdentExpr)
	if !ok || ident.Name != name {
		t.Fatalf("expected with ident %q, got %#v", name, item.Expr)
	}
}

func assertWithIndexStringColumns(t *testing.T, item ast.WithItem, source string, selectors []string) {
	t.Helper()
	assertWithIndexMixedColumns(t, item, source, selectors)
	for i, expr := range item.Expr.(ast.IndexExpr).Items {
		str, ok := expr.(ast.StringExpr)
		if !ok || str.Value != selectors[i] {
			t.Fatalf("expected string selector %q, got %#v", selectors[i], expr)
		}
	}
}

func assertWithIndexMixedColumns(t *testing.T, item ast.WithItem, source string, selectors []string) {
	t.Helper()
	idx, ok := item.Expr.(ast.IndexExpr)
	if !ok {
		t.Fatalf("expected index expression, got %#v", item.Expr)
	}
	base, ok := idx.Base.(ast.IdentExpr)
	if !ok || base.Name != source {
		t.Fatalf("expected index base %q, got %#v", source, idx.Base)
	}
	if len(idx.Items) != len(selectors) {
		t.Fatalf("expected %d selectors, got %#v", len(selectors), idx.Items)
	}
	for i, expr := range idx.Items {
		switch e := expr.(type) {
		case ast.StringExpr:
			if e.Value != selectors[i] {
				t.Fatalf("expected selector %q, got %#v", selectors[i], expr)
			}
		case ast.IdentExpr:
			if e.Name != selectors[i] {
				t.Fatalf("expected selector %q, got %#v", selectors[i], expr)
			}
		default:
			t.Fatalf("unexpected selector expression: %#v", expr)
		}
	}
}
