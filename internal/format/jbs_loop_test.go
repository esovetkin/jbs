package format

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestJBSFormatsLoopStatements(t *testing.T) {
	src := `
for x in range(3){if x==1{continue}y+=x}
while y<10{y+=1
if y==5{break}}
f=function(xs){for x in xs{return x}}
`
	var diags diag.Diagnostics
	got, err := JBS("loop.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	checks := []string{
		"for x in range(3) {",
		"    if x==1 {",
		"        continue",
		"while y<10 {",
		"    y += 1",
		"        break",
		"f = function(xs) {",
		"    for x in xs {",
		"        return x",
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted output missing %q\n%s", needle, got)
		}
	}
}
