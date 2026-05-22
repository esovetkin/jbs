package run

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type limiter struct {
	limit int
	used  int
}

func newLimiter(limit int) limiter {
	if limit <= 0 {
		limit = 1
	}
	return limiter{limit: limit}
}

func (l *limiter) canAcquire() bool {
	return l.used < l.limit
}

func (l *limiter) acquire() {
	l.used++
}

func (l *limiter) release() {
	if l.used > 0 {
		l.used--
	}
}

type workDone struct {
	key    string
	work   ManifestWork
	result processResult
}

var runWorkProcess = runProcess

type SchedulerResult struct {
	Status Status
	Err    error
}

type schedulerStore interface {
	RunManifest() Manifest
	WorkDir(ManifestWork) string
	LoadWorkStatus(ManifestWork) (WorkStatus, error)
	WriteWorkStatus(ManifestWork, WorkStatus) error
}

type Scheduler struct {
	store     schedulerStore
	manifest  Manifest
	progress  *Progress
	statuses  map[string]Status
	startedAt map[string]time.Time
	children  map[string][]string
	depsLeft  map[string]int
	workByKey map[string]ManifestWork
	global    limiter
	steps     map[string]*limiter
	running   map[string]ManifestWork
}

func NewScheduler(store schedulerStore, progress *Progress) *Scheduler {
	manifest := store.RunManifest()
	s := &Scheduler{
		store:     store,
		manifest:  manifest,
		progress:  progress,
		statuses:  make(map[string]Status),
		startedAt: make(map[string]time.Time),
		children:  make(map[string][]string),
		depsLeft:  make(map[string]int),
		workByKey: make(map[string]ManifestWork),
		global:    newLimiter(manifest.GlobalNProc),
		steps:     make(map[string]*limiter),
		running:   make(map[string]ManifestWork),
	}
	for _, step := range manifest.Steps {
		stepLimiter := newLimiter(step.NProc)
		s.steps[step.Name] = &stepLimiter
	}
	for _, work := range manifest.Work {
		key := workKey(work.Step, work.Row)
		s.workByKey[key] = work
	}
	return s
}

func (s *Scheduler) Run(ctx context.Context) SchedulerResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := s.loadStatuses(); err != nil {
		return schedulerError(fmt.Errorf("load work statuses: %w", err))
	}
	s.progress.Update(s.snapshot())
	ready := s.initialReady()
	done := make(chan workDone)
	flushTicker := s.progressFlushTicker()
	var flush <-chan time.Time
	if flushTicker != nil {
		defer flushTicker.Stop()
		flush = flushTicker.C
	}
	for {
		started := false
		for len(ready) > 0 && ctx.Err() == nil {
			idx := s.firstStartable(ready)
			if idx < 0 {
				break
			}
			work := ready[idx]
			ready = append(ready[:idx], ready[idx+1:]...)
			if err := s.start(ctx, work, done); err != nil {
				cancel()
				return s.finishWithSchedulerError(done, err)
			}
			started = true
		}
		if s.allTerminal() {
			return SchedulerResult{Status: s.finalStatus()}
		}
		if ctx.Err() != nil {
			if err := s.waitForRunning(done); err != nil {
				return schedulerError(err)
			}
			return SchedulerResult{Status: StatusInterrupted}
		}
		if !started && len(s.running) == 0 && len(ready) == 0 {
			return SchedulerResult{Status: s.finalStatus()}
		}
		s.progress.Flush()
		select {
		case result := <-done:
			next, err := s.finish(result)
			if err != nil {
				cancel()
				return s.finishWithSchedulerError(done, err)
			}
			ready = append(ready, next...)
		case <-ctx.Done():
			if err := s.waitForRunning(done); err != nil {
				return schedulerError(err)
			}
			return SchedulerResult{Status: StatusInterrupted}
		case <-flush:
			s.progress.Flush()
		}
	}
}

func (s *Scheduler) progressFlushTicker() *time.Ticker {
	interval := s.progress.flushInterval()
	if interval <= 0 {
		return nil
	}
	return time.NewTicker(interval)
}

func schedulerError(err error) SchedulerResult {
	return SchedulerResult{Status: StatusError, Err: err}
}

func (s *Scheduler) loadStatuses() error {
	for _, work := range s.manifest.Work {
		status, err := s.store.LoadWorkStatus(work)
		if err != nil {
			return err
		}
		key := workKey(work.Step, work.Row)
		s.statuses[key] = status.Status
	}
	for _, work := range s.manifest.Work {
		key := workKey(work.Step, work.Row)
		for _, dep := range work.Deps {
			depKey := workKey(dep.Step, dep.Row)
			s.children[depKey] = append(s.children[depKey], key)
			if s.statuses[depKey] != StatusFinished {
				s.depsLeft[key]++
			}
		}
	}
	return nil
}

func (s *Scheduler) initialReady() []ManifestWork {
	ready := make([]ManifestWork, 0)
	for _, work := range s.manifest.Work {
		key := workKey(work.Step, work.Row)
		status := s.statuses[key]
		if status == StatusFinished || status == StatusRunning {
			continue
		}
		if s.depsLeft[key] == 0 {
			ready = append(ready, work)
		}
	}
	return ready
}

func (s *Scheduler) firstStartable(ready []ManifestWork) int {
	for i, work := range ready {
		stepLimiter := s.steps[work.Step]
		if stepLimiter == nil {
			continue
		}
		if s.global.canAcquire() && stepLimiter.canAcquire() {
			return i
		}
	}
	return -1
}

func (s *Scheduler) start(ctx context.Context, work ManifestWork, done chan<- workDone) error {
	key := workKey(work.Step, work.Row)
	s.global.acquire()
	s.steps[work.Step].acquire()
	now := time.Now().UTC()
	status := WorkStatus{Schema: 1, Status: StatusRunning, Step: work.Step, Row: work.Row, StartedAt: &now}
	if err := s.store.WriteWorkStatus(work, status); err != nil {
		s.global.release()
		s.steps[work.Step].release()
		s.statuses[key] = StatusError
		s.progress.Update(s.snapshot())
		return fmt.Errorf("persist RUNNING status for %s: %w", key, err)
	}
	s.startedAt[key] = now
	s.running[key] = work
	s.statuses[key] = StatusRunning
	s.progress.Update(s.snapshot())
	go func() {
		done <- workDone{key: key, work: work, result: runWorkProcess(ctx, s.store.WorkDir(work))}
	}()
	return nil
}

func (s *Scheduler) finish(done workDone) ([]ManifestWork, error) {
	delete(s.running, done.key)
	s.global.release()
	s.steps[done.work.Step].release()
	status := done.result.Status
	now := time.Now().UTC()
	startedAt, ok := s.startedAt[done.key]
	var startedPtr *time.Time
	duration := durationPtr(0)
	if ok {
		started := startedAt
		startedPtr = &started
		duration = durationPtr(durationSeconds(startedAt, now))
	}
	delete(s.startedAt, done.key)
	msg := ""
	if done.result.Err != nil {
		msg = done.result.Err.Error()
	}
	workStatus := WorkStatus{
		Schema:     1,
		Status:     status,
		Step:       done.work.Step,
		Row:        done.work.Row,
		StartedAt:  startedPtr,
		FinishedAt: &now,
		Duration:   duration,
		ExitCode:   done.result.ExitCode,
		Error:      msg,
	}
	if err := s.store.WriteWorkStatus(done.work, workStatus); err != nil {
		s.statuses[done.key] = StatusError
		s.progress.Update(s.snapshot())
		return nil, fmt.Errorf("persist final status for %s: %w", done.key, err)
	}
	s.statuses[done.key] = status
	if status == StatusError {
		if err := s.markBlocked(done.key, fmt.Sprintf("dependency %s failed", done.key)); err != nil {
			s.progress.Update(s.snapshot())
			return nil, err
		}
		s.progress.Update(s.snapshot())
		return nil, nil
	}
	if status == StatusInterrupted {
		s.progress.Update(s.snapshot())
		return nil, nil
	}
	ready := s.releaseChildren(done.key)
	s.progress.Update(s.snapshot())
	return ready, nil
}

func (s *Scheduler) releaseChildren(parentKey string) []ManifestWork {
	ready := make([]ManifestWork, 0)
	for _, childKey := range s.children[parentKey] {
		if s.statuses[childKey] == StatusFinished {
			continue
		}
		if s.depsLeft[childKey] > 0 {
			s.depsLeft[childKey]--
		}
		if s.depsLeft[childKey] == 0 {
			ready = append(ready, s.workByKey[childKey])
		}
	}
	return ready
}

func (s *Scheduler) markBlocked(parentKey string, message string) error {
	var out error
	for _, childKey := range s.children[parentKey] {
		if s.statuses[childKey] == StatusFinished ||
			s.statuses[childKey] == StatusRunning ||
			s.statuses[childKey] == StatusError ||
			s.statuses[childKey] == StatusBlocked {
			continue
		}
		work := s.workByKey[childKey]
		s.statuses[childKey] = StatusBlocked
		now := time.Now().UTC()
		status := WorkStatus{Schema: 1, Status: StatusBlocked, Step: work.Step, Row: work.Row, FinishedAt: &now, Duration: durationPtr(0), Error: message}
		if err := s.store.WriteWorkStatus(work, status); err != nil {
			out = errors.Join(out, fmt.Errorf("persist blocked status for %s: %w", childKey, err))
			continue
		}
		if err := s.markBlocked(childKey, message); err != nil {
			out = errors.Join(out, err)
		}
	}
	return out
}

func (s *Scheduler) waitForRunning(done <-chan workDone) error {
	var out error
	for len(s.running) > 0 {
		result := <-done
		if result.result.Status != StatusInterrupted {
			result.result.Status = StatusInterrupted
		}
		_, err := s.finish(result)
		out = errors.Join(out, err)
	}
	return out
}

func (s *Scheduler) finishWithSchedulerError(done <-chan workDone, err error) SchedulerResult {
	if drainErr := s.waitForRunning(done); drainErr != nil {
		err = errors.Join(err, drainErr)
	}
	s.progress.Update(s.snapshot())
	return schedulerError(err)
}

func (s *Scheduler) allTerminal() bool {
	for key, status := range s.statuses {
		if _, running := s.running[key]; running {
			return false
		}
		if status == StatusNotStarted || status == StatusRunning || status == StatusInterrupted {
			return false
		}
	}
	return true
}

func (s *Scheduler) finalStatus() Status {
	hasError := false
	hasInterrupted := false
	for _, status := range s.statuses {
		switch status {
		case StatusError, StatusBlocked:
			hasError = true
		case StatusInterrupted:
			hasInterrupted = true
		case StatusNotStarted, StatusRunning:
			hasInterrupted = true
		}
	}
	switch {
	case hasInterrupted:
		return StatusInterrupted
	case hasError:
		return StatusError
	default:
		return StatusFinished
	}
}

func (s *Scheduler) snapshot() ProgressSnapshot {
	snap := ProgressSnapshot{Total: len(s.statuses)}
	for _, status := range s.statuses {
		switch status {
		case StatusNotStarted:
			snap.NotStarted++
		case StatusRunning:
			snap.Running++
		case StatusFinished:
			snap.Finished++
		case StatusError:
			snap.Error++
		case StatusBlocked:
			snap.Blocked++
		case StatusInterrupted:
			snap.Interrupted++
		}
	}
	return snap
}
