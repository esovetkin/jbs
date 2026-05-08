package filewait

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitMissingTargetAppears(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WaitWithOptions(ctx, target, Options{Ready: ready})
	}()
	waitReady(t, ready, done)

	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitDone(t, done)
}

func TestWaitAnyMissingSecondTargetAppears(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.flag")
	second := filepath.Join(dir, "second.flag")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan waitResult, 1)
	go func() {
		result, err := WaitAnyWithOptions(ctx, []string{first, second}, Options{Ready: ready})
		done <- waitResult{result: result, err: err}
	}()
	waitReadyResult(t, ready, done)

	if err := os.WriteFile(second, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := waitDoneResult(t, done)
	if result.Path != second || result.Index != 1 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=1", result, second)
	}
}

func TestWaitExistingTargetChanges(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WaitWithOptions(ctx, target, Options{Ready: ready})
	}()
	waitReady(t, ready, done)

	if err := os.WriteFile(target, []byte("new content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitDone(t, done)
}

func TestWaitAnyExistingSecondTargetChanges(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan waitResult, 1)
	go func() {
		result, err := WaitAnyWithOptions(ctx, []string{first, second}, Options{Ready: ready})
		done <- waitResult{result: result, err: err}
	}()
	waitReadyResult(t, ready, done)

	if err := os.WriteFile(second, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := waitDoneResult(t, done)
	if result.Path != second || result.Index != 1 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=1", result, second)
	}
}

func TestWaitExistingTargetIsReplaced(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WaitWithOptions(ctx, target, Options{Ready: ready})
	}()
	waitReady(t, ready, done)

	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}
	waitDone(t, done)
}

func TestWaitAnyReturnsFirstCompletedInArgumentOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.flag")
	second := filepath.Join(dir, "second.flag")
	states, err := prepareTargets([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, ok, err := firstCompleted(states)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a completed target")
	}
	if result.Path != first || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, first)
	}
}

func TestWaitAnyExitIfExistsReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.flag")
	existing := filepath.Join(dir, "existing.flag")
	if err := os.WriteFile(existing, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := WaitAnyWithOptions(context.Background(), []string{missing, existing}, Options{ExitIfExists: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != existing || result.Index != 1 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=1", result, existing)
	}
}

func TestWaitAnyExitIfExistsUsesArgumentOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.flag")
	second := filepath.Join(dir, "second.flag")
	if err := os.WriteFile(first, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := WaitAnyWithOptions(context.Background(), []string{first, second}, Options{ExitIfExists: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != first || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, first)
	}
}

func TestWaitNestedMissingParentAppears(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "done.flag")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WaitWithOptions(ctx, target, Options{Ready: ready})
	}()
	waitReady(t, ready, done)

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitDone(t, done)
}

func TestWaitAnyNestedMissingParentAppears(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a", "b", "first.flag")
	second := filepath.Join(dir, "a", "b", "second.flag")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan waitResult, 1)
	go func() {
		result, err := WaitAnyWithOptions(ctx, []string{first, second}, Options{Ready: ready})
		done <- waitResult{result: result, err: err}
	}()
	waitReadyResult(t, ready, done)

	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := waitDoneResult(t, done)
	if result.Path != second || result.Index != 1 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=1", result, second)
	}
}

func TestWaitDirectoryTargetIsRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Wait(ctx, target)
	if err == nil || !strings.Contains(err.Error(), "target is a directory") {
		t.Fatalf("expected directory target error, got %v", err)
	}
}

func TestWaitAnyRejectsDirectoryAmongTargets(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := func() error {
		_, err := WaitAny(ctx, []string{filepath.Join(dir, "missing"), target})
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "target is a directory") || !strings.Contains(err.Error(), target) {
		t.Fatalf("expected directory target error, got %v", err)
	}
}

func TestWaitContextCancellationReturns(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Wait(ctx, filepath.Join(dir, "missing"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWaitAnyContextCancellationReturns(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := WaitAny(ctx, []string{filepath.Join(dir, "missing"), filepath.Join(dir, "other")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type waitResult struct {
	result Result
	err    error
}

func waitReady(t *testing.T, ready <-chan struct{}, done <-chan error) {
	t.Helper()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("wait returned before ready: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ready")
	}
}

func waitReadyResult(t *testing.T, ready <-chan struct{}, done <-chan waitResult) {
	t.Helper()
	select {
	case <-ready:
	case result := <-done:
		t.Fatalf("wait returned before ready: result=%+v err=%v", result.result, result.err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ready")
	}
}

func waitDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for filewait")
	}
}

func waitDoneResult(t *testing.T, done <-chan waitResult) Result {
	t.Helper()
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return result.result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for filewait")
	}
	return Result{}
}
