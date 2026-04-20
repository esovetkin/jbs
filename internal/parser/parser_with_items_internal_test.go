package parser

import (
	"strings"
	"testing"

	"jbs/internal/diag"
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

func TestParseWithNamesTupleErrorBranches(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantOK    bool
		wantNames []string
	}{
		{name: "empty tuple", src: "()", wantOK: false},
		{name: "trailing comma", src: "(a,)", wantOK: true, wantNames: []string{"a"}},
		{name: "malformed second element", src: "(a,1)", wantOK: true, wantNames: []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			p := newTopLevelParser(tt.src, diags)
			names, _, ok := p.parseWithNames()
			if ok != tt.wantOK {
				t.Fatalf("parseWithNames(%q) ok=%v, want %v", tt.src, ok, tt.wantOK)
			}
			if len(names) != len(tt.wantNames) {
				t.Fatalf("parseWithNames(%q) names=%#v, want %v", tt.src, names, tt.wantNames)
			}
			for i := range names {
				if names[i].Name != tt.wantNames[i] {
					t.Fatalf("name %d=%q, want %q", i, names[i].Name, tt.wantNames[i])
				}
			}
			if !hasDiag(diags, "E023") {
				t.Fatalf("expected E023 for %q, got: %s", tt.src, diags.String())
			}
		})
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

func TestParseWithItemsLegacyFromMigration(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("x from p", diags)
	items := p.parseWithItems()
	if len(items) != 1 {
		t.Fatalf("expected one recovered with item, got %#v", items)
	}
	if items[0].Source != "p" || len(items[0].Selectors) != 1 || items[0].Selectors[0] != "x" {
		t.Fatalf("unexpected recovered legacy item: %#v", items[0])
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected E023 migration diagnostic, got: %s", diags.String())
	}
	if got := diags.String(); !strings.Contains(got, "rewrite `with x from p` as `with p[x]`") {
		t.Fatalf("expected targeted rewrite hint, got: %s", got)
	}
}

func TestParseWithItemsLegacyGroupedMigration(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("(x,y) in p", diags)
	items := p.parseWithItems()
	if len(items) != 1 {
		t.Fatalf("expected one recovered grouped item, got %#v", items)
	}
	if items[0].Source != "p" || len(items[0].Selectors) != 2 || items[0].Selectors[0] != "x" || items[0].Selectors[1] != "y" {
		t.Fatalf("unexpected grouped legacy recovery: %#v", items[0])
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected E023 grouped migration diagnostic, got: %s", diags.String())
	}
	if got := diags.String(); !strings.Contains(got, "rewrite `with (x, y) in p` as `with p[x, y]`") {
		t.Fatalf("expected grouped rewrite hint, got: %s", got)
	}
}

func TestParseWithItemsAliasRejected(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("p[x] as pair", diags)
	items := p.parseWithItems()
	if len(items) != 1 {
		t.Fatalf("expected one with item, got %#v", items)
	}
	if items[0].Source != "p" || len(items[0].Selectors) != 1 || items[0].Selectors[0] != "x" {
		t.Fatalf("unexpected canonical item under alias rejection: %#v", items[0])
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected E023 alias rejection diagnostic, got: %s", diags.String())
	}
	if got := diags.String(); !strings.Contains(got, "with-clause aliasing is no longer supported") {
		t.Fatalf("expected targeted alias rejection, got: %s", got)
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

func TestParseWithNamesNonTupleAndUnterminatedTuple(t *testing.T) {
	t.Run("non-tuple identifier", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("alpha", diags)
		names, _, ok := p.parseWithNames()
		if !ok || len(names) != 1 || names[0].Name != "alpha" {
			t.Fatalf("unexpected parseWithNames result: ok=%v names=%#v", ok, names)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected parse errors: %s", diags.String())
		}
	})

	t.Run("non-tuple invalid token", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser(")", diags)
		names, _, ok := p.parseWithNames()
		if ok || names != nil {
			t.Fatalf("expected parse failure for non-tuple invalid token, got ok=%v names=%#v", ok, names)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023, got: %s", diags.String())
		}
	})

	t.Run("unterminated tuple", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("(a b", diags)
		names, _, ok := p.parseWithNames()
		if !ok || len(names) != 1 || names[0].Name != "a" {
			t.Fatalf("unexpected parseWithNames recovery: ok=%v names=%#v", ok, names)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023 for unterminated tuple, got: %s", diags.String())
		}
	})
}
