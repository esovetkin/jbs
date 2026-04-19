package repl

import "io"

type CheckFunc func(source string) (diagText string, hasErrors bool, err error)

type YAMLFunc func(source string) (yamlText string, diagText string, hasErrors bool, err error)

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
	Check       CheckFunc
	YAML        YAMLFunc
	Commit      CommitFunc
	NewReader   ReaderFactory
}
