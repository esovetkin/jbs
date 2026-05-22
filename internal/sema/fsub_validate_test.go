package sema

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestValidateFileSubstitutionsAcceptsVisibleAndInheritedRefs(t *testing.T) {
	src := `
cases = table(x = [1], y = ["a"])

do prep
        with cases["x"]
        fsub "prep.tpl" { "X": x }
{
        :
}

do run
        after prep
        with cases["y"]
        fsub "run.tpl" { "X": x, "Y": y }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsUseWithAliasVisibility(t *testing.T) {
	valid := `
x = sample(range(10))

do run
        with x as y
        fsub "test0.input" {"x=(x)": y}
{
        echo $y
}
`
	diags := analyzeFSubValidationSource(t, valid)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for alias fsub: %s", diags.String())
	}

	invalid := `
x = sample(range(10))

do run
        with x as y
        fsub "test0.input" {"x=(x)": x}
{
        :
}
`
	diags = analyzeFSubValidationSource(t, invalid)
	if countDiagCode(diags, string(diag.CodeE220)) != 1 {
		t.Fatalf("expected fsub invisible-original diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `references variable "x" that is not visible`) {
		t.Fatalf("missing invisible original-name diagnostic: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsAliasedUsageSuppressesUnusedImportWarning(t *testing.T) {
	src := `
x = [1]

do run
        with x as y
        fsub "input.tpl" { "X": y }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if countDiagCode(diags, string(diag.CodeW313)) != 0 {
		t.Fatalf("did not expect unused-import warning for fsub alias ref: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsRejectsInvisibleRefsInvalidRegexAndDuplicateDest(t *testing.T) {
	src := `
cases = table(x = [1])

do run
        with cases["x"]
        fsub "./a/input.tpl" { "(": x }
        fsub "./b/input.tpl" { "Y": y }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if countDiagCode(diags, string(diag.CodeE220)) < 3 {
		t.Fatalf("expected fsub diagnostics, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "duplicate fsub destination") {
		t.Fatalf("missing duplicate destination diagnostic: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "invalid fsub regex") {
		t.Fatalf("missing invalid regex diagnostic: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `references variable "y" that is not visible`) {
		t.Fatalf("missing invisible reference diagnostic: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsCountAsImportUsage(t *testing.T) {
	src := `
	cases = table(x = [1])

do run
        with cases["x"]
        fsub "input.tpl" { "X": x }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if countDiagCode(diags, string(diag.CodeW313)) != 0 {
		t.Fatalf("did not expect unused-import warning for fsub-only ref: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsAcceptsPercentPlaceholders(t *testing.T) {
	src := `
	cases = table(x = [1], y = [1.5], label = ["case"])

	do run
	        with cases
	        fsub "input.tpl" {
	                "x=%d": x,
	                "y=%f label=%w": (y, label),
	                "literal=%%": "literal=%"
	        }
	{
	        :
	}
	`
	diags := analyzeFSubValidationSource(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsRejectsInvalidPercentPlaceholder(t *testing.T) {
	src := `
	do run
	        fsub "input.tpl" { "x=%x": "bad" }
	{
	        :
	}
	`
	diags := analyzeFSubValidationSource(t, src)
	if countDiagCode(diags, string(diag.CodeE220)) != 1 {
		t.Fatalf("expected one fsub diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), "supported placeholders are %d, %f, %w and %%") {
		t.Fatalf("missing supported-placeholder diagnostic: %s", diags.String())
	}
}

func analyzeFSubValidationSource(t *testing.T, src string) *diag.Diagnostics {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse("fsub.jbs", src, diags)
	_ = Analyze(prog, map[string]eval.Value{
		"jbs_name":  eval.String("bench"),
		"jbs_nproc": eval.Int(0),
	}, diags)
	return diags
}
