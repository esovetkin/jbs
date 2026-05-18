package cli

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestRunDispatchReportsUsageErrorsAndHelp(t *testing.T) {
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
			return runBenchmark(missing, false, false, false, "", stdout, stderr)
		}},
		{name: "continue", run: func(stdout, stderr *bytes.Buffer) int {
			return continueBenchmark(missing, "", stdout, stderr)
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

func TestBenchmarkCommandInputClassification(t *testing.T) {
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

func TestFwaitFilesReportsMissingTargets(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := fwaitFiles(nil, false, &stdout, &stderr); code != 1 {
		t.Fatalf("expected missing-target failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "fwait requires at least one target") {
		t.Fatalf("expected missing-target error, got %q", stderr.String())
	}
}

func TestRunParamErrorBranches(t *testing.T) {
	dir := t.TempDir()
	input := writeCLIFile(t, dir, "input.jbs", strings.Join([]string{
		`do s {`,
		`  echo ok`,
		`}`,
		``,
	}, "\n"))

	t.Run("writes_output_file", func(t *testing.T) {
		output := filepath.Join(dir, "params.csv")
		var stdout, stderr bytes.Buffer
		code := runParam(Flags{Input: input, PrintType: "csv", Output: output}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected param success, code=%d stderr=%s", code, stderr.String())
		}
		if stdout.String() != "" {
			t.Fatalf("expected file output only, got %q", stdout.String())
		}
		data := readFileString(t, output)
		if !strings.Contains(data, "do: s") {
			t.Fatalf("expected param output file, got %q", data)
		}
	})

	t.Run("render_error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runParam(Flags{Input: input, PrintType: "json", Output: "-"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected render failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "failed to render param output") {
			t.Fatalf("expected render error, got %q", stderr.String())
		}
	})

	t.Run("write_error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runParam(Flags{Input: input, PrintType: "pretty", Output: dir}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected write failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "failed to write output") {
			t.Fatalf("expected write error, got %q", stderr.String())
		}
	})

	t.Run("analysis_error", func(t *testing.T) {
		bad := writeCLIFile(t, dir, "bad.jbs", strings.Join([]string{
			`do s after missing {`,
			`  echo ok`,
			`}`,
			``,
		}, "\n"))
		var stdout, stderr bytes.Buffer
		code := runParam(Flags{Input: bad, PrintType: "pretty", Output: "-"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected analysis failure, code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "ERROR") {
			t.Fatalf("expected diagnostics, got %q", stderr.String())
		}
	})
}

func TestFormatDiagnosticsWithSourcesEdges(t *testing.T) {
	if got := formatDiagnosticsWithSources(diag.Diagnostics{}, nil, ""); got != "" {
		t.Fatalf("empty diagnostics formatted as %q", got)
	}

	diags := diag.Diagnostics{Items: []diag.Diagnostic{{
		Severity: diag.SeverityWarning,
		Code:     "WTEST",
		Message:  "uses default file",
		Span: diag.NewSpan("",
			diag.NewPos(0, 2, 2),
			diag.NewPos(0, 2, 5)),
		Hint: "try again",
		Related: []diag.RelatedSpan{{
			Message: "related note",
			Span: diag.NewSpan("rel.jbs",
				diag.NewPos(0, 1, 1),
				diag.NewPos(0, 1, 2)),
		}},
	}, {
		Severity: diag.SeverityError,
		Code:     "ETEST",
		Message:  "falls back to default source",
		Span: diag.NewSpan("other.jbs",
			diag.NewPos(0, 1, 1),
			diag.NewPos(0, 1, 2)),
	}}}
	got := formatDiagnosticsWithSources(diags, map[string]string{
		"main.jbs": "alpha\nbeta\ngamma\n",
	}, "main.jbs")

	for _, want := range []string{
		"WARNING WTEST <input>:2:2",
		"uses default file",
		"   2 | beta",
		"Hint: try again",
		"Related: related note (rel.jbs:1:1)",
		"ERROR ETEST other.jbs:1:1",
		"   1 | alpha",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted diagnostics missing %q:\n%s", want, got)
		}
	}
}

func TestSourceExcerptEdges(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	if got := sourceExcerpt(lines, diag.Span{}); got != "" {
		t.Fatalf("zero span excerpt = %q", got)
	}
	if got := sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 0, 1), diag.NewPos(0, 0, 2))); got != "" {
		t.Fatalf("line zero excerpt = %q", got)
	}
	if got := sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 99, 1), diag.NewPos(0, 99, 2))); got != "" {
		t.Fatalf("out-of-range excerpt = %q", got)
	}

	got := sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 2, 2), diag.NewPos(0, 1, 1)))
	if !strings.Contains(got, "   2 | beta") || !strings.Contains(got, "       |  ^") {
		t.Fatalf("expected end-before-start excerpt, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 4, 1), diag.NewPos(0, 99, 2)))
	if !strings.Contains(got, "   4 | delta") || !strings.Contains(got, "   5 | epsilon") || strings.Contains(got, "   6 |") {
		t.Fatalf("expected excerpt capped at input length, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 5, 2)))
	if !strings.Contains(got, "   3 | gamma") || strings.Contains(got, "   4 | delta") {
		t.Fatalf("expected long excerpt capped to three lines, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 1, 0), diag.NewPos(0, 1, 0)))
	if !strings.Contains(got, "       | ^") {
		t.Fatalf("expected column floor caret, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 1, 2), diag.NewPos(0, 1, 5)))
	if !strings.Contains(got, "       |  ^^^") {
		t.Fatalf("expected multi-column caret, got %q", got)
	}
}

func TestReplSmallHelpers(t *testing.T) {
	if got := appendReplChunk("accepted", "   "); got != "accepted" {
		t.Fatalf("blank chunk appended as %q", got)
	}
	if got := appendReplChunk("", "x = 1"); got != "x = 1" {
		t.Fatalf("empty accepted append = %q", got)
	}
	if got := appendReplChunk("x = 1\n", "x"); got != "x = 1\nx" {
		t.Fatalf("newline append = %q", got)
	}

	if got := replCompletionNames(nil); got != nil {
		t.Fatalf("nil result completions = %#v", got)
	}

	names := replCompletionNames(&sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"":         eval.Int(0),
			"mod.name": eval.Int(1),
			"b":        eval.Int(2),
			"a":        eval.Int(3),
		}},
	})
	if !slices.Equal(names, []string{"a", "b"}) {
		t.Fatalf("completion names = %#v", names)
	}
}
