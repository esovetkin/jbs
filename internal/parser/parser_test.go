package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestParseAfterAndWith(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a
}

do work after prep,seed with p, x from p {
  echo hi
}

submit run after work with p {
  preprocess = {
    export X=1
  }
  args_exec = "python main.py"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("test.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}

	if len(prog.Stmts) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Stmts))
	}
	doBlock, ok := prog.Stmts[1].(ast.DoBlock)
	if !ok {
		t.Fatalf("expected do block at stmt[1]")
	}
	if got := len(doBlock.After); got != 2 {
		t.Fatalf("expected 2 dependencies, got %d", got)
	}
	if doBlock.After[0] != "prep" || doBlock.After[1] != "seed" {
		t.Fatalf("unexpected dependencies: %#v", doBlock.After)
	}
	if got := len(doBlock.WithItems); got != 2 {
		t.Fatalf("expected 2 with items, got %d", got)
	}
	if doBlock.WithItems[0].Name != "p" || doBlock.WithItems[0].From != "" {
		t.Fatalf("unexpected first with item: %#v", doBlock.WithItems[0])
	}
	if doBlock.WithItems[1].Name != "x" || doBlock.WithItems[1].From != "p" {
		t.Fatalf("unexpected second with item: %#v", doBlock.WithItems[1])
	}
}

func TestSubmitMalformedStatementError(t *testing.T) {
	src := `
submit run {
  export X=1
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error")
	}
	found := false
	for _, item := range diags.Items {
		if item.Code == "E076" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E076, got diagnostics: %s", diags.String())
	}
}

func TestParseSubmitRawAndExprFields(t *testing.T) {
	src := `
submit run {
  preprocess = {
    module load CUDA
  }
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("submit.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	sb, ok := prog.Stmts[0].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block")
	}
	if len(sb.Fields) != 2 {
		t.Fatalf("expected 2 submit fields, got %d", len(sb.Fields))
	}
	if sb.Fields[0].Name != "preprocess" || !sb.Fields[0].IsRaw {
		t.Fatalf("expected first field to be raw preprocess, got %#v", sb.Fields[0])
	}
	if sb.Fields[1].Name != "args_exec" || sb.Fields[1].IsRaw || sb.Fields[1].Expr == nil {
		t.Fatalf("expected second field to be expression args_exec, got %#v", sb.Fields[1])
	}
}

func TestAssignmentTrailingTokensError(t *testing.T) {
	src := `
param p {
  a = f(1)
  a
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad.jbs", src, diags)
	found := false
	for _, item := range diags.Items {
		if item.Code == "E061" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E061 trailing token error, got: %s", diags.String())
	}
}

func TestParseModeExprAssignment(t *testing.T) {
	src := `
param p {
  queue = python("__import__(\"os\").environ.get(\"JUBE_QUEUE\", \"devel\")")
  system_name = shell("cat /etc/FZJ/systemname | tr -d '\n'")
  queue * system_name
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("mode.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement")
	}
	pb, ok := prog.Stmts[0].(ast.ParamBlock)
	if !ok || len(pb.Assignments) < 2 {
		t.Fatalf("expected param block assignments")
	}
	if _, ok := pb.Assignments[0].Expr.(ast.ModeExpr); !ok {
		t.Fatalf("expected first assignment to be ModeExpr")
	}
	if _, ok := pb.Assignments[1].Expr.(ast.ModeExpr); !ok {
		t.Fatalf("expected second assignment to be ModeExpr")
	}
}

func TestParseTopLevelGlobalAssignments(t *testing.T) {
	src := `
jbs_name = "demo"
jbs_outpath = "results"
jbs_queue = python("__import__('os').environ.get('JUBE_QUEUE', 'devel')")

param p {
  a = 1
  a
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("globals.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(prog.Stmts) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(prog.Stmts))
	}
	if _, ok := prog.Stmts[0].(ast.GlobalAssign); !ok {
		t.Fatalf("expected stmt 0 to be global assignment")
	}
	if _, ok := prog.Stmts[1].(ast.GlobalAssign); !ok {
		t.Fatalf("expected stmt 1 to be global assignment")
	}
	if _, ok := prog.Stmts[2].(ast.GlobalAssign); !ok {
		t.Fatalf("expected stmt 2 to be global assignment")
	}
	if _, ok := prog.Stmts[3].(ast.ParamBlock); !ok {
		t.Fatalf("expected stmt 3 to be param block")
	}
}

func TestParseMalformedTopLevelGlobalAssignment(t *testing.T) {
	src := `
jbs_name =
`
	diags := &diag.Diagnostics{}
	_ = Parse("bad_globals.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error for malformed global assignment")
	}
}
