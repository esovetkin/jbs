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
	for _, name := range []string{"000000", "1000000", ".creating-1000001-1", "001a", "abc"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := nextRunID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1000001" {
		t.Fatalf("unexpected next run id: %q", got)
	}
}

func TestLatestRunDirSeesSevenDigitRunIDs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"999999", "1000000"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := latestRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "1000000" {
		t.Fatalf("latestRunDir = %q, want 1000000", got)
	}
}

func TestNextRunIDAfterSixDigitCeiling(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "999999"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := nextRunID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1000000" {
		t.Fatalf("nextRunID = %q, want 1000000", got)
	}

	if err := os.Mkdir(filepath.Join(dir, "1000000"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = nextRunID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1000001" {
		t.Fatalf("nextRunID = %q, want 1000001", got)
	}
}

func TestNextRunIDKeepsSixDigitPaddingBelowMillion(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "000009"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := nextRunID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "000010" {
		t.Fatalf("nextRunID = %q, want 000010", got)
	}
}
