package run

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunProcessCancellationReturnsAfterTermExit(t *testing.T) {
	workDir := t.TempDir()
	writeRunProcessScript(t, workDir, `
trap 'exit 0' TERM
printf ready > ready
while :; do sleep 0.05; done
`)

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan processResult, 1)
	go func() {
		resultCh <- runProcess(ctx, workDir)
	}()

	waitForRunProcessFile(t, filepath.Join(workDir, "ready"))
	started := time.Now()
	cancel()

	select {
	case result := <-resultCh:
		elapsed := time.Since(started)
		if elapsed > 2*time.Second {
			t.Fatalf("cancel took %s; expected prompt return", elapsed)
		}
		if result.Status != StatusInterrupted {
			t.Fatalf("status = %s, want %s", result.Status, StatusInterrupted)
		}
		if _, err := os.Stat(filepath.Join(workDir, "exitcode")); err != nil {
			t.Fatalf("expected exitcode after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runProcess did not return after SIGTERM")
	}
}

func TestRunProcessCancellationKillsAfterGrace(t *testing.T) {
	oldGrace := processTerminationGrace
	processTerminationGrace = 100 * time.Millisecond
	defer func() { processTerminationGrace = oldGrace }()

	workDir := t.TempDir()
	writeRunProcessScript(t, workDir, `
trap '' TERM
printf ready > ready
while :; do sleep 0.05; done
`)

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan processResult, 1)
	go func() {
		resultCh <- runProcess(ctx, workDir)
	}()

	waitForRunProcessFile(t, filepath.Join(workDir, "ready"))
	cancel()

	select {
	case result := <-resultCh:
		if result.Status != StatusInterrupted {
			t.Fatalf("status = %s, want %s", result.Status, StatusInterrupted)
		}
		if result.ExitCode == nil {
			t.Fatal("expected exit code after SIGKILL")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runProcess did not kill process after grace period")
	}
}

func TestFinishProcessReportsExitcodeWriteFailure(t *testing.T) {
	cmd := exec.Command("/usr/bin/env", "bash", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	old := writeExitCodeFile
	writeExitCodeFile = func(string, int) error {
		return errors.New("disk full")
	}
	t.Cleanup(func() {
		writeExitCodeFile = old
	})

	result := finishProcess(t.TempDir(), cmd, nil)
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("exit code = %v, want 0", result.ExitCode)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "write exitcode") {
		t.Fatalf("error = %v, want exitcode write error", result.Err)
	}
}

func writeRunProcessScript(t *testing.T, dir, body string) {
	t.Helper()
	script := "#!/usr/bin/env bash\nset -euo pipefail\n" + body
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func waitForRunProcessFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
