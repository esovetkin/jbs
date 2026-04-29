package sema

import (
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

type BindingShape string

const (
	BindingScalar BindingShape = "scalar"
	BindingTable  BindingShape = "table"
)

type ImportContext string

const (
	ImportIntoStep      ImportContext = "step"
	ImportIntoSubmitUse ImportContext = "submit_use"
	ImportIntoAnalyse   ImportContext = "analyse"
)

type GlobalBinding struct {
	Name            string
	PublicName      string
	Value           eval.Value
	Shape           BindingShape
	Rows            []eval.Row
	Vars            map[string][]eval.Value
	BaseVars        map[string][]eval.Value
	Origins         map[string]diag.Span
	Modes           map[string]string
	Order           []string
	Span            diag.Span
	DependsOn       []string
	SyntheticGlobal bool
}

func (b *GlobalBinding) Supports(ctx ImportContext) bool {
	if b == nil {
		return false
	}
	switch ctx {
	case ImportIntoStep:
		return true
	case ImportIntoSubmitUse:
		return b.Shape == BindingScalar
	case ImportIntoAnalyse:
		if b.Shape != BindingScalar {
			return false
		}
		if len(b.Order) != 1 {
			return false
		}
		col := b.Order[0]
		vals := b.Vars[col]
		if len(vals) == 0 {
			return true
		}
		return vals[0].Kind == eval.KindString
	default:
		return false
	}
}

type GlobalState struct {
	Values map[string]eval.Value
	Modes  map[string]string
	Spans  map[string]diag.Span
}

type GlobalVar struct {
	Name      string
	Value     eval.Value
	Mode      string
	Span      diag.Span
	Order     []string
	Vars      map[string][]eval.Value
	Namespace string
	DependsOn []string
}

type Namespace struct {
	Name     string
	Members  []string
	Bindings []string
	Steps    []string
}

type TopLevelExprResult struct {
	Index int
	Span  diag.Span
	Value eval.Value
}

type ScopeSnapshot struct {
	Index           int
	Globals         GlobalState
	GlobalVarByName map[string]*GlobalVar
	GlobalVarOrder  []string
	Bindings        []*GlobalBinding
	BindingsByName  map[string]*GlobalBinding
	Namespaces      map[string]*Namespace
}

type Result struct {
	Program               ast.Program
	BaseDirByFile         map[string]string
	Globals               GlobalState
	GlobalVarByName       map[string]*GlobalVar
	GlobalVarOrder        []string
	TopLevelExprs         []TopLevelExprResult
	Bindings              []*GlobalBinding
	BindingsByName        map[string]*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
	Namespaces            map[string]*Namespace
	DoBlocks              []ast.DoBlock
	Submits               []ast.SubmitBlock
	StepOrder             []string
	SubmitByName          map[string]*SubmitSpec
	StepScopeByName       map[string]*StepScopePlan
	Analyse               []*AnalyseSpec
}

type VisibleBinding struct {
	Name      string
	SourceVar string
	Source    string
	ViaStep   string
	Span      diag.Span
}

type ScopeImport struct {
	Source    string
	Visible   string
	SourceVar string
	Full      bool
	Span      diag.Span
}

type StepScopePlan struct {
	StepName       string
	Inherited      map[string]VisibleBinding
	ExplicitDelta  []ScopeImport
	Effective      map[string]VisibleBinding
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
