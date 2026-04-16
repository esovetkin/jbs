package parser

import (
	"testing"

	"jbs/internal/diag"
)

func TestParseDoBlockBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("do run after prep with x from p max_async=1 procs=2 iterations=3 {\n  echo ${x}\n}\n", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseDoBlock(start)
		if block.Name != "run" {
			t.Fatalf("unexpected do block name: %#v", block)
		}
		if len(block.After) != 1 || block.After[0] != "prep" {
			t.Fatalf("unexpected after list: %#v", block.After)
		}
		if len(block.WithItems) != 1 || block.WithItems[0].Name != "x" || block.WithItems[0].From != "p" {
			t.Fatalf("unexpected with-items: %#v", block.WithItems)
		}
		if block.MaxAsync == nil || *block.MaxAsync != 1 || block.Procs == nil || *block.Procs != 2 || block.Iterations == nil || *block.Iterations != 3 {
			t.Fatalf("unexpected do options: %#v", block)
		}
		if block.Body == "" {
			t.Fatalf("expected non-empty do body")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("missing opening brace", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("do run after prep", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseDoBlock(start)
		if block.Body != "" {
			t.Fatalf("expected empty body on missing brace: %#v", block)
		}
		if !hasDiag(diags, "E031") {
			t.Fatalf("expected E031, got: %s", diags.String())
		}
	})

	t.Run("unterminated body", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("do run {", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseDoBlock(start)
		if block.Body != "" {
			t.Fatalf("expected empty body for unterminated do block, got %#v", block)
		}
		if !hasDiag(diags, "E025") {
			t.Fatalf("expected E025 from unterminated balanced block, got: %s", diags.String())
		}
	})
}

func TestParseSubmitBlockBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		src := "submit run after prep use defs with x from p max_async=1 procs=2 iterations=3 {\n  account = \"a\"\n  queue = \"q\"\n  args_exec = \"-lc hostname\"\n}\n"
		p := newTopLevelParser(src, diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseSubmitBlock(start)
		if block.Name != "run" {
			t.Fatalf("unexpected submit name: %#v", block)
		}
		if len(block.After) != 1 || block.After[0] != "prep" {
			t.Fatalf("unexpected submit after list: %#v", block.After)
		}
		if len(block.UseNames) != 1 || block.UseNames[0] != "defs" {
			t.Fatalf("unexpected submit use list: %#v", block.UseNames)
		}
		if len(block.WithItems) != 1 || block.WithItems[0].Name != "x" || block.WithItems[0].From != "p" {
			t.Fatalf("unexpected submit with-items: %#v", block.WithItems)
		}
		if block.MaxAsync == nil || *block.MaxAsync != 1 || block.Procs == nil || *block.Procs != 2 || block.Iterations == nil || *block.Iterations != 3 {
			t.Fatalf("unexpected submit options: %#v", block)
		}
		if len(block.Fields) != 3 {
			t.Fatalf("expected three submit fields, got %#v", block.Fields)
		}
		if block.BodyRaw == "" {
			t.Fatalf("expected non-empty submit body raw text")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("missing opening brace", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("submit run with p", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseSubmitBlock(start)
		if len(block.Fields) != 0 {
			t.Fatalf("expected zero fields when brace is missing: %#v", block.Fields)
		}
		if !hasDiag(diags, "E041") {
			t.Fatalf("expected E041, got: %s", diags.String())
		}
	})

	t.Run("unterminated body", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("submit run {", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseSubmitBlock(start)
		if len(block.Fields) != 0 || block.BodyRaw != "" {
			t.Fatalf("expected empty parsed fields/body for unterminated submit block: %#v", block)
		}
		if !hasDiag(diags, "E025") {
			t.Fatalf("expected E025 from unterminated balanced block, got: %s", diags.String())
		}
	})
}

func TestParseAnalyseBlockBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		src := "analyse run with x from p {\n  n = \"Number: %d\" in \"out.log\"\n  (x, n as \"N\")\n}\n"
		p := newTopLevelParser(src, diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseAnalyseBlock(start)
		if block.StepName != "run" {
			t.Fatalf("unexpected analyse step name: %#v", block)
		}
		if len(block.WithItems) != 1 || block.WithItems[0].Name != "x" || block.WithItems[0].From != "p" {
			t.Fatalf("unexpected analyse with-items: %#v", block.WithItems)
		}
		if len(block.Assignments) != 1 || len(block.Columns) != 2 {
			t.Fatalf("unexpected analyse body parse result: assignments=%#v columns=%#v", block.Assignments, block.Columns)
		}
		if block.BodyRaw == "" {
			t.Fatalf("expected non-empty analyse body raw text")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("after-clause rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("analyse run after prep {}", diags)
		start := p.pos()
		p.consumeWord()
		_ = p.parseAnalyseBlock(start)
		if !hasDiag(diags, "E416") {
			t.Fatalf("expected E416 for analyse after-clause, got: %s", diags.String())
		}
	})

	t.Run("missing opening brace", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("analyse run with x from p", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseAnalyseBlock(start)
		if len(block.Assignments) != 0 || len(block.Columns) != 0 || block.BodyRaw != "" {
			t.Fatalf("expected empty analyse body on missing brace: %#v", block)
		}
		if !hasDiag(diags, "E416") {
			t.Fatalf("expected E416 for missing brace, got: %s", diags.String())
		}
	})

	t.Run("unterminated body", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser("analyse run {", diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseAnalyseBlock(start)
		if len(block.Assignments) != 0 || len(block.Columns) != 0 || block.BodyRaw != "" {
			t.Fatalf("expected empty analyse parse output for unterminated body: %#v", block)
		}
		if !hasDiag(diags, "E025") {
			t.Fatalf("expected E025 from unterminated balanced block, got: %s", diags.String())
		}
	})
}
