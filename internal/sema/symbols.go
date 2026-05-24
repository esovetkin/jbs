package sema

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type BindingShape string

const (
	BindingScalar BindingShape = "scalar"
	BindingTable  BindingShape = "table"
)

type ImportContext string

const (
	ImportIntoStep    ImportContext = "step"
	ImportIntoAnalyse ImportContext = "analyse"
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
	Order           []string
	Span            diag.Span
	DependsOn       []string
	DependsOnKeys   []BindingVersionKey
	SyntheticGlobal bool
	VersionID       string
}

func (b *GlobalBinding) Supports(ctx ImportContext) bool {
	return b.SupportIssue(ctx) == DisallowedBindingNone
}

func (b *GlobalBinding) SupportIssue(ctx ImportContext) DisallowedBindingReason {
	if b == nil {
		return DisallowedBindingNotData
	}
	switch ctx {
	case ImportIntoStep:
		return DisallowedBindingNone
	case ImportIntoAnalyse:
		if b.Value.Kind == eval.KindComb {
			return DisallowedBindingAnalyseTable
		}
		if b.Value.Kind != eval.KindString {
			return DisallowedBindingAnalyseNonString
		}
		return DisallowedBindingNone
	default:
		return DisallowedBindingNotData
	}
}

type GlobalState struct {
	Values map[string]eval.Value
	Spans  map[string]diag.Span
}

type GlobalVar struct {
	Name          string
	Value         eval.Value
	Span          diag.Span
	Order         []string
	Vars          map[string][]eval.Value
	Namespace     string
	DependsOn     []string
	DependsOnKeys []BindingVersionKey
	VersionID     string
}

type Namespace struct {
	Name     string
	Members  []string
	Bindings []string
	Steps    []string
}

type TopLevelExprResult struct {
	Index int
	Seq   int
	Span  diag.Span
	Value eval.Value
	Echo  bool
}

type PrintEvent struct {
	Index   int
	Seq     int
	Span    diag.Span
	Values  []eval.Value
	Options eval.PrintOptions
}

type ScopeSnapshot struct {
	Index           int
	Globals         GlobalState
	GlobalVarByName map[string]*GlobalVar
	GlobalVarOrder  []string
	Bindings        []*GlobalBinding
	BindingsByName  map[string]*GlobalBinding
	BindingsByKey   map[BindingVersionKey]*GlobalBinding
	Namespaces      map[string]*Namespace
}

type Result struct {
	Program               ast.Program
	BaseDirByFile         map[string]string
	Globals               GlobalState
	GlobalVarByName       map[string]*GlobalVar
	GlobalVarOrder        []string
	TopLevelExprs         []TopLevelExprResult
	PrintEvents           []PrintEvent
	Bindings              []*GlobalBinding
	BindingsByName        map[string]*GlobalBinding
	BindingsByKey         map[BindingVersionKey]*GlobalBinding
	ScopeSnapshotsByIndex map[int]*ScopeSnapshot
	ScopeSnapshotsByBlock map[string]*ScopeSnapshot
	Namespaces            map[string]*Namespace
	DoBlocks              []ast.DoBlock
	StepOrder             []string
	StepScopeByName       map[string]*StepScopePlan
	Analyse               []*AnalyseSpec
}

type VisibleBinding struct {
	Name      string
	SourceVar string
	Source    string
	SourceKey BindingVersionKey
	ViaStep   string
	Span      diag.Span
}

type ScopeImport struct {
	ItemID    int
	Source    string
	SourceKey BindingVersionKey
	Visible   string
	SourceVar string
	Full      bool
	Span      diag.Span
}

type WithExpansion struct {
	ItemID           int
	Source           string
	SourceKey        BindingVersionKey
	DisplaySource    string
	Vars             []ExpandedWithVar
	VarsByName       map[string][]eval.Value
	ProjectionByName map[string][]eval.ProjectionKey
	RowCount         int
	Full             bool
	Span             diag.Span
}

type StepScopePlan struct {
	StepName        string
	Inherited       map[string]VisibleBinding
	ExplicitDelta   []ScopeImport
	Effective       map[string]VisibleBinding
	EffectiveValues map[string][]eval.Value
	InheritedSteps  []string
	Expansions      []WithExpansion
}

type PatternTemplate struct {
	Regex              string
	CaptureTypesByName map[string]string
	Span               diag.Span
}

type AnalyseFileKind string

const (
	AnalyseFileNone  AnalyseFileKind = ""
	AnalyseFileExact AnalyseFileKind = "exact"
	AnalyseFileRegex AnalyseFileKind = "regex"
)

type AnalyseFileTargetSpec struct {
	Kind  AnalyseFileKind
	Value string
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
	Name        string
	DisplayName string
	File        string
	FileTarget  AnalyseFileTargetSpec
	Template    PatternTemplate
	Span        diag.Span
}

type AnalyseColumnSpec struct {
	Name   string
	Title  string
	Source string
	Span   diag.Span
}
