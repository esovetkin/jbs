package sema

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/parser"
)

func TestAnalyzeCollectsSubmitAndCompilesSubmitSpec(t *testing.T) {
	src := `
x = 1

do prep {
  echo prep
}

submit run
  after prep
{
  account = "a"
  queue = "q"
  args_exec = "-lc hostname"
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)

	if res == nil {
		t.Fatalf("Analyze returned nil result")
	}
	if len(res.DoBlocks) != 1 || res.DoBlocks[0].Name != "prep" {
		t.Fatalf("unexpected do blocks in analysis result: %#v", res.DoBlocks)
	}
	if len(res.Submits) != 1 || res.Submits[0].Name != "run" {
		t.Fatalf("unexpected submit blocks in analysis result: %#v", res.Submits)
	}
	if _, ok := res.SubmitByName["run"]; !ok {
		t.Fatalf("expected compiled submit spec for run, got %#v", res.SubmitByName)
	}
	if _, ok := res.StepScopeByName["run"]; !ok {
		t.Fatalf("expected step scope plan for run submit step, got %#v", res.StepScopeByName)
	}
	if _, ok := res.GlobalVarByName["x"]; !ok {
		t.Fatalf("expected global variable x to be compiled, got %#v", res.GlobalVarByName)
	}
}

func TestAnalyzeCollectsAnalyseBlocks(t *testing.T) {
	src := `
do run {
  echo "N: 1" > out.log
}

analyse run {
  n = "N: %d" in "out.log"
  (n)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("bench"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)

	if res == nil {
		t.Fatalf("Analyze returned nil result")
	}
	if len(res.Analyse) != 1 || res.Analyse[0] == nil {
		t.Fatalf("expected one compiled analyse spec, got %#v", res.Analyse)
	}
	if res.Analyse[0].Block.StepName != "run" {
		t.Fatalf("unexpected analyse target step: %#v", res.Analyse[0])
	}
}
