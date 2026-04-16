package lower

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/sema"
)

func TestProjectSourceCommentsNil(t *testing.T) {
	if got := projectSourceComments(nil); got != nil {
		t.Fatalf("expected nil projections for nil result, got %#v", got)
	}
}

func TestProjectSourceCommentsAllBlockKinds(t *testing.T) {
	header := []ast.HeaderElem{
		{
			Kind:    ast.HeaderElemComment,
			Comment: &ast.Comment{Text: "  group comment  "},
		},
		{
			Kind:   ast.HeaderElemAfter,
			Inline: &ast.Comment{Text: "  after inline  "},
		},
		{
			Kind:   ast.HeaderElemUse,
			Inline: &ast.Comment{Text: "use inline"},
		},
		{
			Kind:   ast.HeaderElemWith,
			Inline: &ast.Comment{Text: "with inline"},
		},
		{
			Kind:   ast.HeaderElemOption,
			Inline: &ast.Comment{Text: "opts inline"},
		},
	}

	res := &sema.Result{
		Program: ast.Program{
			Stmts: []ast.Stmt{
				ast.ParamBlock{Name: "p", Header: header},
				ast.LetBlock{Name: "l", Header: header},
				ast.DoBlock{Name: "d", Header: header},
				ast.SubmitBlock{Name: "s", Header: header},
				ast.AnalyseBlock{StepName: "a", Header: header},
				ast.GlobalAssign{
					Name: "x",
					Op:   ast.AssignEq,
					Expr: ast.NumberExpr{Int: true, IntValue: 1, Span: diag.Span{}},
				},
			},
		},
	}

	got := projectSourceComments(res)
	if len(got) != 25 {
		t.Fatalf("expected 25 projections (5 blocks x 5 comments), got %d (%#v)", len(got), got)
	}

	wantTargets := map[string]bool{
		"param:p.header":           false,
		"param:p.header.after":     false,
		"param:p.header.use":       false,
		"param:p.header.with":      false,
		"param:p.header.options":   false,
		"let:l.header":             false,
		"let:l.header.after":       false,
		"let:l.header.use":         false,
		"let:l.header.with":        false,
		"let:l.header.options":     false,
		"do:d.header":              false,
		"do:d.header.after":        false,
		"do:d.header.use":          false,
		"do:d.header.with":         false,
		"do:d.header.options":      false,
		"submit:s.header":          false,
		"submit:s.header.after":    false,
		"submit:s.header.use":      false,
		"submit:s.header.with":     false,
		"submit:s.header.options":  false,
		"analyse:a.header":         false,
		"analyse:a.header.after":   false,
		"analyse:a.header.use":     false,
		"analyse:a.header.with":    false,
		"analyse:a.header.options": false,
	}

	for _, p := range got {
		if _, ok := wantTargets[p.Target]; ok {
			wantTargets[p.Target] = true
		}
	}
	for target, seen := range wantTargets {
		if !seen {
			t.Fatalf("missing projection target %q in %#v", target, got)
		}
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
		t.Fatalf("expected no projections for blank/unknown comments, got %#v", got)
	}
}

func TestHeaderElemLabelDefault(t *testing.T) {
	if got := headerElemLabel(ast.HeaderElemUnknown); got != "header" {
		t.Fatalf("expected default label header, got %q", got)
	}
}
