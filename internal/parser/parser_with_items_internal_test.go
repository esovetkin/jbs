package parser

import (
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
		t.Fatalf("expected E023 for missing identifier in with clause, got: %s", diags.String())
	}
}

func TestParseWithNamesTupleErrorBranches(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantOK    bool
		wantNames []string
	}{
		{
			name:      "empty tuple",
			src:       "()",
			wantOK:    false,
			wantNames: nil,
		},
		{
			name:      "trailing comma",
			src:       "(a,)",
			wantOK:    true,
			wantNames: []string{"a"},
		},
		{
			name:      "malformed second element",
			src:       "(a,1)",
			wantOK:    true,
			wantNames: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			p := newTopLevelParser(tt.src, diags)
			names, ok := p.parseWithNames()
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

func TestParseWithItemsSourceSliceSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("p[x,y] as a", diags)
	items := p.parseWithItems()
	if len(items) != 1 {
		t.Fatalf("expected one with item, got %#v", items)
	}
	it := items[0]
	if it.SourceExpr != "p" {
		t.Fatalf("expected source expr p, got %#v", it)
	}
	if len(it.SourceSlice) != 2 || it.SourceSlice[0] != "x" || it.SourceSlice[1] != "y" {
		t.Fatalf("expected source slice [x,y], got %#v", it)
	}
	if it.CombAlias != "a" {
		t.Fatalf("expected comb alias a, got %#v", it)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseWithItemsTupleInSyntax(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("(x,y) in p", diags)
	items := p.parseWithItems()
	if len(items) != 2 {
		t.Fatalf("expected two with items, got %#v", items)
	}
	if items[0].Name != "x" || items[0].From != "p" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Name != "y" || items[1].From != "p" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseWithItemsCurrentFromAndAliases(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("x from p, (y,z), q as qa, r", diags)
	items := p.parseWithItems()
	if len(items) != 5 {
		t.Fatalf("expected five with items, got %#v", items)
	}
	if items[0].Name != "x" || items[0].From != "p" || items[0].Alias != "" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Name != "y" || items[1].From != "p" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
	if items[2].Name != "z" || items[2].From != "p" {
		t.Fatalf("unexpected third item: %#v", items[2])
	}
	if items[3].Name != "q" || items[3].From != "" || items[3].Alias != "qa" {
		t.Fatalf("unexpected fourth item: %#v", items[3])
	}
	if items[4].Name != "r" || items[4].From != "p" {
		t.Fatalf("unexpected fifth item: %#v", items[4])
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}

func TestParseWithItemsTupleAliasRejected(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("(x,y) from p as pair", diags)
	items := p.parseWithItems()
	if len(items) != 2 {
		t.Fatalf("expected two tuple-expanded with items, got %#v", items)
	}
	if items[0].Name != "x" || items[0].From != "p" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Name != "y" || items[1].From != "p" {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
	if !hasDiag(diags, "E023") {
		t.Fatalf("expected E023 for tuple alias, got: %s", diags.String())
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
		t.Fatalf("expected E023 for empty with-slice selector list, got: %s", diags.String())
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
		names, ok := p.parseWithNames()
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
		names, ok := p.parseWithNames()
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
		names, ok := p.parseWithNames()
		if !ok || len(names) != 1 || names[0].Name != "a" {
			t.Fatalf("unexpected parseWithNames result: ok=%v names=%#v", ok, names)
		}
		if !hasDiag(diags, "E023") {
			t.Fatalf("expected E023 for unterminated tuple, got: %s", diags.String())
		}
	})
}

func TestParseQualifiedNameMultipleSegments(t *testing.T) {
	diags := &diag.Diagnostics{}
	p := newTopLevelParser("a.b.c", diags)
	name, _ := p.parseQualifiedName("E023", "expected identifier in with clause")
	if name != "a.b.c" {
		t.Fatalf("expected qualified name a.b.c, got %q", name)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
}
