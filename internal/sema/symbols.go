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

type SourceKind string

const (
	SourceKindParam SourceKind = "param"
	SourceKindLet   SourceKind = "let"
)

type ImportSource struct {
	Name    string
	Kind    SourceKind
	Vars    map[string][]eval.Value
	Origins map[string]diag.Span
	Modes   map[string]string
	Order   []string
	Span    diag.Span
}

type Result struct {
	Program            ast.Program
	Globals            GlobalState
	LetNamespaces      []*LetNamespace
	LetByName          map[string]*LetNamespace
	ImportSourceByName map[string]*ImportSource
	Paramsets          []*Paramset
	ParamByName        map[string]*Paramset
	DoBlocks           []ast.DoBlock
	Submits            []ast.SubmitBlock
	SubmitByName       map[string]*SubmitSpec
	StepImportByName   map[string]*StepImportPlan
	Analyse            []*AnalyseSpec
}

type VarOrigin struct {
	Name      string
	SourceVar string
	Paramset  string
	Kind      SourceKind
	Span      diag.Span
}

type PlannedImport struct {
	Source    string
	Kind      SourceKind
	Visible   string
	SourceVar string
	Full      bool
	Span      diag.Span
}

type StepImportPlan struct {
	StepName       string
	Inherited      map[string]VarOrigin
	ExplicitDelta  []PlannedImport
	Effective      map[string]VarOrigin
	InheritedSteps []string
}

type SubmitValue struct {
	Name  string
	Mode  string
	Value eval.Value
	Raw   string
	IsRaw bool
	Span  diag.Span
}

type SubmitHelper struct {
	Original string
	Aliased  string
	Mode     string
	Value    eval.Value
	Span     diag.Span
	UseName  string
}

type SubmitSpec struct {
	Name    string
	Values  []SubmitValue
	Helpers []SubmitHelper
	Span    diag.Span
}

type LetNamespace struct {
	Name    string
	Vars    map[string]eval.Value
	Modes   map[string]string
	Origins map[string]diag.Span
	Span    diag.Span
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
