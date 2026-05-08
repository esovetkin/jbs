package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

const (
	rootLockName        = ".jbs.lock"
	rootLockReclaimName = ".jbs.lock.reclaim"
)

type lockInfo struct {
	Schema    int       `json:"schema"`
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname"`
	CreatedAt time.Time `json:"created_at"`
}

type lockRuntime struct {
	pid          func() int
	hostname     func() (string, error)
	now          func() time.Time
	processAlive func(pid int) (bool, error)
}

type lockClass int

const (
	lockLive lockClass = iota
	lockStaleLocal
	lockForeign
	lockMalformed
)

type lockInspection struct {
	Class lockClass
	Info  lockInfo
	Err   error
}

func acquireRootLock(root string) (func(), error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return acquireLockFile(filepath.Join(root, rootLockName))
}

func acquireExistingRootLock(root string) (func(), error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot lock benchmark root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cannot lock benchmark root %s: not a directory", root)
	}
	return acquireLockFile(filepath.Join(root, rootLockName))
}

func acquireLockFile(lockPath string) (func(), error) {
	return acquireLockFileWith(lockPath, defaultLockRuntime())
}

func acquireLockFileWith(lockPath string, rt lockRuntime) (func(), error) {
	rt = normalizeLockRuntime(rt)
	for attempts := 0; attempts < 3; attempts++ {
		unlock, err := createLockFile(lockPath, rt)
		if err == nil {
			return unlock, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("cannot acquire benchmark root lock %s: %w", lockPath, err)
		}
		if err := maybeReclaimStaleLock(lockPath, rt); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("cannot acquire benchmark root lock %s: lock changed during stale reclaim", lockPath)
}

func defaultLockRuntime() lockRuntime {
	return lockRuntime{
		pid:          os.Getpid,
		hostname:     os.Hostname,
		now:          func() time.Time { return time.Now().UTC() },
		processAlive: localProcessAlive,
	}
}

func normalizeLockRuntime(rt lockRuntime) lockRuntime {
	defaults := defaultLockRuntime()
	if rt.pid == nil {
		rt.pid = defaults.pid
	}
	if rt.hostname == nil {
		rt.hostname = defaults.hostname
	}
	if rt.now == nil {
		rt.now = defaults.now
	}
	if rt.processAlive == nil {
		rt.processAlive = defaults.processAlive
	}
	return rt
}

func localProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	case errors.Is(err, syscall.EPERM):
		return true, nil
	default:
		return true, err
	}
}

func createLockFile(lockPath string, rt lockRuntime) (func(), error) {
	rt = normalizeLockRuntime(rt)
	host, err := rt.hostname()
	if err != nil {
		return nil, fmt.Errorf("determine hostname: %w", err)
	}
	info := lockInfo{
		Schema:    1,
		PID:       rt.pid(),
		Hostname:  host,
		CreatedAt: rt.now().UTC(),
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	closed := false
	cleanup := true
	defer func() {
		if !closed {
			_ = f.Close()
		}
		if cleanup {
			_ = os.Remove(lockPath)
		}
	}()

	if err := json.NewEncoder(f).Encode(info); err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		closed = true
		return nil, err
	}
	closed = true
	if err := fsutil.SyncDir(filepath.Dir(lockPath)); err != nil {
		return nil, err
	}
	cleanup = false

	return func() {
		releaseLockFile(lockPath, info)
	}, nil
}

func readLockInfo(lockPath string) (lockInfo, error) {
	var info lockInfo
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return info, err
	}
	return info, nil
}

func inspectLock(lockPath string, rt lockRuntime) lockInspection {
	rt = normalizeLockRuntime(rt)
	info, err := readLockInfo(lockPath)
	if err != nil {
		if pid, ok := readPlainPID(lockPath); ok {
			info.PID = pid
		}
		return lockInspection{Class: lockMalformed, Info: info, Err: err}
	}
	if info.Schema != 1 || info.PID <= 0 || info.Hostname == "" || info.CreatedAt.IsZero() {
		return lockInspection{Class: lockMalformed, Info: info}
	}

	host, err := rt.hostname()
	if err != nil {
		return lockInspection{Class: lockLive, Info: info, Err: err}
	}
	if info.Hostname != host {
		return lockInspection{Class: lockForeign, Info: info}
	}

	alive, err := rt.processAlive(info.PID)
	if err != nil {
		return lockInspection{Class: lockLive, Info: info, Err: err}
	}
	if !alive {
		return lockInspection{Class: lockStaleLocal, Info: info}
	}
	return lockInspection{Class: lockLive, Info: info}
}

func readPlainPID(lockPath string) (int, bool) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func maybeReclaimStaleLock(lockPath string, rt lockRuntime) error {
	inspected := inspectLock(lockPath, rt)
	if inspected.Class == lockMalformed && errors.Is(inspected.Err, os.ErrNotExist) {
		return nil
	}
	if inspected.Class != lockStaleLocal {
		return lockHeldError(lockPath, inspected)
	}

	reclaimPath := filepath.Join(filepath.Dir(lockPath), rootLockReclaimName)
	releaseReclaim, err := createLockFile(reclaimPath, rt)
	if err != nil {
		return fmt.Errorf(
			"benchmark root lock is stale but reclaim is already in progress: %s owned by pid %d on host %q since %s: %w",
			lockPath,
			inspected.Info.PID,
			inspected.Info.Hostname,
			formatLockTime(inspected.Info.CreatedAt),
			err,
		)
	}
	defer releaseReclaim()

	latest := inspectLock(lockPath, rt)
	if latest.Class == lockMalformed && errors.Is(latest.Err, os.ErrNotExist) {
		return nil
	}
	if latest.Class != lockStaleLocal {
		return lockHeldError(lockPath, latest)
	}
	if err := os.Remove(lockPath); err != nil {
		return fmt.Errorf(
			"benchmark root lock is stale but could not be reclaimed: %s owned by pid %d on host %q since %s: %w",
			lockPath,
			latest.Info.PID,
			latest.Info.Hostname,
			formatLockTime(latest.Info.CreatedAt),
			err,
		)
	}
	if err := fsutil.SyncDir(filepath.Dir(lockPath)); err != nil {
		return fmt.Errorf(
			"benchmark root lock is stale but parent directory could not be synced after reclaim: %s owned by pid %d on host %q since %s: %w",
			lockPath,
			latest.Info.PID,
			latest.Info.Hostname,
			formatLockTime(latest.Info.CreatedAt),
			err,
		)
	}
	return nil
}

func releaseLockFile(lockPath string, owner lockInfo) {
	current, err := readLockInfo(lockPath)
	if err == nil && sameLockInfo(current, owner) {
		_ = os.Remove(lockPath)
		_ = fsutil.SyncDir(filepath.Dir(lockPath))
	}
}

func sameLockInfo(a, b lockInfo) bool {
	return a.Schema == b.Schema &&
		a.PID == b.PID &&
		a.Hostname == b.Hostname &&
		a.CreatedAt.Equal(b.CreatedAt)
}

func lockHeldError(lockPath string, inspected lockInspection) error {
	switch inspected.Class {
	case lockForeign:
		return fmt.Errorf(
			"benchmark root is locked: %s is held by pid %d on host %q since %s",
			lockPath,
			inspected.Info.PID,
			inspected.Info.Hostname,
			formatLockTime(inspected.Info.CreatedAt),
		)
	case lockLive:
		if inspected.Err != nil {
			return fmt.Errorf(
				"benchmark root is locked: %s is held by pid %d on host %q since %s: %w",
				lockPath,
				inspected.Info.PID,
				inspected.Info.Hostname,
				formatLockTime(inspected.Info.CreatedAt),
				inspected.Err,
			)
		}
		return fmt.Errorf(
			"benchmark root is locked: %s is held by pid %d on host %q since %s",
			lockPath,
			inspected.Info.PID,
			inspected.Info.Hostname,
			formatLockTime(inspected.Info.CreatedAt),
		)
	case lockMalformed:
		detail := malformedLockDetail(inspected)
		if inspected.Err != nil {
			return fmt.Errorf("benchmark root is locked: %s has invalid lock metadata%s: %w", lockPath, detail, inspected.Err)
		}
		return fmt.Errorf("benchmark root is locked: %s has invalid lock metadata%s", lockPath, detail)
	default:
		return fmt.Errorf("benchmark root is locked: %s", lockPath)
	}
}

func malformedLockDetail(inspected lockInspection) string {
	var parts []string
	if inspected.Info.PID > 0 {
		parts = append(parts, fmt.Sprintf("pid %d", inspected.Info.PID))
	}
	if inspected.Info.Hostname != "" {
		parts = append(parts, fmt.Sprintf("host %q", inspected.Info.Hostname))
	}
	if !inspected.Info.CreatedAt.IsZero() {
		parts = append(parts, "created at "+formatLockTime(inspected.Info.CreatedAt))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func formatLockTime(t time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	return t.UTC().Format(time.RFC3339)
}
