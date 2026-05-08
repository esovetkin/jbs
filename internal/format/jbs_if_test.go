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

func TestJBSFormatsElifStatements(t *testing.T) {
	src := `
if a{x=1}elif b{x=2}else{x=3}
f=function(x){if x < 0{return -1}elif x == 0{return 0}else{return 1}}
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
		"if a {",
		"    x = 1",
		"} elif b {",
		"    x = 2",
		"} else {",
		"    x = 3",
		"f = function(x) {",
		"    if x < 0 {",
		"        return -1",
		"    } elif x == 0 {",
		"        return 0",
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted output missing %q\n%s", needle, got)
		}
	}
}

func TestJBSDoesNotRewriteNestedElseIfToElif(t *testing.T) {
	src := `if a{x=1}else{if b{x=2}else{x=3}}`
	var diags diag.Diagnostics
	got, err := JBS("if.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(got, "} else {\n    if b {") {
		t.Fatalf("expected nested if inside else, got:\n%s", got)
	}
	if strings.Contains(got, "} elif b {") {
		t.Fatalf("did not expect nested else-if rewrite, got:\n%s", got)
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
