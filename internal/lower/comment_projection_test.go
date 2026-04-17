package lower

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/sema"
)

func TestProjectSourceCommentsNil(t *testing.T) {
	if got := projectSourceComments(nil); got != nil {
		t.Fatalf("expected nil projections for nil result, got %#v", got)
	}
}

func TestProjectSourceCommentsCurrentBlockKinds(t *testing.T) {
	header := []ast.HeaderElem{
		{Kind: ast.HeaderElemComment, Comment: &ast.Comment{Text: "  group comment  "}},
		{Kind: ast.HeaderElemAfter, Inline: &ast.Comment{Text: "  after inline  "}},
		{Kind: ast.HeaderElemUse, Inline: &ast.Comment{Text: "use inline"}},
		{Kind: ast.HeaderElemWith, Inline: &ast.Comment{Text: "with inline"}},
		{Kind: ast.HeaderElemOption, Inline: &ast.Comment{Text: "opts inline"}},
	}
	res := &sema.Result{
		Program: ast.Program{Stmts: []ast.Stmt{
			ast.DoBlock{Name: "compile", Header: header},
			ast.SubmitBlock{Name: "run", Header: header},
			ast.AnalyseBlock{StepName: "collect", Header: header},
			ast.GlobalAssign{Name: "x"},
		}},
	}

	got := projectSourceComments(res)
	if len(got) != 15 {
		t.Fatalf("expected 15 projections, got %d (%#v)", len(got), got)
	}

	wantTargets := map[string]bool{
		"do:compile.header":              false,
		"do:compile.header.after":        false,
		"do:compile.header.use":          false,
		"do:compile.header.with":         false,
		"do:compile.header.options":      false,
		"submit:run.header":              false,
		"submit:run.header.after":        false,
		"submit:run.header.use":          false,
		"submit:run.header.with":         false,
		"submit:run.header.options":      false,
		"analyse:collect.header":         false,
		"analyse:collect.header.after":   false,
		"analyse:collect.header.use":     false,
		"analyse:collect.header.with":    false,
		"analyse:collect.header.options": false,
	}
	for _, proj := range got {
		seen, ok := wantTargets[proj.Target]
		if !ok {
			t.Fatalf("unexpected projection target %q in %#v", proj.Target, got)
		}
		if seen {
			t.Fatalf("duplicate projection target %q in %#v", proj.Target, got)
		}
		wantTargets[proj.Target] = true
	}
	for target, seen := range wantTargets {
		if !seen {
			t.Fatalf("missing projection target %q in %#v", target, got)
		}
	}
	if got[0].Text != "group comment" || got[1].Text != "after inline" {
		t.Fatalf("expected projected comments to be trimmed, got %#v", got[:2])
	}
}

func TestProjectHeaderCommentsSkipsBlankAndUnknownKinds(t *testing.T) {
	got := projectHeaderComments("do", "x", []ast.HeaderElem{
		{Kind: ast.HeaderElemComment, Comment: &ast.Comment{Text: "   "}},
		{Kind: ast.HeaderElemComment, Comment: nil},
		{Kind: ast.HeaderElemWith, Inline: &ast.Comment{Text: "  "}},
		{Kind: ast.HeaderElemUnknown, Inline: &ast.Comment{Text: "ignored"}},
	})
	if len(got) != 0 {
		t.Fatalf("expected no projections for blank or unknown header elements, got %#v", got)
	}
}

func TestHeaderElemLabelCoversSupportedKinds(t *testing.T) {
	tests := []struct {
		kind ast.HeaderElemKind
		want string
	}{
		{kind: ast.HeaderElemAfter, want: "after"},
		{kind: ast.HeaderElemUse, want: "use"},
		{kind: ast.HeaderElemWith, want: "with"},
		{kind: ast.HeaderElemOption, want: "options"},
		{kind: ast.HeaderElemUnknown, want: "header"},
	}
	for _, tc := range tests {
		if got := headerElemLabel(tc.kind); got != tc.want {
			t.Fatalf("headerElemLabel(%v) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}
