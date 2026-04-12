package lower

import (
	"reflect"
	"testing"

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
