package sema

import (
	"fmt"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/parser"
)

func analyzeRefValidationSource(t *testing.T, file, src string) (*Result, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	prog := parser.Parse(file, src, diags)
	res := Analyze(prog, map[string]eval.Value{
		"jbs_name":    eval.String("jbs_benchmark"),
		"jbs_outpath": eval.String("out"),
		"jbs_comment": eval.String(""),
	}, diags)
	return res, diags
}

func hasWarningWithParts(diags *diag.Diagnostics, code diag.Code, parts ...string) bool {
	if diags == nil {
		return false
	}
	for _, item := range diags.Items {
		if item.Code != string(code) {
			continue
		}
		match := true
		for _, part := range parts {
			if !strings.Contains(item.Message, part) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func firstWarning(diags *diag.Diagnostics, code diag.Code) (diag.Diagnostic, bool) {
	if diags == nil {
		return diag.Diagnostic{}, false
	}
	for _, item := range diags.Items {
		if item.Code == string(code) {
			return item, true
		}
	}
	return diag.Diagnostic{}, false
}

func hasW310ForGlobal(diags *diag.Diagnostics, variable, source string) bool {
	return hasWarningWithParts(diags, diag.CodeW310,
		fmt.Sprintf("exposed variable '%s'", variable),
		fmt.Sprintf("global '%s'", source),
	)
}

func countWarningsWithParts(diags *diag.Diagnostics, code diag.Code, parts ...string) int {
	if diags == nil {
		return 0
	}
	count := 0
	for _, item := range diags.Items {
		if item.Code != string(code) {
			continue
		}
		match := true
		for _, part := range parts {
			if !strings.Contains(item.Message, part) {
				match = false
				break
			}
		}
		if match {
			count++
		}
	}
	return count
}

func TestValidateStepVarReferencesWarnsMissingImportAndFallsBackToSourceSpan(t *testing.T) {
	xOrigin := diag.NewSpan("refs.jbs", diag.NewPos(10, 2, 3), diag.NewPos(11, 2, 4))
	yOrigin := diag.NewSpan("refs.jbs", diag.NewPos(20, 3, 5), diag.NewPos(21, 3, 6))
	patternSpan := diag.NewSpan("refs.jbs", diag.NewPos(30, 4, 1), diag.NewPos(37, 4, 8))
	bodySpan := diag.NewSpan("refs.jbs", diag.NewPos(40, 6, 1), diag.NewPos(70, 8, 10))

	params := &GlobalBinding{
		Name:  "params",
		Shape: BindingTable,
		Order: []string{"x", "y"},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1)},
			"y": {eval.Int(2)},
		},
		Origins: map[string]diag.Span{
			"x": xOrigin,
			"y": yOrigin,
		},
		Span: bodySpan,
	}
	pattern := &GlobalBinding{
		Name:  "pattern",
		Shape: BindingScalar,
		Order: []string{"pattern"},
		Vars: map[string][]eval.Value{
			"pattern": {eval.String("id=%d")},
		},
		Origins: map[string]diag.Span{
			"pattern": patternSpan,
		},
		Span: patternSpan,
	}
	res := &Result{
		Bindings: []*GlobalBinding{params, pattern},
		BindingsByName: map[string]*GlobalBinding{
			"params":  params,
			"pattern": pattern,
		},
		DoBlocks: []ast.DoBlock{
			{Name: "step_missing", Body: "echo ${x}", BodyStart: diag.NewPos(100, 10, 1), Span: bodySpan},
			{Name: "step_used", Body: "echo ${y}", BodyStart: diag.NewPos(120, 12, 1), Span: bodySpan},
			{Name: "step_unused", Body: "echo done", BodyStart: diag.NewPos(140, 14, 1), Span: bodySpan},
		},
		StepScopeByName: map[string]*StepScopePlan{
			"step_used": {
				StepName: "step_used",
				Effective: map[string]VisibleBinding{
					"y": {Name: "y", Source: "params", SourceVar: "y", Span: yOrigin},
				},
			},
			"step_unused": {
				StepName: "step_unused",
				Effective: map[string]VisibleBinding{
					"y": {Name: "y", Source: "params", SourceVar: "y", Span: diag.Span{}},
				},
				ExplicitDelta: []ScopeImport{{Source: "params", Visible: "y", SourceVar: "y", Span: diag.Span{}}},
			},
		},
		Program: ast.Program{Stmts: []ast.Stmt{
			ast.AnalyseBlock{StepName: "step_used", WithItems: []ast.WithItem{{Source: "pattern", Span: patternSpan}}, Span: bodySpan},
		}},
		SubmitByName: map[string]*SubmitSpec{},
	}

	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)

	if !hasWarningWithParts(diags, diag.CodeW311, "variable 'x'", "step 'step_missing'") {
		t.Fatalf("expected W311 for missing import, got: %s", diags.String())
	}
	w313, ok := firstWarning(diags, diag.CodeW313)
	if !ok {
		t.Fatalf("expected W313 for explicit unused import, got: %s", diags.String())
	}
	if w313.Span != yOrigin {
		t.Fatalf("expected W313 to fall back to source origin span, got %+v want %+v", w313.Span, yOrigin)
	}
	if !hasWarningWithParts(diags, diag.CodeW313, "variable 'y'", "step 'step_unused'") {
		t.Fatalf("expected W313 message for y in step_unused, got: %s", diags.String())
	}
	if hasW310ForGlobal(diags, "pattern", "pattern") {
		t.Fatalf("did not expect W310 for analyse-imported pattern, got: %s", diags.String())
	}
	if hasW310ForGlobal(diags, "x", "params") {
		t.Fatalf("did not expect W310 for candidate-marked x, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesWarnsForMissingInheritedSourceVarAfterRebind(t *testing.T) {
	src := `
cases = table(x = range(5)) + table(y = ("a","b","c"))

do step0
        with cases[x]
{
        echo $x
}

cases = table(a = ("a","b","c"))

do step1
        after step0
        with cases
{
        echo $x $y $a
}
`
	_, diags := analyzeRefValidationSource(t, "rebound_w311.jbs", src)
	if !hasWarningWithParts(diags, diag.CodeW311, "variable 'y'", "step 'step1'") {
		t.Fatalf("expected W311 for old cases.y after rebind, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesStillWarnsForDifferentPublicNameControl(t *testing.T) {
	src := `
cases = table(x = range(5)) + table(y = ("a","b","c"))

do step0
        with cases[x]
{
        echo $x
}

cases0 = table(a = ("a","b","c"))

do step1
        after step0
        with cases0
{
        echo $x $y $a
}
`
	_, diags := analyzeRefValidationSource(t, "control_w311.jbs", src)
	if !hasWarningWithParts(diags, diag.CodeW311, "variable 'y'", "step 'step1'") {
		t.Fatalf("expected W311 control warning for y, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesDoesNotWarnForVisibleReboundVars(t *testing.T) {
	src := `
cases = table(x = range(5)) + table(y = ("a","b","c"))

do step0
        with cases[x]
{
        echo $x
}

cases = table(a = ("a","b","c"))

do step1
        after step0
        with cases
{
        echo $x $a
}
`
	_, diags := analyzeRefValidationSource(t, "rebound_no_w311.jbs", src)
	if countDiagCode(diags, string(diag.CodeW311)) != 0 {
		t.Fatalf("did not expect W311 for inherited x or new cases.a, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesCountsSubmitUseBindingsAndHelperRefs(t *testing.T) {
	span := diag.NewSpan("submit_refs.jbs", diag.NewPos(1, 1, 1), diag.NewPos(5, 5, 5))
	queueBinding := &GlobalBinding{
		Name:  "defaults.queue",
		Shape: BindingScalar,
		Order: []string{"queue"},
		Vars: map[string][]eval.Value{
			"queue": {eval.String("batch")},
		},
		Origins: map[string]diag.Span{"queue": span},
		Span:    span,
	}
	helperBinding := &GlobalBinding{
		Name:  "defaults.helper",
		Shape: BindingScalar,
		Order: []string{"helper"},
		Vars: map[string][]eval.Value{
			"helper": {eval.String("hostname")},
		},
		Origins: map[string]diag.Span{"helper": span},
		Span:    span,
	}
	res := &Result{
		Bindings: []*GlobalBinding{queueBinding, helperBinding},
		BindingsByName: map[string]*GlobalBinding{
			"defaults.queue":  queueBinding,
			"defaults.helper": helperBinding,
		},
		Namespaces: map[string]*Namespace{
			"defaults": {Name: "defaults", Bindings: []string{"defaults.queue", "defaults.helper"}},
		},
		Submits: []ast.SubmitBlock{{
			Name:     "run",
			UseNames: []string{"defaults"},
			Fields: []ast.SubmitField{
				{Name: "args_exec", Op: ast.AssignEq, Expr: ast.IdentExpr{Name: "helper", Span: span}, Span: span},
				{Name: "preprocess", IsRaw: true, Raw: "echo ${helper}", RawStart: span.Start, Span: span},
			},
			Span: span,
		}},
		SubmitByName: map[string]*SubmitSpec{
			"run": {
				Name:    "run",
				Helpers: []SubmitHelper{{Original: "helper", UseName: "defaults", Span: span}},
				Values: []SubmitValue{
					{Name: "launcher", Value: eval.String("$helper"), Span: span},
					{Name: "postprocess", IsRaw: true, Raw: "echo ${helper}", Span: span},
				},
			},
		},
		StepScopeByName: map[string]*StepScopePlan{},
	}

	diags := &diag.Diagnostics{}
	validateStepVarReferences(res, diags)

	if hasW310ForGlobal(diags, "queue", "defaults.queue") || hasW310ForGlobal(diags, "helper", "defaults.helper") {
		t.Fatalf("did not expect W310 for submit-use bindings, got: %s", diags.String())
	}
	if countDiagCode(diags, string(diag.CodeW311)) != 0 {
		t.Fatalf("did not expect W311 for helper refs imported from submit use, got: %s", diags.String())
	}
	if countDiagCode(diags, string(diag.CodeW313)) != 0 {
		t.Fatalf("did not expect W313 for submit-use helper refs, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesUsesVersionedGlobalDependenciesForW310(t *testing.T) {
	src := `
base = (1, 2)
derived = table(base = base)
base = (3, 4)

do s
        with derived
{
        echo $base
}
`
	_, diags := analyzeRefValidationSource(t, "w310_rebind_deps.jbs", src)
	if got := countWarningsWithParts(diags, diag.CodeW310, "exposed variable 'base'", "global 'base'"); got != 1 {
		t.Fatalf("expected exactly one W310 for the later base version, got %d: %s", got, diags.String())
	}
}

func TestValidateStepVarReferencesKeepsTransitiveDependencyVersionsForW310(t *testing.T) {
	src := `
base = (1, 2)
mid = table(base = base)
base = (3, 4)
derived = mid

do s
        with derived
{
        echo $base
}
`
	_, diags := analyzeRefValidationSource(t, "w310_transitive_rebind_deps.jbs", src)
	if got := countWarningsWithParts(diags, diag.CodeW310, "exposed variable 'base'", "global 'base'"); got != 1 {
		t.Fatalf("expected only the later base version to be unused through transitive deps, got %d: %s", got, diags.String())
	}
	if hasW310ForGlobal(diags, "base", "mid") {
		t.Fatalf("did not expect W310 for transitive source mid.base, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesDoesNotSuppressW313AcrossReboundVersions(t *testing.T) {
	src := `
cases = table(x = (1, 2))

do step0
        with cases[x]
{
        echo $x
}

cases = table(x = (3, 4))

do step1
        with cases[x]
{
        echo done
}
`
	_, diags := analyzeRefValidationSource(t, "w313_rebind.jbs", src)
	if countDiagCode(diags, string(diag.CodeW313)) != 0 {
		t.Fatalf("did not expect W313 for unused new cases.x only because old cases.x was used, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesW313UsesPublicRelatedSpanForSnapshotImport(t *testing.T) {
	src := `
cases = table(x = (1, 2))

do step0
        with cases[x]
{
        echo $x
}

do step1
        with cases[x]
{
        echo done
}
`
	_, diags := analyzeRefValidationSource(t, "w313_snapshot_related.jbs", src)
	w313, ok := firstWarning(diags, diag.CodeW313)
	if !ok {
		t.Fatalf("expected W313 for unused snapshot import, got: %s", diags.String())
	}
	if len(w313.Related) == 0 || w313.Related[0].Message != "source 'cases'" {
		t.Fatalf("expected W313 related span to use public source name, got %#v", w313.Related)
	}
	if w313.Related[0].Span.IsZero() {
		t.Fatalf("expected W313 related span to point at source origin, got %#v", w313.Related[0])
	}
}

func TestValidateStepVarReferencesCountsSubmitUseSnapshotBeforeRebind(t *testing.T) {
	src := `
helper = "old"

submit run
        use helper
{
        args_exec = "$helper"
}

helper = "new"
`
	_, diags := analyzeRefValidationSource(t, "submit_rebind_use.jbs", src)
	if got := countWarningsWithParts(diags, diag.CodeW310, "exposed variable 'helper'", "global 'helper'"); got != 1 {
		t.Fatalf("expected only rebound helper version to be unused, got %d: %s", got, diags.String())
	}
	if countDiagCode(diags, string(diag.CodeW311)) != 0 {
		t.Fatalf("did not expect W311 for submit helper from use snapshot, got: %s", diags.String())
	}
}

func TestValidateStepVarReferencesCountsAnalyseSnapshotBeforeRebind(t *testing.T) {
	src := `
pattern = "%d"

do run {
        echo 1
}

analyse run
        with pattern
{
        value = pattern in "out.txt"
        (value)
}

pattern = "%f"
`
	_, diags := analyzeRefValidationSource(t, "analyse_rebind_use.jbs", src)
	if got := countWarningsWithParts(diags, diag.CodeW310, "exposed variable 'pattern'", "global 'pattern'"); got != 1 {
		t.Fatalf("expected only rebound pattern version to be unused, got %d: %s", got, diags.String())
	}
}

func TestValidateStepVarReferencesRealProgramsTrackTransitiveUsage(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		absent [][2]string
	}{
		{
			name: "table_import",
			src: `
x = (1, 2)
a = ("a", "b", "c")

params = product(table(a = a), table(x = x))

do ex_step with params {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}
`,
			absent: [][2]string{{"x", "x"}, {"a", "a"}},
		},
		{
			name: "transitive_chain",
			src: `
x = (1,2)
m = table(x = x)
p = select(m, x)

do s with p { echo ${x} }
`,
			absent: [][2]string{{"x", "x"}, {"x", "m"}},
		},
		{
			name: "qualified_dependency",
			src: `
x = (1,2)
a = ("a","b")
params = product(table(a = a), table(x = x))
only_a = params[a]

do s with only_a { echo ${a} }
`,
			absent: [][2]string{{"a", "a"}, {"x", "x"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := analyzeRefValidationSource(t, tc.name+".jbs", tc.src)
			for _, pair := range tc.absent {
				if hasW310ForGlobal(diags, pair[0], pair[1]) {
					t.Fatalf("did not expect W310 for %s/%s, got: %s", pair[0], pair[1], diags.String())
				}
			}
		})
	}
}

func TestValidateStepVarReferencesRealProgramWarnsForUnusedGlobal(t *testing.T) {
	src := `
x = (1,2)
a = ("a","b")
params = product(table(a = a), table(x = x))
unused = (10,20)

do s with params { echo ${a} ${x} }
`

	_, diags := analyzeRefValidationSource(t, "unused_refs.jbs", src)
	if !hasW310ForGlobal(diags, "unused", "unused") {
		t.Fatalf("expected W310 for unused global, got: %s", diags.String())
	}
	if hasW310ForGlobal(diags, "x", "x") || hasW310ForGlobal(diags, "a", "a") {
		t.Fatalf("did not expect W310 for used globals, got: %s", diags.String())
	}
}
