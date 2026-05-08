package run

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

type AnalyseColumnKind string

const (
	analyseColumnWorkValue AnalyseColumnKind = "work_value"
	analyseColumnPattern   AnalyseColumnKind = "pattern"
)

type AnalysePlan struct {
	Step     string
	CSV      string
	Header   []string
	Columns  []AnalyseColumnPlan
	Patterns map[string]AnalysePatternPlan
}

type AnalyseColumnPlan struct {
	Kind       AnalyseColumnKind
	Source     string
	Title      string
	GroupCount int
}

type AnalysePatternPlan struct {
	Name         string
	File         string
	Regex        string
	GroupCount   int
	CompiledExpr *regexp.Regexp
}

type patternMatches map[string][][]string

func RunAnalyses(store *Store, analyses map[string]AnalysePlan) error {
	for _, step := range store.Manifest.Steps {
		if step.AnalyseCSV == "" {
			continue
		}
		plan, ok := analyses[step.Name]
		if !ok {
			return fmt.Errorf("missing analyse plan for step %q", step.Name)
		}
		if err := runStepAnalyse(store, step, plan); err != nil {
			return err
		}
	}
	return nil
}

func runStepAnalyse(store *Store, step ManifestStep, plan AnalysePlan) error {
	rows := [][]string{append([]string(nil), plan.Header...)}
	for _, work := range store.Manifest.Work {
		if work.Step != step.Name {
			continue
		}
		status, err := store.LoadWorkStatus(work)
		if err != nil {
			return fmt.Errorf("analyse %s/%s: %w", work.Step, work.Dir, err)
		}
		if status.Status != StatusFinished {
			return fmt.Errorf("cannot analyse %s/%s: status is %s", work.Step, work.Dir, status.Status)
		}
		workRows, err := analyseWorkPackage(store.WorkDir(work), work, plan)
		if err != nil {
			return fmt.Errorf("analyse %s/%s: %w", work.Step, work.Dir, err)
		}
		rows = append(rows, workRows...)
	}
	path := filepath.Join(store.RunDir, step.Dir, step.AnalyseCSV)
	return fsutil.WriteCSVAtomic(path, rows, 0o644, durableWrite)
}

func analyseWorkPackage(workDir string, work ManifestWork, plan AnalysePlan) ([][]string, error) {
	matches, err := collectPatternMatches(workDir, plan.Patterns)
	if err != nil {
		return nil, err
	}
	rowCount := analyseRowCount(plan, matches)
	if rowCount == 0 {
		return nil, nil
	}
	rows := make([][]string, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		row := []string{work.Dir}
		for _, col := range plan.Columns {
			row = append(row, valuesForColumn(work, matches, col, i)...)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func analyseRowCount(plan AnalysePlan, matches patternMatches) int {
	hasPatternColumn := false
	maxRows := 0
	for _, col := range plan.Columns {
		if col.Kind != analyseColumnPattern {
			continue
		}
		hasPatternColumn = true
		if n := len(matches[col.Source]); n > maxRows {
			maxRows = n
		}
	}
	if !hasPatternColumn {
		return 1
	}
	return maxRows
}

func valuesForColumn(work ManifestWork, matches patternMatches, col AnalyseColumnPlan, row int) []string {
	switch col.Kind {
	case analyseColumnWorkValue:
		return []string{work.Values[col.Source]}
	case analyseColumnPattern:
		groups := make([]string, col.GroupCount)
		if row < len(matches[col.Source]) {
			copy(groups, matches[col.Source][row])
		}
		return groups
	default:
		return []string{""}
	}
}

func collectPatternMatches(workDir string, patterns map[string]AnalysePatternPlan) (patternMatches, error) {
	byFile := make(map[string][]AnalysePatternPlan)
	for _, p := range patterns {
		byFile[p.File] = append(byFile[p.File], p)
	}
	out := make(patternMatches)
	for rel, ps := range byFile {
		path, err := analyseFilePath(workDir, rel)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read analyse file %q: %w", rel, err)
		}
		text := string(data)
		for _, p := range ps {
			raw := p.CompiledExpr.FindAllStringSubmatch(text, -1)
			out[p.Name] = submatchGroups(raw)
		}
	}
	return out, nil
}

func analyseFilePath(workDir, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("analyse file path is empty")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("analyse file path %q must be relative", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("analyse file path %q escapes the workpackage directory", rel)
	}
	return filepath.Join(workDir, clean), nil
}

func submatchGroups(raw [][]string) [][]string {
	out := make([][]string, 0, len(raw))
	for _, m := range raw {
		out = append(out, append([]string(nil), m[1:]...))
	}
	return out
}
