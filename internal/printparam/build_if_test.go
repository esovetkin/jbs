package printparam

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestBuildUsesSelectedIfBranchColumns(t *testing.T) {
	src := `
flag = false
if flag {
	cases = t(x = range(2))
} else {
	cases = t(y = range(3))
}

do run with cases {
	echo $y
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
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("printparam failed: %s", diags.String())
	}
	if !reflect.DeepEqual(table.Columns, []string{"cases.y"}) {
		t.Fatalf("unexpected columns: %#v", table.Columns)
	}
	if len(table.Rows) != 3 {
		t.Fatalf("expected three rows, got %#v", table.Rows)
	}
	if table.Rows[0].Values["cases.y"] != "0" || table.Rows[2].Values["cases.y"] != "2" {
		t.Fatalf("unexpected selected branch row values: %#v", table.Rows)
	}
}

func TestBuildUsesSelectedElifBranchColumns(t *testing.T) {
	src := `
flag = "elif"
if flag == "if" {
	cases = t(x = range(2))
} elif flag == "elif" {
	cases = t(y = range(3))
} else {
	cases = t(z = range(4))
}

do run with cases {
	echo $y
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
	table := Build(res, diags)
	if diags.HasErrors() {
		t.Fatalf("printparam failed: %s", diags.String())
	}
	if !reflect.DeepEqual(table.Columns, []string{"cases.y"}) {
		t.Fatalf("unexpected columns: %#v", table.Columns)
	}
	if len(table.Rows) != 3 {
		t.Fatalf("expected three rows, got %#v", table.Rows)
	}
	if table.Rows[0].Values["cases.y"] != "0" || table.Rows[2].Values["cases.y"] != "2" {
		t.Fatalf("unexpected selected branch row values: %#v", table.Rows)
	}
}
