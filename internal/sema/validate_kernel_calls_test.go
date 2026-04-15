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
