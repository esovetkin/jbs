package version

import (
	"errors"
	"os"
	"runtime/debug"
	"testing"
	"time"
)

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

func TestCommitHashUsesPseudoVersionWhenVCSRevisionIsAbsent(t *testing.T) {
	withVersionHooks(t,
		&debug.BuildInfo{
			Main: debug.Module{
				Version: "v0.0.0-20260512000000-1234567890abcdef",
				Sum:     "h1:modulesum",
			},
		},
		true,
		"",
		errors.New("no executable"),
	)

	if got, want := Current().Commit, "1234567890ab"; got != want {
		t.Fatalf("Current().Commit = %q, want %q", got, want)
	}
}

func TestCommitHashUsesModuleSumFallback(t *testing.T) {
	withVersionHooks(t,
		&debug.BuildInfo{
			Main: debug.Module{
				Version: "v0.1.0",
				Sum:     "h1:modulesum",
			},
		},
		true,
		"",
		errors.New("no executable"),
	)

	if got, want := Current().Commit, "h1:modulesum"; got != want {
		t.Fatalf("Current().Commit = %q, want %q", got, want)
	}
}

func TestBuiltTimeUsesExecutableModTimeBeforeVCSTime(t *testing.T) {
	withVersionHooks(t,
		&debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "2020-01-02T03:04:05Z"},
			},
		},
		true,
		testExecutableWithModTime(t, "2026-05-12T00:00:00Z"),
		nil,
	)

	if got, want := Current().Built, "2026-05-12T00:00:00Z"; got != want {
		t.Fatalf("Current().Built = %q, want %q", got, want)
	}
}

func TestBuiltTimeUsesVCSTimeWhenExecutableLookupFails(t *testing.T) {
	withVersionHooks(t,
		&debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "2026-05-12T02:03:04+02:00"},
			},
		},
		true,
		"",
		errors.New("no executable"),
	)

	if got, want := Current().Built, "2026-05-12T00:03:04Z"; got != want {
		t.Fatalf("Current().Built = %q, want %q", got, want)
	}
}

func TestCurrentReturnsUnknownForMissingMetadata(t *testing.T) {
	withVersionHooks(t, nil, false, "", errors.New("no executable"))

	info := Current()
	if info.Commit != "unknown" {
		t.Fatalf("Current().Commit = %q, want unknown", info.Commit)
	}
	if info.Built != "unknown" {
		t.Fatalf("Current().Built = %q, want unknown", info.Built)
	}
}

func TestCurrentCleansEmptyVersion(t *testing.T) {
	oldVersion := Version
	t.Cleanup(func() {
		Version = oldVersion
	})
	Version = " "
	withVersionHooks(t, nil, false, "", errors.New("no executable"))

	if got, want := Current().Version, "unknown"; got != want {
		t.Fatalf("Current().Version = %q, want %q", got, want)
	}
}

func withVersionHooks(t *testing.T, info *debug.BuildInfo, ok bool, exePath string, exeErr error) {
	t.Helper()

	oldReadBuildInfo := readBuildInfo
	oldExecutable := executable
	t.Cleanup(func() {
		readBuildInfo = oldReadBuildInfo
		executable = oldExecutable
	})

	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return info, ok
	}
	executable = func() (string, error) {
		return exePath, exeErr
	}
}

func testExecutableWithModTime(t *testing.T, stamp string) string {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatal(err)
	}

	path := t.TempDir() + "/jbs"
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, parsed, parsed); err != nil {
		t.Fatal(err)
	}
	return path
}
