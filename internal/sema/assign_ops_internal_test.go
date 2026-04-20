package sema

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestMapAssignOpToBinary(t *testing.T) {
	cases := []struct {
		op     ast.AssignOp
		wantOp string
		wantOK bool
	}{
		{op: ast.AssignPlusEq, wantOp: "+", wantOK: true},
		{op: ast.AssignMinusEq, wantOp: "-", wantOK: true},
		{op: ast.AssignStarEq, wantOp: "*", wantOK: true},
		{op: ast.AssignSlashEq, wantOp: "/", wantOK: true},
		{op: ast.AssignPctEq, wantOp: "%", wantOK: true},
		{op: ast.AssignEq, wantOp: "", wantOK: false},
		{op: ast.AssignOp("??"), wantOp: "", wantOK: false},
	}
	for i, tc := range cases {
		gotOp, gotOK := mapAssignOpToBinary(tc.op)
		if gotOp != tc.wantOp || gotOK != tc.wantOK {
			t.Fatalf("case %d: mapAssignOpToBinary(%q)=(%q,%v), want (%q,%v)", i, tc.op, gotOp, gotOK, tc.wantOp, tc.wantOK)
		}
	}
}

func TestAssignmentExprBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	rhs := ast.NumberExpr{Int: true, IntValue: 3, Span: span}

	if got := assignmentExpr("x", ast.AssignEq, rhs, span); got != rhs {
		t.Fatalf("expected '=' assignment to return rhs unchanged, got %#v", got)
	}
	if got := assignmentExpr("x", ast.AssignPlusEq, nil, span); got != nil {
		t.Fatalf("expected nil rhs to stay nil, got %#v", got)
	}
	if got := assignmentExpr("x", ast.AssignOp("??"), rhs, span); got != rhs {
		t.Fatalf("expected unknown assign op to keep rhs unchanged, got %#v", got)
	}

	got := assignmentExpr("x", ast.AssignStarEq, rhs, span)
	bin, ok := got.(ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected binary expansion for '*=', got %T %#v", got, got)
	}
	if bin.Op != "*" {
		t.Fatalf("expected binary op '*', got %#v", bin)
	}
	lhs, ok := bin.Left.(ast.IdentExpr)
	if !ok || lhs.Name != "x" {
		t.Fatalf("expected binary lhs ident x, got %#v", bin.Left)
	}
	if bin.Right != rhs {
		t.Fatalf("expected rhs to be preserved in binary expression, got %#v", bin.Right)
	}
}

func TestTopLevelCompoundAssignmentRejectedButFunctionLocalCompoundAssignmentStillWorks(t *testing.T) {
	prog := parseSemaProgram(t, "assign_ops.jbs", `
bump = function(x) {
	x += 1
	x
}
result = bump(1)
seed += 1
`)

	diags := &diag.Diagnostics{}
	out, order := compileUserGlobals(prog, nil, diags)
	if countDiagCode(diags, "E307") != 1 {
		t.Fatalf("expected one top-level compound-assignment diagnostic, got %d: %s", countDiagCode(diags, "E307"), diags.String())
	}
	if !reflect.DeepEqual(order, []string{"bump", "result"}) {
		t.Fatalf("unexpected compiled global order: %#v", order)
	}
	if out["result"] == nil || !eval.Equal(out["result"].Value, eval.Int(2)) {
		t.Fatalf("expected local += inside function body to remain valid, got %#v", out["result"])
	}
}
