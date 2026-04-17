package repl

import "io"

type CheckFunc func(source string) (diagText string, hasErrors bool, err error)

type YAMLFunc func(source string) (yamlText string, diagText string, hasErrors bool, err error)

type InspectFunc func(source string, name string) (text string, ok bool, err error)

type EvalExprFunc func(source string, expr string) (resultText string, diagText string, handled bool, hasErrors bool, err error)

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
	Inspect     InspectFunc
	EvalExpr    EvalExprFunc
	NewReader   ReaderFactory
}
