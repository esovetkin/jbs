package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStepDirNameAvoidsReservedStatus(t *testing.T) {
	used := map[string]struct{}{}
	if got := stepDirName("status", used); got != "status__step" {
		t.Fatalf("unexpected status step dir: %q", got)
	}
	if got := stepDirName("status", used); got != "status__step__1" {
		t.Fatalf("unexpected collision step dir: %q", got)
	}
}

func TestNextRunIDIgnoresNonNumericDirectories(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"000000", "000002", ".creating-000003-1", "abc"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := nextRunID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "000003" {
		t.Fatalf("unexpected next run id: %q", got)
	}
}
