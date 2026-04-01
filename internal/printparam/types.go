package printparam

type Row struct {
	StepKind string
	StepName string
	Values   map[string]string
}

type Table struct {
	Columns []string
	Rows    []Row
}

type RenderType string

const (
	RenderPretty RenderType = "pretty"
	RenderCSV    RenderType = "csv"
)
