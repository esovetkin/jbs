package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if _, err := acquireExistingRootLock(root); err == nil {
		t.Fatal("expected second lock acquisition to fail")
	} else if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected lock error to mention locked, got %q", err.Error())
	}

	unlock()

	unlock, err = acquireExistingRootLock(root)
	if err != nil {
		t.Fatalf("expected lock acquisition after unlock to succeed: %v", err)
	}
	unlock()
}
