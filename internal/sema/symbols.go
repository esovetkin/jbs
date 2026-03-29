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
	Program        ast.Program
	Globals        GlobalState
	Paramsets      []*Paramset
	ParamByName    map[string]*Paramset
	DoBlocks       []ast.DoBlock
	Submits        []ast.SubmitBlock
	SubmitByName   map[string]*SubmitSpec
	Patterns       []*PatternGroup
	PatternByGroup map[string]*PatternGroup
	PatternByKey   map[string]*PatternTemplate
	Analyse        []*AnalyseSpec
}

type SubmitValue struct {
	Name  string
	Mode  string
	Value eval.Value
	Raw   string
	IsRaw bool
	Span  diag.Span
}

type SubmitSpec struct {
	Name   string
	Values []SubmitValue
	Span   diag.Span
}

type PatternGroup struct {
	Name     string
	Patterns []PatternTemplate
	Span     diag.Span
}

type PatternTemplate struct {
	Group string
	Name  string
	Regex string
	Type  string
	Span  diag.Span
}

type AnalyseSpec struct {
	Name        string
	Block       ast.AnalyseBlock
	StepKind    string
	StepVars    map[string]diag.Span
	Assignments []AnalyseAssignmentSpec
	Columns     []AnalyseColumnSpec
	Span        diag.Span
}

type AnalyseAssignmentSpec struct {
	Name     string
	Group    string
	Pattern  string
	File     string
	Template PatternTemplate
	Span     diag.Span
}

type AnalyseColumnSpec struct {
	Name   string
	Title  string
	Source string
	Span   diag.Span
}
