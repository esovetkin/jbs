package repl

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	helpdocs "gitlab.jsc.fz-juelich.de/sdlaml/jbs/docs"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
	parserpkg "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"

	"github.com/chzyer/readline"
)

const (
	primaryPrompt      = "jbs> "
	continuationPrompt = "...> "
)

const helpText = `REPL commands:
:help                  show this help
:help <function_name>  show help for an internal function
?                      list internal functions with focused help
?<function_name>       shortcut for :help <function_name>
:show                  print accepted session source
:save <filename>       write accepted session source to file
:reset                 clear accepted source and pending input
:quit / :exit          exit REPL`

type sessionState struct {
	accepted string
	pending  string
}

var replWrite = fsutil.AtomicWriteOptions{SyncDir: true, TempSuffix: "repl"}

func Run(opts Options) int {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	if opts.Commit == nil {
		fmt.Fprintln(stderr, "repl evaluator is not configured")
		return 1
	}

	cwd := strings.TrimSpace(opts.Cwd)
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "failed to determine working directory: %v\n", err)
			return 1
		}
		cwd = wd
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "failed to resolve working directory: %v\n", err)
		return 1
	}
	historyPath := strings.TrimSpace(opts.HistoryFile)
	if historyPath == "" {
		path, err := ResolveHistoryPath(absCwd)
		if err != nil {
			fmt.Fprintf(stderr, "warning: failed to resolve history path: %v\n", err)
		} else {
			historyPath = path
		}
	}
	if historyPath != "" {
		if err := EnsureHistoryDir(historyPath); err != nil {
			fmt.Fprintf(stderr, "warning: failed to prepare history path %q: %v\n", historyPath, err)
			historyPath = ""
		}
	}

	readerFactory := opts.NewReader
	if readerFactory == nil {
		readerFactory = defaultReaderFactory
	}
	reader, err := readerFactory(historyPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to initialize repl input: %v\n", err)
		return 1
	}
	defer func() { _ = reader.Close() }()

	fmt.Fprintf(stdout, "JBS, %s\n\n", welcomeBuildInfo(opts.BuildInfo))
	fmt.Fprintln(stdout, "Type :help for commands, Ctrl+D to exit")

	state := sessionState{}
	for {
		if state.pending == "" {
			reader.SetPrompt(primaryPrompt)
		} else {
			reader.SetPrompt(continuationPrompt)
		}
		line, err := reader.Readline()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			if isInterrupt(err) {
				fmt.Fprintln(stdout, "^C")
				state.pending = ""
				continue
			}
			fmt.Fprintf(stderr, "repl input error: %v\n", err)
			return 1
		}

		trimmed := strings.TrimSpace(line)
		if state.pending == "" && trimmed == "" {
			continue
		}
		if state.pending == "" && isREPLCommandLine(trimmed) {
			exit, code := handleCommand(trimmed, absCwd, &state, stdout, stderr)
			if exit {
				return code
			}
			continue
		}
		if state.pending == "" {
			state.pending = line
		} else {
			state.pending += "\n" + line
		}
		if parserpkg.ScanStructuralState(state.pending).NeedsMoreInput() {
			continue
		}

		commit, err := opts.Commit(state.accepted, state.pending)
		if err != nil {
			fmt.Fprintf(stderr, "repl evaluation failed: %v\n", err)
			state.pending = ""
			continue
		}
		if strings.TrimSpace(commit.DiagText) != "" {
			fmt.Fprintln(stderr, commit.DiagText)
		}
		if !commit.HasErrors {
			state.accepted = commit.Source
			for _, line := range commit.ExprOutput {
				fmt.Fprintln(stdout, line)
			}
		}
		state.pending = ""
	}
}

func welcomeBuildInfo(info string) string {
	info = strings.TrimSpace(info)
	if info == "" {
		return "version unknown, commit unknown, built unknown"
	}
	return info
}

func defaultReaderFactory(historyPath string) (LineReader, error) {
	cfg := readline.Config{
		Prompt:          primaryPrompt,
		HistoryFile:     historyPath,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	}
	inst, err := readline.NewEx(&cfg)
	if err != nil {
		return nil, err
	}
	return &readlineLineReader{inst: inst}, nil
}

type readlineLineReader struct {
	inst *readline.Instance
}

func (r *readlineLineReader) Readline() (string, error) {
	return r.inst.Readline()
}

func (r *readlineLineReader) SetPrompt(prompt string) {
	r.inst.SetPrompt(prompt)
}

func (r *readlineLineReader) Close() error {
	return r.inst.Close()
}

func isInterrupt(err error) bool {
	return errors.Is(err, readline.ErrInterrupt)
}

func isREPLCommandLine(line string) bool {
	return strings.HasPrefix(line, ":") || strings.HasPrefix(line, "?")
}

func handleCommand(
	line string,
	cwd string,
	state *sessionState,
	stdout io.Writer,
	stderr io.Writer,
) (bool, int) {
	if q := parseQuestionHelp(line); q.kind != questionHelpNone {
		switch q.kind {
		case questionHelpList:
			printAvailableFunctions(stdout)
		case questionHelpFunction:
			printFunctionHelp(stdout, stderr, q.name)
		case questionHelpInvalid:
			fmt.Fprintln(stderr, "usage: ?<function_name>")
		}
		return false, 0
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false, 0
	}
	switch fields[0] {
	case ":help":
		switch len(fields) {
		case 1:
			fmt.Fprintln(stdout, helpText)
		case 2:
			printFunctionHelp(stdout, stderr, fields[1])
		default:
			fmt.Fprintln(stderr, "usage: :help [function_name]")
		}
	case ":show":
		if strings.TrimSpace(state.accepted) == "" {
			fmt.Fprintln(stdout, "(empty)")
			return false, 0
		}
		_, _ = io.WriteString(stdout, state.accepted)
		if !strings.HasSuffix(state.accepted, "\n") {
			_, _ = io.WriteString(stdout, "\n")
		}
	case ":reset":
		state.accepted = ""
		state.pending = ""
	case ":save":
		if len(fields) != 2 {
			fmt.Fprintln(stderr, "usage: :save <filename>")
			return false, 0
		}
		if strings.TrimSpace(state.accepted) == "" {
			fmt.Fprintln(stderr, "no accepted input to save")
			return false, 0
		}
		target := fields[1]
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwd, target)
		}
		perm := os.FileMode(0o644)
		if info, statErr := os.Stat(target); statErr == nil {
			perm = info.Mode().Perm()
		}
		if err := fsutil.WriteFileAtomic(target, []byte(state.accepted), perm, replWrite); err != nil {
			fmt.Fprintf(stderr, "failed to write %q: %v\n", target, err)
			return false, 0
		}
		fmt.Fprintln(stdout, target)
	case ":quit", ":exit":
		return true, 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", fields[0])
	}
	return false, 0
}

type questionHelpKind int

const (
	questionHelpNone questionHelpKind = iota
	questionHelpList
	questionHelpFunction
	questionHelpInvalid
)

type questionHelpCommand struct {
	kind questionHelpKind
	name string
}

func parseQuestionHelp(line string) questionHelpCommand {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "?") {
		return questionHelpCommand{kind: questionHelpNone}
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "?"))
	if rest == "" {
		return questionHelpCommand{kind: questionHelpList}
	}
	fields := strings.Fields(rest)
	if len(fields) != 1 {
		return questionHelpCommand{kind: questionHelpInvalid}
	}
	return questionHelpCommand{kind: questionHelpFunction, name: fields[0]}
}

func printAvailableFunctions(w io.Writer) {
	fmt.Fprintf(w, "available internal functions: %s\n", helpdocs.FunctionNamesText())
}

func printFunctionHelp(stdout io.Writer, stderr io.Writer, name string) {
	page, err := helpdocs.FunctionPage(name)
	if err != nil {
		fmt.Fprintf(stderr, "unknown internal function: %s\n", name)
		printAvailableFunctions(stderr)
		return
	}
	_, _ = io.WriteString(stdout, page)
	if !strings.HasSuffix(page, "\n") {
		fmt.Fprintln(stdout)
	}
}
