package lower

import (
	"testing"

	"jbs/internal/diag"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func TestToJUBEYAMLUsesLoopComputedGlobals(t *testing.T) {
	src := `
jbs_name = "loop_demo"
values = ()
for x in range(3) {
	values += (x,)
}
cases = t(v = values)

do run with cases {
	echo $v
}
`
	diags := &diag.Diagnostics{}
	prog := parser.Parse("loop.jbs", src, diags)
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
	var cases *ParameterSet
	for i := range doc.ParameterSet {
		if len(doc.ParameterSet[i].Parameter) == 2 && doc.ParameterSet[i].Parameter[1].Name == "v" {
			cases = &doc.ParameterSet[i]
			break
		}
	}
	if cases == nil {
		t.Fatalf("expected cases parameter set, got %#v", doc.ParameterSet)
	}
	if cases.Parameter[0].Value != "0,1,2" {
		t.Fatalf("expected loop-computed values, got %#v", cases.Parameter)
	}
}
