package cli

import (
	"bytes"
	"encoding/json"
	"os"
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
