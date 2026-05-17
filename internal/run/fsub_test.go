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
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

func TestFileSubPlansByStepResolvesPathsAndCompilesRules(t *testing.T) {
	base := t.TempDir()
	span := diag.Span{File: "module.jbs"}
	res := &sema.Result{
		BaseDirByFile: map[string]string{"module.jbs": base},
		DoBlocks: []ast.DoBlock{
			{Name: "empty"},
			{
				Name: "s",
				Span: span,
				FSubs: []ast.FileSubstitution{{
					Path: "templates/input.tpl",
					Rules: []ast.FileSubstitutionRule{{
						Pattern: "X+",
						Expr:    ast.StringExpr{Value: "y"},
					}},
					Span: span,
				}},
			},
		},
	}

	plans, err := fileSubPlansByStep(res)
	if err != nil {
		t.Fatal(err)
	}
	specs := plans["s"]
	if len(specs) != 1 {
		t.Fatalf("plans = %#v", plans)
	}
	if got, want := specs[0].SourcePath, filepath.Join(base, "templates", "input.tpl"); got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}
	if specs[0].DestName != "input.tpl" || len(specs[0].Rules) != 1 || !specs[0].Rules[0].Regex.MatchString("XX") {
		t.Fatalf("unexpected plan: %#v", specs[0])
	}
	if _, ok := plans["empty"]; ok {
		t.Fatalf("empty step unexpectedly planned: %#v", plans)
	}

	empty, err := fileSubPlansByStep(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("nil result plans = %#v", empty)
	}
}

func TestCompileFSubRulesRejectsInvalidRegex(t *testing.T) {
	_, err := compileFSubRules([]ast.FileSubstitutionRule{{
		Pattern: "(",
		Expr:    ast.StringExpr{Value: "x"},
	}})
	if err == nil || !strings.Contains(err.Error(), "invalid fsub regex") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestFileSubBaseDirAndTemplatePathFallbacks(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	span := diag.Span{File: "file.jbs"}
	if got := fileSubBaseDir(&sema.Result{BaseDirByFile: map[string]string{"file.jbs": "  " + base + "  "}}, span); got != filepath.Clean(base) {
		t.Fatalf("base dir from result = %q, want %q", got, filepath.Clean(base))
	}

	src := filepath.Join(t.TempDir(), "nested", "case.jbs")
	if got, want := fileSubBaseDir(nil, diag.Span{File: src}), filepath.Dir(filepath.Clean(src)); got != want {
		t.Fatalf("base dir from span = %q, want %q", got, want)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fileSubBaseDir(nil, diag.Span{File: "<repl>"}), filepath.Clean(cwd); got != want {
		t.Fatalf("base dir from cwd = %q, want %q", got, want)
	}

	abs := filepath.Join(t.TempDir(), "input.tpl")
	if got := resolveFSubTemplatePath("ignored", abs); got != filepath.Clean(abs) {
		t.Fatalf("absolute template path = %q", got)
	}
	if got, want := resolveFSubTemplatePath("", "input.tpl"), filepath.Clean("input.tpl"); got != want {
		t.Fatalf("relative template path = %q, want %q", got, want)
	}
}

func TestCloneFileSubstitutionPlansCopiesRules(t *testing.T) {
	if got := cloneFileSubstitutionPlans(nil); got != nil {
		t.Fatalf("nil clone = %#v", got)
	}
	in := []FileSubstitutionPlan{{
		SourcePath: "in.tpl",
		DestName:   "out.tpl",
		Rules: []FileSubstitutionRulePlan{{
			Pattern: "X",
			Regex:   regexp.MustCompile("X"),
			Expr:    ast.StringExpr{Value: "x"},
		}},
	}}
	cloned := cloneFileSubstitutionPlans(in)
	cloned[0].Rules[0].Pattern = "Y"
	if in[0].Rules[0].Pattern != "X" {
		t.Fatalf("clone mutated source: %#v", in)
	}
}

func TestSourceHashWithFileSubsIncludesTemplatesAndRejectsInvalidSource(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "input.tpl")
	if err := os.WriteFile(template, []byte("X\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(template, 0o755); err != nil {
		t.Fatal(err)
	}

	hash, templates, err := sourceHashWithFileSubs(map[string]string{"main.jbs": "do s {}"}, map[string][]FileSubstitutionPlan{
		"s": {{
			SourcePath: template,
			DestName:   "input.tpl",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("empty source hash")
	}
	if len(templates) != 1 || templates[0].Step != "s" || templates[0].SourcePath != filepath.Clean(template) || templates[0].SHA256 != sha256Hex([]byte("X\n")) {
		t.Fatalf("template hashes = %#v", templates)
	}
	if templates[0].Mode != "0755" {
		t.Fatalf("template mode = %q, want 0755", templates[0].Mode)
	}

	_, _, err = sourceHashWithFileSubs(nil, map[string][]FileSubstitutionPlan{
		"s": {{SourcePath: filepath.Join(dir, "missing.tpl"), DestName: "missing.tpl"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing template error, got %v", err)
	}
}

func TestValidateFSubTemplateSourceRejectsDirectory(t *testing.T) {
	err := validateFSubTemplateSource(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected regular-file error, got %v", err)
	}
}

func TestValidateTemplateHashes(t *testing.T) {
	stored := []TemplateHash{{Step: "s", SourcePath: "input.tpl", DestName: "input.tpl", SHA256: "old"}}
	current := []TemplateHash{{Step: "s", SourcePath: filepath.Clean("input.tpl"), DestName: "input.tpl", SHA256: "old"}}
	if err := validateTemplateHashes("run", stored, current); err != nil {
		t.Fatal(err)
	}
	legacyStored := []TemplateHash{{Step: "s", SourcePath: "input.tpl", DestName: "input.tpl", SHA256: "old"}}
	currentWithMode := []TemplateHash{{Step: "s", SourcePath: filepath.Clean("input.tpl"), DestName: "input.tpl", SHA256: "old", Mode: "0755"}}
	if err := validateTemplateHashes("run", legacyStored, currentWithMode); err != nil {
		t.Fatalf("legacy manifest should skip mode comparison: %v", err)
	}

	cases := []struct {
		name    string
		stored  []TemplateHash
		current []TemplateHash
		want    string
	}{
		{
			name:    "removed",
			stored:  stored,
			current: nil,
			want:    "no longer configured",
		},
		{
			name:    "changed",
			stored:  stored,
			current: []TemplateHash{{Step: "s", SourcePath: "input.tpl", DestName: "input.tpl", SHA256: "new"}},
			want:    "hash does not match",
		},
		{
			name:    "added",
			stored:  nil,
			current: current,
			want:    "was not part of the prepared run",
		},
		{
			name:    "mode",
			stored:  []TemplateHash{{Step: "s", SourcePath: "input.tpl", DestName: "input.tpl", SHA256: "old", Mode: "0755"}},
			current: []TemplateHash{{Step: "s", SourcePath: "input.tpl", DestName: "input.tpl", SHA256: "old", Mode: "0644"}},
			want:    "mode does not match",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTemplateHashes("run", tc.stored, tc.current)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestWorkValuesByKey(t *testing.T) {
	values := map[string]eval.Value{"x": eval.Int(1)}
	got := workValuesByKey(workplan.Plan{Work: []workplan.WorkPackage{{
		ID:       workplan.WorkID{Step: "ignored", Row: 2},
		StepName: "s",
		Values:   values,
	}}})
	if got["s/000002"]["x"].I != 1 {
		t.Fatalf("work values = %#v", got)
	}
}

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

func TestApplyFSubRuleRejectsInvalidReplacementShapes(t *testing.T) {
	cases := []struct {
		name string
		rule FileSubstitutionRulePlan
		text string
		want string
	}{
		{
			name: "non scalar list item",
			rule: FileSubstitutionRulePlan{
				Pattern: "X",
				Regex:   regexp.MustCompile("X"),
				Expr: ast.ListExpr{Items: []ast.Expr{
					ast.ListExpr{Items: []ast.Expr{ast.StringExpr{Value: "x"}}},
				}},
			},
			text: "X",
			want: "must be scalar or tuple/list of scalars",
		},
		{
			name: "tuple without capture groups",
			rule: FileSubstitutionRulePlan{
				Pattern: "X",
				Regex:   regexp.MustCompile("X"),
				Expr: ast.TupleExpr{Items: []ast.Expr{
					ast.StringExpr{Value: "a"},
					ast.StringExpr{Value: "b"},
				}},
			},
			text: "X",
			want: "no capture groups",
		},
		{
			name: "optional group not matched",
			rule: FileSubstitutionRulePlan{
				Pattern: `x=(a)?`,
				Regex:   regexp.MustCompile(`x=(a)?`),
				Expr:    ast.StringExpr{Value: "b"},
			},
			text: "x=",
			want: "matched without capture group",
		},
		{
			name: "shell call",
			rule: FileSubstitutionRulePlan{
				Pattern: "X",
				Regex:   regexp.MustCompile("X"),
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "shell"},
					Args:   ast.PosCallArgs(ast.StringExpr{Value: "echo x"}),
				},
			},
			text: "X",
			want: "shell() is not supported",
		},
		{
			name: "unknown variable",
			rule: FileSubstitutionRulePlan{
				Pattern: "X",
				Regex:   regexp.MustCompile("X"),
				Expr:    ast.IdentExpr{Name: "missing"},
			},
			text: "X",
			want: "unknown variable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := applyFSubRule(tc.text, tc.rule, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestReplacementPartsAcceptsScalarKindsAndRejectsNull(t *testing.T) {
	cases := []struct {
		name  string
		value eval.Value
		want  []string
		ok    bool
	}{
		{name: "string", value: eval.String("x"), want: []string{"x"}, ok: true},
		{name: "int", value: eval.Int(3), want: []string{"3"}, ok: true},
		{name: "float", value: eval.Float(1.25), want: []string{"1.25"}, ok: true},
		{name: "bool", value: eval.Bool(true), want: []string{"true"}, ok: true},
		{name: "null", value: eval.Null(), ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := replacementParts(tc.value)
			if ok != tc.ok || strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("replacementParts(%s) = %#v, %v", tc.value.Kind, got, ok)
			}
		})
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

func TestMaterializeFileSubstitutionsPreservesTemplatePermissions(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "tool.sh")
	if err := os.WriteFile(template, []byte("#!/usr/bin/env bash\necho TOKEN\n"), 0o751); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(template, 0o751); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	specs := []FileSubstitutionPlan{{
		SourcePath: template,
		DestName:   "tool.sh",
		Rules: []FileSubstitutionRulePlan{{
			Pattern: "TOKEN",
			Regex:   regexp.MustCompile("TOKEN"),
			Expr:    ast.StringExpr{Value: "ok"},
		}},
	}}
	if _, err := materializeFileSubstitutions(workDir, ManifestWork{Step: "s", Row: 0}, specs, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(workDir, "tool.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o751 {
		t.Fatalf("mode = %04o, want 0751", got)
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

func TestMaterializeFileSubstitutionsReportsReadAndWriteErrors(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := FileSubstitutionRulePlan{
		Pattern: "X",
		Regex:   regexp.MustCompile("X"),
		Expr:    ast.StringExpr{Value: "ok"},
	}

	_, err := materializeFileSubstitutions(workDir, ManifestWork{Step: "s", Row: 0}, []FileSubstitutionPlan{{
		SourcePath: filepath.Join(dir, "missing.tpl"),
		DestName:   "input.tpl",
		Rules:      []FileSubstitutionRulePlan{rule},
	}}, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing-template error, got %v", err)
	}

	template := filepath.Join(dir, "input.tpl")
	if err := os.WriteFile(template, []byte("X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = materializeFileSubstitutions(workDir, ManifestWork{Step: "s", Row: 0}, []FileSubstitutionPlan{{
		SourcePath: template,
		DestName:   filepath.Join("missing", "input.tpl"),
		Rules:      []FileSubstitutionRulePlan{rule},
	}}, nil)
	if err == nil || !strings.Contains(err.Error(), "write fsub output") {
		t.Fatalf("expected write error, got %v", err)
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
