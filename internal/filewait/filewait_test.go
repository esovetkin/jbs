package filewait

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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

func TestWaitAnyPollingDetectsMissingTargetAppears(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan waitResult, 1)
	go func() {
		result, err := WaitAnyWithOptions(ctx, []string{target}, Options{
			Ready:           ready,
			DisableFSNotify: true,
			PollInterval:    10 * time.Millisecond,
		})
		done <- waitResult{result: result, err: err}
	}()
	waitReadyResult(t, ready, done)

	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := waitDoneResult(t, done)
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitAnyPollingDetectsExistingTargetChange(t *testing.T) {
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
		done <- WaitWithOptions(ctx, target, Options{
			Ready:           ready,
			DisableFSNotify: true,
			PollInterval:    10 * time.Millisecond,
		})
	}()
	waitReady(t, ready, done)

	if err := os.WriteFile(target, []byte("new content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitDone(t, done)
}

func TestWaitAnyPollingDetectsExistingTargetTouch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WaitWithOptions(ctx, target, Options{
			Ready:           ready,
			DisableFSNotify: true,
			PollInterval:    10 * time.Millisecond,
		})
	}()
	waitReady(t, ready, done)

	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(target, later, later); err != nil {
		t.Fatal(err)
	}
	waitDone(t, done)
}

func TestWaitAnyPollingReturnsFirstCompletedInArgumentOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.flag")
	second := filepath.Join(dir, "second.flag")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan waitResult, 1)
	go func() {
		result, err := WaitAnyWithOptions(ctx, []string{first, second}, Options{
			Ready:           ready,
			DisableFSNotify: true,
			PollInterval:    100 * time.Millisecond,
		})
		done <- waitResult{result: result, err: err}
	}()
	waitReadyResult(t, ready, done)

	if err := os.WriteFile(second, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := waitDoneResult(t, done)
	if result.Path != first || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, first)
	}
}

func TestWaitMissingTargetCreatedDuringSetupReturns(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	afterPrepareTargetsForTest = func() {
		if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { afterPrepareTargetsForTest = nil }()

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	result, err := WaitAnyWithOptions(ctx, []string{target}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitExistingTargetChangedDuringSetupReturns(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterPrepareTargetsForTest = func() {
		// Use a different size so the post-watch snapshot observes the setup-window
		// change even on filesystems with coarse timestamp resolution.
		if err := os.WriteFile(target, []byte("new content after setup\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { afterPrepareTargetsForTest = nil }()

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := WaitWithOptions(ctx, target, Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestWaitSetupRaceReturnsBeforeReady(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	ready := make(chan struct{})
	afterPrepareTargetsForTest = func() {
		if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { afterPrepareTargetsForTest = nil }()

	_, err := WaitAnyWithOptions(context.Background(), []string{target}, Options{Ready: ready})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
		t.Fatal("ready should not be signaled when setup completion returns immediately")
	default:
	}
}

func TestSnapshotDistinguishesSameMetadataFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	fixedTime := time.Unix(123, 0)
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
			t.Fatal(err)
		}
	}

	firstSnapshot, err := statSnapshot(first)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := statSnapshot(second)
	if err != nil {
		t.Fatal(err)
	}
	if sameSnapshot(firstSnapshot, secondSnapshot) {
		t.Fatal("expected snapshots for distinct files to differ")
	}
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

func TestWaitLoopHandlesClosedWatcherErrors(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	states, err := prepareTargets([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	watcher := newFakeFileWatcher()
	close(watcher.errors)
	done, cancel := runWaitLoopForTest(t, states, watcher, time.Hour)
	defer cancel()

	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher.events <- fsnotify.Event{Name: target, Op: fsnotify.Create}

	result := waitDoneResult(t, done)
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitLoopHandlesClosedEventChannel(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	states, err := prepareTargets([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	watcher := newFakeFileWatcher()
	close(watcher.events)
	done, cancel := runWaitLoopForTest(t, states, watcher, 10*time.Millisecond)
	defer cancel()

	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := waitDoneResult(t, done)
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitLoopFallsBackToPollingAfterWatcherError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	states, err := prepareTargets([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	watcher := newFakeFileWatcher()
	done, cancel := runWaitLoopForTest(t, states, watcher, 10*time.Millisecond)
	defer cancel()

	watcher.errors <- errors.New("watcher failed")
	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := waitDoneResult(t, done)
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitLoopFallsBackAfterEventRefreshFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	states, err := prepareTargets([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	watcher := newFakeFileWatcher()
	watcher.addErr = errors.New("add failed")
	done, cancel := runWaitLoopForTest(t, states, watcher, 10*time.Millisecond)
	defer cancel()

	watcher.events <- fsnotify.Event{Name: filepath.Join(dir, "other.flag"), Op: fsnotify.Create}
	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := waitDoneResult(t, done)
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitLoopFallsBackAfterTickRefreshFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	states, err := prepareTargets([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	watcher := newFakeFileWatcher()
	watcher.addErr = errors.New("add failed")
	watcher.addCalls = make(chan string, 1)
	done, cancel := runWaitLoopForTest(t, states, watcher, 10*time.Millisecond)
	defer cancel()

	select {
	case <-watcher.addCalls:
	case result := <-done:
		t.Fatalf("wait returned before refresh failure was exercised: result=%+v err=%v", result.result, result.err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for refresh attempt")
	}
	if err := os.WriteFile(target, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := waitDoneResult(t, done)
	if result.Path != target || result.Index != 0 {
		t.Fatalf("unexpected result: got=%+v want path=%q index=0", result, target)
	}
}

func TestWaitLoopReturnsStatErrorAfterEvent(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "done.flag")
	states, err := prepareTargets([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	watcher := newFakeFileWatcher()
	done, cancel := runWaitLoopForTest(t, states, watcher, time.Hour)
	defer cancel()

	watcher.events <- fsnotify.Event{Name: target, Op: fsnotify.Create}

	result := waitLoopDone(t, done)
	if result.err == nil || !strings.Contains(result.err.Error(), "stat fwait target") {
		t.Fatalf("expected target stat error, got result=%+v err=%v", result.result, result.err)
	}
}

func TestRefreshWatchesFollowsMissingParentChain(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "done.flag")
	watcher := newFakeFileWatcher()
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}

	if err := state.refreshWatches(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.refreshWatches(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.refreshWatches(target); err != nil {
		t.Fatal(err)
	}

	want := []string{dir, filepath.Join(dir, "a"), filepath.Join(dir, "a", "b")}
	if got := watcher.added(); !sameStringSlice(got, want) {
		t.Fatalf("unexpected watched dirs:\ngot  %v\nwant %v", got, want)
	}
}

func TestRefreshWatchesAddsParentThatAppearsDuringRefresh(t *testing.T) {
	dir := t.TempDir()
	aDir := filepath.Join(dir, "a")
	bDir := filepath.Join(aDir, "b")
	target := filepath.Join(bDir, "done.flag")
	if err := os.MkdirAll(bDir, 0o755); err != nil {
		t.Fatal(err)
	}
	restore := hidePathOnceForStat(t, bDir, os.ErrNotExist)
	defer restore()

	watcher := newFakeFileWatcher()
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}
	if err := state.refreshWatches(target); err != nil {
		t.Fatal(err)
	}

	want := []string{aDir, bDir}
	if got := watcher.added(); !sameStringSlice(got, want) {
		t.Fatalf("unexpected watched dirs:\ngot  %v\nwant %v", got, want)
	}
}

func TestRefreshWatchesReturnsStatFailureForNextParent(t *testing.T) {
	dir := t.TempDir()
	aDir := filepath.Join(dir, "a")
	bDir := filepath.Join(aDir, "b")
	target := filepath.Join(bDir, "done.flag")
	if err := os.Mkdir(aDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statErr := errors.New("stat failed")
	restore := sequenceStatErrorsForPath(t, bDir, []error{os.ErrNotExist, statErr})
	defer restore()

	watcher := newFakeFileWatcher()
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}
	err := state.refreshWatches(target)
	if !errors.Is(err, statErr) {
		t.Fatalf("expected stat failure, got %v", err)
	}
}

func TestRefreshWatchesReportsParentBecomingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "done.flag")
	aDir := filepath.Join(dir, "a")
	bDir := filepath.Join(aDir, "b")
	if err := os.Mkdir(aDir, 0o755); err != nil {
		t.Fatal(err)
	}
	originalStat := statPath
	statPath = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == bDir {
			statPath = originalStat
			return nil, os.ErrNotExist
		}
		return originalStat(path)
	}
	t.Cleanup(func() { statPath = originalStat })

	watcher := newFakeFileWatcher()
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}
	if err := os.WriteFile(bDir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := state.refreshWatches(target)
	if err == nil || !strings.Contains(err.Error(), "parent is not a directory") {
		t.Fatalf("expected parent file error, got %v", err)
	}
}

func TestRefreshWatchesReturnsNestedAddWatchFailure(t *testing.T) {
	dir := t.TempDir()
	aDir := filepath.Join(dir, "a")
	bDir := filepath.Join(aDir, "b")
	target := filepath.Join(bDir, "done.flag")
	if err := os.MkdirAll(bDir, 0o755); err != nil {
		t.Fatal(err)
	}
	restore := hidePathOnceForStat(t, bDir, os.ErrNotExist)
	defer restore()

	watcher := newFakeFileWatcher()
	watcher.addErrs = map[string]error{bDir: errors.New("nested add failed")}
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}

	err := state.refreshWatches(target)
	if err == nil || !strings.Contains(err.Error(), "nested add failed") {
		t.Fatalf("expected nested add-watch error, got %v", err)
	}
}

func TestRefreshWatchesReturnsAddWatchFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "done.flag")
	watcher := newFakeFileWatcher()
	watcher.addErr = errors.New("add failed")
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}

	err := state.refreshWatches(target)
	if err == nil || !strings.Contains(err.Error(), "add failed") {
		t.Fatalf("expected add-watch error, got %v", err)
	}
}

func TestAddWatchSkipsAlreadyWatchedDirectory(t *testing.T) {
	dir := t.TempDir()
	watcher := newFakeFileWatcher()
	state := &waitState{
		watcher: watcher,
		watched: map[string]struct{}{
			filepath.Clean(dir): {},
		},
	}

	if err := state.addWatch(dir); err != nil {
		t.Fatal(err)
	}
	if got := watcher.added(); len(got) != 0 {
		t.Fatalf("expected no watcher adds for already-watched dir, got %v", got)
	}
}

type waitResult struct {
	result Result
	err    error
}

type fakeFileWatcher struct {
	events   chan fsnotify.Event
	errors   chan error
	addCalls chan string

	mu      sync.Mutex
	adds    []string
	addErr  error
	addErrs map[string]error
}

func newFakeFileWatcher() *fakeFileWatcher {
	return &fakeFileWatcher{
		events: make(chan fsnotify.Event, 8),
		errors: make(chan error, 8),
	}
}

func (w *fakeFileWatcher) Add(dir string) error {
	dir = filepath.Clean(dir)
	w.mu.Lock()
	w.adds = append(w.adds, dir)
	err := w.addErr
	if w.addErrs != nil {
		if pathErr := w.addErrs[dir]; pathErr != nil {
			err = pathErr
		}
	}
	addCalls := w.addCalls
	w.mu.Unlock()
	if addCalls != nil {
		select {
		case addCalls <- dir:
		default:
		}
	}
	return err
}

func (w *fakeFileWatcher) Close() error {
	return nil
}

func (w *fakeFileWatcher) EventChan() <-chan fsnotify.Event {
	return w.events
}

func (w *fakeFileWatcher) ErrorChan() <-chan error {
	return w.errors
}

func (w *fakeFileWatcher) added() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.adds...)
}

func runWaitLoopForTest(t *testing.T, states []targetState, watcher *fakeFileWatcher, interval time.Duration) (<-chan waitResult, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	ticker := time.NewTicker(interval)
	done := make(chan waitResult, 1)
	go func() {
		defer ticker.Stop()
		result, err := waitLoop(ctx, states, &waitState{watcher: watcher, watched: map[string]struct{}{}}, ticker)
		done <- waitResult{result: result, err: err}
	}()
	return done, cancel
}

func waitLoopDone(t *testing.T, done <-chan waitResult) waitResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for filewait")
	}
	return waitResult{}
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hidePathOnceForStat(t *testing.T, path string, err error) func() {
	t.Helper()
	return sequenceStatErrorsForPath(t, path, []error{err})
}

func sequenceStatErrorsForPath(t *testing.T, path string, errs []error) func() {
	t.Helper()
	originalStat := statPath
	cleanPath := filepath.Clean(path)
	remaining := append([]error(nil), errs...)
	statPath = func(candidate string) (os.FileInfo, error) {
		if filepath.Clean(candidate) == cleanPath && len(remaining) > 0 {
			err := remaining[0]
			remaining = remaining[1:]
			return nil, err
		}
		return originalStat(candidate)
	}
	return func() {
		statPath = originalStat
	}
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
