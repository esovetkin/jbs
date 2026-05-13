package repl

import "io"

type CommitResult struct {
	Source     string
	ExprOutput []string
	DiagText   string
	HasErrors  bool
}

type CommitFunc func(source string, chunk string) (CommitResult, error)

type LineReader interface {
	Readline() (string, error)
	SetPrompt(string)
	Close() error
}

type ReaderFactory func(historyPath string) (LineReader, error)

type Options struct {
	Stdout      io.Writer
	Stderr      io.Writer
	Cwd         string
	HistoryFile string
	BuildInfo   string
	Commit      CommitFunc
	NewReader   ReaderFactory
}
