package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestImportRebindingIsLocal(t *testing.T) {
	src := `
param base {
  x = (1,2)
  x
}

param derived with x from base {
  x = x + 10
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	base := res.ParamByName["base"]
	derived := res.ParamByName["derived"]
	if got := base.Vars["x"][0].I; got != 1 {
		t.Fatalf("base.x mutated, first value=%d", got)
	}
	if got := derived.Vars["x"][0].I; got != 11 {
		t.Fatalf("unexpected derived.x first value=%d", got)
	}
}

func TestUnknownImportVariableError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do work with missing from p {
  echo hi
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)

	found := false
	for _, d := range diags.Items {
		if d.Code == "E021" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E021 for missing imported variable, got: %s", diags.String())
	}
}

func TestDependencyCycleError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
do a after b {
  echo a
}
do b after a {
  echo b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)

	found := false
	for _, d := range diags.Items {
		if d.Code == "E213" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E213 cycle error, got: %s", diags.String())
	}
}

func TestMixedWithVariableAndParamsetImport(t *testing.T) {
	src := `
param p1 {
  a = (1,2)
  a
}
param p2 {
  b = ("x","y")
  b
}
do work with a from p1, p2 {
  echo ${a} ${b}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected mixed with import to be valid, got: %s", diags.String())
	}
}

func TestUnknownTopLevelGlobalRejected(t *testing.T) {
	src := `
not_a_global = "x"
param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E300" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E300, got: %s", diags.String())
	}
}

func TestSpecialRootGlobalsValidation(t *testing.T) {
	src := `
jbs_name = python("abc")
jbs_outpath = 12
param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	has301 := false
	has302 := false
	has303 := false
	for _, d := range diags.Items {
		if d.Code == "E301" {
			has301 = true
		}
		if d.Code == "E302" {
			has302 = true
		}
		if d.Code == "E303" {
			has303 = true
		}
	}
	if !has303 || !has302 {
		t.Fatalf("expected E303 and E302, got: %s", diags.String())
	}
	if has301 {
		t.Fatalf("unexpected E301; jbs_name mode error should be E303")
	}
}

func TestGlobalScalarOnlyRule(t *testing.T) {
	src := `
jbs_outpath = (1,2)
param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E302" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E302, got: %s", diags.String())
	}
}

func TestSubmitUnknownKeyError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  not_allowed = "x"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E072" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E072, got: %s", diags.String())
	}
}

func TestSubmitPreprocessRequiresRawBlock(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  preprocess = "echo hi"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E073" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E073, got: %s", diags.String())
	}
}

func TestSubmitNonRawKeyRejectsRawBlock(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  queue = {
    devel
  }
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E074" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E074, got: %s", diags.String())
	}
}

func TestSubmitDuplicateKeyError(t *testing.T) {
	src := `
param p {
  a = 1
  a
}
submit run with p {
  queue = "q1"
  queue = "q2"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E075" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E075, got: %s", diags.String())
	}
}
