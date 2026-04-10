package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestParamAssignmentTupleRepeat(t *testing.T) {
	src := `
param p {
  a = 4
  b = (1,2,3) * a
  b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("tuple_repeat.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	got := p.Vars["b"]
	if len(got) != 12 {
		t.Fatalf("expected 12 values in repeated tuple series, got %d: %#v", len(got), got)
	}
	if got[0].Kind != eval.KindInt || got[0].I != 1 || got[3].I != 1 || got[11].I != 3 {
		t.Fatalf("unexpected repeated tuple values: %#v", got)
	}
}

func TestParamAssignmentTupleConcat(t *testing.T) {
	src := `
param p {
  x = (1,2,3) + (4,)
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("tuple_concat.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	got := p.Vars["x"]
	if len(got) != 4 {
		t.Fatalf("expected 4 values in concatenated tuple series, got %d: %#v", len(got), got)
	}
	if got[0].I != 1 || got[1].I != 2 || got[2].I != 3 || got[3].I != 4 {
		t.Fatalf("unexpected concatenated tuple values: %#v", got)
	}
}

func TestParamAssignmentTupleConcatScalarError(t *testing.T) {
	src := `
param p {
  x = (1,2,3) + 4
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("tuple_error.jbs", src, diags)
	_ = sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if !hasDiagCode(diags, "E106") {
		t.Fatalf("expected E106, got: %s", diags.String())
	}
}

func TestParamAssignmentListArithmeticUnchanged(t *testing.T) {
	src := `
param p {
  x = [1,2,3] * 4
  x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("list_vector.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	got := p.Vars["x"]
	if len(got) != 3 {
		t.Fatalf("expected 3 list-vector values, got %d: %#v", len(got), got)
	}
	if got[0].I != 4 || got[1].I != 8 || got[2].I != 12 {
		t.Fatalf("unexpected list-vector values: %#v", got)
	}
}

func TestParamAssignmentConversionsControlSemantics(t *testing.T) {
	src := `
param p {
  a = tuple([1,2,3]) * 2
  b = list((1,2,3)) * 2
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("convert_control.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	a := p.Vars["a"]
	if len(a) != 6 || a[0].I != 1 || a[3].I != 1 || a[5].I != 3 {
		t.Fatalf("unexpected converted tuple-repeat values: %#v", a)
	}
	b := p.Vars["b"]
	if len(b) != 6 || b[0].I != 2 || b[1].I != 4 || b[2].I != 6 || b[3].I != 2 || b[4].I != 4 || b[5].I != 6 {
		t.Fatalf("unexpected converted list-vector values: %#v", b)
	}
}

func TestParamFinalCombinationSemanticsUnchanged(t *testing.T) {
	src := `
param p {
  a = (1,2)
  b = ("x","y")
  a + b
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("comb_unchanged.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	p := res.ParamByName["p"]
	if p == nil {
		t.Fatalf("missing param p")
	}
	if len(p.Rows) != 2 {
		t.Fatalf("expected two rows from final combination direct sum, got %d", len(p.Rows))
	}
}
