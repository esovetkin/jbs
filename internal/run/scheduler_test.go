package run

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSchedulerStore struct {
	manifest Manifest
	statuses map[string]WorkStatus
	load     func(ManifestWork) (WorkStatus, error)
	write    func(ManifestWork, WorkStatus) error
}

func newFakeSchedulerStore(manifest Manifest) *fakeSchedulerStore {
	statuses := make(map[string]WorkStatus, len(manifest.Work))
	for _, work := range manifest.Work {
		statuses[workKey(work.Step, work.Row)] = WorkStatus{
			Schema: 1,
			Status: StatusNotStarted,
			Step:   work.Step,
			Row:    work.Row,
		}
	}
	return &fakeSchedulerStore{manifest: manifest, statuses: statuses}
}

func (f *fakeSchedulerStore) RunManifest() Manifest {
	return f.manifest
}

func (f *fakeSchedulerStore) WorkDir(work ManifestWork) string {
	return workKey(work.Step, work.Row)
}

func (f *fakeSchedulerStore) LoadWorkStatus(work ManifestWork) (WorkStatus, error) {
	if f.load != nil {
		return f.load(work)
	}
	return f.statuses[workKey(work.Step, work.Row)], nil
}

func (f *fakeSchedulerStore) WriteWorkStatus(work ManifestWork, status WorkStatus) error {
	if f.write != nil {
		if err := f.write(work, status); err != nil {
			return err
		}
	}
	f.statuses[workKey(work.Step, work.Row)] = status
	return nil
}

func TestSchedulerStartStatusWriteFailureStopsBeforeProcess(t *testing.T) {
	manifest := schedulerTestManifest(
		ManifestWork{Step: "s", Row: 0, Dir: "000000"},
	)
	store := newFakeSchedulerStore(manifest)
	store.write = func(work ManifestWork, status WorkStatus) error {
		if status.Status == StatusRunning {
			return errors.New("disk full")
		}
		return nil
	}

	var launched atomic.Int32
	withRunWorkProcess(t, func(context.Context, string) processResult {
		launched.Add(1)
		return processResult{Status: StatusFinished}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "persist RUNNING status") {
		t.Fatalf("error = %v, want RUNNING persistence error", result.Err)
	}
	if launched.Load() != 0 {
		t.Fatalf("process launches = %d, want 0", launched.Load())
	}
}

func TestSchedulerFinishStatusWriteFailureStopsDependents(t *testing.T) {
	parent := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	child := ManifestWork{Step: "s", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	store := newFakeSchedulerStore(schedulerTestManifest(parent, child))
	store.write = func(work ManifestWork, status WorkStatus) error {
		if work.Row == parent.Row && status.Status == StatusFinished {
			return errors.New("stale file handle")
		}
		return nil
	}

	var launched atomic.Int32
	withRunWorkProcess(t, func(context.Context, string) processResult {
		launched.Add(1)
		code := 0
		return processResult{Status: StatusFinished, ExitCode: &code}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "persist final status") {
		t.Fatalf("error = %v, want final status persistence error", result.Err)
	}
	if launched.Load() != 1 {
		t.Fatalf("process launches = %d, want only parent", launched.Load())
	}
	if got := store.statuses[workKey(child.Step, child.Row)].Status; got != StatusNotStarted {
		t.Fatalf("child status = %s, want %s", got, StatusNotStarted)
	}
}

func TestSchedulerBlockedStatusWriteFailureIsReturned(t *testing.T) {
	parent := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	child := ManifestWork{Step: "s", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	store := newFakeSchedulerStore(schedulerTestManifest(parent, child))
	store.write = func(work ManifestWork, status WorkStatus) error {
		if work.Row == child.Row && status.Status == StatusError {
			return errors.New("permission denied")
		}
		return nil
	}

	var launched atomic.Int32
	withRunWorkProcess(t, func(context.Context, string) processResult {
		launched.Add(1)
		code := 2
		return processResult{Status: StatusError, ExitCode: &code, Err: errors.New("exit status 2")}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "persist blocked status") {
		t.Fatalf("error = %v, want blocked status persistence error", result.Err)
	}
	if launched.Load() != 1 {
		t.Fatalf("process launches = %d, want only parent", launched.Load())
	}
}

func TestSchedulerLoadStatusFailureIsReturned(t *testing.T) {
	store := newFakeSchedulerStore(schedulerTestManifest(
		ManifestWork{Step: "s", Row: 0, Dir: "000000"},
	))
	store.load = func(ManifestWork) (WorkStatus, error) {
		return WorkStatus{}, errors.New("cannot read status")
	}

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "load work statuses") {
		t.Fatalf("error = %v, want load status error", result.Err)
	}
}

func TestSchedulerRunsDependencyTreeToFinished(t *testing.T) {
	parent := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	child := ManifestWork{Step: "s", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	alreadyFinished := ManifestWork{Step: "s", Row: 2, Dir: "000002", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	store := newFakeSchedulerStore(schedulerTestManifest(parent, child, alreadyFinished))
	store.statuses[workKey(alreadyFinished.Step, alreadyFinished.Row)] = WorkStatus{
		Schema: 1,
		Status: StatusFinished,
		Step:   alreadyFinished.Step,
		Row:    alreadyFinished.Row,
	}

	var launched []string
	withRunWorkProcess(t, func(_ context.Context, dir string) processResult {
		launched = append(launched, dir)
		code := 0
		return processResult{Status: StatusFinished, ExitCode: &code}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusFinished {
		t.Fatalf("status = %s, want %s: %v", result.Status, StatusFinished, result.Err)
	}
	if result.Err != nil {
		t.Fatalf("error = %v, want nil", result.Err)
	}
	wantLaunches := []string{"s/000000", "s/000001"}
	if strings.Join(launched, ",") != strings.Join(wantLaunches, ",") {
		t.Fatalf("launches = %v, want %v", launched, wantLaunches)
	}
	for _, work := range []ManifestWork{parent, child, alreadyFinished} {
		if got := store.statuses[workKey(work.Step, work.Row)].Status; got != StatusFinished {
			t.Fatalf("%s status = %s, want %s", workKey(work.Step, work.Row), got, StatusFinished)
		}
	}
}

func TestSchedulerAlreadyFinishedWorkCompletesWithoutLaunch(t *testing.T) {
	work := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	store := newFakeSchedulerStore(schedulerTestManifest(work))
	store.statuses[workKey(work.Step, work.Row)] = WorkStatus{
		Schema: 1,
		Status: StatusFinished,
		Step:   work.Step,
		Row:    work.Row,
	}

	var launched atomic.Int32
	withRunWorkProcess(t, func(context.Context, string) processResult {
		launched.Add(1)
		return processResult{Status: StatusFinished}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusFinished {
		t.Fatalf("status = %s, want %s", result.Status, StatusFinished)
	}
	if launched.Load() != 0 {
		t.Fatalf("process launches = %d, want 0", launched.Load())
	}
}

func TestSchedulerInitialRunningStatusReturnsInterruptedWithoutLaunch(t *testing.T) {
	work := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	store := newFakeSchedulerStore(schedulerTestManifest(work))
	store.statuses[workKey(work.Step, work.Row)] = WorkStatus{
		Schema: 1,
		Status: StatusRunning,
		Step:   work.Step,
		Row:    work.Row,
	}

	var launched atomic.Int32
	withRunWorkProcess(t, func(context.Context, string) processResult {
		launched.Add(1)
		return processResult{Status: StatusFinished}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusInterrupted {
		t.Fatalf("status = %s, want %s", result.Status, StatusInterrupted)
	}
	if launched.Load() != 0 {
		t.Fatalf("process launches = %d, want 0", launched.Load())
	}
}

func TestSchedulerMarksDependentTreeBlockedOnProcessError(t *testing.T) {
	parent := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	child := ManifestWork{Step: "s", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	grandchild := ManifestWork{Step: "s", Row: 2, Dir: "000002", Deps: []ManifestWorkRef{{Step: "s", Row: 1}}}
	store := newFakeSchedulerStore(schedulerTestManifest(parent, child, grandchild))

	var launched atomic.Int32
	withRunWorkProcess(t, func(context.Context, string) processResult {
		launched.Add(1)
		code := 2
		return processResult{Status: StatusError, ExitCode: &code, Err: errors.New("exit status 2")}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err != nil {
		t.Fatalf("error = %v, want nil scheduler error", result.Err)
	}
	if launched.Load() != 1 {
		t.Fatalf("process launches = %d, want only parent", launched.Load())
	}
	for _, work := range []ManifestWork{parent, child, grandchild} {
		key := workKey(work.Step, work.Row)
		if got := store.statuses[key].Status; got != StatusError {
			t.Fatalf("%s status = %s, want %s", key, got, StatusError)
		}
	}
	wantMessage := "dependency s/000000 failed"
	if got := store.statuses[workKey(grandchild.Step, grandchild.Row)].Error; got != wantMessage {
		t.Fatalf("grandchild error = %q, want %q", got, wantMessage)
	}
}

func TestSchedulerMarkBlockedSkipsTerminalChildren(t *testing.T) {
	parent := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	finished := ManifestWork{Step: "s", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	running := ManifestWork{Step: "s", Row: 2, Dir: "000002", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	failed := ManifestWork{Step: "s", Row: 3, Dir: "000003", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	store := newFakeSchedulerStore(schedulerTestManifest(parent, finished, running, failed))

	var writes atomic.Int32
	store.write = func(ManifestWork, WorkStatus) error {
		writes.Add(1)
		return nil
	}

	s := NewScheduler(store, nil)
	parentKey := workKey(parent.Step, parent.Row)
	s.children[parentKey] = []string{
		workKey(finished.Step, finished.Row),
		workKey(running.Step, running.Row),
		workKey(failed.Step, failed.Row),
	}
	s.statuses[workKey(finished.Step, finished.Row)] = StatusFinished
	s.statuses[workKey(running.Step, running.Row)] = StatusRunning
	s.statuses[workKey(failed.Step, failed.Row)] = StatusError

	if err := s.markBlocked(parentKey, "blocked"); err != nil {
		t.Fatalf("markBlocked error = %v, want nil", err)
	}
	if writes.Load() != 0 {
		t.Fatalf("writes = %d, want 0", writes.Load())
	}
}

func TestSchedulerRecursiveBlockedStatusWriteFailureIsReturned(t *testing.T) {
	parent := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	child := ManifestWork{Step: "s", Row: 1, Dir: "000001", Deps: []ManifestWorkRef{{Step: "s", Row: 0}}}
	grandchild := ManifestWork{Step: "s", Row: 2, Dir: "000002", Deps: []ManifestWorkRef{{Step: "s", Row: 1}}}
	store := newFakeSchedulerStore(schedulerTestManifest(parent, child, grandchild))
	store.write = func(work ManifestWork, status WorkStatus) error {
		if work.Row == grandchild.Row && status.Status == StatusError {
			return errors.New("cannot persist grandchild")
		}
		return nil
	}

	withRunWorkProcess(t, func(context.Context, string) processResult {
		code := 2
		return processResult{Status: StatusError, ExitCode: &code, Err: errors.New("exit status 2")}
	})

	result := NewScheduler(store, nil).Run(context.Background())
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "cannot persist grandchild") {
		t.Fatalf("error = %v, want recursive blocked status persistence error", result.Err)
	}
}

func TestSchedulerCancellationInterruptsRunningWork(t *testing.T) {
	store := newFakeSchedulerStore(schedulerTestManifest(
		ManifestWork{Step: "s", Row: 0, Dir: "000000"},
	))
	started := make(chan struct{})
	withRunWorkProcess(t, func(ctx context.Context, _ string) processResult {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		return processResult{Status: StatusFinished}
	})

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan SchedulerResult, 1)
	go func() {
		resultCh <- NewScheduler(store, nil).Run(ctx)
	}()

	<-started
	cancel()
	select {
	case result := <-resultCh:
		if result.Status != StatusInterrupted {
			t.Fatalf("status = %s, want %s", result.Status, StatusInterrupted)
		}
		if result.Err != nil {
			t.Fatalf("error = %v, want nil", result.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler cancellation")
	}
	if got := store.statuses[workKey("s", 0)].Status; got != StatusInterrupted {
		t.Fatalf("work status = %s, want %s", got, StatusInterrupted)
	}
}

func TestSchedulerCancellationStatusWriteFailureIsReturned(t *testing.T) {
	work := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	store := newFakeSchedulerStore(schedulerTestManifest(work))
	store.write = func(_ ManifestWork, status WorkStatus) error {
		if status.Status == StatusInterrupted {
			return errors.New("cannot persist cancellation")
		}
		return nil
	}

	started := make(chan struct{})
	withRunWorkProcess(t, func(ctx context.Context, _ string) processResult {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		return processResult{Status: StatusInterrupted}
	})

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan SchedulerResult, 1)
	go func() {
		resultCh <- NewScheduler(store, nil).Run(ctx)
	}()

	<-started
	cancel()
	select {
	case result := <-resultCh:
		if result.Status != StatusError {
			t.Fatalf("status = %s, want %s", result.Status, StatusError)
		}
		if result.Err == nil || !strings.Contains(result.Err.Error(), "cannot persist cancellation") {
			t.Fatalf("error = %v, want cancellation persistence error", result.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scheduler cancellation")
	}
}

func TestSchedulerFinishWithSchedulerErrorJoinsDrainFailure(t *testing.T) {
	work := ManifestWork{Step: "s", Row: 0, Dir: "000000"}
	store := newFakeSchedulerStore(schedulerTestManifest(work))
	store.write = func(_ ManifestWork, status WorkStatus) error {
		if status.Status == StatusInterrupted {
			return errors.New("cannot persist interrupt")
		}
		return nil
	}
	s := NewScheduler(store, nil)
	key := workKey(work.Step, work.Row)
	s.running[key] = work
	s.statuses[key] = StatusRunning

	done := make(chan workDone, 1)
	done <- workDone{key: key, work: work, result: processResult{Status: StatusFinished}}

	result := s.finishWithSchedulerError(done, errors.New("scheduler failure"))
	if result.Status != StatusError {
		t.Fatalf("status = %s, want %s", result.Status, StatusError)
	}
	if result.Err == nil {
		t.Fatal("error = nil, want joined scheduler and drain errors")
	}
	for _, want := range []string{"scheduler failure", "cannot persist interrupt"} {
		if !strings.Contains(result.Err.Error(), want) {
			t.Fatalf("error = %q, want to contain %q", result.Err.Error(), want)
		}
	}
}

func TestSchedulerFirstStartableSkipsUnknownStep(t *testing.T) {
	s := &Scheduler{
		global: newLimiter(1),
		steps:  map[string]*limiter{"s": limiterPtr(newLimiter(1))},
	}
	ready := []ManifestWork{{Step: "missing", Row: 0}, {Step: "s", Row: 0}}
	if got := s.firstStartable(ready); got != 1 {
		t.Fatalf("firstStartable = %d, want 1", got)
	}
}

func TestSchedulerFinalStatusAndAllTerminalClassifyIncompleteState(t *testing.T) {
	s := &Scheduler{
		statuses: map[string]Status{
			"done":    StatusFinished,
			"pending": StatusNotStarted,
		},
		running: map[string]ManifestWork{},
	}
	if s.allTerminal() {
		t.Fatal("allTerminal = true, want false for not-started work")
	}
	if got := s.finalStatus(); got != StatusInterrupted {
		t.Fatalf("finalStatus = %s, want %s", got, StatusInterrupted)
	}

	s.statuses["pending"] = StatusError
	if !s.allTerminal() {
		t.Fatal("allTerminal = false, want true for finished/error work")
	}
	if got := s.finalStatus(); got != StatusError {
		t.Fatalf("finalStatus = %s, want %s", got, StatusError)
	}

	s.statuses["pending"] = StatusInterrupted
	if s.allTerminal() {
		t.Fatal("allTerminal = true, want false for interrupted work")
	}
	if got := s.finalStatus(); got != StatusInterrupted {
		t.Fatalf("finalStatus = %s, want %s", got, StatusInterrupted)
	}
}

func TestSchedulerResultMessageUsesSchedulerError(t *testing.T) {
	err := errors.New("persist final status for s/000000: disk full")
	got := schedulerResultMessage(SchedulerResult{Status: StatusError, Err: err})
	if got != err.Error() {
		t.Fatalf("message = %q, want %q", got, err.Error())
	}
	if got := schedulerResultMessage(SchedulerResult{Status: StatusError}); got != finalMessage(StatusError) {
		t.Fatalf("message = %q, want generic status error", got)
	}
}

func schedulerTestManifest(work ...ManifestWork) Manifest {
	return Manifest{
		Schema:      1,
		GlobalNProc: 1,
		Steps:       []ManifestStep{{Name: "s", Dir: "s", NProc: 1}},
		Work:        work,
	}
}

func withRunWorkProcess(t *testing.T, fn func(context.Context, string) processResult) {
	t.Helper()
	old := runWorkProcess
	runWorkProcess = fn
	t.Cleanup(func() {
		runWorkProcess = old
	})
}
