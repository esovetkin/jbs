package run

import (
	"context"
	"os"
	"path/filepath"
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
