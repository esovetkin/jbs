package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesProfilesForHelpCommand(t *testing.T) {
	dir := t.TempDir()
	cpuPath := filepath.Join(dir, "cpu.pprof")
	memPath := filepath.Join(dir, "mem.pprof")

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"--cpuprof=" + cpuPath,
		"--memprof=" + memPath,
		"help",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed: code=%d stderr=%s", code, stderr.String())
	}
	assertNonEmptyFile(t, cpuPath)
	assertNonEmptyFile(t, memPath)
}

func TestRunProfileSetupFailureDoesNotRunCommand(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing")
	cpuPath := filepath.Join(missingDir, "cpu.pprof")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--cpuprof=" + cpuPath, "help"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("command ran despite profile setup failure: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "create CPU profile") {
		t.Fatalf("missing setup error: %q", stderr.String())
	}
}

func TestRunAcceptsProfileFlagsAfterCommandName(t *testing.T) {
	memPath := filepath.Join(t.TempDir(), "mem.pprof")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "--memprof=" + memPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run failed: code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
	assertNonEmptyFile(t, memPath)
}

func TestRunUsageErrorDoesNotCreateProfile(t *testing.T) {
	cpuPath := filepath.Join(t.TempDir(), "cpu.pprof")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--cpuprof=" + cpuPath, "run"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if _, err := os.Stat(cpuPath); !os.IsNotExist(err) {
		t.Fatalf("profile file should not exist after usage error, stat err=%v", err)
	}
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}
