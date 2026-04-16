package parser

import (
	"testing"

	"jbs/internal/diag"
)

func TestParseProgramDispatchAndSpan(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := `
jbs_name = "bench"
do run {
  echo hi
}
submit s {
  account = "a"
  queue = "q"
  args_exec = "-lc hostname"
}
analyse run {
  n = "N: %d" in "out.log"
  (n)
}
`
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 4 {
		t.Fatalf("expected 4 top-level statements, got %d (%#v)", len(prog.Stmts), prog.Stmts)
	}
	if prog.File != "in.jbs" {
		t.Fatalf("unexpected program file: %q", prog.File)
	}
	if prog.Span.Start.Offset >= prog.Span.End.Offset {
		t.Fatalf("expected non-empty merged program span, got %+v", prog.Span)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid program: %s", diags.String())
	}
}

func TestParseProgramUnknownTokenAndKeywordBranches(t *testing.T) {
	diags := &diag.Diagnostics{}
	src := "@\nunknownblock x\n"
	prog := Parse("in.jbs", src, diags)
	if len(prog.Stmts) != 0 {
		t.Fatalf("expected no valid statements for malformed source, got %#v", prog.Stmts)
	}
	if !hasDiag(diags, "E010") {
		t.Fatalf("expected E010 for non-word token at top level, got: %s", diags.String())
	}
	if !hasDiag(diags, "E011") {
		t.Fatalf("expected E011 for unknown block keyword, got: %s", diags.String())
	}
}
