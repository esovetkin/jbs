package run

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
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
