package filewait

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Options struct {
	Ready           chan<- struct{}
	ExitIfExists    bool
	PollInterval    time.Duration
	DisableFSNotify bool
}

type Result struct {
	Path  string
	Index int
}

var afterPrepareTargetsForTest func()

const defaultPollInterval = time.Second

func Wait(ctx context.Context, target string) error {
	_, err := WaitAnyWithOptions(ctx, []string{target}, Options{})
	return err
}

func WaitWithOptions(ctx context.Context, target string, opts Options) error {
	_, err := WaitAnyWithOptions(ctx, []string{target}, opts)
	return err
}

func WaitAny(ctx context.Context, targets []string) (Result, error) {
	return WaitAnyWithOptions(ctx, targets, Options{})
}

func WaitAnyWithOptions(ctx context.Context, targets []string, opts Options) (Result, error) {
	if len(targets) == 0 {
		return Result{}, errors.New("fwait requires at least one target")
	}

	states, err := prepareTargets(targets)
	if err != nil {
		return Result{}, err
	}
	if opts.ExitIfExists {
		if result, ok := firstExisting(states); ok {
			return result, nil
		}
	}
	if afterPrepareTargetsForTest != nil {
		afterPrepareTargetsForTest()
	}

	watcher, state, err := startWatcher(states, opts)
	if err != nil {
		return Result{}, err
	}
	if watcher != nil {
		defer watcher.Close()
	}
	if result, ok, err := firstCompleted(states); err != nil {
		return Result{}, err
	} else if ok {
		return result, nil
	}

	ticker := time.NewTicker(pollInterval(opts))
	defer ticker.Stop()
	signalReady(opts.Ready)

	return waitLoop(ctx, states, state, ticker)
}

func pollInterval(opts Options) time.Duration {
	if opts.PollInterval > 0 {
		return opts.PollInterval
	}
	return defaultPollInterval
}

func startWatcher(states []targetState, opts Options) (*fsnotify.Watcher, *waitState, error) {
	if opts.DisableFSNotify {
		return nil, nil, nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, nil
	}
	state := &waitState{watcher: watcher, watched: map[string]struct{}{}}
	if err := state.refreshAllWatches(states); err != nil {
		watcher.Close()
		return nil, nil, nil
	}
	return watcher, state, nil
}

func waitLoop(ctx context.Context, states []targetState, state *waitState, ticker *time.Ticker) (Result, error) {
	var events <-chan fsnotify.Event
	var watcherErrors <-chan error
	if state != nil && state.watcher != nil {
		events = state.watcher.Events
		watcherErrors = state.watcher.Errors
	}

	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case err, ok := <-watcherErrors:
			if !ok {
				watcherErrors = nil
				continue
			}
			if err != nil {
				watcherErrors = nil
				events = nil
			}
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if state != nil {
				if err := state.refreshAllWatches(states); err != nil {
					events = nil
					watcherErrors = nil
				}
			}
			result, ok, err := firstCompletedAfterEvent(states, event)
			if err != nil {
				return Result{}, err
			}
			if ok {
				return result, nil
			}
		case <-ticker.C:
			if state != nil && events != nil {
				if err := state.refreshAllWatches(states); err != nil {
					events = nil
					watcherErrors = nil
				}
			}
			result, ok, err := firstCompleted(states)
			if err != nil {
				return Result{}, err
			}
			if ok {
				return result, nil
			}
		}
	}
}

type targetState struct {
	input   string
	abs     string
	initial snapshot
}

func prepareTargets(inputs []string) ([]targetState, error) {
	states := make([]targetState, 0, len(inputs))
	for _, input := range inputs {
		abs, err := filepath.Abs(input)
		if err != nil {
			return nil, fmt.Errorf("resolve fwait target %q: %w", input, err)
		}
		abs = filepath.Clean(abs)
		initial, err := statSnapshot(abs)
		if err != nil {
			return nil, fmt.Errorf("stat fwait target %s: %w", abs, err)
		}
		if initial.exists && initial.isDir {
			return nil, fmt.Errorf("fwait target is a directory: %s", input)
		}
		states = append(states, targetState{
			input:   input,
			abs:     abs,
			initial: initial,
		})
	}
	return states, nil
}

func firstExisting(targets []targetState) (Result, bool) {
	for i, target := range targets {
		if target.initial.exists {
			return Result{Path: target.input, Index: i}, true
		}
	}
	return Result{}, false
}

func firstCompleted(targets []targetState) (Result, bool, error) {
	for i, target := range targets {
		current, err := statSnapshot(target.abs)
		if err != nil {
			return Result{}, false, fmt.Errorf("stat fwait target %s: %w", target.abs, err)
		}
		ok, completeErr := complete(target.initial, current)
		if completeErr != nil {
			return Result{}, false, fmt.Errorf("%w: %s", completeErr, target.input)
		}
		if ok {
			return Result{Path: target.input, Index: i}, true, nil
		}
	}
	return Result{}, false, nil
}

func firstCompletedAfterEvent(targets []targetState, event fsnotify.Event) (Result, bool, error) {
	for i, target := range targets {
		current, err := statSnapshot(target.abs)
		if err != nil {
			return Result{}, false, fmt.Errorf("stat fwait target %s: %w", target.abs, err)
		}
		ok, completeErr := complete(target.initial, current)
		if completeErr != nil {
			return Result{}, false, fmt.Errorf("%w: %s", completeErr, target.input)
		}
		if ok || (target.initial.exists && eventNamesTarget(target.abs, event)) {
			return Result{Path: target.input, Index: i}, true, nil
		}
	}
	return Result{}, false, nil
}

type snapshot struct {
	exists  bool
	isDir   bool
	size    int64
	mode    os.FileMode
	modTime time.Time
	info    os.FileInfo
}

func statSnapshot(path string) (snapshot, error) {
	info, err := os.Stat(path)
	if err == nil {
		return snapshot{
			exists:  true,
			isDir:   info.IsDir(),
			size:    info.Size(),
			mode:    info.Mode(),
			modTime: info.ModTime(),
			info:    info,
		}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return snapshot{}, nil
	}
	return snapshot{}, err
}

func complete(initial, current snapshot) (bool, error) {
	if current.exists && current.isDir {
		return false, errors.New("fwait target is a directory")
	}
	if !initial.exists {
		return current.exists, nil
	}
	return !sameSnapshot(initial, current), nil
}

func sameSnapshot(a, b snapshot) bool {
	return a.exists == b.exists &&
		a.isDir == b.isDir &&
		sameFile(a, b) &&
		a.size == b.size &&
		a.mode == b.mode &&
		a.modTime.Equal(b.modTime)
}

func sameFile(a, b snapshot) bool {
	if !a.exists || !b.exists {
		return true
	}
	return os.SameFile(a.info, b.info)
}

type waitState struct {
	watcher *fsnotify.Watcher
	watched map[string]struct{}
}

func (s *waitState) refreshAllWatches(targets []targetState) error {
	for _, target := range targets {
		if err := s.refreshWatches(target.abs); err != nil {
			return err
		}
	}
	return nil
}

func (s *waitState) refreshWatches(target string) error {
	ancestor, err := nearestExistingDir(target)
	if err != nil {
		return err
	}
	if err := s.addWatch(ancestor); err != nil {
		return err
	}

	parent := filepath.Dir(target)
	for dir := ancestor; dir != parent; {
		next, err := nextPathComponent(dir, parent)
		if err != nil {
			return err
		}
		info, err := os.Stat(next)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("fwait parent is not a directory: %s", next)
		}
		if err := s.addWatch(next); err != nil {
			return err
		}
		dir = next
	}
	return nil
}

func (s *waitState) addWatch(dir string) error {
	dir = filepath.Clean(dir)
	if _, ok := s.watched[dir]; ok {
		return nil
	}
	if err := s.watcher.Add(dir); err != nil {
		return err
	}
	s.watched[dir] = struct{}{}
	return nil
}

func nearestExistingDir(path string) (string, error) {
	dir := filepath.Dir(path)
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("fwait parent is not a directory: %s", dir)
			}
			return dir, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		dir = next
	}
}

func nextPathComponent(dir, target string) (string, error) {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return dir, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%s is not below %s", target, dir)
	}
	name := rel
	if idx := strings.IndexRune(rel, os.PathSeparator); idx >= 0 {
		name = rel[:idx]
	}
	return filepath.Join(dir, name), nil
}

func eventNamesTarget(target string, event fsnotify.Event) bool {
	return filepath.Clean(event.Name) == target
}

func signalReady(ch chan<- struct{}) {
	if ch != nil {
		close(ch)
	}
}
