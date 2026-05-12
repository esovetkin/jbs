package version

import "testing"

func TestStringCleansEmptyVersion(t *testing.T) {
	oldVersion := Version
	t.Cleanup(func() {
		Version = oldVersion
	})

	Version = "  "
	if got, want := String(), "unknown"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestFullIncludesVersionCommitAndBuildDate(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldBuildDate := BuildDate
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		BuildDate = oldBuildDate
	})

	Version = " v0.1.0 "
	Commit = " abc123 "
	BuildDate = " 2026-05-12T00:00:00Z "

	want := "version v0.1.0, commit abc123, built 2026-05-12T00:00:00Z"
	if got := Full(); got != want {
		t.Fatalf("Full() = %q, want %q", got, want)
	}
}
