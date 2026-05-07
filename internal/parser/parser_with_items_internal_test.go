package parser

import (
	"strings"
	"testing"

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

func TestParseQualifiedNameErrorBranches(t *testing.T) {
	t.Run("missing head identifier", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser(".", diags)
		name, _ := p.parseQualifiedName("E023", "expected identifier in with clause")
		if name != "" {
			t.Fatalf("expected empty qualified name, got %q", name)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023 for missing qualified-name head, got: %s", diags.String())
		}
	})

	t.Run("missing tail identifier after dot", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("ns.", diags)
		name, _ := p.parseQualifiedName("E023", "expected identifier in with clause")
		if name != "ns" {
			t.Fatalf("expected partial qualified name 'ns', got %q", name)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023 for missing qualified-name tail, got: %s", diags.String())
		}
	})
}

func TestParseWithItemsCanonicalSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("cases[id,label], env", diags)
	items := p.parseWithItems()
	if len(items) != 2 {
		t.Fatalf("expected two with items, got %#v", items)
	}
	if items[0].Source != "cases" || len(items[0].Selectors) != 2 || items[0].Selectors[0] != "id" || items[0].Selectors[1] != "label" {
		t.Fatalf("unexpected projection item: %#v", items[0])
	}
	if items[1].Source != "env" || len(items[1].Selectors) != 0 {
		t.Fatalf("unexpected full-source item: %#v", items[1])
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseWithItemsSliceFailureStopsItemParsing(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("p[]", diags)
	items := p.parseWithItems()
	if len(items) != 0 {
		t.Fatalf("expected no items when slice parsing fails, got %#v", items)
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected E023 for empty selector list, got: %s", diags.String())
	}
}

func TestParseWithSliceNamesErrorBranches(t *testing.T) {
	t.Run("missing opening bracket", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("x", diags)
		names, _, ok := p.parseWithSliceNames()
		if ok || names != nil {
			t.Fatalf("expected parse failure for missing '[', got ok=%v names=%v", ok, names)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023, got: %s", diags.String())
		}
	})

	t.Run("invalid identifier in selector", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("[1]", diags)
		names, _, ok := p.parseWithSliceNames()
		if ok || names != nil {
			t.Fatalf("expected parse failure for invalid selector name, got ok=%v names=%v", ok, names)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023, got: %s", diags.String())
		}
	})

	t.Run("unterminated selector list", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("[x y]", diags)
		names, _, ok := p.parseWithSliceNames()
		if ok || names != nil {
			t.Fatalf("expected parse failure for unterminated selector list, got ok=%v names=%v", ok, names)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023, got: %s", diags.String())
		}
	})
}

func TestParseWithItemsRejectsUnsupportedSyntax(t *testing.T) {
	tests := []string{
		"x from p",
		"x in p",
		"p[x] as pair",
		"(x, y) from p",
		"(x, y) in p",
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
