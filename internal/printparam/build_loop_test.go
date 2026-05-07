package printparam

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestBuildUsesLoopComputedGlobals(t *testing.T) {
	src := `
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
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("printparam failed: %s", diags.String())
	}
	if !reflect.DeepEqual(table.Columns, []string{"cases.v"}) {
		t.Fatalf("unexpected columns: %#v", table.Columns)
	}
	if len(table.Rows) != 3 {
		t.Fatalf("expected three rows, got %#v", table.Rows)
	}
	if table.Rows[0].Values["cases.v"] != "0" || table.Rows[2].Values["cases.v"] != "2" {
		t.Fatalf("unexpected row values: %#v", table.Rows)
	}
}
