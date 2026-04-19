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

func defaultCommitForTest(source string, chunk string) (CommitResult, error) {
	return CommitResult{Source: appendAcceptedForTest(source, chunk), ExprOutput: []string{}}, nil
}

func appendAcceptedForTest(accepted string, chunk string) string {
	if strings.TrimSpace(chunk) == "" {
		return accepted
	}
	if accepted == "" {
		return chunk
	}
	if strings.HasSuffix(accepted, "\n") {
		return accepted + chunk
	}
	return accepted + "\n" + chunk
}

func baseOptions(t *testing.T, reader *fakeReader) Options {
	t.Helper()
	return Options{
		Cwd:       t.TempDir(),
		NewReader: fakeFactory(reader),
		Check: func(source string) (string, bool, error) {
			return "", false, nil
		},
		YAML: func(source string) (string, string, bool, error) {
			return "name: test\n", "", false, nil
		},
		Commit: defaultCommitForTest,
	}
}

func TestRunExitsOnEOF(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !reader.closed {
		t.Fatalf("reader was not closed")
	}
}

func TestRunCommitsAndShowPrintsAcceptedSource(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "x = 1"}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "x = 1") {
		t.Fatalf("expected accepted source in output, got: %q", out.String())
	}
}

func TestRunErrorsRollbackAcceptedSource(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "x = 1"}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		return CommitResult{Source: source, DiagText: "ERROR E100 in.jbs:1:1", HasErrors: true}, nil
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected empty show output, got: %q", out.String())
	}
	if !strings.Contains(err.String(), "ERROR E100") {
		t.Fatalf("expected diagnostics in stderr, got: %q", err.String())
	}
}

func TestRunInterruptClearsPendingInput(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "do run {"}, {err: readline.ErrInterrupt}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	commitCalled := false
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		commitCalled = true
		return defaultCommitForTest(source, chunk)
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if commitCalled {
		t.Fatalf("did not expect commit callback for interrupted incomplete input")
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected empty accepted source after interrupt, got: %q", out.String())
	}
}

func TestRunYAMLCommand(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "x = 1"}, {line: ":yaml"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "name: test") {
		t.Fatalf("expected yaml output, got: %q", out.String())
	}
}

func TestRunSaveCommandWritesFile(t *testing.T) {
	cwd := t.TempDir()
	reader := &fakeReader{events: []fakeEvent{{line: "x = 1"}, {line: ":save out.yaml"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Cwd = cwd
	code := Run(opts)
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
	reader := &fakeReader{events: []fakeEvent{{line: ":unknown"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(err.String(), "unknown command") {
		t.Fatalf("expected unknown-command error, got: %q", err.String())
	}
}

func TestRunUsesLocalHistoryPathByDefault(t *testing.T) {
	cwd := t.TempDir()
	reader := &fakeReader{events: []fakeEvent{{err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Cwd = cwd
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	want := filepath.Join(cwd, ".jbs_history")
	if reader.historyPath != want {
		t.Fatalf("history path mismatch: got=%q want=%q", reader.historyPath, want)
	}
}

func TestRunExprLinePrintsValueAndCommits(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "x = 1"}, {line: "x"}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		result := CommitResult{Source: appendAcceptedForTest(source, chunk), ExprOutput: []string{}}
		if strings.TrimSpace(chunk) == "x" {
			result.ExprOutput = []string{"1"}
		}
		return result, nil
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "\n1\n") {
		t.Fatalf("expected expression value in stdout, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "x = 1\nx") {
		t.Fatalf("expected expr line to be committed into accepted source, got: %q", out.String())
	}
}

func TestRunMultilineBlockWaitsForClosingBrace(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "do run {"}, {line: "echo hi"}, {line: "}"}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	commitCalls := 0
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		commitCalls++
		if chunk != "do run {\necho hi\n}" {
			t.Fatalf("unexpected committed block chunk: %q", chunk)
		}
		return defaultCommitForTest(source, chunk)
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if commitCalls != 1 {
		t.Fatalf("expected one commit after closing brace, got %d", commitCalls)
	}
	if !strings.Contains(out.String(), "do run {\necho hi\n}") {
		t.Fatalf("expected committed block in show output, got: %q", out.String())
	}
}

func TestRunTopLevelExprMultilineRequiresBackslash(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "1 + \\"}, {line: "2"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		if chunk != "1 + \\\n2" {
			t.Fatalf("unexpected multiline expr chunk: %q", chunk)
		}
		return CommitResult{Source: appendAcceptedForTest(source, chunk), ExprOutput: []string{"3"}}, nil
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "\n3\n") {
		t.Fatalf("expected multiline expr result in stdout, got: %q", out.String())
	}
}

func TestRunOpenParenDoesNotTriggerContinuation(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "range("}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	commitCalls := 0
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		commitCalls++
		return CommitResult{Source: source, DiagText: "ERROR E058 <repl>:1:1", HasErrors: true}, nil
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if commitCalls != 1 {
		t.Fatalf("expected immediate commit attempt for open-paren expr, got %d", commitCalls)
	}
	if !strings.Contains(err.String(), "ERROR E058") {
		t.Fatalf("expected parse diagnostic in stderr, got: %q", err.String())
	}
	if !strings.Contains(out.String(), "(empty)") {
		t.Fatalf("expected accepted source to remain empty, got: %q", out.String())
	}
}
