package run

import (
	"context"
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

type Scheduler struct {
	store     *Store
	progress  *Progress
	statuses  map[string]Status
	children  map[string][]string
	depsLeft  map[string]int
	workByKey map[string]ManifestWork
	global    limiter
	steps     map[string]*limiter
	running   map[string]ManifestWork
}

func NewScheduler(store *Store, progress *Progress) *Scheduler {
	s := &Scheduler{
		store:     store,
		progress:  progress,
		statuses:  make(map[string]Status),
		children:  make(map[string][]string),
		depsLeft:  make(map[string]int),
		workByKey: make(map[string]ManifestWork),
		global:    newLimiter(store.Manifest.GlobalNProc),
		steps:     make(map[string]*limiter),
		running:   make(map[string]ManifestWork),
	}
	for _, step := range store.Manifest.Steps {
		stepLimiter := newLimiter(step.NProc)
		s.steps[step.Name] = &stepLimiter
	}
	for _, work := range store.Manifest.Work {
		key := workKey(work.Step, work.Row)
		s.workByKey[key] = work
	}
	return s
}

func (s *Scheduler) Run(ctx context.Context) Status {
	if err := s.loadStatuses(); err != nil {
		return StatusError
	}
	s.progress.Update(s.snapshot())
	ready := s.initialReady()
	done := make(chan workDone)
	for {
		started := false
		for len(ready) > 0 && ctx.Err() == nil {
			idx := s.firstStartable(ready)
			if idx < 0 {
				break
			}
			work := ready[idx]
			ready = append(ready[:idx], ready[idx+1:]...)
			s.start(ctx, work, done)
			started = true
		}
		if s.allTerminal() {
			return s.finalStatus()
		}
		if ctx.Err() != nil {
			s.waitForRunning(done)
			return StatusInterrupted
		}
		if !started && len(s.running) == 0 && len(ready) == 0 {
			return s.finalStatus()
		}
		select {
		case result := <-done:
			ready = append(ready, s.finish(result)...)
		case <-ctx.Done():
			s.waitForRunning(done)
			return StatusInterrupted
		}
	}
}

func (s *Scheduler) loadStatuses() error {
	for _, work := range s.store.Manifest.Work {
		status, err := s.store.LoadWorkStatus(work)
		if err != nil {
			return err
		}
		key := workKey(work.Step, work.Row)
		s.statuses[key] = status.Status
	}
	for _, work := range s.store.Manifest.Work {
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
	for _, work := range s.store.Manifest.Work {
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

func (s *Scheduler) start(ctx context.Context, work ManifestWork, done chan<- workDone) {
	key := workKey(work.Step, work.Row)
	s.global.acquire()
	s.steps[work.Step].acquire()
	s.running[key] = work
	s.statuses[key] = StatusRunning
	now := time.Now().UTC()
	status := WorkStatus{Schema: 1, Status: StatusRunning, Step: work.Step, Row: work.Row, StartedAt: &now}
	_ = s.store.WriteWorkStatus(work, status)
	s.progress.Update(s.snapshot())
	go func() {
		done <- workDone{key: key, work: work, result: runProcess(ctx, s.store.WorkDir(work))}
	}()
}

func (s *Scheduler) finish(done workDone) []ManifestWork {
	delete(s.running, done.key)
	s.global.release()
	s.steps[done.work.Step].release()
	status := done.result.Status
	s.statuses[done.key] = status
	now := time.Now().UTC()
	msg := ""
	if done.result.Err != nil {
		msg = done.result.Err.Error()
	}
	workStatus := WorkStatus{
		Schema:     1,
		Status:     status,
		Step:       done.work.Step,
		Row:        done.work.Row,
		FinishedAt: &now,
		ExitCode:   done.result.ExitCode,
		Error:      msg,
	}
	_ = s.store.WriteWorkStatus(done.work, workStatus)
	if status == StatusError {
		s.markBlocked(done.key, fmt.Sprintf("dependency %s failed", done.key))
		s.progress.Update(s.snapshot())
		return nil
	}
	if status == StatusInterrupted {
		s.progress.Update(s.snapshot())
		return nil
	}
	ready := make([]ManifestWork, 0)
	for _, childKey := range s.children[done.key] {
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
	s.progress.Update(s.snapshot())
	return ready
}

func (s *Scheduler) markBlocked(parentKey string, message string) {
	for _, childKey := range s.children[parentKey] {
		if s.statuses[childKey] == StatusFinished || s.statuses[childKey] == StatusRunning || s.statuses[childKey] == StatusError {
			continue
		}
		work := s.workByKey[childKey]
		s.statuses[childKey] = StatusError
		now := time.Now().UTC()
		status := WorkStatus{Schema: 1, Status: StatusError, Step: work.Step, Row: work.Row, FinishedAt: &now, Error: message}
		_ = s.store.WriteWorkStatus(work, status)
		s.markBlocked(childKey, message)
	}
}

func (s *Scheduler) waitForRunning(done <-chan workDone) {
	for len(s.running) > 0 {
		result := <-done
		if result.result.Status != StatusInterrupted {
			result.result.Status = StatusInterrupted
		}
		s.finish(result)
	}
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
		case StatusError:
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
		case StatusInterrupted:
			snap.Interrupted++
		}
	}
	return snap
}
