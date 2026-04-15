package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestBuildImportSources(t *testing.T) {
	paramSpan := diag.NewSpan("param.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 5))
	letSpan := diag.NewSpan("let.jbs", diag.NewPos(0, 2, 1), diag.NewPos(0, 2, 5))

	res := &Result{
		ImportSourceByName: map[string]*ImportSource{
			"stale": {Name: "stale", Kind: SourceKindParam},
		},
		Paramsets: []*Paramset{
			nil, // ensure nil paramsets are skipped
			{
				Name: "same",
				Block: ast.ParamBlock{
					Span: paramSpan,
				},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1)},
				},
				Origins: map[string]diag.Span{
					"x": paramSpan,
				},
				Modes: map[string]string{
					"x": "python",
				},
				Order: []string{"x"},
			},
			{
				Name: "p",
				Block: ast.ParamBlock{
					Span: paramSpan,
				},
				Vars: map[string][]eval.Value{
					"a": {eval.Int(1), eval.Int(2)},
				},
				Origins: map[string]diag.Span{
					"a": paramSpan,
				},
				Modes: map[string]string{
					"a": "shell",
				},
				Order: []string{"a"},
			},
		},
		LetNamespaces: []*LetNamespace{
			nil, // ensure nil let namespaces are skipped
			{
				Name: "same", // ensure let source overwrites same-name param source
				Vars: map[string]eval.Value{
					"q": eval.String("queue"),
				},
				Origins: map[string]diag.Span{
					"q": letSpan,
				},
				Modes: map[string]string{
					"q": "python",
				},
				Span: letSpan,
			},
			{
				Name: "l",
				Vars: map[string]eval.Value{
					"k": eval.Bool(true),
				},
				Span: letSpan,
			},
		},
	}

	buildImportSources(res)

	if _, ok := res.ImportSourceByName["stale"]; ok {
		t.Fatalf("expected stale import-source map to be replaced, got %#v", res.ImportSourceByName)
	}
	if len(res.ImportSourceByName) != 3 {
		t.Fatalf("expected exactly 3 import sources (same,p,l), got %#v", res.ImportSourceByName)
	}

	if got := res.ImportSourceByName["p"]; got == nil || got.Kind != SourceKindParam {
		t.Fatalf("expected param source for p, got %#v", got)
	}
	if got := res.ImportSourceByName["l"]; got == nil || got.Kind != SourceKindLet {
		t.Fatalf("expected let source for l, got %#v", got)
	}
	if got := res.ImportSourceByName["same"]; got == nil || got.Kind != SourceKindLet {
		t.Fatalf("expected let source to win for same-name source, got %#v", got)
	}
}

func TestValueAsSeries(t *testing.T) {
	t.Run("scalar becomes single-item series", func(t *testing.T) {
		got := valueAsSeries(eval.Int(7))
		if len(got) != 1 || !eval.Equal(got[0], eval.Int(7)) {
			t.Fatalf("expected scalar to become one-element series, got %#v", got)
		}
	})

	t.Run("list is cloned", func(t *testing.T) {
		src := eval.List([]eval.Value{eval.Int(1), eval.Int(2)})
		got := valueAsSeries(src)
		if len(got) != 2 || !eval.Equal(got[0], eval.Int(1)) || !eval.Equal(got[1], eval.Int(2)) {
			t.Fatalf("unexpected list conversion result: %#v", got)
		}
		got[0] = eval.Int(99)
		if !eval.Equal(src.L[0], eval.Int(1)) {
			t.Fatalf("expected cloned list slice, source mutated to %#v", src.L)
		}
	})

	t.Run("tuple is cloned", func(t *testing.T) {
		src := eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")})
		got := valueAsSeries(src)
		if len(got) != 2 || !eval.Equal(got[0], eval.String("a")) || !eval.Equal(got[1], eval.String("b")) {
			t.Fatalf("unexpected tuple conversion result: %#v", got)
		}
		got[1] = eval.String("z")
		if !eval.Equal(src.L[1], eval.String("b")) {
			t.Fatalf("expected cloned tuple slice, source mutated to %#v", src.L)
		}
	})
}
