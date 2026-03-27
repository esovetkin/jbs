package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

type Paramset struct {
	Name    string
	Block   ast.ParamBlock
	Rows    []eval.Row
	Vars    map[string][]eval.Value
	Origins map[string]diag.Span
	Modes   map[string]string
	Order   []string
	HasPlus bool
}

type GlobalState struct {
	Values map[string]eval.Value
	Modes  map[string]string
	Spans  map[string]diag.Span
}

type Result struct {
	Program     ast.Program
	Globals     GlobalState
	Paramsets   []*Paramset
	ParamByName map[string]*Paramset
	DoBlocks    []ast.DoBlock
	Submits     []ast.SubmitBlock
}
