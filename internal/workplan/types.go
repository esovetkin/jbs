package workplan

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

type WorkID struct {
	Step string
	Row  int
}

type Plan struct {
	BenchmarkName string
	SourceHash    string
	GlobalNProc   int
	Steps         []Step
	Work          []WorkPackage
}

type Step struct {
	Name    string
	Kind    string
	After   []string
	DirName string
	NProc   int
	Body    string
	Span    diag.Span
}

type WorkPackage struct {
	ID         WorkID
	StepName   string
	StepKind   string
	Values     map[string]eval.Value
	SourceRows map[sema.BindingVersionKey][]int
	Deps       []WorkID
	Span       diag.Span
}
