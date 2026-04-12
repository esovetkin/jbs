package lower

import (
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestProjectSourceCommentsDeterministicOrder(t *testing.T) {
	src := `param p
{
        a = (1,2)
        a
}

do run
        with p  # c0
        # c1
        procs=4 # c2
{
        echo ${a}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	res := sema.Analyze(prog, BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected sema errors: %s", diags.String())
	}
	first := projectSourceComments(res)
	second := projectSourceComments(res)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("comment projection is not deterministic\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first) != 3 {
		t.Fatalf("expected 3 projections, got %d (%#v)", len(first), first)
	}
	if first[0].Target != "do:run.header.with" || first[0].Text != "c0" {
		t.Fatalf("unexpected first projection: %#v", first[0])
	}
	if first[1].Target != "do:run.header" || first[1].Text != "c1" {
		t.Fatalf("unexpected second projection: %#v", first[1])
	}
	if first[2].Target != "do:run.header.options" || first[2].Text != "c2" {
		t.Fatalf("unexpected third projection: %#v", first[2])
	}
}

func TestToJUBEYAMLProjectsComments(t *testing.T) {
	src := `param p
{
        a = (1,2)
        a
}

do run
        with p  # c0
{
        echo ${a}
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	res := sema.Analyze(prog, BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected sema errors: %s", diags.String())
	}
	doc := ToJUBEYAML(res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected lower errors: %s", diags.String())
	}
	if len(doc.Meta.SourceComments) == 0 {
		t.Fatalf("expected projected comments in meta")
	}
}

func TestProjectSourceCommentsIncludesParamLetAnalyseTargets(t *testing.T) {
	src := `let l
        # l0
{
        number = "Number: %d"
}

param p
        # p0
{
        a = (1,2)
        a
}

do write
{
        echo "Number: 1" > out
}

analyse write
        with l
        # a0
{
        n = number in "out"
        (n)
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse errors: %s", diags.String())
	}
	res := sema.Analyze(prog, BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected sema errors: %s", diags.String())
	}
	got := projectSourceComments(res)
	expect := map[string]bool{
		"let:l.header:l0":         false,
		"param:p.header:p0":       false,
		"analyse:write.header:a0": false,
	}
	for _, c := range got {
		key := c.Target + ":" + c.Text
		if _, ok := expect[key]; ok {
			expect[key] = true
		}
	}
	for key, seen := range expect {
		if !seen {
			t.Fatalf("missing projected comment %q in %#v", key, got)
		}
	}
}

func TestProjectSourceCommentsNilResult(t *testing.T) {
	if got := projectSourceComments(nil); got != nil {
		t.Fatalf("expected nil projection for nil sema result, got %#v", got)
	}
}

func TestProjectSourceCommentsAllBlockKindsAndIgnoredStmts(t *testing.T) {
	comment := func(text string) *ast.Comment {
		return &ast.Comment{
			Text: text,
			Span: diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2)),
		}
	}
	res := &sema.Result{
		Program: ast.Program{
			File: "in.jbs",
			Stmts: []ast.Stmt{
				ast.ParamBlock{
					Name: "p",
					Header: []ast.HeaderElem{
						{Kind: ast.HeaderElemComment, Comment: comment(" p0 ")},
						{Kind: ast.HeaderElemComment, Comment: nil},
					},
				},
				ast.LetBlock{
					Name: "l",
					Header: []ast.HeaderElem{
						{Kind: ast.HeaderElemComment, Comment: comment("l0")},
					},
				},
				ast.DoBlock{
					Name: "d",
					Header: []ast.HeaderElem{
						{Kind: ast.HeaderElemAfter, Inline: comment(" after0 ")},
						{Kind: ast.HeaderElemUse, Inline: comment(" ")},
					},
				},
				ast.SubmitBlock{
					Name: "s",
					Header: []ast.HeaderElem{
						{Kind: ast.HeaderElemUse, Inline: comment("use0")},
						{Kind: ast.HeaderElemWith, Inline: nil},
					},
				},
				ast.AnalyseBlock{
					StepName: "a",
					Header: []ast.HeaderElem{
						{Kind: ast.HeaderElemOption, Inline: comment(" opt0 ")},
						{Kind: ast.HeaderElemUnknown, Inline: comment("ignored")},
					},
				},
				ast.GlobalAssign{Name: "g"},
				ast.UseStmt{},
			},
		},
	}
	got := projectSourceComments(res)
	want := []CommentProjection{
		{Target: "param:p.header", Text: "p0"},
		{Target: "let:l.header", Text: "l0"},
		{Target: "do:d.header.after", Text: "after0"},
		{Target: "submit:s.header.use", Text: "use0"},
		{Target: "analyse:a.header.options", Text: "opt0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected projected comments\ngot=%#v\nwant=%#v", got, want)
	}
}

func TestHeaderElemLabelAllKinds(t *testing.T) {
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
	for _, tt := range tests {
		if got := headerElemLabel(tt.kind); got != tt.want {
			t.Fatalf("headerElemLabel(%q)=%q, want=%q", tt.kind, got, tt.want)
		}
	}
}
