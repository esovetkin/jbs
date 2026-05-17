package repl

import "io"

type CommitResult struct {
	Source          string
	ExprOutput      []string
	DiagText        string
	HasErrors       bool
	CompletionNames []string
}

type CommitFunc func(source string, chunk string) (CommitResult, error)

type AutoCompleter interface {
	Do(line []rune, pos int) ([][]rune, int)
}

type LineReader interface {
	Readline() (string, error)
	SetPrompt(string)
	Close() error
}

type ReaderConfig struct {
	HistoryPath  string
	AutoComplete AutoCompleter
}

type ReaderFactory func(ReaderConfig) (LineReader, error)

type Options struct {
	Stdout                 io.Writer
	Stderr                 io.Writer
	Cwd                    string
	HistoryFile            string
	BuildInfo              string
	Commit                 CommitFunc
	NewReader              ReaderFactory
	InitialCompletionNames []string
}
