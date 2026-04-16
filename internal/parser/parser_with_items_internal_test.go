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
