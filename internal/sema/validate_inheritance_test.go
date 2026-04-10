package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestAfterInheritancePrunesExplicitDelta(t *testing.T) {
	src := `
param pm0 {
  a = (1,2)
  b = ("x","y")
  c = (true,false)
  a * b * c
}
do step0 with (a,b) from pm0 {
  echo ${a} ${b}
}
do step1 after step0 with (b,c) from pm0 {
  echo ${a} ${b} ${c}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected valid inheritance pruning, got: %s", diags.String())
	}
	plan := res.StepImportByName["step1"]
	if plan == nil {
		t.Fatalf("missing step import plan for step1")
	}
	if len(plan.ExplicitDelta) != 1 {
		t.Fatalf("expected one explicit delta item (c), got %#v", plan.ExplicitDelta)
	}
	if plan.ExplicitDelta[0].Visible != "c" || plan.ExplicitDelta[0].Source != "pm0" || plan.ExplicitDelta[0].SourceVar != "c" {
		t.Fatalf("unexpected explicit delta for step1: %#v", plan.ExplicitDelta[0])
	}
	for _, name := range []string{"a", "b", "c"} {
		origin, ok := plan.Effective[name]
		if !ok {
			t.Fatalf("missing effective inherited/imported variable %q in step1 plan", name)
		}
		if origin.Paramset != "pm0" {
			t.Fatalf("expected %q from pm0, got %#v", name, origin)
		}
	}
}

func TestAfterInheritanceConflictAcrossDependencies(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
do a with x from p1 {
  echo ${x}
}
do b with x from p2 {
  echo ${x}
}
do c after a,b {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E214" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E214 for conflicting inherited dependencies, got: %s", diags.String())
	}
}

func TestAfterInheritanceConflictWithExplicitImport(t *testing.T) {
	src := `
param p1 {
  x = (1,2)
  x
}
param p2 {
  x = ("a","b")
  x
}
do a with x from p1 {
  echo ${x}
}
do c after a with x from p2 {
  echo ${x}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	found := false
	for _, d := range diags.Items {
		if d.Code == "E214" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E214 for explicit import conflicting with inherited variable source, got: %s", diags.String())
	}
}

func TestSubmitAfterInheritsVarsForExpressions(t *testing.T) {
	src := `
param p {
  n = (1,2)
  n
}
do prep with n from p {
  echo ${n}
}
submit run after prep {
  nodes = n
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("expected submit expression to resolve inherited variable n, got: %s", diags.String())
	}
}
