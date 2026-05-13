package parser

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestParseDoBlockBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		p := newTopLevelParser(`do run after prep with p["x"] nproc 2 {
  echo ${x}
}
`, diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseDoBlock(start)
		if block.Name != "run" {
			t.Fatalf("unexpected do block name: %#v", block)
		}
		if len(block.After) != 1 || block.After[0] != "prep" {
			t.Fatalf("unexpected after list: %#v", block.After)
		}
		if len(block.WithItems) != 1 {
			t.Fatalf("unexpected with-items: %#v", block.WithItems)
		}
		assertWithIndexStringColumns(t, block.WithItems[0], "p", []string{"x"})
		if block.NProc == nil || *block.NProc != 2 {
			t.Fatalf("unexpected nproc option: %#v", block)
		}
		if block.Body == "" {
			t.Fatalf("expected non-empty do body")
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags.String())
		}
	})

	t.Run("with fsub", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		src := `do run with p["x"] fsub "input.tpl" { "###X###": x, "Y": "lit" } {
  cat input.tpl
}
`
		p := newTopLevelParser(src, diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseDoBlock(start)
		if len(block.FSubs) != 1 {
			t.Fatalf("expected one fsub, got %#v", block.FSubs)
		}
		fsub := block.FSubs[0]
		if fsub.Path != "input.tpl" || len(fsub.Rules) != 2 {
			t.Fatalf("unexpected fsub parse: %#v", fsub)
		}
		if fsub.Rules[0].Pattern != "###X###" {
			t.Fatalf("unexpected first pattern: %#v", fsub.Rules[0])
		}
		if _, ok := fsub.Rules[0].Expr.(ast.IdentExpr); !ok {
			t.Fatalf("expected identifier replacement, got %#v", fsub.Rules[0].Expr)
		}
		if block.Body == "" || block.BodyStart.Line == 0 {
			t.Fatalf("expected raw do body after fsub, got %#v", block)
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

func TestParseAnalyseBlockBranches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		src := `analyse run with p {
  n = "Number: %d" in "out.log"
  (x, n as "N")
}
`
		p := newTopLevelParser(src, diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseAnalyseBlock(start)
		if block.StepName != "run" {
			t.Fatalf("unexpected analyse step name: %#v", block)
		}
		if len(block.WithItems) != 1 {
			t.Fatalf("unexpected analyse with-items: %#v", block.WithItems)
		}
		assertWithIdent(t, block.WithItems[0], "p")
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

	t.Run("deep qualified result column", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		src := "analyse run {\n  (pkg.ns.value as \"Value\")\n}\n"
		p := newTopLevelParser(src, diags)
		start := p.pos()
		p.consumeWord()
		block := p.parseAnalyseBlock(start)
		if len(block.Columns) != 1 || block.Columns[0].Name != "pkg.ns.value" || block.Columns[0].Title != "Value" {
			t.Fatalf("unexpected analyse columns: %#v", block.Columns)
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
		p := newTopLevelParser(`analyse run with p["x"]`, diags)
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
