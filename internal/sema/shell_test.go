package sema

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

func TestAnalyzeShellCapturesScalarVariablesAndDependencies(t *testing.T) {
	seen := make([]string, 0, 2)
	runner := func(spec eval.ShellCommand) ([]byte, error) {
		env := shellTestEnvMap(spec.Env)
		seen = append(seen, env["x"])
		return []byte(env["x"] + "\n"), nil
	}
	src := strings.Join([]string{
		"x = 1",
		`y = shell("echo $x")`,
		"x = 2",
		`z = shell("echo $x")`,
		"",
	}, "\n")
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := AnalyzeWithOptions(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, AnalyzeOptions{ShellRunner: runner}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !slices.Equal(seen, []string{"1", "2"}) {
		t.Fatalf("unexpected captured env values: %#v", seen)
	}
	if got := res.GlobalVarByName["y"].Value; got.Kind != eval.KindString || got.S != "1" {
		t.Fatalf("unexpected y value: %#v", got)
	}
	if got := res.GlobalVarByName["z"].Value; got.Kind != eval.KindString || got.S != "2" {
		t.Fatalf("unexpected z value: %#v", got)
	}
	if !slices.Contains(res.GlobalVarByName["y"].DependsOn, "x") {
		t.Fatalf("expected y to depend on x, got %#v", res.GlobalVarByName["y"].DependsOn)
	}
	if !slices.Contains(res.GlobalVarByName["z"].DependsOn, "x") {
		t.Fatalf("expected z to depend on x, got %#v", res.GlobalVarByName["z"].DependsOn)
	}
}

func TestAnalyzeShellLeavesUnassignedLocalsAsShellVariables(t *testing.T) {
	t.Setenv("x", "from-os")
	runner := func(spec eval.ShellCommand) ([]byte, error) {
		return []byte(shellTestEnvMap(spec.Env)["x"] + "\n"), nil
	}
	src := strings.Join([]string{
		`f = function() {`,
		`        y = shell("printf $x")`,
		`        x = 1`,
		`        y`,
		`}`,
		`out = f()`,
		"",
	}, "\n")
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := AnalyzeWithOptions(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, AnalyzeOptions{ShellRunner: runner}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if diagCountCode(diags, "E100") != 0 || diagCountCode(diags, "W103") != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got := res.GlobalVarByName["out"].Value; got.Kind != eval.KindString || got.S != "from-os" {
		t.Fatalf("unexpected out value: %#v", got)
	}
	if slices.Contains(res.GlobalVarByName["out"].DependsOn, "x") {
		t.Fatalf("unassigned local shell variable should not create global x dependency: %#v", res.GlobalVarByName["out"].DependsOn)
	}
}

func TestAnalyzeWithImportsShellRunnerAndModuleWorkingDir(t *testing.T) {
	cwd := t.TempDir()
	libDir := filepath.Join(cwd, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	libPath := filepath.Join(libDir, "lib.jbs")
	if err := os.WriteFile(libPath, []byte(`host = shell("pwd")`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs := make([]string, 0, 1)
	runner := func(spec eval.ShellCommand) ([]byte, error) {
		dirs = append(dirs, spec.Dir)
		return []byte(spec.Dir + "\n"), nil
	}
	diags := &diag.Diagnostics{}
	loadRes, err := imports.LoadAndExpandSource("<repl>", `use "./lib/lib.jbs" as lib`+"\nvalue = lib.host\n", cwd, cwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpandSource failed: %v", err)
	}
	res := AnalyzeWithImportsOptions(loadRes, map[string]eval.Value{"jbs_name": eval.String("bench")}, AnalyzeOptions{ShellRunner: runner}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !slices.Equal(dirs, []string{libDir}) {
		t.Fatalf("unexpected shell working dirs: %#v", dirs)
	}
	if got := res.GlobalVarByName["value"].Value; got.Kind != eval.KindString || got.S != libDir {
		t.Fatalf("unexpected imported shell value: %#v", got)
	}
	if len(res.PrintEvents) != 0 {
		t.Fatalf("shell runner propagation should not change print collection, got %#v", res.PrintEvents)
	}
}

func TestAnalyzeAssumeStringShellDoesNotRunAndKeepsDependencies(t *testing.T) {
	src := strings.Join([]string{
		"x = 1",
		`y = shell("printf $x")`,
		`do s with y {`,
		`  echo "$y"`,
		`}`,
		"",
	}, "\n")
	diags := &diag.Diagnostics{}
	prog := parser.Parse("in.jbs", src, diags)
	res := AnalyzeWithOptions(prog, map[string]eval.Value{"jbs_name": eval.String("bench")}, AnalyzeOptions{
		ShellMode: eval.ShellAssumeString,
		ShellRunner: func(eval.ShellCommand) ([]byte, error) {
			t.Fatal("shell runner should not be called")
			return nil, nil
		},
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got := res.GlobalVarByName["y"].Value; got.Kind != eval.KindString || got.S != "" {
		t.Fatalf("unexpected y value: %#v", got)
	}
	if !slices.Contains(res.GlobalVarByName["y"].DependsOn, "x") {
		t.Fatalf("expected y to depend on x, got %#v", res.GlobalVarByName["y"].DependsOn)
	}
}

func TestCompileAnalyseBlockAssumeStringShellDoesNotRun(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		Globals:         GlobalState{Values: map[string]eval.Value{}},
		BindingsByName:  map[string]*GlobalBinding{},
		DoBlocks:        []ast.DoBlock{{Name: "run", Span: span}},
		StepScopeByName: map[string]*StepScopePlan{"run": {Effective: map[string]VisibleBinding{}}},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{
				Name: "parsed",
				File: "out.txt",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "shell", Span: span},
					Args:   ast.PosCallArgs(ast.StringExpr{Value: "touch marker; printf '%d'", Span: span}),
					Span:   span,
				},
				Span: span,
			},
		},
		Columns: []ast.AnalyseColumn{{Name: "parsed", Span: span}},
		Span:    span,
	}
	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, AnalyzeOptions{
		ShellMode: eval.ShellAssumeString,
		ShellRunner: func(eval.ShellCommand) ([]byte, error) {
			t.Fatal("shell runner should not be called")
			return nil, nil
		},
	}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(spec.Assignments) != 1 {
		t.Fatalf("expected one analyse assignment, got %#v", spec.Assignments)
	}
	if strings.Contains(diags.String(), "shell() command failed") {
		t.Fatalf("unexpected shell failure diagnostic: %s", diags.String())
	}
}

func shellTestEnvMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}

func diagCountCode(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == code {
			count++
		}
	}
	return count
}
