package format

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestJBSFormatsIfStatements(t *testing.T) {
	src := `
if enabled{x=1
x}else{if other{y=2}}
f=function(x){if x < 0{return -x}else{x}}
`
	var diags diag.Diagnostics
	got, err := JBS("if.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	checks := []string{
		"if enabled {",
		"    x = 1",
		"    x",
		"} else {",
		"    if other {",
		"        y = 2",
		"f = function(x) {",
		"    if x < 0 {",
		"        return -x",
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted output missing %q\n%s", needle, got)
		}
	}
}

func TestJBSRejectsDeclarationInsideIf(t *testing.T) {
	var diags diag.Diagnostics
	got, err := JBS("if.jbs", "if true { do run { echo hi } }\n", &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty formatted output, got %q", got)
	}
	if !diags.HasErrors() {
		t.Fatalf("expected parser diagnostics")
	}
}
