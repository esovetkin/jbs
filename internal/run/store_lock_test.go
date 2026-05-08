package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAcquireExistingRootLockDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	_, err := acquireExistingRootLock(root)
	if err == nil {
		t.Fatal("expected missing root lock acquisition to fail")
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("continue lock should not create missing root, stat error: %v", statErr)
	}
}

func TestAcquireExistingRootLockRejectsNonDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	if err := os.WriteFile(root, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireExistingRootLock(root); err == nil {
		t.Fatal("expected non-directory root lock acquisition to fail")
	}
}

func TestRootLockExcludesSecondOwnerAndAllowsRelock(t *testing.T) {
	root := t.TempDir()
	unlock, err := acquireExistingRootLock(root)
	if err != nil {
		t.Fatal(err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireExistingRootLock(root); err == nil {
		t.Fatal("expected second lock acquisition to fail")
	} else if text := err.Error(); !strings.Contains(text, "locked") ||
		!strings.Contains(text, ".jbs.lock") ||
		!strings.Contains(text, strconv.Itoa(os.Getpid())) ||
		!strings.Contains(text, hostname) {
		t.Fatalf("expected metadata-rich lock error, got %q", text)
	}

	unlock()

	unlock, err = acquireExistingRootLock(root)
	if err != nil {
		t.Fatalf("expected lock acquisition after unlock to succeed: %v", err)
	}
	unlock()
}

func TestRootLockWritesMetadata(t *testing.T) {
	root := t.TempDir()
	unlock, err := acquireExistingRootLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	info := readLockInfoForTest(t, filepath.Join(root, rootLockName))
	if info.Schema != 1 {
		t.Fatalf("schema = %d, want 1", info.Schema)
	}
	if info.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", info.PID, os.Getpid())
	}
	if info.Hostname != hostname {
		t.Fatalf("hostname = %q, want %q", info.Hostname, hostname)
	}
	if info.CreatedAt.IsZero() {
		t.Fatal("expected non-zero creation time")
	}
}

func TestRootLockReclaimsStaleLocalLock(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	host := "node-a"
	writeLockInfoForTest(t, lockPath, lockInfo{
		Schema:    1,
		PID:       4444,
		Hostname:  host,
		CreatedAt: time.Unix(100, 0).UTC(),
	})

	rt := testLockRuntime(host, 9999, func(pid int) (bool, error) {
		if pid == 4444 {
			return false, nil
		}
		return true, nil
	})
	unlock, err := acquireLockFileWith(lockPath, rt)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	info := readLockInfoForTest(t, lockPath)
	if info.PID != 9999 || info.Hostname != host {
		t.Fatalf("unexpected replacement lock: %#v", info)
	}
	if _, err := os.Stat(filepath.Join(root, rootLockReclaimName)); !os.IsNotExist(err) {
		t.Fatalf("expected reclaim guard cleanup, stat error: %v", err)
	}
}

func TestRootLockDoesNotReclaimForeignHost(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	original := lockInfo{
		Schema:    1,
		PID:       4444,
		Hostname:  "other-node",
		CreatedAt: time.Unix(100, 0).UTC(),
	}
	writeLockInfoForTest(t, lockPath, original)

	_, err := acquireLockFileWith(lockPath, testLockRuntime("this-node", 9999, nil))
	if err == nil {
		t.Fatal("expected foreign host lock to block acquisition")
	}
	if text := err.Error(); !strings.Contains(text, "other-node") || !strings.Contains(text, "4444") || !strings.Contains(text, ".jbs.lock") {
		t.Fatalf("expected foreign owner diagnostic, got %q", text)
	}
	if got := readLockInfoForTest(t, lockPath); !sameLockInfo(got, original) {
		t.Fatalf("foreign lock changed: %#v", got)
	}
}

func TestRootLockDoesNotReclaimMalformedLock(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	if err := os.WriteFile(lockPath, []byte("4444\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := acquireLockFileWith(lockPath, testLockRuntime("this-node", 9999, nil))
	if err == nil {
		t.Fatal("expected malformed lock to block acquisition")
	}
	if text := err.Error(); !strings.Contains(text, ".jbs.lock") ||
		!strings.Contains(text, "invalid lock metadata") ||
		!strings.Contains(text, "4444") {
		t.Fatalf("expected malformed lock diagnostic, got %q", text)
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "4444\n" {
		t.Fatalf("malformed lock changed: %q", string(data))
	}
}

func TestRootLockProbeErrorDoesNotReclaimLocalLock(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	original := lockInfo{
		Schema:    1,
		PID:       4444,
		Hostname:  "node-a",
		CreatedAt: time.Unix(100, 0).UTC(),
	}
	writeLockInfoForTest(t, lockPath, original)

	rt := testLockRuntime("node-a", 9999, func(pid int) (bool, error) {
		return true, syscall.EPERM
	})
	_, err := acquireLockFileWith(lockPath, rt)
	if err == nil {
		t.Fatal("expected probe error to block acquisition")
	}
	if text := err.Error(); !strings.Contains(text, "locked") || !strings.Contains(text, "4444") {
		t.Fatalf("expected live owner diagnostic, got %q", text)
	}
	if got := readLockInfoForTest(t, lockPath); !sameLockInfo(got, original) {
		t.Fatalf("local lock changed after probe error: %#v", got)
	}
}

func TestReleaseLockFileDoesNotRemoveDifferentOwner(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, rootLockName)
	original := lockInfo{Schema: 1, PID: 1, Hostname: "h", CreatedAt: time.Unix(1, 0).UTC()}
	replacement := lockInfo{Schema: 1, PID: 2, Hostname: "h", CreatedAt: time.Unix(2, 0).UTC()}
	writeLockInfoForTest(t, lockPath, replacement)

	releaseLockFile(lockPath, original)

	got := readLockInfoForTest(t, lockPath)
	if !sameLockInfo(got, replacement) {
		t.Fatalf("replacement lock was removed or changed: %#v", got)
	}
}

func TestRootLockUnlockRemovesOwnedLock(t *testing.T) {
	root := t.TempDir()
	unlock, err := acquireExistingRootLock(root)
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	if _, err := os.Stat(filepath.Join(root, rootLockName)); !os.IsNotExist(err) {
		t.Fatalf("expected root lock removal, stat error: %v", err)
	}
}

func testLockRuntime(host string, pid int, alive func(int) (bool, error)) lockRuntime {
	if alive == nil {
		alive = func(int) (bool, error) { return true, nil }
	}
	return lockRuntime{
		pid:          func() int { return pid },
		hostname:     func() (string, error) { return host, nil },
		now:          func() time.Time { return time.Unix(200, 0).UTC() },
		processAlive: alive,
	}
}

func readLockInfoForTest(t *testing.T, path string) lockInfo {
	t.Helper()
	info, err := readLockInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func writeLockInfoForTest(t *testing.T, path string, info lockInfo) {
	t.Helper()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
