package sema_test

import (
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestCompoundAssignGlobalLetParamAnalyse(t *testing.T) {
	src := `
jbs_comment = "A"
jbs_comment += "B"

let l {
  jbs_comment += "C"
  pat = "Number: %d"
}

param p {
  a = (1,2)
  a += (3,4)
  a
}

do step0
  with p, l
{
  echo "Number: ${a} suffix ${jbs_comment}" > out.log
}

analyse step0
  with l
{
  pat += " suffix"
  n = pat in "out.log"
  (n)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("compound_semantics.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected sema errors: %s", diags.String())
	}

	if got := res.Globals.Values["jbs_comment"]; got.Kind != eval.KindString || got.S != "AB" {
		t.Fatalf("unexpected jbs_comment value: %#v", got)
	}

	ns := res.LetByName["l"]
	if ns == nil {
		t.Fatalf("expected let namespace 'l'")
	}
	if got := ns.Vars["jbs_comment"]; got.Kind != eval.KindString || got.S != "ABC" {
		t.Fatalf("unexpected let value jbs_comment: %#v", got)
	}

	ps := res.ParamByName["p"]
	if ps == nil {
		t.Fatalf("expected paramset 'p'")
	}
	gotVals := ps.Vars["a"]
	if len(gotVals) != 4 {
		t.Fatalf("unexpected param row count for a: got=%d want=4", len(gotVals))
	}
	want := []int64{1, 2, 3, 4}
	for i, w := range want {
		if gotVals[i].Kind != eval.KindInt || gotVals[i].I != w {
			t.Fatalf("a[%d] mismatch: got=%#v want=%d", i, gotVals[i], w)
		}
	}
}

func TestCompoundAssignSubmitUsesDefaultFromHeaderUse(t *testing.T) {
	src := `
let defaults {
  account = "acc"
  queue = "batch"
  executable = "/bin/bash"
  args_exec = "-lc hostname"
}

submit run
  use defaults
{
  args_exec += " && echo done"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("compound_submit.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected sema errors: %s", diags.String())
	}
	spec := res.SubmitByName["run"]
	if spec == nil {
		t.Fatalf("expected submit spec 'run'")
	}

	found := false
	for _, v := range spec.Values {
		if v.Name != "args_exec" {
			continue
		}
		found = true
		if v.Value.Kind != eval.KindString || v.Value.S != "-lc hostname && echo done" {
			t.Fatalf("unexpected args_exec value: %#v", v.Value)
		}
	}
	if !found {
		t.Fatalf("expected args_exec in submit values")
	}
}

func TestCompoundAssignUndefinedLhsEmitsE100(t *testing.T) {
	srcTemplate := `
let l {
  x %s 1
}
`
	ops := []string{"+=", "-=", "*=", "/=", "%="}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			src := strings.Replace(srcTemplate, "%s", op, 1)
			diags := &diag.Diagnostics{}
			prog := parser.Parse("compound_undefined_lhs.jbs", src, diags)
			if diags.HasErrors() {
				t.Fatalf("unexpected parse errors: %s", diags.String())
			}
			_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
			found := false
			for _, d := range diags.Items {
				if d.Code == string(diag.CodeE100) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected E100 for undefined lhs with %s, got: %s", op, diags.String())
			}
		})
	}
}

func TestCompoundAssignParamEquivalence(t *testing.T) {
	type tc struct {
		name  string
		op    string
		binOp string
		rhs   string
	}
	cases := []tc{
		{name: "plus", op: "+=", binOp: "+", rhs: "2"},
		{name: "minus", op: "-=", binOp: "-", rhs: "2"},
		{name: "star", op: "*=", binOp: "*", rhs: "2"},
		{name: "slash", op: "/=", binOp: "/", rhs: "2"},
		{name: "percent", op: "%=", binOp: "%", rhs: "3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compound := strings.ReplaceAll(`
param p {
  x = [10, 20]
  x __OP__ __RHS__
  x
}
`, "__OP__", tc.op)
			compound = strings.ReplaceAll(compound, "__RHS__", tc.rhs)

			expanded := strings.ReplaceAll(`
param p {
  x = [10, 20]
  x = x __BIN__ __RHS__
  x
}
`, "__BIN__", tc.binOp)
			expanded = strings.ReplaceAll(expanded, "__RHS__", tc.rhs)

			diagsCompound := &diag.Diagnostics{}
			progCompound := parser.Parse("compound_equivalence.jbs", compound, diagsCompound)
			if diagsCompound.HasErrors() {
				t.Fatalf("unexpected parse errors in compound case: %s", diagsCompound.String())
			}
			resCompound := sema.Analyze(progCompound, lower.BuiltinGlobalValues(), diagsCompound)
			if diagsCompound.HasErrors() {
				t.Fatalf("unexpected sema errors in compound case: %s", diagsCompound.String())
			}

			diagsExpanded := &diag.Diagnostics{}
			progExpanded := parser.Parse("expanded_equivalence.jbs", expanded, diagsExpanded)
			if diagsExpanded.HasErrors() {
				t.Fatalf("unexpected parse errors in expanded case: %s", diagsExpanded.String())
			}
			resExpanded := sema.Analyze(progExpanded, lower.BuiltinGlobalValues(), diagsExpanded)
			if diagsExpanded.HasErrors() {
				t.Fatalf("unexpected sema errors in expanded case: %s", diagsExpanded.String())
			}

			got := resCompound.ParamByName["p"].Vars["x"]
			want := resExpanded.ParamByName["p"].Vars["x"]
			if len(got) != len(want) {
				t.Fatalf("row count mismatch: got=%d want=%d", len(got), len(want))
			}
			for i := range want {
				if !eval.Equal(got[i], want[i]) {
					t.Fatalf("value mismatch at %d: got=%#v want=%#v", i, got[i], want[i])
				}
			}
		})
	}
}
