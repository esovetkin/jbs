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

type Result struct {
	Program     ast.Program
	Paramsets   []*Paramset
	ParamByName map[string]*Paramset
	DoBlocks    []ast.DoBlock
	Submits     []ast.SubmitBlock
}
