// define the internal YAML document model produced by lowering
//
// declare strongly-typed structs for root data, parameter/pattern
// sets, step/use/submit operations, analyse/result sections
package lower

import (
	"gopkg.in/yaml.v3"

	"jbs/internal/diag"
	"jbs/internal/sema"
)

// ReservedSeparator keeps grouped source-row IDs opaque in synthetic _jr__
// helpers until inherited row-context expansion is explicitly requested.
const ReservedSeparator = "####"
const escapedAliasPrefix = "_ja__"

type Literal string

func (l Literal) MarshalYAML() (interface{}, error) {
	n := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(l), Style: yaml.LiteralStyle}
	return &n, nil
}

type SingleQuoted string

func (s SingleQuoted) MarshalYAML() (interface{}, error) {
	n := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(s), Style: yaml.SingleQuotedStyle}
	return &n, nil
}

type Document struct {
	Name         string         `yaml:"name"`
	Outpath      string         `yaml:"outpath"`
	Comment      string         `yaml:"comment,omitempty"`
	ParameterSet []ParameterSet `yaml:"parameterset,omitempty"`
	PatternSet   []PatternSet   `yaml:"patternset,omitempty"`
	Step         []Step         `yaml:"step,omitempty"`
	Analyser     []Analyser     `yaml:"analyser,omitempty"`
	Result       *ResultObject  `yaml:"result,omitempty"`
	Meta         DocumentMeta   `yaml:"-"`
}

type DocumentMeta struct {
	SourceComments []CommentProjection
}

type CommentProjection struct {
	Target string
	Text   string
}

type ParameterSetKind string

const (
	ParameterSetKindGlobalTable ParameterSetKind = "global_table"
	ParameterSetKindSubset      ParameterSetKind = "subset"
	ParameterSetKindSubmitInit  ParameterSetKind = "submit_system"
)

type ParameterSetMeta struct {
	Kind   ParameterSetKind
	Source string
	Step   string
}

type ParameterSet struct {
	Name      string           `yaml:"name"`
	InitWith  string           `yaml:"init_with,omitempty"`
	Parameter []Parameter      `yaml:"parameter,omitempty"`
	Meta      ParameterSetMeta `yaml:"-"`
}

type Parameter struct {
	Name      string      `yaml:"name"`
	Type      string      `yaml:"type,omitempty"`
	Mode      string      `yaml:"mode,omitempty"`
	Separator string      `yaml:"separator,omitempty"`
	Value     interface{} `yaml:"_"`
}

type PatternSetKind string

const (
	PatternSetKindImportedGlobals PatternSetKind = "imported_globals"
	PatternSetKindInlineAnalyse   PatternSetKind = "analyse_inline"
)

type PatternSetMeta struct {
	Kind   PatternSetKind
	Source string
}

type PatternSet struct {
	Name     string         `yaml:"name"`
	InitWith string         `yaml:"init_with,omitempty"`
	Pattern  []Pattern      `yaml:"pattern,omitempty"`
	Meta     PatternSetMeta `yaml:"-"`
}

type Pattern struct {
	Name  string      `yaml:"name"`
	Type  string      `yaml:"type,omitempty"`
	Value interface{} `yaml:"_"`
	Meta  PatternMeta `yaml:"-"`
}

type PatternMeta struct {
	IsAnalyseAlias bool
	AnalyseStep    string
	AliasName      string
	PatternRef     string
}

type Step struct {
	Name       string        `yaml:"name"`
	Depend     string        `yaml:"depend,omitempty"`
	MaxAsync   *int          `yaml:"max_async,omitempty"`
	Procs      *int          `yaml:"procs,omitempty"`
	Iterations *int          `yaml:"iterations,omitempty"`
	Use        []interface{} `yaml:"use,omitempty"`
	Do         []interface{} `yaml:"do,omitempty"`
	Meta       StepMeta      `yaml:"-"`
}

type StepKind string

const (
	StepKindDo     StepKind = "do"
	StepKindSubmit StepKind = "submit"
)

type StepMeta struct {
	Kind          StepKind
	Source        string
	InheritsFrom  []string
	InheritedVars []string
}

type UseEntry struct {
	From  string `yaml:"from,omitempty"`
	Value string `yaml:"_"`
}

type SubmitOperation struct {
	DoneFile  string `yaml:"done_file"`
	ErrorFile string `yaml:"error_file"`
	Command   string `yaml:"_"`
}

type AnalyserMeta struct {
	Source string
}

type Analyser struct {
	Name    string        `yaml:"name"`
	Use     string        `yaml:"use,omitempty"`
	Analyse []AnalyseItem `yaml:"analyse"`
	Meta    AnalyserMeta  `yaml:"-"`
}

type AnalyseItem struct {
	Step string        `yaml:"step"`
	File []AnalyseFile `yaml:"file"`
}

type AnalyseFile struct {
	Use   string `yaml:"use,omitempty"`
	Value string `yaml:"_"`
}

type ResultMeta struct{}

type ResultObject struct {
	Use   []string      `yaml:"use"`
	Table []ResultTable `yaml:"table"`
	Meta  ResultMeta    `yaml:"-"`
}

type ResultTableMeta struct {
	Source string
}

type ResultTable struct {
	Name   string          `yaml:"name"`
	Style  string          `yaml:"style"`
	Column []ResultColumn  `yaml:"column"`
	Meta   ResultTableMeta `yaml:"-"`
}

type ResultColumn struct {
	Title string `yaml:"title,omitempty"`
	Expr  string `yaml:"_"`
}

type subsetKey struct {
	Step          string
	Source        string
	Vars          string
	Full          bool
	InheritedRows string
}

type sourceRowContext struct {
	VarName string
	Groups  []string
}

type sourceRowKey struct {
	Public  string
	Version string
}

func (k sourceRowKey) display() string {
	if k.Public != "" {
		return k.Public
	}
	return k.Version
}

type subsetInfo struct {
	Name       string
	RowContext sourceRowContext
}

type lowerContext struct {
	res                       *sema.Result
	doc                       Document
	diags                     *diag.Diagnostics
	names                     map[string]struct{}
	sourceParameterSetEmitted map[string]struct{}
	subsetNames               map[subsetKey]subsetInfo
	stepSourceRows            map[string]map[sourceRowKey]sourceRowContext
	patternSetIndexByGroup    map[string]int
	analyserNames             map[string]string
}
