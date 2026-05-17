package cli

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	jbsrun "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/run"
)

func TestRunCommandCreatesAndExecutesBenchmark(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x=[1, 2])`,
		`do run with cases nproc 1 {`,
		`echo "x=$x"`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	progressOut := stdout.String()
	if !strings.Contains(progressOut, "0% (0/2)") {
		t.Fatalf("expected initial progress output, got %q", progressOut)
	}
	if !strings.Contains(progressOut, "100% (2/2)") {
		t.Fatalf("expected final progress output, got %q", progressOut)
	}

	status := readRootStatus(t, filepath.Join(cwd, "bench", "000000", "status"))
	if status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected root status: %#v", status)
	}
	out0, err := os.ReadFile(filepath.Join(cwd, "bench", "000000", "run", "000000", "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	out1, err := os.ReadFile(filepath.Join(cwd, "bench", "000000", "run", "000001", "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out0) != "x=1\n" || string(out1) != "x=2\n" {
		t.Fatalf("unexpected stdout files: %q %q", string(out0), string(out1))
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "000000", "run", "000000", "exitcode")); err != nil {
		t.Fatalf("expected exitcode after run: %v", err)
	}
}

func TestRunCommandCreatesConfiguredBenchmarkComponents(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "[small]\n") || !strings.Contains(out, "[large]\n") {
		t.Fatalf("expected component progress headers, got %q", out)
	}
	for _, component := range []string{"small", "large"} {
		status := readRootStatus(t, filepath.Join(cwd, "bench", component, "000000", "status"))
		if status.Status != jbsrun.StatusFinished {
			t.Fatalf("%s status = %#v", component, status)
		}
		if _, err := os.Stat(filepath.Join(cwd, "bench", component, "000000", "unrelated")); !os.IsNotExist(err) {
			t.Fatalf("%s should not contain unrelated step, stat error: %v", component, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "small", "000000", "run_large")); !os.IsNotExist(err) {
		t.Fatalf("small component should not contain large step, stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "large", "000000", "run_small")); !os.IsNotExist(err) {
		t.Fatalf("large component should not contain small step, stat error: %v", err)
	}
	smallCSV := readFileString(t, filepath.Join(cwd, "bench", "small", "000000", "run_small", "analyse.csv"))
	if smallCSV != "run_id,value\n000000,1\n" {
		t.Fatalf("unexpected small analyse csv: %q", smallCSV)
	}
	largeCSV := readFileString(t, filepath.Join(cwd, "bench", "large", "000000", "run_large", "analyse.csv"))
	if largeCSV != "run_id,value\n000000,2\n" {
		t.Fatalf("unexpected large analyse csv: %q", largeCSV)
	}
}

func TestRunCommandSelectsConfiguredBenchmark(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-b", "small", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("selected run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "small", "000000", "status")); err != nil {
		t.Fatalf("expected small component run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "large")); !os.IsNotExist(err) {
		t.Fatalf("large component should not be created, stat error: %v", err)
	}
	if strings.Contains(stdout.String(), "[small]") {
		t.Fatalf("single selected component should not print component header: %q", stdout.String())
	}
}

func TestRunCommandSelectsConfiguredDoOnlyBenchmark(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeDoOnlyBenchmarkInput(t, cwd)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-b", "smoke", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("selected do-only run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	status := readRootStatus(t, filepath.Join(cwd, "bench", "smoke", "000000", "status"))
	if status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected do-only root status: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "smoke", "000000", "prepare")); err != nil {
		t.Fatalf("expected prepare step: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "smoke", "000000", "run")); !os.IsNotExist(err) {
		t.Fatalf("do-only benchmark should not run dependent child step, stat error: %v", err)
	}
	if strings.Contains(stdout.String(), "analyse.csv") {
		t.Fatalf("do-only benchmark should not print analyse outputs: %q", stdout.String())
	}
}

func TestRunCommandBenchmarkSelectionErrors(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	plain := filepath.Join(cwd, "plain.jbs")
	if err := os.WriteFile(plain, []byte(strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo "value: 1" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-b", "small", plain}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected empty-config selection failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--benchmark requires non-empty jbs_benchmarks") {
		t.Fatalf("unexpected empty-config error: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	input := writeMultiBenchmarkInput(t, cwd, "")
	if code := Run([]string{"run", "--benchmark", "missing", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected missing selection failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `unknown benchmark "missing"`) {
		t.Fatalf("unexpected missing benchmark error: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	badTarget := filepath.Join(cwd, "bad_target.jbs")
	if err := os.WriteFile(badTarget, []byte(strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_benchmarks = {"small": "missing"}`,
		`do run {`,
		`echo run`,
		`}`,
		``,
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"run", badTarget}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected unknown target failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `unknown benchmark target "missing"`) {
		t.Fatalf("unexpected unknown target error: %s", stderr.String())
	}
}

func TestRunCommandDryRunContinueSelectedBenchmark(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", "-b", "small", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	status := readRootStatus(t, filepath.Join(cwd, "bench", "small", "000000", "status"))
	if status.Status != jbsrun.StatusNotStarted {
		t.Fatalf("dry-run status = %#v", status)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "large")); !os.IsNotExist(err) {
		t.Fatalf("large component should not be created by selected dry-run, stat error: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", "-b", "small", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	status = readRootStatus(t, filepath.Join(cwd, "bench", "small", "000000", "status"))
	if status.Status != jbsrun.StatusFinished {
		t.Fatalf("continue status = %#v", status)
	}
}

func TestRunCommandWritesComponentPrefixedAnalyseSQLiteTables(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, `jbs_database = "results.sqlite"`)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	dbPath := filepath.Join(cwd, "results.sqlite")
	assertSQLiteTable(t, dbPath, "bench_small_000000_run_small", []string{"run_id", "value"}, [][]string{{"000000", "1"}})
	assertSQLiteTable(t, dbPath, "bench_large_000000_run_large", []string{"run_id", "value"}, [][]string{{"000000", "2"}})
	if sqliteTableExists(t, dbPath, "bench_000000_run_small") {
		t.Fatalf("unexpected non-component-prefixed sqlite table")
	}
	out := stdout.String()
	if !strings.Contains(out, "results.sqlite:bench_small_000000_run_small") || !strings.Contains(out, "results.sqlite:bench_large_000000_run_large") {
		t.Fatalf("missing sqlite analyse output in stdout: %q", out)
	}
}

func TestRunCommandSupportsBoolConversionInWorkpackages(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`enabled = bool("yes")`,
		`do s with enabled {`,
		`echo "${enabled}"`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "unknown function 'bool'") {
		t.Fatalf("run reported bool as unknown:\n%s", stderr.String())
	}
	workOut := readFileString(t, filepath.Join(cwd, "bench", "000000", "s", "000000", "stdout"))
	if workOut != "true\n" {
		t.Fatalf("expected bool cast value in workpackage stdout, got %q", workOut)
	}
}

func TestRunCommandSupportsShellCompileTimeFunction(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`value = shell("printf hi")`,
		`do s with value {`,
		`printf "%s\n" "$value"`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	workOut := readFileString(t, filepath.Join(cwd, "bench", "000000", "s", "000000", "stdout"))
	if workOut != "hi\n" {
		t.Fatalf("expected shell result in workpackage stdout, got %q", workOut)
	}
	script := readFileString(t, filepath.Join(cwd, "bench", "000000", "s", "000000", "run.sh"))
	if strings.Contains(script, `shell("printf hi")`) {
		t.Fatalf("compile-time shell call leaked into run.sh:\n%s", script)
	}
}

func TestRunShellNonScalarWarningDoesNotAbort(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`xs = [1]`,
		`value = shell("printf ok $xs")`,
		`do s {`,
		`true`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run with shell warning failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "shell() referenced non-scalar JBS variable 'xs'") {
		t.Fatalf("expected non-scalar shell warning, got:\n%s", stderr.String())
	}
	status := readRootStatus(t, filepath.Join(cwd, "bench", "000000", "status"))
	if status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected root status after warning: %#v", status)
	}
}

func TestRunCommandPrintsJBSPrintOutput(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`print("starting", [1, 2, 3, 4], dict(name = "case"))`,
		`do run {`,
		`echo shell`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.HasPrefix(out, "starting [1, 2, 3, 4] {\"name\": \"case\"}\n") {
		t.Fatalf("expected print output before progress, got %q", out)
	}
	workOut := readFileString(t, filepath.Join(cwd, "bench", "000000", "run", "000000", "stdout"))
	if workOut != "shell\n" {
		t.Fatalf("expected shell stdout to stay in workpackage file, got %q", workOut)
	}
}

func TestRunCommandPrintHonorsNRow(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`print(range(100), nrow = 1)`,
		`do run {`,
		`echo shell`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	firstLine := strings.SplitN(stdout.String(), "\n", 2)[0]
	if !strings.HasPrefix(firstLine, "[0, 1, 2") || !strings.HasSuffix(firstLine, "...]") {
		t.Fatalf("expected nrow-truncated print output, got %q", firstLine)
	}
}

func TestRunCommandSupportsEnvFunction(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	t.Setenv("JBS_ENV_RUN_TEST", "from-run")

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`value = env("JBS_ENV_RUN_TEST", "missing")`,
		`print(value)`,
		`do run {`,
		`true`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "from-run\n") {
		t.Fatalf("expected env print output before progress, got %q", stdout.String())
	}
}

func TestRunCommandSupportsDeleteBuiltin(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`delete_me_token = 1`,
		`delete(delete_me_token)`,
		`keep_token = 2`,
		`do s with keep_token {`,
		`echo "keep=$keep_token"`,
		`if [ "${delete_me_token+set}" = set ]; then echo "delete_me_token=$delete_me_token"; fi`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	workDir := filepath.Join(cwd, "bench", "000000", "s", "000000")
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "keep=2\n" {
		t.Fatalf("unexpected work output: %q", got)
	}
	script := readFileString(t, filepath.Join(workDir, "run.sh"))
	if strings.Contains(script, "export delete_me_token=") {
		t.Fatalf("deleted variable leaked into run.sh:\n%s", script)
	}
}

func TestDefaultRunPrintsJBSPrintOutput(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`print("default")`,
		`do run {`,
		`true`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{input}, &stdout, &stderr); code != 0 {
		t.Fatalf("default run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "default\n") {
		t.Fatalf("expected default run print output, got %q", stdout.String())
	}
}

func TestRunContinueDoesNotReplayPrintOutput(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`print("once")`,
		`do run {`,
		`true`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "once") {
		t.Fatalf("continue replayed print output: %q", stdout.String())
	}
}

func TestRunCommandDoesNotPrintWhenRuntimePlanFails(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte("print(\"no work\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run to fail without do blocks")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no print output when runtime plan fails, got %q", stdout.String())
	}
}

func TestRunCommandRunScriptExportsFinalDirectories(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	srcDir := filepath.Join(cwd, "cases")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "payload.txt"), []byte("payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`printf "run=%s\n" "$JBS_RUN_DIR"`,
		`printf "work=%s\n" "$JBS_WORK_DIR"`,
		`printf "src=%s\n" "$JBS_SRC_DIR"`,
		`cat "$JBS_SRC_DIR/payload.txt"`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(srcDir, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	finalRunDir := filepath.Join(cwd, "bench", "000000")
	finalWorkDir := filepath.Join(finalRunDir, "s", "000000")
	script := readFileString(t, filepath.Join(finalWorkDir, "run.sh"))
	if strings.Contains(script, ".creating-") {
		t.Fatalf("run.sh leaked staging directory:\n%s", script)
	}
	if !strings.Contains(script, "export JBS_RUN_DIR='"+finalRunDir+"'") {
		t.Fatalf("run.sh did not export final run dir:\n%s", script)
	}
	if !strings.Contains(script, "export JBS_WORK_DIR='"+finalWorkDir+"'") {
		t.Fatalf("run.sh did not export final work dir:\n%s", script)
	}
	if !strings.Contains(script, "export JBS_SRC_DIR='"+srcDir+"'") {
		t.Fatalf("run.sh did not export absolute source dir %q:\n%s", srcDir, script)
	}

	out := readFileString(t, filepath.Join(finalWorkDir, "stdout"))
	if !strings.Contains(out, "payload\n") {
		t.Fatalf("JBS_SRC_DIR did not resolve payload file from work dir:\n%s", out)
	}
}

func TestRunCommandUsesStrictShellByDefault(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`echo before`,
		`false`,
		`echo after`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected strict run to fail\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	workDir := filepath.Join(runDir, "s", "000000")
	script := readFileString(t, filepath.Join(workDir, "run.sh"))
	if !strings.Contains(script, "\nset -euo pipefail\n\n") {
		t.Fatalf("run.sh missing strict mode:\n%s", script)
	}
	out := readFileString(t, filepath.Join(workDir, "stdout"))
	if !strings.Contains(out, "before\n") || strings.Contains(out, "after\n") {
		t.Fatalf("unexpected strict stdout: %q", out)
	}
	status := readRootStatus(t, filepath.Join(runDir, "status"))
	if status.Status != jbsrun.StatusError {
		t.Fatalf("unexpected strict root status: %#v", status)
	}
}

func TestRunCommandMarksDependentsBlockedAndPrintsStatusSummary(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do prep {`,
		`false`,
		`}`,
		`do run after prep {`,
		`echo should-not-run`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	root := readRootStatus(t, filepath.Join(runDir, "status"))
	if root.Status != jbsrun.StatusError {
		t.Fatalf("root status = %#v, want ERROR", root)
	}
	if status := readWorkStatus(t, filepath.Join(runDir, "prep", "000000", "status")); status.Status != jbsrun.StatusError {
		t.Fatalf("prep status = %#v, want ERROR", status)
	}
	if status := readWorkStatus(t, filepath.Join(runDir, "run", "000000", "status")); status.Status != jbsrun.StatusBlocked {
		t.Fatalf("run status = %#v, want BLOCKED", status)
	}
	out := stdout.String()
	for _, want := range []string{
		"BLOCKED",
		"└── prep",
		"└── run",
		"total:",
		"failed workpackage directories:",
		filepath.Join("bench", "000000", "prep", "000000"),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status summary missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, filepath.Join("bench", "000000", "run", "000000")) {
		t.Fatalf("blocked work directory should not be listed as failed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(runDir, "run", "000000", "exitcode")); !os.IsNotExist(err) {
		t.Fatalf("blocked work should not have exitcode, stat error: %v", err)
	}
}

func TestContinueRetriesBlockedWorkAfterDependencySucceeds(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do prep {`,
		`false`,
		`}`,
		`do run after prep {`,
		`echo child`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected initial run failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}

	runDir := filepath.Join(cwd, "bench", "000000")
	prepScript := filepath.Join(runDir, "prep", "000000", "run.sh")
	if err := os.WriteFile(prepScript, []byte("#!/usr/bin/env bash\ntrue\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if status := readRootStatus(t, filepath.Join(runDir, "status")); status.Status != jbsrun.StatusFinished {
		t.Fatalf("root status = %#v, want FINISHED", status)
	}
	if status := readWorkStatus(t, filepath.Join(runDir, "prep", "000000", "status")); status.Status != jbsrun.StatusFinished {
		t.Fatalf("prep status = %#v, want FINISHED", status)
	}
	if status := readWorkStatus(t, filepath.Join(runDir, "run", "000000", "status")); status.Status != jbsrun.StatusFinished {
		t.Fatalf("run status = %#v, want FINISHED", status)
	}
	if got := readFileString(t, filepath.Join(runDir, "run", "000000", "stdout")); got != "child\n" {
		t.Fatalf("child stdout = %q, want child", got)
	}
}

func TestStatusCommandPrintsLatestRunStatus(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`echo should-not-run`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "NOTSTARTED") || !strings.Contains(out, "└── s") || !strings.Contains(out, "|          1 |") {
		t.Fatalf("status output missing not-started summary:\n%s", out)
	}
	if got := readFileString(t, filepath.Join(cwd, "bench", "000000", "s", "000000", "stdout")); got != "" {
		t.Fatalf("status should not run workpackage, stdout=%q", got)
	}
}

func TestStatusCommandAcceptsBenchmarkDirectory(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`echo should-not-run`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "NOTSTARTED") || !strings.Contains(out, "└── s") || !strings.Contains(out, "|          1 |") {
		t.Fatalf("status output missing not-started summary:\n%s", out)
	}
}

func TestStatusCommandBenchmarkDirectoryDoesNotReadSource(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(strings.Join([]string{
		`jbs_name = "bench"`,
		`print("compile-time")`,
		`do s { echo hi }`,
		``,
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if err := os.WriteFile(input, []byte("do broken {\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "compile-time") {
		t.Fatalf("directory status replayed compile-time print output: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "NOTSTARTED") {
		t.Fatalf("status output missing summary:\n%s", stdout.String())
	}
}

func TestStatusCommandSupportsBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "-b", "small", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "[large]") || strings.Contains(out, "run_large") {
		t.Fatalf("selected status output included large component:\n%s", out)
	}
	if !strings.Contains(out, "run_small") {
		t.Fatalf("selected status output missing small component:\n%s", out)
	}
}

func TestStatusCommandBenchmarkDirectoryListsComponents(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "[small]") || !strings.Contains(out, "[large]") {
		t.Fatalf("status output missing component sections:\n%s", out)
	}
}

func TestStatusCommandBenchmarkDirectorySupportsBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "-b", "small", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "[large]") || strings.Contains(out, "run_large") {
		t.Fatalf("selected status output included large component:\n%s", out)
	}
	if !strings.Contains(out, "run_small") {
		t.Fatalf("selected status output missing small component:\n%s", out)
	}
}

func TestStatusCommandBenchmarkDirectoryRejectsMismatchedBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "-b", "large", filepath.Join(cwd, "bench", "small")}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected status failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `does not match --benchmark "large"`) {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestStatusCommandAllowsRunningStatus(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`true`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	root := readRootStatus(t, filepath.Join(runDir, "status"))
	root.Status = jbsrun.StatusRunning
	writeRootStatus(t, filepath.Join(runDir, "status"), root)
	writeWorkStatus(t, filepath.Join(runDir, "s", "000000", "status"), jbsrun.WorkStatus{
		Schema: 1,
		Status: jbsrun.StatusRunning,
		Step:   "s",
		Row:    0,
	})

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "RUNNING") || !strings.Contains(stdout.String(), "|       1 |") {
		t.Fatalf("status output missing running count:\n%s", stdout.String())
	}
}

func TestStatusCommandPrintsFailedWorkDirectories(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`false`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	failedDir := filepath.Join(cwd, "bench", "000000", "s", "000000")
	writeWorkStatus(t, filepath.Join(failedDir, "status"), jbsrun.WorkStatus{
		Schema: 1,
		Status: jbsrun.StatusError,
		Step:   "s",
		Row:    0,
	})

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("status failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "failed workpackage directories:") {
		t.Fatalf("status output missing failed directory header:\n%s", out)
	}
	if !strings.Contains(out, filepath.Join("bench", "000000", "s", "000000")) {
		t.Fatalf("status output missing failed directory path:\n%s", out)
	}
}

func TestTreeCommandPrintsPlannedDependencyTreeWithoutCreatingRunDir(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x=[1, 2])`,
		`do prep with cases {`,
		`echo prep`,
		`}`,
		`do run after prep {`,
		`echo run`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"tree", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("tree failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"| step", "| #", "└── prep", "└── run", "total:", "4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench")); !os.IsNotExist(err) {
		t.Fatalf("tree should not create benchmark directory, stat error: %v", err)
	}
}

func TestTreeCommandSupportsBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"tree", "-b", "small", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("tree failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "run_large") || strings.Contains(out, "unrelated") {
		t.Fatalf("selected tree output included unrelated steps:\n%s", out)
	}
	if !strings.Contains(out, "prep") || !strings.Contains(out, "run_small") {
		t.Fatalf("selected tree output missing required steps:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench")); !os.IsNotExist(err) {
		t.Fatalf("tree should not create benchmark directory, stat error: %v", err)
	}
}

func TestTreeCommandSupportsDoOnlyBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeDoOnlyBenchmarkInput(t, cwd)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"tree", "-b", "smoke", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("tree failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "prepare") || strings.Contains(out, "run") {
		t.Fatalf("do-only tree output should include only prepare:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench")); !os.IsNotExist(err) {
		t.Fatalf("tree should not create benchmark directory, stat error: %v", err)
	}
}

func TestLsAnalyseCommandListsCSVOutputs(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo "value: 7" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{filepath.Join("bench", "000000", "run", "analyse.csv"), "nrows", "ncols", "|     1 |", "|     2 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls-analyse output missing %q:\n%s", want, out)
		}
	}
}

func TestLsAnalyseCommandAcceptsBenchmarkDirectoryCSV(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo "value: 7" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{filepath.Join("bench", "000000", "run", "analyse.csv"), "nrows", "ncols", "|     1 |", "|     2 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls-analyse output missing %q:\n%s", want, out)
		}
	}
}

func TestLsAnalyseCommandListsSQLiteOutputs(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`do run {`,
		`echo "value: 7" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"results.sqlite:bench_000000_run", "nrows", "ncols", "|     1 |", "|     2 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls-analyse output missing %q:\n%s", want, out)
		}
	}
}

func TestLsAnalyseCommandAcceptsBenchmarkDirectorySQLite(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`do run {`,
		`echo "value: 7" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"results.sqlite:bench_000000_run", "nrows", "ncols", "|     1 |", "|     2 |"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls-analyse output missing %q:\n%s", want, out)
		}
	}
}

func TestLsAnalyseCommandSupportsBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", "-b", "small", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "run_large") || strings.Contains(out, "[large]") {
		t.Fatalf("selected ls-analyse output included large component:\n%s", out)
	}
	if !strings.Contains(out, filepath.Join("bench", "small", "000000", "run_small", "analyse.csv")) {
		t.Fatalf("selected ls-analyse output missing small analyse path:\n%s", out)
	}
}

func TestLsAnalyseCommandBenchmarkDirectorySupportsBenchmarkSelection(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeMultiBenchmarkInput(t, cwd, "")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", "-b", "small", filepath.Join(cwd, "bench")}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "run_large") || strings.Contains(out, "[large]") {
		t.Fatalf("selected ls-analyse output included large component:\n%s", out)
	}
	if !strings.Contains(out, filepath.Join("bench", "small", "000000", "run_small", "analyse.csv")) {
		t.Fatalf("selected ls-analyse output missing small analyse path:\n%s", out)
	}
}

func TestLsAnalyseCommandDoOnlyBenchmarkHasNoOutput(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	input := writeDoOnlyBenchmarkInput(t, cwd)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-b", "smoke", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"ls-analyse", "-b", "smoke", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls-analyse failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("do-only ls-analyse should be silent, got:\n%s", stdout.String())
	}
}

func TestDefaultRunNoStrictOmitsStrictShell(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`echo before`,
		`false`,
		`echo after`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{input, "--no-strict"}, &stdout, &stderr); code != 0 {
		t.Fatalf("no-strict run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	workDir := filepath.Join(runDir, "s", "000000")
	script := readFileString(t, filepath.Join(workDir, "run.sh"))
	if strings.Contains(script, "set -euo pipefail") {
		t.Fatalf("run.sh should not contain strict mode:\n%s", script)
	}
	out := readFileString(t, filepath.Join(workDir, "stdout"))
	if !strings.Contains(out, "before\n") || !strings.Contains(out, "after\n") {
		t.Fatalf("unexpected no-strict stdout: %q", out)
	}
	status := readRootStatus(t, filepath.Join(runDir, "status"))
	if status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected no-strict root status: %#v", status)
	}
}

func TestRunCommandDryRunCreatesDirectoryWithoutExecuting(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do prep {`,
		`echo prep >> ../../marker`,
		`}`,
		`do run after prep {`,
		`echo run >> ../../marker`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "0% (") || strings.Contains(stdout.String(), "100% (") {
		t.Fatalf("dry-run emitted progress output: %q", stdout.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	rootStatus := readRootStatus(t, filepath.Join(runDir, "status"))
	if rootStatus.Status != jbsrun.StatusNotStarted || rootStatus.PID != 0 {
		t.Fatalf("unexpected dry-run root status: %#v", rootStatus)
	}
	for _, step := range []string{"prep", "run"} {
		workDir := filepath.Join(runDir, step, "000000")
		status := readWorkStatus(t, filepath.Join(workDir, "status"))
		if status.Status != jbsrun.StatusNotStarted {
			t.Fatalf("%s status = %s, want %s", step, status.Status, jbsrun.StatusNotStarted)
		}
		for _, name := range []string{"run.sh", "stdout", "stderr"} {
			if _, err := os.Stat(filepath.Join(workDir, name)); err != nil {
				t.Fatalf("expected %s in %s: %v", name, workDir, err)
			}
		}
		if _, err := os.Stat(filepath.Join(workDir, "exitcode")); !os.IsNotExist(err) {
			t.Fatalf("dry-run should not create exitcode in %s, stat error: %v", workDir, err)
		}
	}
	linkPath := filepath.Join(runDir, "run", "000000", "prep")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected dependency symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dependency path is not a symlink: %s", linkPath)
	}
	if _, err := os.Stat(filepath.Join(runDir, "marker")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not execute work, marker stat error: %v", err)
	}
}

func TestContinueStartsDryRunDirectory(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo hello`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-n", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	runDir := filepath.Join(cwd, "bench", "000000")
	if status := readRootStatus(t, filepath.Join(runDir, "status")); status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected root status: %#v", status)
	}
	workDir := filepath.Join(runDir, "run", "000000")
	if status := readWorkStatus(t, filepath.Join(workDir, "status")); status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected work status: %#v", status)
	}
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "hello\n" {
		t.Fatalf("unexpected work stdout: %q", got)
	}
	if _, err := os.Stat(filepath.Join(workDir, "exitcode")); err != nil {
		t.Fatalf("expected exitcode after continue: %v", err)
	}
}

func TestRunCommandFSubCreatesSubstitutedFilesAndWarnings(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	if err := os.WriteFile(filepath.Join(cwd, "input.tpl"), []byte("x=###X###\nagain=###X###\ny=###Y###\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x=[7], y=["label"])`,
		`do run`,
		`        with cases`,
		`        fsub "input.tpl" {`,
		`                "###X###": x,`,
		`                "###Y###": y,`,
		`        }`,
		`{`,
		`cat input.tpl`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	workDir := filepath.Join(cwd, "bench", "000000", "run", "000000")
	want := "x=7\nagain=7\ny=label\n"
	if got := readFileString(t, filepath.Join(workDir, "input.tpl")); got != want {
		t.Fatalf("substituted file = %q", got)
	}
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != want {
		t.Fatalf("stdout = %q", got)
	}
	if !strings.Contains(stderr.String(), `warning: fsub step run row 000000 file input.tpl pattern "###X###" matched 2 times`) {
		t.Fatalf("missing fsub warning in stderr: %q", stderr.String())
	}
}

func TestRunCommandFSubDryRunContinueAndTemplateHash(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	if err := os.WriteFile(filepath.Join(cwd, "input.tpl"), []byte("value=TOKEN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run`,
		`        fsub "input.tpl" { "TOKEN": "prepared" }`,
		`{`,
		`cat input.tpl`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-n", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	workDir := filepath.Join(cwd, "bench", "000000", "run", "000000")
	if got := readFileString(t, filepath.Join(workDir, "input.tpl")); got != "value=prepared\n" {
		t.Fatalf("dry-run substituted file = %q", got)
	}
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "" {
		t.Fatalf("dry-run stdout = %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "value=prepared\n" {
		t.Fatalf("continue stdout = %q", got)
	}

	if err := os.WriteFile(filepath.Join(cwd, "input.tpl"), []byte("value=changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("continue should fail after template change\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "fsub template input.tpl hash does not match current file") {
		t.Fatalf("missing template hash diagnostic: %q", stderr.String())
	}

	if err := os.Remove(filepath.Join(cwd, "input.tpl")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("continue should fail after template removal\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "fsub template") || !strings.Contains(stderr.String(), "not found") {
		t.Fatalf("missing removed-template diagnostic: %q", stderr.String())
	}
}

func TestRunCommandFSubPreservesExecutableTemplate(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	if err := os.WriteFile(filepath.Join(cwd, "tool.sh"), []byte("#!/usr/bin/env bash\necho TOKEN\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(cwd, "tool.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run`,
		`        fsub "tool.sh" { "TOKEN": "ok" }`,
		`{`,
		`./tool.sh`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	workDir := filepath.Join(cwd, "bench", "000000", "run", "000000")
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "ok\n" {
		t.Fatalf("stdout = %q", got)
	}
	assertFilePerm(t, filepath.Join(workDir, "tool.sh"), 0o755)
}

func TestRunCommandFSubDryRunContinuePreservesExecutableTemplate(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	if err := os.WriteFile(filepath.Join(cwd, "tool.sh"), []byte("#!/usr/bin/env bash\necho TOKEN\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(cwd, "tool.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run`,
		`        fsub "tool.sh" { "TOKEN": "ok" }`,
		`{`,
		`./tool.sh`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-n", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	workDir := filepath.Join(cwd, "bench", "000000", "run", "000000")
	assertFilePerm(t, filepath.Join(workDir, "tool.sh"), 0o755)
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "" {
		t.Fatalf("dry-run stdout = %q", got)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if got := readFileString(t, filepath.Join(workDir, "stdout")); got != "ok\n" {
		t.Fatalf("continue stdout = %q", got)
	}
	assertFilePerm(t, filepath.Join(workDir, "tool.sh"), 0o755)
}

func TestRunCommandFSubContinueRejectsTemplateModeChange(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	template := filepath.Join(cwd, "tool.sh")
	if err := os.WriteFile(template, []byte("#!/usr/bin/env bash\necho TOKEN\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(template, 0o755); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run`,
		`        fsub "tool.sh" { "TOKEN": "ok" }`,
		`{`,
		`./tool.sh`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "-n", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if err := os.Chmod(template, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("continue should fail after template mode change\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "mode does not match") {
		t.Fatalf("missing template mode diagnostic: %q", stderr.String())
	}
}

func TestRunCommandFSubCreationFailureDoesNotCommitRun(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	if err := os.WriteFile(filepath.Join(cwd, "input.tpl"), []byte("present\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run`,
		`        fsub "input.tpl" { "missing": "x" }`,
		`{`,
		`cat input.tpl`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("run should fail\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "did not match") {
		t.Fatalf("missing fsub failure diagnostic: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "000000")); !os.IsNotExist(err) {
		t.Fatalf("failed creation should not commit final run dir, stat error: %v", err)
	}
}

func TestRunCommandDryRunNoStrictPersistsToRunScript(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do s {`,
		`echo before`,
		`false`,
		`echo after`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", "--no-strict", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	workDir := filepath.Join(runDir, "s", "000000")
	script := readFileString(t, filepath.Join(workDir, "run.sh"))
	if strings.Contains(script, "set -euo pipefail") {
		t.Fatalf("run.sh should not contain strict mode:\n%s", script)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if status := readRootStatus(t, filepath.Join(runDir, "status")); status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected root status after continue: %#v", status)
	}
	out := readFileString(t, filepath.Join(workDir, "stdout"))
	if !strings.Contains(out, "before\n") || !strings.Contains(out, "after\n") {
		t.Fatalf("unexpected no-strict stdout: %q", out)
	}
}

func TestRunCommandDryRunDoesNotRunAnalyse(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo "Number: 5" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--dry-run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	analysePath := filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")
	if got := readFileString(t, analysePath); got != "run_id,number\n" {
		t.Fatalf("dry-run should only write analyse header, got %q", got)
	}
	if strings.Contains(stdout.String(), "run/analyse.csv") {
		t.Fatalf("dry-run should not print analyse table: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if got := readFileString(t, analysePath); got != "run_id,number\n000000,5\n" {
		t.Fatalf("continue did not populate analyse output: %q", got)
	}
	if !strings.Contains(stdout.String(), "bench/000000/run/analyse.csv") ||
		!strings.Contains(stdout.String(), "|     1 |     2 |") {
		t.Fatalf("continue did not print analyse summary: %q", stdout.String())
	}
}

func TestDefaultRunDryRunShorthandCreatesDirectoryWithoutExecuting(t *testing.T) {
	cases := []struct {
		name string
		args func(string) []string
	}{
		{name: "before_input", args: func(input string) []string { return []string{"-n", input} }},
		{name: "after_input", args: func(input string) []string { return []string{input, "-n"} }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			oldwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(cwd); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(oldwd)

			src := strings.Join([]string{
				`jbs_name = "bench"`,
				`do s {`,
				`echo executed >> ../../marker`,
				`}`,
				``,
			}, "\n")
			input := filepath.Join(cwd, "bench.jbs")
			if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			if code := Run(tc.args(input), &stdout, &stderr); code != 0 {
				t.Fatalf("dry-run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
			}
			runDir := filepath.Join(cwd, "bench", "000000")
			if status := readRootStatus(t, filepath.Join(runDir, "status")); status.Status != jbsrun.StatusNotStarted {
				t.Fatalf("unexpected root status: %#v", status)
			}
			if status := readWorkStatus(t, filepath.Join(runDir, "s", "000000", "status")); status.Status != jbsrun.StatusNotStarted {
				t.Fatalf("unexpected work status: %#v", status)
			}
			if _, err := os.Stat(filepath.Join(runDir, "marker")); !os.IsNotExist(err) {
				t.Fatalf("dry-run should not execute work, marker stat error: %v", err)
			}
		})
	}
}

func TestContinueRejectsRunningRoot(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo ok`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d: %s", code, stderr.String())
	}
	statusPath := filepath.Join(cwd, "bench", "000000", "status")
	status := readRootStatus(t, statusPath)
	status.Status = jbsrun.StatusRunning
	writeRootStatus(t, statusPath, status)

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected continue to fail for RUNNING root")
	}
	if !strings.Contains(stderr.String(), "RUNNING") {
		t.Fatalf("expected RUNNING error, got %q", stderr.String())
	}
}

func TestContinueHashMismatchMentionsSourcePathIdentity(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	realDir := filepath.Join(cwd, "real")
	linkDir := filepath.Join(cwd, "link")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo ok`,
		`}`,
		``,
	}, "\n")
	realInput := filepath.Join(realDir, "bench.jbs")
	if err := os.WriteFile(realInput, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	linkInput := filepath.Join(linkDir, "bench.jbs")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", realInput}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", linkInput}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected continue through alternate path to fail")
	}
	errText := stderr.String()
	for _, want := range []string{"source identity includes loaded source path labels", "same path used for jbs run", "stored sha256:", "current sha256:"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("expected continue error to mention %q, got %q", want, errText)
		}
	}
}

func TestContinueRejectsConcurrentProcess(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run nproc 1 {`,
		`echo "$$" >> ../../started.log`,
		`sleep 1`,
		`echo done`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	runDir := filepath.Join(cwd, "bench", "000000")
	marker := filepath.Join(runDir, "started.log")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	rootStatusPath := filepath.Join(runDir, "status")
	rootStatus := readRootStatus(t, rootStatusPath)
	rootStatus.Status = jbsrun.StatusInterrupted
	rootStatus.PID = 0
	rootStatus.Error = ""
	writeRootStatus(t, rootStatusPath, rootStatus)

	workStatusPath := filepath.Join(runDir, "run", "000000", "status")
	writeWorkStatus(t, workStatusPath, jbsrun.WorkStatus{
		Schema: 1,
		Status: jbsrun.StatusNotStarted,
		Step:   "run",
		Row:    0,
	})

	first := startContinueChild(t, cwd, input)
	second := startContinueChild(t, cwd, input)
	results := []childResult{
		waitContinueChild(t, first),
		waitContinueChild(t, second),
	}

	successes := 0
	failures := 0
	failureText := ""
	for _, result := range results {
		if result.code == 0 {
			successes++
		} else {
			failures++
			failureText += result.stderr
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one successful continue and one rejected continue, got %d successes and %d failures: %#v", successes, failures, results)
	}
	if !strings.Contains(failureText, "locked") && !strings.Contains(failureText, "RUNNING") {
		t.Fatalf("expected rejected continue to mention lock or RUNNING status, got %q", failureText)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(data))
	if len(lines) != 1 {
		t.Fatalf("expected exactly one scheduler entry, got %d: %q", len(lines), string(data))
	}
	if status := readRootStatus(t, rootStatusPath); status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected final root status: %#v", status)
	}
	if status := readWorkStatus(t, workStatusPath); status.Status != jbsrun.StatusFinished {
		t.Fatalf("unexpected final work status: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", ".jbs.lock")); !os.IsNotExist(err) {
		t.Fatalf("expected no stale lock file, stat error: %v", err)
	}
}

func TestContinueCommandHelper(t *testing.T) {
	if os.Getenv("JBS_CONTINUE_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(Run(os.Args[i+1:], os.Stdout, os.Stderr))
		}
	}
	os.Exit(2)
}

func TestRunCommandPrintsAnalyseAfterProgress(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x=[1])`,
		`do run with cases {`,
		`echo "x=$x"`,
		`}`,
		`analyse run {`,
		`(x)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "\r| analysis") {
		t.Fatalf("analyse summary starts on a carriage-return line: %q", out)
	}
	if !strings.Contains(out, "\n| analysis") ||
		!strings.Contains(out, "bench/000000/run/analyse.csv") ||
		!strings.Contains(out, "|     1 |     2 |") {
		t.Fatalf("analyse summary missing or not separated from progress: %q", out)
	}
}

func TestRunCommandPopulatesAnalyseWithPatterns(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x=[1])`,
		`do run with cases {`,
		`echo "Number: 1" > out.log`,
		`echo "Pair: AA-17" >> out.log`,
		`echo "Number: 2" >> out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`pair = "Pair: ([A-Z]+)-([0-9]+)" in "out.log"`,
		`(x, number, pair as "Pair")`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	wantCSV := "run_id,x,number,Pair.0,Pair.1\n000000,1,1,AA,17\n000000,1,2,,\n"
	if got := readFileString(t, filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")); got != wantCSV {
		t.Fatalf("analyse csv = %q, want %q", got, wantCSV)
	}
	if !strings.Contains(stdout.String(), "bench/000000/run/analyse.csv") ||
		!strings.Contains(stdout.String(), "|     2 |     5 |") {
		t.Fatalf("analyse summary missing\nstdout:\n%s", stdout.String())
	}
}

func TestRunCommandAnalysisFailureMarksRootError(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo ok`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "missing.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run to fail\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	status := readRootStatus(t, filepath.Join(cwd, "bench", "000000", "status"))
	if status.Status != jbsrun.StatusError || !strings.Contains(status.Error, "missing.log") {
		t.Fatalf("unexpected root status after analysis failure: %#v", status)
	}
	errText := stderr.String()
	if !strings.Contains(errText, "bench: benchmark ERROR:") {
		t.Fatalf("expected detailed benchmark error, got %q", errText)
	}
	if !strings.Contains(errText, "read analyse file") || !strings.Contains(errText, "missing.log") {
		t.Fatalf("expected analysis cause in stderr, got %q", errText)
	}
	if strings.Contains(stdout.String(), "\nrun/analyse.csv\n") {
		t.Fatalf("did not expect analysis table after failure: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "failed workpackage directories:") {
		t.Fatalf("analysis-only failure printed failed work directories:\n%s", stdout.String())
	}
}

func TestRunCommandWeakWritesAnalyseCSVAfterFailedJobs(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x = [1, 2])`,
		`do run with cases nproc 1 {`,
		`if [ "$x" = "2" ]; then exit 2; fi`,
		`echo "value=$x" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value=%d" in "out.log"`,
		`(x, value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--weak", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected weak run to keep failing exit code\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	if status := readRootStatus(t, filepath.Join(runDir, "status")); status.Status != jbsrun.StatusError {
		t.Fatalf("weak run root status = %#v, want ERROR", status)
	}
	assertStringRows(t, readCSVFileRows(t, filepath.Join(runDir, "run", "analyse.csv")), [][]string{
		{"run_id", "x", "value", "jbs_status"},
		{"000000", "1", "1", "FINISHED"},
		{"000001", "", "", "ERROR"},
	})
	out := stdout.String()
	for _, want := range []string{"failed workpackage directories:", "analyse.csv", "nrows", "ncols"} {
		if !strings.Contains(out, want) {
			t.Fatalf("weak run stdout missing %q:\n%s", want, out)
		}
	}
}

func TestRunCommandNonWeakDoesNotPrintAnalyseSummaryAfterFailure(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x = [1, 2])`,
		`do run with cases nproc 1 {`,
		`if [ "$x" = "2" ]; then exit 2; fi`,
		`echo "value=$x" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value=%d" in "out.log"`,
		`(x, value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "analyse.csv") {
		t.Fatalf("non-weak failed run should not print analyse summary:\n%s", stdout.String())
	}
	if got := readFileString(t, filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")); got != "run_id,x,value\n" {
		t.Fatalf("non-weak failed run should leave only initial analyse header, got %q", got)
	}
}

func TestRunCommandWeakWritesAnalyseSQLiteAfterFailedJobs(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`cases = table(x = [1, 2])`,
		`do run with cases nproc 1 {`,
		`if [ "$x" = "2" ]; then exit 2; fi`,
		`echo "value=$x" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value=%d" in "out.log"`,
		`(x, value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--weak", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected weak run to keep failing exit code\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	dbPath := filepath.Join(cwd, "results.sqlite")
	assertSQLiteTable(t, dbPath, "bench_000000_run",
		[]string{"run_id", "x", "value", "jbs_status"},
		[][]string{
			{"000000", "1", "1", "FINISHED"},
			{"000001", "", "", "ERROR"},
		},
	)
	if !strings.Contains(stdout.String(), "results.sqlite:bench_000000_run") {
		t.Fatalf("weak sqlite run did not print analyse summary:\n%s", stdout.String())
	}
}

func TestRunCommandWeakWritesBlockedAnalyseRows(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`x = 1`,
		`do prep with x {`,
		`false`,
		`}`,
		`do run after prep {`,
		`echo "value=$x" > out.log`,
		`}`,
		`analyse run {`,
		`(x)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--weak", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected weak blocked run to keep failing exit code\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	runDir := filepath.Join(cwd, "bench", "000000")
	if status := readWorkStatus(t, filepath.Join(runDir, "run", "000000", "status")); status.Status != jbsrun.StatusBlocked {
		t.Fatalf("run work status = %#v, want BLOCKED", status)
	}
	assertStringRows(t, readCSVFileRows(t, filepath.Join(runDir, "run", "analyse.csv")), [][]string{
		{"run_id", "x", "jbs_status"},
		{"000000", "", "BLOCKED"},
	})
}

func TestRunCommandWeakSuccessfulRunAddsStatusColumn(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`cases = table(x = [1, 2])`,
		`do run with cases nproc 1 {`,
		`echo "value=$x" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value=%d" in "out.log"`,
		`(x, value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", "--weak", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("weak successful run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	assertStringRows(t, readCSVFileRows(t, filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")), [][]string{
		{"run_id", "x", "value", "jbs_status"},
		{"000000", "1", "1", "FINISHED"},
		{"000001", "2", "2", "FINISHED"},
	})
}

func TestContinueRegeneratesAnalyse(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`do run {`,
		`echo "Number: 7" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	analysePath := filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")
	if err := os.WriteFile(analysePath, []byte("run_id,number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(analysePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "run_id,number\n000000,7\n" {
		t.Fatalf("analysis was not regenerated: %q", string(data))
	}
}

func TestRunCommandWritesAnalyseSQLiteDatabase(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`cases = table(x=[1])`,
		`do run with cases {`,
		`echo "Number: 7" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(x, number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	dbPath := filepath.Join(cwd, "results.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")); !os.IsNotExist(err) {
		t.Fatalf("did not expect analyse.csv in sqlite mode, stat error: %v", err)
	}
	manifest, err := jbsrun.LoadManifest(filepath.Join(cwd, "bench", "000000", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != "000000" {
		t.Fatalf("manifest RunID = %q", manifest.RunID)
	}
	if manifest.Steps[0].AnalyseTable != "bench_000000_run" {
		t.Fatalf("manifest analyse table = %q", manifest.Steps[0].AnalyseTable)
	}
	header, rows := readSQLiteTable(t, dbPath, "bench_000000_run")
	assertStringSlices(t, header, []string{"run_id", "x", "number"})
	assertStringRows(t, rows, [][]string{{"000000", "1", "7"}})
	assertStringRows(t, readSQLiteColumnTypes(t, dbPath, "bench_000000_run"), [][]string{
		{"run_id", "TEXT"},
		{"x", "INTEGER"},
		{"number", "INTEGER"},
	})
	assertSQLiteValueTypes(t, dbPath, "bench_000000_run", []string{"x", "number"}, []string{"integer", "integer"})
	if !strings.Contains(stdout.String(), "results.sqlite:bench_000000_run") ||
		!strings.Contains(stdout.String(), "|     1 |     3 |") {
		t.Fatalf("sqlite analyse summary missing\nstdout:\n%s", stdout.String())
	}
}

func TestRunCommandWritesTypedAnalyseSQLiteWorkValues(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`cases = table(i = [1], f = [1.5], b = [true], s = ["x"])`,
		`do run with cases {`,
		`echo ok > out.log`,
		`}`,
		`analyse run {`,
		`(i, f, b, s)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	dbPath := filepath.Join(cwd, "results.sqlite")
	header, rows := readSQLiteTable(t, dbPath, "bench_000000_run")
	assertStringSlices(t, header, []string{"run_id", "i", "f", "b", "s"})
	assertStringRows(t, rows, [][]string{{"000000", "1", "1.5", "1", "x"}})
	assertStringRows(t, readSQLiteColumnTypes(t, dbPath, "bench_000000_run"), [][]string{
		{"run_id", "TEXT"},
		{"i", "INTEGER"},
		{"f", "REAL"},
		{"b", "INTEGER"},
		{"s", "TEXT"},
	})
	assertSQLiteValueTypes(t, dbPath, "bench_000000_run", []string{"i", "f", "b", "s"}, []string{"integer", "real", "integer", "text"})
}

func TestRunCommandAnalyseSQLiteAccumulatesTablesAcrossRuns(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`x = ("a",)`,
		`do step with x {`,
		`echo "Value: ${x}" > out.log`,
		`}`,
		`analyse step {`,
		`value = "Value: %w" in "out.log"`,
		`(x, value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("first run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("second run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}

	dbPath := filepath.Join(cwd, "results.sqlite")
	assertSQLiteTable(t, dbPath, "bench_000000_step",
		[]string{"run_id", "x", "value"},
		[][]string{{"000000", "a", "a"}})
	assertSQLiteTable(t, dbPath, "bench_000001_step",
		[]string{"run_id", "x", "value"},
		[][]string{{"000000", "a", "a"}})
}

func TestRunCommandWritesMultipleAnalyseSQLiteTables(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`do prep {`,
		`echo "Prep: 1" > out.log`,
		`}`,
		`do run {`,
		`echo "Run: 2" > out.log`,
		`}`,
		`analyse prep {`,
		`value = "Prep: %d" in "out.log"`,
		`(value)`,
		`}`,
		`analyse run {`,
		`value = "Run: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	dbPath := filepath.Join(cwd, "results.sqlite")
	_, prepRows := readSQLiteTable(t, dbPath, "bench_000000_prep")
	_, runRows := readSQLiteTable(t, dbPath, "bench_000000_run")
	assertStringRows(t, prepRows, [][]string{{"000000", "1"}})
	assertStringRows(t, runRows, [][]string{{"000000", "2"}})
}

func TestContinueRegeneratesAnalyseSQLite(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`do run {`,
		`echo "Number: 7" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	outPath := filepath.Join(cwd, "bench", "000000", "run", "000000", "out.log")
	if err := os.WriteFile(outPath, []byte("Number: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	_, rows := readSQLiteTable(t, filepath.Join(cwd, "results.sqlite"), "bench_000000_run")
	assertStringRows(t, rows, [][]string{{"000000", "9"}})
	if sqliteTableExists(t, filepath.Join(cwd, "results.sqlite"), "bench_000001_run") {
		t.Fatalf("continue created a fresh run table")
	}
}

func TestContinueRejectsLegacyAnalyseSQLiteTableName(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`do run {`,
		`echo "Number: 7" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	manifestPath := filepath.Join(cwd, "bench", "000000", "manifest.json")
	manifest, err := jbsrun.LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Steps[0].AnalyseTable = "run"
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected continue failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "manifest analyse table") {
		t.Fatalf("expected manifest table validation error, stderr:\n%s", stderr.String())
	}
}

func TestContinueUsesManifestAnalyseSQLitePath(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`do run {`,
		`echo "Number: 7" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	outPath := filepath.Join(cwd, "bench", "000000", "run", "000000", "out.log")
	if err := os.WriteFile(outPath, []byte("Number: 11\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(cwd, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cwd, "bench"), filepath.Join(other, "bench")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"continue", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("continue failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	_, rows := readSQLiteTable(t, filepath.Join(cwd, "results.sqlite"), "bench_000000_run")
	assertStringRows(t, rows, [][]string{{"000000", "11"}})
	if _, err := os.Stat(filepath.Join(other, "results.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("continue wrote database relative to continue cwd, stat error: %v", err)
	}
}

func TestRunCommandAcceptsAbsoluteAnalyseSQLitePath(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	dbPath := filepath.Join(cwd, "absolute.sqlite")
	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "` + dbPath + `"`,
		`do run {`,
		`echo "Number: 5" > out.log`,
		`}`,
		`analyse run {`,
		`number = "Number: %d" in "out.log"`,
		`(number)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	_, rows := readSQLiteTable(t, dbPath, "bench_000000_run")
	assertStringRows(t, rows, [][]string{{"000000", "5"}})
}

func TestRunCommandRejectsDuplicateAnalyseSQLiteColumnsBeforeDirectoryCreation(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = "results.sqlite"`,
		`cases = table(x=[1])`,
		`do run with cases {`,
		`echo ok`,
		`}`,
		`analyse run {`,
		`(x, x as "x")`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected run failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "duplicate result column") {
		t.Fatalf("expected duplicate column error, stderr:\n%s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench")); !os.IsNotExist(err) {
		t.Fatalf("run directory should not be created, stat error: %v", err)
	}
}

func TestRunCommandEmptyAnalyseDatabaseKeepsCSV(t *testing.T) {
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	src := strings.Join([]string{
		`jbs_name = "bench"`,
		`jbs_database = ""`,
		`cases = table(x=[1])`,
		`do run with cases {`,
		`echo "$x"`,
		`}`,
		`analyse run {`,
		`(x)`,
		`}`,
		``,
	}, "\n")
	input := filepath.Join(cwd, "bench.jbs")
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"run", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("run failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "bench", "000000", "run", "analyse.csv")); err != nil {
		t.Fatalf("expected analyse.csv in csv mode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "results.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("did not expect sqlite database in csv mode, stat error: %v", err)
	}
}

func readRootStatus(t *testing.T, path string) jbsrun.RootStatus {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var status jbsrun.RootStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func writeMultiBenchmarkInput(t *testing.T, cwd string, extra ...string) string {
	t.Helper()
	lines := []string{
		`jbs_name = "bench"`,
		`jbs_benchmarks = {"small": "run_small", "large": "run_large"}`,
	}
	for _, line := range extra {
		if line != "" {
			lines = append(lines, line)
		}
	}
	lines = append(lines,
		`do prep {`,
		`echo "prep" > prep.txt`,
		`}`,
		`do run_small after prep {`,
		`echo "small: 1" > out.log`,
		`}`,
		`do run_large after prep {`,
		`echo "large: 2" > out.log`,
		`}`,
		`do unrelated {`,
		`echo "unrelated" > out.log`,
		`}`,
		`analyse run_small {`,
		`value = "small: %d" in "out.log"`,
		`(value)`,
		`}`,
		`analyse run_large {`,
		`value = "large: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	)
	input := filepath.Join(cwd, "multi.jbs")
	if err := os.WriteFile(input, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return input
}

func writeDoOnlyBenchmarkInput(t *testing.T, cwd string) string {
	t.Helper()
	lines := []string{
		`jbs_name = "bench"`,
		`jbs_benchmarks = {"smoke": "prepare", "results": "run"}`,
		`do prepare {`,
		`echo prepare`,
		`}`,
		`do run after prepare {`,
		`echo "value: 1" > out.log`,
		`}`,
		`analyse run {`,
		`value = "value: %d" in "out.log"`,
		`(value)`,
		`}`,
		``,
	}
	input := filepath.Join(cwd, "do_only.jbs")
	if err := os.WriteFile(input, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return input
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertFilePerm(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func readCSVFileRows(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func readSQLiteTable(t *testing.T, dbPath, table string) ([]string, [][]string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	headerRows, err := db.Query(`PRAGMA table_info(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		t.Fatal(err)
	}
	header := make([]string, 0)
	for headerRows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := headerRows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			headerRows.Close()
			t.Fatal(err)
		}
		header = append(header, name)
	}
	if err := headerRows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := headerRows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(header) == 0 {
		t.Fatalf("table %q has no columns", table)
	}

	cols := make([]string, 0, len(header))
	for _, col := range header {
		cols = append(cols, quoteSQLiteIdent(col))
	}
	dataRows, err := db.Query(`SELECT ` + strings.Join(cols, ", ") + ` FROM ` + quoteSQLiteIdent(table) + ` ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer dataRows.Close()

	rows := make([][]string, 0)
	for dataRows.Next() {
		values := make([]any, len(header))
		dest := make([]any, len(header))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := dataRows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		row := make([]string, len(header))
		for i, value := range values {
			row[i] = sqliteValueString(value)
		}
		rows = append(rows, row)
	}
	if err := dataRows.Err(); err != nil {
		t.Fatal(err)
	}
	return header, rows
}

func sqliteValueString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func readSQLiteColumnTypes(t *testing.T, dbPath, table string) [][]string {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := make([][]string, 0)
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		out = append(out, []string{name, typ})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertSQLiteValueTypes(t *testing.T, dbPath, table string, columns []string, want []string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exprs := make([]string, 0, len(columns))
	for _, column := range columns {
		exprs = append(exprs, `typeof(`+quoteSQLiteIdent(column)+`)`)
	}
	row := db.QueryRow(`SELECT ` + strings.Join(exprs, ", ") + ` FROM ` + quoteSQLiteIdent(table) + ` ORDER BY rowid LIMIT 1`)
	got := make([]string, len(columns))
	dest := make([]any, len(columns))
	for i := range got {
		dest[i] = &got[i]
	}
	if err := row.Scan(dest...); err != nil {
		t.Fatal(err)
	}
	assertStringSlices(t, got, want)
}

func assertSQLiteTable(t *testing.T, dbPath, table string, wantHeader []string, wantRows [][]string) {
	t.Helper()
	header, rows := readSQLiteTable(t, dbPath, table)
	assertStringSlices(t, header, wantHeader)
	assertStringRows(t, rows, wantRows)
}

func sqliteTableExists(t *testing.T, dbPath, table string) bool {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var name string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func assertStringSlices(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("slice = %#v, want %#v", got, want)
	}
}

func assertStringRows(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}
	for i := range got {
		if strings.Join(got[i], "\x00") != strings.Join(want[i], "\x00") {
			t.Fatalf("row %d = %#v, want %#v (all rows %#v)", i, got[i], want[i], got)
		}
	}
}

func writeRootStatus(t *testing.T, path string, status jbsrun.RootStatus) {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readWorkStatus(t *testing.T, path string) jbsrun.WorkStatus {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var status jbsrun.WorkStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func writeWorkStatus(t *testing.T, path string, status jbsrun.WorkStatus) {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

type continueChild struct {
	cmd    *exec.Cmd
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

type childResult struct {
	code   int
	stdout string
	stderr string
}

func startContinueChild(t *testing.T, cwd, input string) continueChild {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestContinueCommandHelper", "--", "continue", input)
	cmd.Env = append(os.Environ(), "JBS_CONTINUE_HELPER=1")
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return continueChild{cmd: cmd, stdout: &stdout, stderr: &stderr}
}

func waitContinueChild(t *testing.T, child continueChild) childResult {
	t.Helper()
	code := 0
	if err := child.cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatal(err)
		}
		code = exitErr.ExitCode()
	}
	return childResult{code: code, stdout: child.stdout.String(), stderr: child.stderr.String()}
}
