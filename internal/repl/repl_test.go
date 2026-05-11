package repl

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	helpdocs "gitlab.jsc.fz-juelich.de/sdlaml/jbs/docs"

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

func TestRunSaveCommandWritesFile(t *testing.T) {
	cwd := t.TempDir()
	reader := &fakeReader{events: []fakeEvent{{line: "x = 1"}, {line: ":save out.jbs"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Cwd = cwd
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	target := filepath.Join(cwd, "out.jbs")
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("failed to read saved file: %v", readErr)
	}
	if string(data) != "x = 1" {
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

func TestRunMultilineFunctionLiteralWaitsForClosingBrace(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "add = function(x) {"}, {line: "x + 1"}, {line: "}"}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	commitCalls := 0
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		commitCalls++
		if chunk != "add = function(x) {\nx + 1\n}" {
			t.Fatalf("unexpected committed function chunk: %q", chunk)
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
	if !strings.Contains(out.String(), "add = function(x) {\nx + 1\n}") {
		t.Fatalf("expected committed function in show output, got: %q", out.String())
	}
}

func TestRunMultilineLoopsWaitForClosingBrace(t *testing.T) {
	tests := []struct {
		name string
		in   []fakeEvent
		want string
	}{
		{
			name: "for",
			in:   []fakeEvent{{line: "for x in range(2) {"}, {line: "x"}, {line: "}"}, {line: ":show"}, {err: io.EOF}},
			want: "for x in range(2) {\nx\n}",
		},
		{
			name: "while",
			in:   []fakeEvent{{line: "while false {"}, {line: "break"}, {line: "}"}, {line: ":show"}, {err: io.EOF}},
			want: "while false {\nbreak\n}",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeReader{events: tc.in}
			var out, err strings.Builder
			commitCalls := 0
			opts := baseOptions(t, reader)
			opts.Stdout = &out
			opts.Stderr = &err
			opts.Commit = func(source string, chunk string) (CommitResult, error) {
				commitCalls++
				if chunk != tc.want {
					t.Fatalf("unexpected committed loop chunk: %q", chunk)
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
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("expected committed loop in show output, got: %q", out.String())
			}
		})
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

func TestRunOpenParenTriggersContinuation(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "names("}, {line: ")"}, {line: ":show"}, {err: io.EOF}}}
	var out, err strings.Builder
	commitCalls := 0
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err
	opts.Commit = func(source string, chunk string) (CommitResult, error) {
		commitCalls++
		if chunk != "names(\n)" {
			t.Fatalf("unexpected multiline chunk: %q", chunk)
		}
		return CommitResult{Source: appendAcceptedForTest(source, chunk), ExprOutput: []string{"[]"}}, nil
	}
	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if commitCalls != 1 {
		t.Fatalf("expected one commit after closing paren, got %d", commitCalls)
	}
	if err.String() != "" {
		t.Fatalf("did not expect stderr output, got: %q", err.String())
	}
	if !strings.Contains(out.String(), "names(\n)") {
		t.Fatalf("expected accepted source to include completed chunk, got: %q", out.String())
	}
}

func TestRunHelpFunctionCommand(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		contains string
	}{
		{name: "range", line: ":help range", contains: "# `range(...)`"},
		{name: "t alias", line: ":help t", contains: "Alias of `table(...)`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeReader{events: []fakeEvent{{line: tc.line}, {err: io.EOF}}}
			var out, err strings.Builder
			opts := baseOptions(t, reader)
			opts.Stdout = &out
			opts.Stderr = &err

			code := Run(opts)
			if code != 0 {
				t.Fatalf("Run returned %d, want 0", code)
			}
			if !strings.Contains(out.String(), tc.contains) {
				t.Fatalf("expected %q in stdout, got %q", tc.contains, out.String())
			}
			if err.String() != "" {
				t.Fatalf("did not expect stderr, got %q", err.String())
			}
		})
	}
}

func TestRunQuestionHelpCommand(t *testing.T) {
	tests := []string{"?range", "? range"}
	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			reader := &fakeReader{events: []fakeEvent{{line: line}, {err: io.EOF}}}
			var out, err strings.Builder
			opts := baseOptions(t, reader)
			opts.Stdout = &out
			opts.Stderr = &err

			code := Run(opts)
			if code != 0 {
				t.Fatalf("Run returned %d, want 0", code)
			}
			if !strings.Contains(out.String(), "# `range(...)`") {
				t.Fatalf("expected range help in stdout, got %q", out.String())
			}
			if err.String() != "" {
				t.Fatalf("did not expect stderr, got %q", err.String())
			}
		})
	}
}

func TestRunBareQuestionListsFunctionHelp(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "?"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err

	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	want := "available internal functions: " + helpdocs.FunctionNamesText()
	if !strings.Contains(out.String(), want) {
		t.Fatalf("expected %q in stdout, got %q", want, out.String())
	}
	if err.String() != "" {
		t.Fatalf("did not expect stderr, got %q", err.String())
	}
}

func TestRunHelpInvalidForms(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "question extra spaced", line: "? range extra", want: "usage: ?<function_name>"},
		{name: "question extra compact", line: "?range extra", want: "usage: ?<function_name>"},
		{name: "help extra", line: ":help range extra", want: "usage: :help [function_name]"},
		{name: "unknown function", line: ":help nope", want: "unknown internal function: nope"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeReader{events: []fakeEvent{{line: tc.line}, {err: io.EOF}}}
			var out, err strings.Builder
			opts := baseOptions(t, reader)
			opts.Stdout = &out
			opts.Stderr = &err

			code := Run(opts)
			if code != 0 {
				t.Fatalf("Run returned %d, want 0", code)
			}
			if !strings.Contains(err.String(), tc.want) {
				t.Fatalf("expected %q in stderr, got %q", tc.want, err.String())
			}
			if tc.name == "unknown function" && !strings.Contains(err.String(), "available internal functions:") {
				t.Fatalf("expected available function list, got %q", err.String())
			}
		})
	}
}

func TestRunHelpQueriesDoNotCommitSource(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{line: "?", want: "available internal functions: " + helpdocs.FunctionNamesText()},
		{line: "?range", want: "# `range(...)`"},
		{line: ":help range", want: "# `range(...)`"},
	}
	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			reader := &fakeReader{events: []fakeEvent{{line: tc.line}, {line: ":show"}, {err: io.EOF}}}
			var out, err strings.Builder
			opts := baseOptions(t, reader)
			opts.Stdout = &out
			opts.Stderr = &err

			code := Run(opts)
			if code != 0 {
				t.Fatalf("Run returned %d, want 0", code)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("expected %q in stdout, got %q", tc.want, out.String())
			}
			if !strings.Contains(out.String(), "(empty)") {
				t.Fatalf("expected help query not to commit source, got %q", out.String())
			}
			if err.String() != "" {
				t.Fatalf("did not expect stderr, got %q", err.String())
			}
		})
	}
}

func TestRunBareHelpStillPrintsCommandHelp(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: ":help"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err

	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "REPL commands:") ||
		!strings.Contains(out.String(), "?                      list internal functions with focused help") ||
		!strings.Contains(out.String(), "?<function_name>       shortcut for :help <function_name>") {
		t.Fatalf("expected command help, got %q", out.String())
	}
	if err.String() != "" {
		t.Fatalf("did not expect stderr, got %q", err.String())
	}
}

func TestQuestionAndColonCommandsInsidePendingInputAreNotCommands(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "question", line: "?range"},
		{name: "colon", line: ":show"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeReader{events: []fakeEvent{
				{line: "add = function(x) {"},
				{line: tc.line},
				{line: "}"},
				{err: io.EOF},
			}}
			var out, err strings.Builder
			opts := baseOptions(t, reader)
			opts.Stdout = &out
			opts.Stderr = &err
			commitCalled := false
			opts.Commit = func(source string, chunk string) (CommitResult, error) {
				commitCalled = true
				if !strings.Contains(chunk, tc.line) {
					t.Fatalf("expected pending chunk to contain %q, got %q", tc.line, chunk)
				}
				return CommitResult{Source: source, HasErrors: true}, nil
			}

			code := Run(opts)
			if code != 0 {
				t.Fatalf("Run returned %d, want 0", code)
			}
			if !commitCalled {
				t.Fatalf("expected pending input to be committed")
			}
			if out.String() != "Type :help for commands, Ctrl+D to exit\n" {
				t.Fatalf("did not expect command output, got %q", out.String())
			}
			if err.String() != "" {
				t.Fatalf("did not expect stderr, got %q", err.String())
			}
		})
	}
}

func TestQuestionHelpIgnoresAcceptedSourceBindings(t *testing.T) {
	reader := &fakeReader{events: []fakeEvent{{line: "len = function(x) { x }"}, {line: "?len"}, {err: io.EOF}}}
	var out, err strings.Builder
	opts := baseOptions(t, reader)
	opts.Stdout = &out
	opts.Stderr = &err

	code := Run(opts)
	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "# `len(<string/tuple/list/table/dict>)`") {
		t.Fatalf("expected internal len help, got %q", out.String())
	}
	if err.String() != "" {
		t.Fatalf("did not expect stderr, got %q", err.String())
	}
}
