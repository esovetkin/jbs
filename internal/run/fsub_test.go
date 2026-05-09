package run

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestApplyFSubRuleReplacesWholeMatchLiterally(t *testing.T) {
	rule := FileSubstitutionRulePlan{
		Pattern: "###X###",
		Regex:   regexp.MustCompile("###X###"),
		Expr:    ast.StringExpr{Value: `$1\${x}`},
	}
	got, matches, err := applyFSubRule("a ###X### b", rule, nil)
	if err != nil {
		t.Fatal(err)
	}
	if matches != 1 || got != `a $1\${x} b` {
		t.Fatalf("replacement = %q matches=%d", got, matches)
	}
}

func TestApplyFSubRuleReplacesCaptureGroups(t *testing.T) {
	rule := FileSubstitutionRulePlan{
		Pattern: `x = ([0-9]+), y = ([A-Za-z]+)`,
		Regex:   regexp.MustCompile(`x = ([0-9]+), y = ([A-Za-z]+)`),
		Expr: ast.TupleExpr{Items: []ast.Expr{
			ast.IdentExpr{Name: "x"},
			ast.IdentExpr{Name: "y"},
		}},
	}
	got, matches, err := applyFSubRule("x = 1, y = old", rule, map[string]eval.Value{
		"x": eval.Int(42),
		"y": eval.String("new"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if matches != 1 || got != "x = 42, y = new" {
		t.Fatalf("replacement = %q matches=%d", got, matches)
	}
}

func TestApplyFSubRuleRejectsMismatchedAndOverlappingGroups(t *testing.T) {
	mismatch := FileSubstitutionRulePlan{
		Pattern: `x=([0-9]+)([A-Z]+)`,
		Regex:   regexp.MustCompile(`x=([0-9]+)([A-Z]+)`),
		Expr:    ast.IdentExpr{Name: "x"},
	}
	if _, _, err := applyFSubRule("x=1A", mismatch, map[string]eval.Value{"x": eval.Int(1)}); err == nil || !strings.Contains(err.Error(), "2 capture groups") {
		t.Fatalf("expected capture-count error, got %v", err)
	}

	overlap := FileSubstitutionRulePlan{
		Pattern: `(a(b))`,
		Regex:   regexp.MustCompile(`(a(b))`),
		Expr: ast.TupleExpr{Items: []ast.Expr{
			ast.StringExpr{Value: "x"},
			ast.StringExpr{Value: "y"},
		}},
	}
	if _, _, err := applyFSubRule("ab", overlap, nil); err == nil || !strings.Contains(err.Error(), "overlapping capture groups") {
		t.Fatalf("expected overlap error, got %v", err)
	}
}

func TestMaterializeFileSubstitutionsWritesOutputAndWarnings(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "input.tpl")
	if err := os.WriteFile(template, []byte("X X\nY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specs := []FileSubstitutionPlan{{
		SourcePath: template,
		DestName:   "input.tpl",
		Rules: []FileSubstitutionRulePlan{
			{Pattern: "X", Regex: regexp.MustCompile("X"), Expr: ast.IdentExpr{Name: "x"}},
			{Pattern: "Y", Regex: regexp.MustCompile("Y"), Expr: ast.StringExpr{Value: "done"}},
		},
	}}
	warnings, err := materializeFileSubstitutions(workDir, ManifestWork{Step: "s", Row: 3}, specs, map[string]eval.Value{"x": eval.Int(7)})
	if err != nil {
		t.Fatal(err)
	}
	if got := readRunTestFile(t, filepath.Join(workDir, "input.tpl")); got != "7 7\ndone\n" {
		t.Fatalf("output = %q", got)
	}
	if len(warnings) != 1 || warnings[0].Matches != 2 || warnings[0].Row != 3 {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestMaterializeFileSubstitutionsRejectsZeroMatches(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "input.tpl")
	if err := os.WriteFile(template, []byte("none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specs := []FileSubstitutionPlan{{
		SourcePath: template,
		DestName:   "input.tpl",
		Rules: []FileSubstitutionRulePlan{{
			Pattern: "missing",
			Regex:   regexp.MustCompile("missing"),
			Expr:    ast.StringExpr{Value: "x"},
		}},
	}}
	if _, err := materializeFileSubstitutions(workDir, ManifestWork{Step: "s", Row: 0}, specs, nil); err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("expected zero-match error, got %v", err)
	}
}

func TestRuntimePlanFiltersFSubTemplatesByBenchmark(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "kept.tpl"), []byte("X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	suite, err := buildRuntimeSuiteFromSource(t, cwd, `
jbs_name = "bench"
jbs_benchmarks = {"small": "run_small"}

do run_small
        fsub "kept.tpl" { "X": "small" }
{
        echo ok > out.log
}

do run_large
        fsub "missing.tpl" { "X": "large" }
{
        echo ok > out.log
}

analyse run_small {
        ()
}
`, "small")
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Plans) != 1 {
		t.Fatalf("plans = %d", len(suite.Plans))
	}
	plan := suite.Plans[0]
	if len(plan.FileSubs) != 1 || len(plan.FileSubs["run_small"]) != 1 {
		t.Fatalf("filtered fsubs = %#v", plan.FileSubs)
	}
	if _, ok := plan.FileSubs["run_large"]; ok {
		t.Fatalf("unselected fsub was retained: %#v", plan.FileSubs)
	}
}

func buildRuntimeSuiteFromSource(t *testing.T, cwd, source, benchmark string) (runtimeSuitePlan, error) {
	t.Helper()
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("test.jbs", strings.TrimSpace(source)+"\n", cwd, cwd, diags)
	if err != nil {
		t.Fatal(err)
	}
	res := sema.AnalyzeWithImports(loadRes, sema.BuiltinGlobalValues(), diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	return buildRuntimeSuitePlan(Options{
		Result:      res,
		Sources:     loadRes.Sources,
		ProgramFile: filepath.Join(cwd, "test.jbs"),
		Benchmark:   benchmark,
	}, diags)
}

func readRunTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
