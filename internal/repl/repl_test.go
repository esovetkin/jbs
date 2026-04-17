package repl

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chzyer/readline"
)

type fakeEvent struct {
	line string
	err  error
}

type fakeReader struct {
	events      []fakeEvent
	prompts     []string
	historyPath string
	closed      bool
}

func (f *fakeReader) Readline() (string, error) {
	if len(f.events) == 0 {
		return "", io.EOF
	}
	ev := f.events[0]
	f.events = f.events[1:]
	return ev.line, ev.err
}

func (f *fakeReader) SetPrompt(prompt string) {
	f.prompts = append(f.prompts, prompt)
}

func (f *fakeReader) Close() error {
	f.closed = true
	return nil
}

func fakeFactory(fr *fakeReader) ReaderFactory {
	return func(historyPath string) (LineReader, error) {
		fr.historyPath = historyPath
		return fr, nil
	}
}

func defaultInspectForTest(source string, name string) (string, bool, error) {
	return "", false, nil
}

func defaultEvalExprForTest(source string, expr string) (string, string, bool, bool, error) {
	return "", "", false, false, nil
}

func TestRunExitsOnEOF(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{{err: io.EOF}},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !reader.closed {
		t.Fatalf("reader was not closed")
	}
}

func TestRunCommitsAndShowPrintsAcceptedSource(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x = 1"},
			{line: ":show"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "x = 1") {
		t.Fatalf("expected accepted source in output, got: %q", out.String())
	}
}

func TestRunErrorsRollbackAcceptedSource(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x = 1"},
			{line: ":show"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "ERROR", true, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected empty show output, got: %q", out.String())
	}
	if !strings.Contains(err.String(), "ERROR") {
		t.Fatalf("expected diagnostics in stderr, got: %q", err.String())
	}
}

func TestRunWarningsCommitAcceptedSource(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x = 1"},
			{line: ":show"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "x = 1") {
		t.Fatalf("expected accepted source after warning, got: %q", out.String())
	}
	if strings.TrimSpace(err.String()) != "" {
		t.Fatalf("did not expect diagnostics output, got: %q", err.String())
	}
}

func TestRunInterruptClearsPendingInput(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "do run {"},
			{err: readline.ErrInterrupt},
			{line: ":show"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	checkCalled := false
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			checkCalled = true
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if checkCalled {
		t.Fatalf("did not expect check callback for interrupted incomplete input")
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected empty accepted source after interrupt, got: %q", out.String())
	}
}

func TestRunYAMLCommand(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x = 1"},
			{line: ":yaml"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "name: test") {
		t.Fatalf("expected yaml output, got: %q", out.String())
	}
}

func TestRunSaveCommandWritesFile(t *testing.T) {
	cwd := t.TempDir()
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x = 1"},
			{line: ":save out.yaml"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       cwd,
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	target := filepath.Join(cwd, "out.yaml")
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("failed to read saved file: %v", readErr)
	}
	if string(data) != "name: test\n" {
		t.Fatalf("unexpected saved file content: %q", string(data))
	}
}

func TestRunUnknownCommand(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: ":unknown"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(err.String(), "unknown command") {
		t.Fatalf("expected unknown-command error, got: %q", err.String())
	}
}

func TestRunUsesLocalHistoryPathByDefault(t *testing.T) {
	cwd := t.TempDir()
	reader := &fakeReader{
		events: []fakeEvent{{err: io.EOF}},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       cwd,
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect:  defaultInspectForTest,
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	want := filepath.Join(cwd, ".jbs_history")
	if reader.historyPath != want {
		t.Fatalf("history path mismatch: got=%q want=%q", reader.historyPath, want)
	}
}

func TestRunBareIdentifierPrintsInspectedValue(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x = 1"},
			{line: "x"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	checkCount := 0
	inspectCount := 0
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			checkCount++
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: func(source string, name string) (string, bool, error) {
			inspectCount++
			if name == "x" {
				return "1", true, nil
			}
			return "", false, nil
		},
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if checkCount != 1 {
		t.Fatalf("expected exactly one check call for assignment, got %d", checkCount)
	}
	if inspectCount != 1 {
		t.Fatalf("expected exactly one inspect call, got %d", inspectCount)
	}
	if !strings.Contains(out.String(), "\n1\n") {
		t.Fatalf("expected inspected value in stdout, got: %q", out.String())
	}
}

func TestRunBareIdentifierUnknownPrintsMessage(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "missing"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	checkCalled := false
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			checkCalled = true
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: func(source string, name string) (string, bool, error) {
			return "", false, nil
		},
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if checkCalled {
		t.Fatalf("did not expect check callback for bare-identifier inspect")
	}
	if !strings.Contains(err.String(), "unknown variable 'missing'") {
		t.Fatalf("expected unknown-variable message, got: %q", err.String())
	}
}

func TestRunBareIdentifierIgnoredWhenPendingMultiline(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "do run {"},
			{line: "x"},
			{err: readline.ErrInterrupt},
			{line: ":show"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	inspectCalled := false
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: func(source string, name string) (string, bool, error) {
			inspectCalled = true
			return "unexpected", true, nil
		},
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if inspectCalled {
		t.Fatalf("did not expect inspect callback while multiline input is pending")
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected accepted source to remain empty, got: %q", out.String())
	}
}

func TestRunInspectFailurePrintsMessage(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "x"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: func(source string, name string) (string, bool, error) {
			return "", false, io.ErrUnexpectedEOF
		},
		EvalExpr: defaultEvalExprForTest,
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(err.String(), "inspect failed:") {
		t.Fatalf("expected inspect failure message, got: %q", err.String())
	}
}

func TestRunStandaloneExpressionPrintsValueAndDoesNotCommit(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "range(10)"},
			{line: ":show"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	checkCalled := false
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			checkCalled = true
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: defaultInspectForTest,
		EvalExpr: func(source string, expr string) (string, string, bool, bool, error) {
			if expr != "range(10)" {
				t.Fatalf("unexpected expression text: %q", expr)
			}
			return "[0, 1, 2, ...]", "", true, false, nil
		},
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if checkCalled {
		t.Fatalf("did not expect statement check for handled expression")
	}
	if !strings.Contains(out.String(), "[0, 1, 2, ...]") {
		t.Fatalf("expected expression result in stdout, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected expression input to not be committed, got: %q", out.String())
	}
}

func TestRunStandaloneExpressionDiagnostic(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "range(, )"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	checkCalled := false
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			checkCalled = true
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: defaultInspectForTest,
		EvalExpr: func(source string, expr string) (string, string, bool, bool, error) {
			return "", "ERROR E058 expr.jbs:1:1", true, true, nil
		},
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if checkCalled {
		t.Fatalf("did not expect statement check for handled expression with diagnostics")
	}
	if !strings.Contains(err.String(), "ERROR E058") {
		t.Fatalf("expected expression diagnostic in stderr, got: %q", err.String())
	}
}

func TestRunStandaloneExpressionMultiline(t *testing.T) {
	reader := &fakeReader{
		events: []fakeEvent{
			{line: "range("},
			{line: "10)"},
			{err: io.EOF},
		},
	}
	var out, err strings.Builder
	code := Run(Options{
		Stdout:    &out,
		Stderr:    &err,
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "", "", false, nil
		},
		Inspect: defaultInspectForTest,
		EvalExpr: func(source string, expr string) (string, string, bool, bool, error) {
			if expr != "range(\n10)" {
				t.Fatalf("unexpected multiline expression text: %q", expr)
			}
			return "[0, 1, 2, ...]", "", true, false, nil
		},
	})
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "[0, 1, 2, ...]") {
		t.Fatalf("expected multiline expression result, got: %q", out.String())
	}
}

func TestIsBareIdentifierInput(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "a", want: "a", ok: true},
		{in: "_x1", want: "_x1", ok: true},
		{in: "a1", want: "a1", ok: true},
		{in: " a ", want: "a", ok: true},
		{in: "", want: "", ok: false},
		{in: "1a", want: "", ok: false},
		{in: "a.b", want: "", ok: false},
		{in: "a b", want: "", ok: false},
		{in: "a=1", want: "", ok: false},
		{in: ":show", want: "", ok: false},
	}

	for _, tc := range cases {
		got, ok := isBareIdentifierInput(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("isBareIdentifierInput(%q)=(%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
