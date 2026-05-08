package cli

import (
	"bytes"
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
