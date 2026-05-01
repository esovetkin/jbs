package lower

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestToJUBEYAMLUsesSelectedIfBranchValues(t *testing.T) {
	src := `
jbs_name = "if_demo"
flag = true
if flag {
	cases = t(x = range(2))
} else {
	cases = t(x = range(5))
}

do run with cases {
	echo $x
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("if.jbs", src, diags)
	if diags.HasErrors() {
		t.Fatalf("parse failed: %s", diags.String())
	}
	res := sema.Analyze(prog, nil, diags)
	if diags.HasErrors() {
		t.Fatalf("analyze failed: %s", diags.String())
	}
	doc := ToJUBEYAML(res, diags)
	if diags.HasErrors() {
		t.Fatalf("lower failed: %s", diags.String())
	}
	if len(doc.Step) != 1 || doc.Step[0].Name != "run" {
		t.Fatalf("expected run step, got %#v", doc.Step)
	}
	var cases *ParameterSet
	for i := range doc.ParameterSet {
		if len(doc.ParameterSet[i].Parameter) == 2 && doc.ParameterSet[i].Parameter[1].Name == "x" {
			cases = &doc.ParameterSet[i]
			break
		}
	}
	if cases == nil {
		t.Fatalf("expected cases parameter set, got %#v", doc.ParameterSet)
	}
	if cases.Parameter[0].Value != "0,1" {
		t.Fatalf("expected true branch cases x=0,1, got %#v", cases.Parameter)
	}
}

func TestToJUBEYAMLRejectsDeclarationInsideIf(t *testing.T) {
	src := `
if true {
	do run { echo bad }
}
`
	diags := &diag.Diagnostics{}
	_ = parser.Parse("if.jbs", src, diags)
	if !diags.HasErrors() {
		t.Fatalf("expected parser diagnostics for nested do")
	}
}
