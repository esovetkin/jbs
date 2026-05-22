package run

import (
	"io"
	"time"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

type Options struct {
	Input       string
	Result      *sema.Result
	Sources     map[string]string
	ProgramFile string
	Benchmark   string
	NoStrict    bool
	Weak        bool
	Limit       int
	PrintEvents []sema.PrintEvent
	Stdout      io.Writer
	Stderr      io.Writer
}

type BenchmarkDirOptions struct {
	Root      string
	Benchmark string
	Stdout    io.Writer
	Stderr    io.Writer
}

type Status string

const (
	StatusNotStarted  Status = "NOTSTARTED"
	StatusRunning     Status = "RUNNING"
	StatusFinished    Status = "FINISHED"
	StatusError       Status = "ERROR"
	StatusBlocked     Status = "BLOCKED"
	StatusInterrupted Status = "INTERRUPTED"
)

type RootStatus struct {
	Schema     int       `json:"schema"`
	Status     Status    `json:"status"`
	SourceHash string    `json:"source_hash"`
	PID        int       `json:"pid,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Error      string    `json:"error,omitempty"`
}

type WorkStatus struct {
	Schema     int        `json:"schema"`
	Status     Status     `json:"status"`
	Step       string     `json:"step"`
	Row        int        `json:"row"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Duration   *float64   `json:"duration,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type Manifest struct {
	Schema              int            `json:"schema"`
	SourceHash          string         `json:"source_hash"`
	BenchmarkName       string         `json:"benchmark_name"`
	BenchmarkComponent  string         `json:"benchmark_component,omitempty"`
	AnalyseTablePrefix  string         `json:"analyse_table_prefix,omitempty"`
	RunID               string         `json:"run_id"`
	GlobalNProc         int            `json:"global_nproc"`
	AnalyseDatabase     string         `json:"analyse_database,omitempty"`
	AnalyseDatabasePath string         `json:"analyse_database_path,omitempty"`
	WorkLimit           int            `json:"work_limit,omitempty"`
	TemplateHashes      []TemplateHash `json:"template_hashes,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	Steps               []ManifestStep `json:"steps"`
	Work                []ManifestWork `json:"work"`
}

type TemplateHash struct {
	Step       string `json:"step"`
	SourcePath string `json:"source_path"`
	DestName   string `json:"dest_name"`
	SHA256     string `json:"sha256"`
	Mode       string `json:"mode,omitempty"`
}

type ManifestStep struct {
	Name         string `json:"name"`
	Dir          string `json:"dir"`
	NProc        int    `json:"nproc"`
	AnalyseCSV   string `json:"analyse_csv,omitempty"`
	AnalyseTable string `json:"analyse_table,omitempty"`
}

func (s ManifestStep) HasAnalyse() bool {
	return s.AnalyseCSV != "" || s.AnalyseTable != ""
}

type ManifestWork struct {
	Step   string            `json:"step"`
	Row    int               `json:"row"`
	Dir    string            `json:"dir"`
	Deps   []ManifestWorkRef `json:"deps,omitempty"`
	Values map[string]string `json:"values"`
}

type ManifestWorkRef struct {
	Step string `json:"step"`
	Row  int    `json:"row"`
	Link string `json:"link,omitempty"`
}
