package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDispatchReportsUsageAndHelp(t *testing.T) {
	t.Run("usage_error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"run"}, &stdout, &stderr); code != 2 {
			t.Fatalf("expected usage exit code, got %d", code)
		}
		if stdout.String() != "" {
			t.Fatalf("expected no stdout, got %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "usage: jbs run") {
			t.Fatalf("expected run usage, got %q", stderr.String())
		}
	})

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"help"}, want: "Usage:"},
		{name: "short_help", args: []string{"-h"}, want: "Usage:"},
		{name: "help_topic", args: []string{"help", "use"}, want: "use value from"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(tc.args, &stdout, &stderr); code != 0 {
				t.Fatalf("expected successful help, code=%d stderr=%s", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("expected %q in stdout, got %q", tc.want, stdout.String())
			}
			if stderr.String() != "" {
				t.Fatalf("expected no stderr, got %q", stderr.String())
			}
		})
	}
}

func TestCommandHelpersReportLoadFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.jbs")
	for _, tc := range []struct {
		name string
		run  func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "run", run: func(stdout, stderr *bytes.Buffer) int {
			return runBenchmark(missing, false, false, false, "", 0, stdout, stderr)
		}},
		{name: "continue", run: func(stdout, stderr *bytes.Buffer) int {
			return continueBenchmark(missing, "", false, stdout, stderr)
		}},
		{name: "status", run: func(stdout, stderr *bytes.Buffer) int {
			return statusBenchmark(missing, "", stdout, stderr)
		}},
		{name: "tree", run: func(stdout, stderr *bytes.Buffer) int {
			return treeBenchmark(missing, "", stdout, stderr)
		}},
		{name: "ls_analyse", run: func(stdout, stderr *bytes.Buffer) int {
			return listAnalyseBenchmark(missing, "", stdout, stderr)
		}},
		{name: "archive", run: func(stdout, stderr *bytes.Buffer) int {
			return archiveBenchmark(missing, stdout, stderr)
		}},
		{name: "param", run: func(stdout, stderr *bytes.Buffer) int {
			return runParam(Flags{Input: missing, PrintType: "pretty", Output: "-"}, stdout, stderr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := tc.run(&stdout, &stderr); code != 1 {
				t.Fatalf("expected load failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if stdout.String() != "" {
				t.Fatalf("expected no stdout, got %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), "failed to load input") {
				t.Fatalf("expected load failure text, got %q", stderr.String())
			}
		})
	}
}

func TestBenchmarkDirectoryCommandsReportInspectionErrors(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		run  func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "status", run: func(stdout, stderr *bytes.Buffer) int {
			return statusBenchmark(dir, "", stdout, stderr)
		}},
		{name: "ls_analyse", run: func(stdout, stderr *bytes.Buffer) int {
			return listAnalyseBenchmark(dir, "", stdout, stderr)
		}},
		{name: "archive", run: func(stdout, stderr *bytes.Buffer) int {
			return archiveBenchmark(dir, stdout, stderr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := tc.run(&stdout, &stderr); code != 1 {
				t.Fatalf("expected inspection failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if stdout.String() != "" {
				t.Fatalf("expected no stdout, got %q", stdout.String())
			}
			if strings.TrimSpace(stderr.String()) == "" {
				t.Fatalf("expected stderr")
			}
		})
	}
}

func TestBenchmarkCommandsClassifySourceAndDirectoryInputs(t *testing.T) {
	dir := t.TempDir()
	source := writeCLIFile(t, dir, "input.jbs", "x = 1\n")
	missing := filepath.Join(dir, "missing.jbs")

	for _, tc := range []struct {
		name string
		path string
		want benchmarkCommandInputKind
	}{
		{name: "directory", path: dir, want: benchmarkCommandInputDirectory},
		{name: "source", path: source, want: benchmarkCommandInputSource},
		{name: "missing_source", path: missing, want: benchmarkCommandInputSource},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classifyBenchmarkCommandInput(tc.path)
			if err != nil {
				t.Fatalf("classify failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("classification = %v, want %v", got, tc.want)
			}
		})
	}

	if _, err := classifyBenchmarkCommandInput("bad\x00path"); err == nil {
		t.Fatalf("expected invalid path classification to fail")
	}

	for _, tc := range []struct {
		name string
		run  func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "status", run: func(stdout, stderr *bytes.Buffer) int {
			return statusBenchmark("bad\x00path", "", stdout, stderr)
		}},
		{name: "ls_analyse", run: func(stdout, stderr *bytes.Buffer) int {
			return listAnalyseBenchmark("bad\x00path", "", stdout, stderr)
		}},
		{name: "archive", run: func(stdout, stderr *bytes.Buffer) int {
			return archiveBenchmark("bad\x00path", stdout, stderr)
		}},
	} {
		t.Run("command_"+tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := tc.run(&stdout, &stderr); code != 1 {
				t.Fatalf("expected classification failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "inspect input") {
				t.Fatalf("expected inspect error, got %q", stderr.String())
			}
		})
	}
}
