package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestFWaitCommandReturnsWhenMissingFileAppears(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run([]string{"fwait", target}, &stdout, &stderr)
	}()

	waitFWaitCommand(t, done, func(i int) {
		if err := os.WriteFile(target, []byte("created "+strconv.Itoa(i)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if stdout.String() != target+"\n" {
		t.Fatalf("unexpected stdout: got %q want %q", stdout.String(), target+"\n")
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestFWaitCommandReturnsWhenExistingFileChanges(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run([]string{"fwait", target}, &stdout, &stderr)
	}()

	waitFWaitCommand(t, done, func(i int) {
		if err := os.WriteFile(target, []byte("new "+strconv.Itoa(i)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if stdout.String() != target+"\n" {
		t.Fatalf("unexpected stdout: got %q want %q", stdout.String(), target+"\n")
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestFWaitCommandReturnsChangedPathForMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.flag")
	second := filepath.Join(dir, "second.flag")

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Run([]string{"fwait", first, second}, &stdout, &stderr)
	}()

	waitFWaitCommand(t, done, func(i int) {
		if err := os.WriteFile(second, []byte("created "+strconv.Itoa(i)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if stdout.String() != second+"\n" {
		t.Fatalf("unexpected stdout: got %q want %q", stdout.String(), second+"\n")
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestFWaitCommandExitExistingReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.flag")
	existing := filepath.Join(dir, "existing.flag")
	if err := os.WriteFile(existing, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"fwait", "-e", missing, existing}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("fwait exited with code %d", code)
	}
	if stdout.String() != existing+"\n" {
		t.Fatalf("unexpected stdout: got %q want %q", stdout.String(), existing+"\n")
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func waitFWaitCommand(t *testing.T, done <-chan int, mutate func(int)) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(2 * time.Second)
	for i := 0; ; i++ {
		select {
		case code := <-done:
			if code != 0 {
				t.Fatalf("fwait exited with code %d", code)
			}
			return
		case <-ticker.C:
			mutate(i)
		case <-timeout:
			t.Fatal("timed out waiting for fwait command")
		}
	}
}
