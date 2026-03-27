package lower_test

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func compileDoc(t *testing.T, src string) (lower.Document, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	doc := lower.ToJUBEYAML(res, lower.Options{InputPath: "in.jbs"}, diags)
	return doc, diags
}

func TestPureOuterProductLowering(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a * b
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.ParameterSet) != 1 {
		t.Fatalf("expected one parameterset")
	}
	ps := doc.ParameterSet[0]
	if ps.Name != "p" {
		t.Fatalf("unexpected parameterset name: %s", ps.Name)
	}
	if len(ps.Parameter) != 2 {
		t.Fatalf("expected 2 params, got %d", len(ps.Parameter))
	}
	if ps.Parameter[0].Separator != lower.ReservedSeparator || ps.Parameter[1].Separator != lower.ReservedSeparator {
		t.Fatalf("expected reserved separator in both template params")
	}
}

func TestGroupedDirectSumLowering(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y","z")
  a + b
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	ps := doc.ParameterSet[0]
	if ps.Parameter[0].Name != "i" {
		t.Fatalf("expected grouped index parameter i, got %s", ps.Parameter[0].Name)
	}
	if ps.Parameter[0].Value != "0,1,2" {
		t.Fatalf("expected grouped indices 0,1,2 got %#v", ps.Parameter[0].Value)
	}
	if ps.Parameter[1].Mode != "python" || ps.Parameter[2].Mode != "python" {
		t.Fatalf("expected python-indexed grouped payload parameters")
	}
}

func TestReservedSeparatorError(t *testing.T) {
	src := `
param p {
  a = ("ok","bad####value")
  a
}
`
	_, diags := compileDoc(t, src)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E053" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected E053, got: %s", diags.String())
	}
}

func TestSubmitAndSubsetLowering(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}

do prep with a from p {
  echo prep
}

submit run after prep with p {
  export X=1
} {
  python main.py
}
`
	doc, diags := compileDoc(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(doc.Step) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(doc.Step))
	}
	if len(doc.ParameterSet) < 3 {
		t.Fatalf("expected main + subset + submit parameter sets")
	}

	var hasSubset bool
	var hasSubmitSet bool
	for _, ps := range doc.ParameterSet {
		if strings.HasPrefix(ps.Name, "__subset_") {
			hasSubset = true
		}
		if strings.HasSuffix(ps.Name, "__submit_params") {
			hasSubmitSet = true
		}
	}
	if !hasSubset {
		t.Fatalf("expected synthetic subset parameterset")
	}
	if !hasSubmitSet {
		t.Fatalf("expected synthetic submit parameterset")
	}
}
