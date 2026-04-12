package sema_test

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestAnalyzeDuplicateLetBlockName(t *testing.T) {
	src := `
let defs {
  a = "x"
}
let defs {
  a = "y"
}
do run with defs {
  echo ${a}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("dup_let.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)

	if !hasDiagCode(diags, "E400") {
		t.Fatalf("expected E400 for duplicate let block names, got: %s", diags.String())
	}
	if got := len(res.LetNamespaces); got != 1 {
		t.Fatalf("expected one compiled let namespace, got %d", got)
	}
	if _, ok := res.LetByName["defs"]; !ok {
		t.Fatalf("expected defs let namespace to be present")
	}
	if got := len(res.DoBlocks); got != 1 {
		t.Fatalf("expected one do block, got %d", got)
	}
}

func TestAnalyzeDuplicateParamBlockName(t *testing.T) {
	src := `
param matrix {
  a = (1,2)
  a
}
param matrix {
  a = (3,4)
  a
}
do run with matrix {
  echo ${a}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("dup_param.jbs", src, diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)

	if !hasDiagCode(diags, "E210") {
		t.Fatalf("expected E210 for duplicate param block names, got: %s", diags.String())
	}
	if got := len(res.Paramsets); got != 1 {
		t.Fatalf("expected one compiled paramset, got %d", got)
	}
	if _, ok := res.ParamByName["matrix"]; !ok {
		t.Fatalf("expected matrix paramset to be present")
	}
	if got := len(res.DoBlocks); got != 1 {
		t.Fatalf("expected one do block, got %d", got)
	}
}
