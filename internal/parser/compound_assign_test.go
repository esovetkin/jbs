package parser

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestParseCompoundAssignmentsAcrossBlocks(t *testing.T) {
	src := `
g += 1

let l {
  x = 1
  x += 2
  x -= 3
  x *= 4
  x /= 5
  x %= 6
}

param p {
  a = (1,2)
  a += (3,4)
  a
}

analyse step0 {
  h = "Number: %d"
  h += " suffix"
  v = h in "out.log"
  (v)
}

submit run {
  args_exec = "-lc hostname"
  args_exec += " && echo done"
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("compound.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}

	if got := len(prog.Stmts); got != 5 {
		t.Fatalf("expected 5 statements, got %d", got)
	}

	g, ok := prog.Stmts[0].(ast.GlobalAssign)
	if !ok {
		t.Fatalf("expected global assign at stmt 0, got %T", prog.Stmts[0])
	}
	if g.Op != ast.AssignPlusEq {
		t.Fatalf("unexpected global operator: got=%q want=%q", g.Op, ast.AssignPlusEq)
	}

	lb, ok := prog.Stmts[1].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block at stmt 1, got %T", prog.Stmts[1])
	}
	wantLetOps := []ast.AssignOp{
		ast.AssignEq,
		ast.AssignPlusEq,
		ast.AssignMinusEq,
		ast.AssignStarEq,
		ast.AssignSlashEq,
		ast.AssignPctEq,
	}
	if len(lb.Assignments) != len(wantLetOps) {
		t.Fatalf("unexpected let assignment count: got=%d want=%d", len(lb.Assignments), len(wantLetOps))
	}
	for i, want := range wantLetOps {
		if got := lb.Assignments[i].Op; got != want {
			t.Fatalf("let assignment %d op mismatch: got=%q want=%q", i, got, want)
		}
	}

	pb, ok := prog.Stmts[2].(ast.ParamBlock)
	if !ok {
		t.Fatalf("expected param block at stmt 2, got %T", prog.Stmts[2])
	}
	if len(pb.Assignments) != 2 {
		t.Fatalf("expected 2 param assignments, got %d", len(pb.Assignments))
	}
	if pb.Assignments[0].Op != ast.AssignEq || pb.Assignments[1].Op != ast.AssignPlusEq {
		t.Fatalf("unexpected param assignment ops: %#v", pb.Assignments)
	}

	ab, ok := prog.Stmts[3].(ast.AnalyseBlock)
	if !ok {
		t.Fatalf("expected analyse block at stmt 3, got %T", prog.Stmts[3])
	}
	if len(ab.Assignments) != 3 {
		t.Fatalf("expected 3 analyse assignments, got %d", len(ab.Assignments))
	}
	if ab.Assignments[0].Op != ast.AssignEq || ab.Assignments[1].Op != ast.AssignPlusEq || ab.Assignments[2].Op != ast.AssignEq {
		t.Fatalf("unexpected analyse assignment ops: %#v", ab.Assignments)
	}

	sb, ok := prog.Stmts[4].(ast.SubmitBlock)
	if !ok {
		t.Fatalf("expected submit block at stmt 4, got %T", prog.Stmts[4])
	}
	if len(sb.Fields) != 2 {
		t.Fatalf("expected 2 submit fields, got %d", len(sb.Fields))
	}
	if sb.Fields[0].Op != ast.AssignEq || sb.Fields[1].Op != ast.AssignPlusEq {
		t.Fatalf("unexpected submit field ops: %#v", sb.Fields)
	}
}

func TestParseSubmitRawBlockRejectsCompoundOperator(t *testing.T) {
	src := `
submit run {
  preprocess += {
    echo hi
  }
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("submit_raw_compound.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error for compound operator on submit raw block")
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == string(diag.CodeE077) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E077 for submit raw block compound operator, got: %s", diags.String())
	}
}

func TestParseParamCompoundOnlyStillRequiresFinalExpression(t *testing.T) {
	src := `
param p {
  a = (1,2)
  a += (3,4)
}
`
	diags := &diag.Diagnostics{}
	_ = Parse("param_compound_no_final.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parse error for missing final param expression")
	}
	found := false
	for _, d := range diags.Items {
		if d.Code == string(diag.CodeE027) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected E027 for missing final param expression, got: %s", diags.String())
	}
}

func TestParseCompoundAssignmentsWithSemicolonSeparators(t *testing.T) {
	src := `
let l {
  x = 1; y = 2; x += y;
}
`
	diags := &diag.Diagnostics{}
	prog := Parse("compound_semicolon.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	if len(prog.Stmts) != 1 {
		t.Fatalf("expected one statement, got %d", len(prog.Stmts))
	}
	lb, ok := prog.Stmts[0].(ast.LetBlock)
	if !ok {
		t.Fatalf("expected let block, got %T", prog.Stmts[0])
	}
	if len(lb.Assignments) != 3 {
		t.Fatalf("expected three assignments, got %d", len(lb.Assignments))
	}
	if lb.Assignments[2].Name != "x" || lb.Assignments[2].Op != ast.AssignPlusEq {
		t.Fatalf("unexpected third assignment: %#v", lb.Assignments[2])
	}
}
