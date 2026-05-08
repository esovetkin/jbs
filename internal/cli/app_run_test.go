package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

func TestCheckReportsShellCommandFailure(t *testing.T) {
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
	if err := os.WriteFile(input, []byte(`value = shell("printf shellerr >&2; exit 7")`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", input}, &stdout, &stderr); code == 0 {
		t.Fatalf("expected check failure\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	errText := stderr.String()
	if !strings.Contains(errText, "shell() command failed with exit code 7") || !strings.Contains(errText, "shellerr") {
		t.Fatalf("expected shell failure diagnostic with stderr, got:\n%s", errText)
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
		`print("starting", [1, 2, 3, 4])`,
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
	if !strings.HasPrefix(out, "starting [1, 2, 3, ...]\n") {
		t.Fatalf("expected print output before progress, got %q", out)
	}
	workOut := readFileString(t, filepath.Join(cwd, "bench", "000000", "run", "000000", "stdout"))
	if workOut != "shell\n" {
		t.Fatalf("expected shell stdout to stay in workpackage file, got %q", workOut)
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

func TestRunCheckAndContinueDoNotReplayPrintOutput(t *testing.T) {
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
	if code := Run([]string{"--check", input}, &stdout, &stderr); code != 0 {
		t.Fatalf("check failed with code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected check to be quiet, got %q", stdout.String())
	}

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
	if !strings.Contains(stdout.String(), "\nrun/analyse.csv\nrun_id,number\n000000,5\n") {
		t.Fatalf("continue did not print analyse output: %q", stdout.String())
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
	if strings.Contains(out, "\rrun/analyse.csv") {
		t.Fatalf("analyse table starts on a carriage-return line: %q", out)
	}
	if !strings.Contains(out, "\nrun/analyse.csv\nrun_id,x\n000000,1\n") {
		t.Fatalf("analyse table missing or not separated from progress: %q", out)
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
	want := "\nrun/analyse.csv\nrun_id,x,number,Pair.0,Pair.1\n000000,1,1,AA,17\n000000,1,2,,\n"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("analyse output missing\nwant fragment:\n%s\nstdout:\n%s", want, stdout.String())
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
	if strings.Contains(stdout.String(), "\nrun/analyse.csv\n") {
		t.Fatalf("did not expect analysis table after failure: %q", stdout.String())
	}
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
	want := "\nresults.sqlite:bench_000000_run\nrun_id,x,number\n000000,1,7\n"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("sqlite analyse output missing\nwant fragment:\n%s\nstdout:\n%s", want, stdout.String())
	}
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

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
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
		values := make([]sql.NullString, len(header))
		dest := make([]any, len(header))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := dataRows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		row := make([]string, len(header))
		for i, value := range values {
			if value.Valid {
				row[i] = value.String
			}
		}
		rows = append(rows, row)
	}
	if err := dataRows.Err(); err != nil {
		t.Fatal(err)
	}
	return header, rows
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
