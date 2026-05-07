package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestDoNProcParses(t *testing.T) {
	diags := &diag.Diagnostics{}
	prog := Parse("x.jbs", "do run nproc 4 {\necho ok\n}\n", diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	block, ok := prog.Stmts[0].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block, got %#v", prog.Stmts[0])
	}
	if block.NProc == nil || *block.NProc != 4 {
		t.Fatalf("unexpected nproc: %#v", block.NProc)
	}
}

func TestDoNProcRejectsMissingValue(t *testing.T) {
	diags := &diag.Diagnostics{}
	_ = Parse("x.jbs", "do run nproc {\necho ok\n}\n", diags)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostic for missing nproc value")
	}
}
