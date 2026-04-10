package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestAnalyseUnknownStepError(t *testing.T) {
	src := `
let p {
  a = "A: %d"
}
analyse missing_step {
  p0 = p.a in "stdout"
  (p0)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E410" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E410, got: %s", diags.String())
	}
}

func TestAnalyseUnknownNamespaceAndMember(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
let g {
  one = "A: %d"
}
analyse run {
  p0 = missing.one in "stdout"
  p1 = g.missing in "stdout"
  (a, p0, p1)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has100 := false
	for _, d := range diags.Items {
		if d.Code == "E100" {
			has100 = true
		}
	}
	if !has100 {
		t.Fatalf("expected E100, got: %s", diags.String())
	}
}

func TestAnalyseAssignmentCollisionAndDuplicate(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
let g {
  one = "A: %d"
}
analyse run with g {
  a = one in "stdout"
  p0 = one in "stdout"
  p0 = one in "stdout"
  (a, p0)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has413 := false
	has414 := false
	for _, d := range diags.Items {
		if d.Code == "E413" {
			has413 = true
		}
		if d.Code == "E414" {
			has414 = true
		}
	}
	if !has413 || !has414 {
		t.Fatalf("expected E413 and E414, got: %s", diags.String())
	}
}

func TestAnalyseUnknownTupleSymbol(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
let g {
  one = "A: %d"
}
analyse run with g {
  p0 = one in "stdout"
  (a, p0, unknown)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E415" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E415, got: %s", diags.String())
	}
}

func TestPatternPlaceholderNormalizationAndTypeInference(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  i = "Count: %d" in "stdout"
  f = "Time: %f" in "stdout"
  w = "Word: %w" in "stdout"
  (a, i, f, w)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(res.Analyse) != 1 {
		t.Fatalf("expected one analyse spec, got %d", len(res.Analyse))
	}
	var i, f, w *sema.AnalyseAssignmentSpec
	for idx := range res.Analyse[0].Assignments {
		asn := &res.Analyse[0].Assignments[idx]
		switch asn.Name {
		case "i":
			i = asn
		case "f":
			f = asn
		case "w":
			w = asn
		}
	}
	if i == nil || f == nil || w == nil {
		t.Fatalf("expected analyse extraction assignments i/f/w, got %#v", res.Analyse[0].Assignments)
	}
	if i.Template.Regex != "Count: $jube_pat_int" || i.Template.Type != "int" {
		t.Fatalf("unexpected int pattern normalization: %#v", i.Template)
	}
	if f.Template.Regex != "Time: $jube_pat_fp" || f.Template.Type != "float" {
		t.Fatalf("unexpected float pattern normalization: %#v", f.Template)
	}
	if w.Template.Regex != "Word: $jube_pat_wrd" || w.Template.Type != "string" {
		t.Fatalf("unexpected word pattern normalization: %#v", w.Template)
	}
}

func TestPatternRejectsPercentS(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  bad = "Letter: %s" in "stdout"
  (a, bad)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E402" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E402 for %%s placeholder, got: %s", diags.String())
	}
}

func TestAnalyseWithParamRejected(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run with p {
  x = "A: %d" in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E420" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E420 for analyse with param import, got: %s", diags.String())
	}
}

func TestAnalyseWithLetImportRequiresString(t *testing.T) {
	src := `
let l {
  p = 3
  s = "A: %d"
}
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run with l {
  x = s in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E422" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E422 for non-string let import in analyse, got: %s", diags.String())
	}
}

func TestAnalyseWithSelectedNonStringLetImportRejected(t *testing.T) {
	src := `
let l {
  p = 3
  s = "A: %d"
}
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run with p from l {
  x = "A: %d" in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E422" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E422 for selected non-string let import in analyse, got: %s", diags.String())
	}
}

func TestNestedTupleTransitiveAcrossBlocksRejected(t *testing.T) {
	src := `
let l {
  t = (1,2)
}
param p {
  x = (l.t, 3)
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E403" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E403 for let tuple rejection, got: %s", diags.String())
	}
}

func TestLetImplicitCrossNamespaceLookupRejected(t *testing.T) {
	src := `
let a {
  x = "A"
}
let b {
  x = "B"
}
let c {
  y = x
}
do s with c {
  echo ${y}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E100") {
		t.Fatalf("expected E100 for implicit cross-namespace let lookup, got: %s", diags.String())
	}
}

func TestLetLocalSequentialLookupStillWorks(t *testing.T) {
	src := `
let c {
  x = "B"
  y = x
}
do s with c {
  echo ${y}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if hasDiagCode(diags, "E100") {
		t.Fatalf("did not expect E100 for local sequential let lookup, got: %s", diags.String())
	}
}

func TestLetImplicitCrossNamespaceLookupDeterministic(t *testing.T) {
	src := `
let a { x = "A" }
let b { x = "B" }
let c { y = x }
do s with c { echo ${y} }
`
	for i := 0; i < 50; i++ {
		diags := &diag.Diagnostics{}
		prog := parser.Parse("in.jbs", src, diags)
		_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
		if got := diagCount(diags, "E100"); got == 0 {
			t.Fatalf("iteration %d: expected E100 for implicit let lookup, got: %s", i, diags.String())
		}
	}
}

func TestAnalyseLocalHelperCollisionWarning(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  a = "Number: %d"
  x = a in "stdout"
  (a, x)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	hasW320 := false
	for _, d := range diags.Items {
		if d.Code == "W320" {
			hasW320 = true
		}
	}
	if !hasW320 {
		t.Fatalf("expected W320 for analyse helper collision, got: %s", diags.String())
	}
}
