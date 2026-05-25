package sema

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
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

func TestValidateFileSubstitutionsIgnoresFunctionParameterRefs(t *testing.T) {
	src := `
x = 1

do s with x
        fsub "template.txt" { "TOKEN": map(function(v) { v + 1 }, [x])[0] }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsChecksCapturedFunctionRefs(t *testing.T) {
	src := `
x = 1

do s
        fsub "template.txt" { "TOKEN": map(function(v) { v + x }, [1])[0] }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if countDiagCode(diags, string(diag.CodeE220)) != 1 {
		t.Fatalf("expected missing captured x diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `references variable "x" that is not visible`) {
		t.Fatalf("missing captured x diagnostic: %s", diags.String())
	}
	if strings.Contains(diags.String(), `references variable "v"`) {
		t.Fatalf("function parameter should not be reported: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsNestedFunctionCapturesVisibleRef(t *testing.T) {
	src := `
x = 1

do s with x
        fsub "template.txt" {
                "TOKEN": map(function(v) {
                        inner = function(w) { w + v + x }
                        inner(1)
                }, [1])[0]
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

func TestValidateFileSubstitutionsFunctionRefsRespectWithAlias(t *testing.T) {
	valid := `
x = 1

do s with x as y
        fsub "template.txt" { "TOKEN": map(function(v) { v + y }, [1])[0] }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, valid)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}

	invalid := `
x = 1

do s with x as y
        fsub "template.txt" { "TOKEN": map(function(v) { v + x }, [1])[0] }
{
        :
}
`
	diags = analyzeFSubValidationSource(t, invalid)
	if countDiagCode(diags, string(diag.CodeE220)) != 1 {
		t.Fatalf("expected invisible original-name diagnostic, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `references variable "x" that is not visible`) {
		t.Fatalf("missing invisible original-name diagnostic: %s", diags.String())
	}
	if strings.Contains(diags.String(), `references variable "v"`) {
		t.Fatalf("function parameter should not be reported: %s", diags.String())
	}
}

func TestValidateFileSubstitutionsFunctionLocalsDoNotSuppressUnusedImportWarnings(t *testing.T) {
	span := diag.NewSpan("fsub_usage.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	sourceKey := BindingVersionKey{Public: "cases", Version: "cases:v1"}
	binding := &GlobalBinding{
		Name:       "cases",
		PublicName: "cases",
		Shape:      BindingTable,
		Order:      []string{"v", "x"},
		Vars: map[string][]eval.Value{
			"v": {eval.Int(10)},
			"x": {eval.Int(1)},
		},
		Origins: map[string]diag.Span{
			"v": span,
			"x": span,
		},
		Span:      span,
		VersionID: "cases:v1",
	}
	expr := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "map", Span: span},
		Args: ast.PosCallArgs(
			ast.FunctionExpr{
				Params: []ast.FuncParam{{Name: "v", Span: span}},
				Body: []ast.FuncBodyStmt{
					ast.ExprStmt{Expr: ast.BinaryExpr{
						Left:  ast.IdentExpr{Name: "v", Span: span},
						Op:    "+",
						Right: ast.IdentExpr{Name: "x", Span: span},
						Span:  span,
					}, Span: span},
				},
				Span: span,
			},
			ast.ListExpr{Items: []ast.Expr{
				ast.NumberExpr{Raw: "1", Int: true, IntValue: 1, Span: span},
			}, Span: span},
		),
		Span: span,
	}
	res := &Result{
		Program: ast.Program{File: "fsub_usage.jbs"},
		Bindings: []*GlobalBinding{
			binding,
		},
		BindingsByName: map[string]*GlobalBinding{
			"cases": binding,
		},
		BindingsByKey: map[BindingVersionKey]*GlobalBinding{
			sourceKey: binding,
		},
		DoBlocks: []ast.DoBlock{{
			Name: "s",
			Body: ":",
			FSubs: []ast.FileSubstitution{{
				Rules: []ast.FileSubstitutionRule{{Expr: expr}},
			}},
			Span: span,
		}, {
			Name:      "usev",
			Body:      "echo $v",
			BodyStart: span.Start,
			Span:      span,
		}},
		StepScopeByName: map[string]*StepScopePlan{
			"s": {
				StepName: "s",
				Effective: map[string]VisibleBinding{
					"v": {Name: "v", Source: "cases", SourceVar: "v", SourceKey: sourceKey, Span: span},
					"x": {Name: "x", Source: "cases", SourceVar: "x", SourceKey: sourceKey, Span: span},
				},
				ExplicitDelta: []ScopeImport{
					{Source: "cases", SourceKey: sourceKey, Visible: "v", SourceVar: "v", Span: span},
					{Source: "cases", SourceKey: sourceKey, Visible: "x", SourceVar: "x", Span: span},
				},
			},
			"usev": {
				StepName: "usev",
				Effective: map[string]VisibleBinding{
					"v": {Name: "v", Source: "cases", SourceVar: "v", SourceKey: sourceKey, Span: span},
				},
			},
		},
	}
	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)
	if countDiagCode(diags, string(diag.CodeW313)) != 1 {
		t.Fatalf("expected exactly one unused-import warning, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `variable 'v' is imported`) {
		t.Fatalf("expected unused-import warning for v, got: %s", diags.String())
	}
	if strings.Contains(diags.String(), `variable 'x' is imported`) {
		t.Fatalf("did not expect unused-import warning for x, got: %s", diags.String())
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

func TestValidateFileSubstitutionsRejectsReservedDestinations(t *testing.T) {
	src := `
do run
        fsub "stdout" { "TOKEN": "ok" }
        fsub "run.sh" { "TOKEN": "ok" }
{
        :
}
`
	diags := analyzeFSubValidationSource(t, src)
	if countDiagCode(diags, string(diag.CodeE220)) != 2 {
		t.Fatalf("expected two reserved-destination diagnostics, got: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `fsub destination "stdout" is reserved`) {
		t.Fatalf("missing stdout reserved diagnostic: %s", diags.String())
	}
	if !strings.Contains(diags.String(), `fsub destination "run.sh" is reserved`) {
		t.Fatalf("missing run.sh reserved diagnostic: %s", diags.String())
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
