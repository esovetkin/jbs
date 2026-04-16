package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestStepImportProjectionImportsFromStepPlan(t *testing.T) {
	if got := importsFromStepPlan(nil); len(got) != 0 {
		t.Fatalf("expected empty imports for nil plan, got %#v", got)
	}

	span := diag.NewSpan("plan.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	plan := &StepImportPlan{
		Effective: map[string]VarOrigin{
			"a": {
				Name:      "a",
				SourceVar: "",
				Paramset:  "p",
				Kind:      SourceKindParam,
				Span:      span,
			},
			"b": {
				Name:      "b",
				SourceVar: "orig_b",
				Paramset:  "l",
				Kind:      SourceKindLet,
				Span:      span,
			},
		},
	}

	got := importsFromStepPlan(plan)
	if len(got) != 2 {
		t.Fatalf("expected 2 imported names, got %#v", got)
	}
	if len(got["a"]) != 1 || got["a"][0].SourceVar != "a" || got["a"][0].Paramset != "p" || got["a"][0].Kind != SourceKindParam {
		t.Fatalf("unexpected projected origin for a: %#v", got["a"])
	}
	if len(got["b"]) != 1 || got["b"][0].SourceVar != "orig_b" || got["b"][0].Paramset != "l" || got["b"][0].Kind != SourceKindLet {
		t.Fatalf("unexpected projected origin for b: %#v", got["b"])
	}
}

func TestStepImportProjectionExplicitImportsFromStepPlan(t *testing.T) {
	if got := explicitImportsFromStepPlan(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty explicit imports for nil plan, got %#v", got)
	}

	span := diag.NewSpan("explicit.jbs", diag.NewPos(0, 2, 1), diag.NewPos(1, 2, 2))
	p := &ImportSource{
		Name:  "p",
		Kind:  SourceKindParam,
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}, "b": {eval.Int(2)}},
		Order: []string{"a", "b"},
	}
	l := &ImportSource{
		Name: "l",
		Kind: SourceKindLet,
		Vars: map[string][]eval.Value{"s": {eval.String("v")}},
	}
	sources := map[string]*ImportSource{
		"p": p,
		"l": l,
	}

	plan := &StepImportPlan{
		ExplicitDelta: []PlannedImport{
			{Source: "p", Full: true, Span: span},                             // full + kind fallback from source
			{Source: "missing", Full: true, Span: span},                       // full + missing source skipped
			{Source: "l", Visible: "v", SourceVar: "", Span: span},            // non-full + source-var fallback + kind from source
			{Source: "unknown", Visible: "w", SourceVar: "origw", Span: span}, // non-full + unknown source kind stays empty
		},
	}

	got := explicitImportsFromStepPlan(plan, sources)

	if len(got["a"]) != 1 || got["a"][0].Paramset != "p" || got["a"][0].SourceVar != "a" || got["a"][0].Kind != SourceKindParam {
		t.Fatalf("unexpected full-import projection for a: %#v", got["a"])
	}
	if len(got["b"]) != 1 || got["b"][0].Paramset != "p" || got["b"][0].SourceVar != "b" || got["b"][0].Kind != SourceKindParam {
		t.Fatalf("unexpected full-import projection for b: %#v", got["b"])
	}
	if len(got["v"]) != 1 || got["v"][0].SourceVar != "v" || got["v"][0].Kind != SourceKindLet {
		t.Fatalf("unexpected non-full projection with source fallback for v: %#v", got["v"])
	}
	if len(got["w"]) != 1 || got["w"][0].SourceVar != "origw" || got["w"][0].Kind != "" {
		t.Fatalf("unexpected non-full projection for unknown source w: %#v", got["w"])
	}
}

func TestStepImportProjectionResolveImportedVars(t *testing.T) {
	span := diag.NewSpan("with.jbs", diag.NewPos(0, 3, 1), diag.NewPos(1, 3, 2))
	p := &ImportSource{
		Name:  "p",
		Kind:  SourceKindParam,
		Vars:  map[string][]eval.Value{"a": {eval.Int(1)}, "b": {eval.Int(2)}},
		Order: []string{"a", "b"},
	}
	q := &ImportSource{
		Name:  "q",
		Kind:  SourceKindParam,
		Vars:  map[string][]eval.Value{"x": {eval.Int(3)}},
		Order: []string{"x"},
	}
	l := &ImportSource{
		Name: "l",
		Kind: SourceKindLet,
		Vars: map[string][]eval.Value{"s": {eval.String("v")}},
	}
	sources := map[string]*ImportSource{
		"p": p,
		"q": q,
		"l": l,
	}

	items := []ast.WithItem{
		{SourceExpr: "p", SourceSlice: []string{"a", "missing", "a"}, Span: span}, // source-slice known+unknown+duplicate
		{SourceExpr: "missing", SourceSlice: []string{"a"}, Span: span},           // source-slice unknown source
		{Name: "p", Span: span},                           // full source
		{Name: "missing_full", Span: span},                // full source unknown
		{Name: "a", From: "p", Alias: "aa", Span: span},   // variable import alias
		{Name: "a", From: "p", Alias: "aa", Span: span},   // duplicate alias import (dedup)
		{Name: "a", From: "missing_src", Span: span},      // unknown from source
		{Name: "q", From: "p", Span: span},                // fallback to source q (full)
		{Name: "missing_fallback", From: "p", Span: span}, // missing fallback
	}

	got := resolveImportedVars(items, sources)

	if len(got["a"]) != 1 {
		t.Fatalf("expected single deduplicated source-slice/full entry for a, got %#v", got["a"])
	}
	if got["a"][0].Paramset != "p" || got["a"][0].SourceVar != "a" {
		t.Fatalf("unexpected projection for a: %#v", got["a"])
	}
	if len(got["b"]) != 1 || got["b"][0].Paramset != "p" || got["b"][0].SourceVar != "b" {
		t.Fatalf("unexpected projection for b: %#v", got["b"])
	}
	if len(got["aa"]) != 1 || got["aa"][0].Paramset != "p" || got["aa"][0].SourceVar != "a" {
		t.Fatalf("unexpected alias projection for aa: %#v", got["aa"])
	}
	if len(got["x"]) != 1 || got["x"][0].Paramset != "q" || got["x"][0].SourceVar != "x" {
		t.Fatalf("unexpected fallback projection for x: %#v", got["x"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatalf("did not expect unknown source-slice var to be present: %#v", got)
	}
	if _, ok := got["missing_fallback"]; ok {
		t.Fatalf("did not expect missing fallback var to be present: %#v", got)
	}
	if _, ok := got["s"]; ok {
		t.Fatalf("did not expect unrelated source entries, got %#v", got)
	}
}
