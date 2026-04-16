package sema_test

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestKernelRangeRevAllowedInParamAssignments(t *testing.T) {
	src := `
param p {
  a = range(5)
  b = rev(a)
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("range_rev_param.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	a := p.Vars["a"]
	b := p.Vars["b"]
	if len(a) != 5 || len(b) != 5 {
		t.Fatalf("unexpected series lengths: len(a)=%d len(b)=%d", len(a), len(b))
	}
	for i := 0; i < 5; i++ {
		if a[i].I != int64(i) {
			t.Fatalf("unexpected a[%d]=%d", i, a[i].I)
		}
		if b[i].I != int64(4-i) {
			t.Fatalf("unexpected b[%d]=%d", i, b[i].I)
		}
	}
}

func TestKernelRangeRevRejectedOutsideParamAssignments(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "let assignment",
			src: `
let l {
  x = range(3)
}
`,
		},
		{
			name: "submit field",
			src: `
param p {
  a = 1
  a
}
submit run with p {
  account = "acct"
  queue = "batch"
  executable = "/bin/bash"
  args_exec = rev([1,2,3])
}
`,
		},
		{
			name: "analyse helper assignment",
			src: `
param p {
  a = 1
  a
}
do run with p {
  echo ok
}
analyse run {
  x = range(3)
  (a, x)
}
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			prog := parser.Parse("range_rev_outside_param.jbs", tc.src, diags)
			_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
			if !hasDiagCode(diags, "E199") {
				t.Fatalf("expected E199, got: %s", diags.String())
			}
		})
	}
}

func TestKernelCallDependenciesContributeToFinalExpression(t *testing.T) {
	src := `
param p {
  x = 3
  y = range(x)
  y
}
	`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("range_dep.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if hasUnusedWarningForVar(diags, "x") {
		t.Fatalf("did not expect W312 for x used through range(x), got: %s", diags.String())
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
}

func TestCombAssignmentCanContributeViaFinalCombIdentifier(t *testing.T) {
	src := `
param p {
  x = (1,2)
  y = (3,4)
  a = comb(x * y)
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("comb_assign_used.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if hasUnusedWarningForVar(diags, "a") {
		t.Fatalf("did not expect W312 for comb variable a used in final expression, got: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	if len(p.Rows) != 4 {
		t.Fatalf("expected 4 rows for comb(x*y), got %d", len(p.Rows))
	}
	if len(p.Vars["x"]) != 4 || len(p.Vars["y"]) != 4 {
		t.Fatalf("expected expanded x/y series from comb rows, got len(x)=%d len(y)=%d", len(p.Vars["x"]), len(p.Vars["y"]))
	}
}

func TestCombAssignmentMultiplyKeepsNewIdentifierColumn(t *testing.T) {
	src := `
param p {
  x = (1,2)
  y = (3,4)
  a = comb(x * y)
  z = range(2)
  a *= z
  a
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("comb_assign_mul_ident.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	if len(p.Rows) != 8 {
		t.Fatalf("expected 8 rows for (x*y)*z, got %d", len(p.Rows))
	}
	if len(p.Vars["z"]) != 8 {
		t.Fatalf("expected z to be exposed across rows, got len(z)=%d", len(p.Vars["z"]))
	}
}

func TestFinalExpressionRejectsUnnamedCallLeaf(t *testing.T) {
	src := `
param p {
  x = (1,2)
  y = (3,4)
  a = comb(x * y)
  a * range(2)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("comb_final_call_diag.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E106") {
		t.Fatalf("expected E106 for unnamed call leaf in final expression, got: %s", diags.String())
	}
	if hasDiagCode(diags, "E111") {
		t.Fatalf("did not expect cascaded E111, got: %s", diags.String())
	}
}

func TestFinalCombCallExpressionIsAccepted(t *testing.T) {
	src := `
param p {
  x = (1,2)
  comb(x*x as b)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("comb_final_call_ok.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if hasUnusedWarningForVar(diags, "x") {
		t.Fatalf("did not expect W312 for x used in final comb call, got: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	if len(p.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(p.Rows))
	}
	if len(p.Vars["x"]) != 4 {
		t.Fatalf("expected 4 values for x, got %d", len(p.Vars["x"]))
	}
	if len(p.Vars["b"]) != 4 {
		t.Fatalf("expected 4 values for b, got %d", len(p.Vars["b"]))
	}
}

func TestCombCallAliasForms(t *testing.T) {
	srcOK := `
param p {
  x = (1,2)
  y = (3,4)
  a = comb(x + y)
  b = comb(a + x as z)
  b
}
`
	diagsOK := &diag.Diagnostics{}
	progOK := parser.Parse("comb_alias_ok.jbs", srcOK, diagsOK)
	resOK := sema.Analyze(progOK, lower.BuiltinGlobalValues(), diagsOK)
	if diagsOK.HasErrors() {
		t.Fatalf("unexpected errors for alias success case: %s", diagsOK.String())
	}
	p := resOK.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	if len(p.Vars["z"]) == 0 {
		t.Fatalf("expected aliased column z to be exposed")
	}

	srcBad := `
param p {
  x = (1,2)
  y = (3,4)
  a = comb(x + y)
  b = comb(x as z + a as t)
  b
}
`
	diagsBad := &diag.Diagnostics{}
	progBad := parser.Parse("comb_alias_bad.jbs", srcBad, diagsBad)
	_ = sema.Analyze(progBad, lower.BuiltinGlobalValues(), diagsBad)
	if !hasDiagCode(diagsBad, "E106") {
		t.Fatalf("expected E106 for alias on comb operand, got: %s", diagsBad.String())
	}
}

func hasUnusedWarningForVar(diags *diag.Diagnostics, name string) bool {
	target := "param variable '" + name + "'"
	for _, item := range diags.Items {
		if item.Code != "W312" {
			continue
		}
		if strings.Contains(item.Message, target) {
			return true
		}
	}
	return false
}
