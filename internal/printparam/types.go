package printparam

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"

type Row struct {
	Benchmark string
	StepKind  string
	StepName  string
	Values    map[string]string
}

type Table struct {
	Columns         []string
	Rows            []Row
	BenchmarkColumn bool
}

type ComponentPlan struct {
	Name     string
	WorkPlan workplan.Plan
}

type RenderType string

const (
	RenderPretty RenderType = "pretty"
	RenderCSV    RenderType = "csv"
)
