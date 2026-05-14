package run

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsutil"
)

type AnalyseColumnKind string

const (
	analyseColumnWorkValue AnalyseColumnKind = "work_value"
	analyseColumnPattern   AnalyseColumnKind = "pattern"
)

type AnalyseValueKind string

const (
	analyseValueString AnalyseValueKind = "string"
	analyseValueInt    AnalyseValueKind = "int"
	analyseValueFloat  AnalyseValueKind = "float"
	analyseValueBool   AnalyseValueKind = "bool"
)

type AnalysePlan struct {
	Step        string
	CSV         string
	Header      []string
	ColumnTypes []AnalyseValueKind
	Columns     []AnalyseColumnPlan
	Patterns    map[string]AnalysePatternPlan
}

type AnalyseColumnPlan struct {
	Kind       AnalyseColumnKind
	Source     string
	Title      string
	GroupCount int
	GroupTypes []AnalyseValueKind
}

type AnalysePatternPlan struct {
	Name         string
	File         string
	Regex        string
	GroupCount   int
	GroupTypes   []AnalyseValueKind
	CompiledExpr *regexp.Regexp
}

type AnalyseCell struct {
	Kind  AnalyseValueKind
	Valid bool
	Text  string
	Int   int64
	Float float64
	Bool  bool
}

type patternMatches map[string][][]AnalyseCell

func stringAnalyseCell(s string) AnalyseCell {
	return AnalyseCell{Kind: analyseValueString, Valid: true, Text: s}
}

func missingAnalyseCell(kind AnalyseValueKind) AnalyseCell {
	return AnalyseCell{Kind: kind}
}

func analyseCellFromCapture(text string, kind AnalyseValueKind) (AnalyseCell, error) {
	if text == "" && kind != analyseValueString {
		return missingAnalyseCell(kind), nil
	}
	switch kind {
	case analyseValueInt:
		v, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return AnalyseCell{}, fmt.Errorf("parse integer capture %q: %w", text, err)
		}
		return AnalyseCell{Kind: kind, Valid: true, Text: text, Int: v}, nil
	case analyseValueFloat:
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return AnalyseCell{}, fmt.Errorf("parse float capture %q: %w", text, err)
		}
		return AnalyseCell{Kind: kind, Valid: true, Text: text, Float: v}, nil
	default:
		return stringAnalyseCell(text), nil
	}
}

func analyseCellFromWorkValue(text string, kind AnalyseValueKind) (AnalyseCell, error) {
	switch kind {
	case analyseValueInt:
		v, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return AnalyseCell{}, fmt.Errorf("parse integer value %q: %w", text, err)
		}
		return AnalyseCell{Kind: kind, Valid: true, Text: text, Int: v}, nil
	case analyseValueFloat:
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return AnalyseCell{}, fmt.Errorf("parse float value %q: %w", text, err)
		}
		return AnalyseCell{Kind: kind, Valid: true, Text: text, Float: v}, nil
	case analyseValueBool:
		v, err := strconv.ParseBool(text)
		if err != nil {
			return AnalyseCell{}, fmt.Errorf("parse boolean value %q: %w", text, err)
		}
		return AnalyseCell{Kind: kind, Valid: true, Text: text, Bool: v}, nil
	default:
		return stringAnalyseCell(text), nil
	}
}

func (c AnalyseCell) CSVString() string {
	if !c.Valid {
		return ""
	}
	return c.Text
}

func (c AnalyseCell) SQLiteValue() any {
	if !c.Valid {
		return nil
	}
	switch c.Kind {
	case analyseValueInt:
		return c.Int
	case analyseValueFloat:
		return c.Float
	case analyseValueBool:
		if c.Bool {
			return int64(1)
		}
		return int64(0)
	default:
		return c.Text
	}
}

func RunAnalyses(store *Store, analyses map[string]AnalysePlan) error {
	if store.Manifest.AnalyseDatabasePath != "" {
		return runAnalysesSQLite(store, analyses)
	}
	return runAnalysesCSV(store, analyses)
}

func runAnalysesCSV(store *Store, analyses map[string]AnalysePlan) error {
	for _, step := range store.Manifest.Steps {
		if step.AnalyseCSV == "" {
			continue
		}
		plan, ok := analyses[step.Name]
		if !ok {
			return fmt.Errorf("missing analyse plan for step %q", step.Name)
		}
		if err := runStepAnalyseCSV(store, step, plan); err != nil {
			return err
		}
	}
	return nil
}

func runStepAnalyseCSV(store *Store, step ManifestStep, plan AnalysePlan) error {
	dataRows, err := collectStepAnalyseRows(store, step, plan)
	if err != nil {
		return err
	}
	rows := [][]string{append([]string(nil), plan.Header...)}
	rows = append(rows, analyseCellsToCSVRows(dataRows)...)
	path := filepath.Join(store.RunDir, step.Dir, step.AnalyseCSV)
	return fsutil.WriteCSVAtomic(path, rows, 0o644, durableWrite)
}

func collectStepAnalyseRows(store *Store, step ManifestStep, plan AnalysePlan) ([][]AnalyseCell, error) {
	rows := make([][]AnalyseCell, 0)
	for _, work := range store.Manifest.Work {
		if work.Step != step.Name {
			continue
		}
		status, err := store.LoadWorkStatus(work)
		if err != nil {
			return nil, fmt.Errorf("analyse %s/%s: %w", work.Step, work.Dir, err)
		}
		if status.Status != StatusFinished {
			return nil, fmt.Errorf("cannot analyse %s/%s: status is %s", work.Step, work.Dir, status.Status)
		}
		workRows, err := analyseWorkPackage(store.WorkDir(work), work, plan)
		if err != nil {
			return nil, fmt.Errorf("analyse %s/%s: %w", work.Step, work.Dir, err)
		}
		rows = append(rows, workRows...)
	}
	return rows, nil
}

func analyseWorkPackage(workDir string, work ManifestWork, plan AnalysePlan) ([][]AnalyseCell, error) {
	matches, err := collectPatternMatches(workDir, plan.Patterns)
	if err != nil {
		return nil, err
	}
	rowCount := analyseRowCount(plan, matches)
	if rowCount == 0 {
		return nil, nil
	}
	rows := make([][]AnalyseCell, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		row := []AnalyseCell{stringAnalyseCell(work.Dir)}
		for _, col := range plan.Columns {
			values, err := valuesForColumn(work, matches, col, i)
			if err != nil {
				return nil, err
			}
			row = append(row, values...)
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

func valuesForColumn(work ManifestWork, matches patternMatches, col AnalyseColumnPlan, row int) ([]AnalyseCell, error) {
	switch col.Kind {
	case analyseColumnWorkValue:
		kind := analyseValueString
		if len(col.GroupTypes) > 0 && col.GroupTypes[0] != "" {
			kind = col.GroupTypes[0]
		}
		text, ok := work.Values[col.Source]
		if !ok {
			return []AnalyseCell{missingAnalyseCell(kind)}, nil
		}
		cell, err := analyseCellFromWorkValue(text, kind)
		if err != nil {
			return nil, fmt.Errorf("work variable %q: %w", col.Source, err)
		}
		return []AnalyseCell{cell}, nil
	case analyseColumnPattern:
		if row < len(matches[col.Source]) {
			return matches[col.Source][row], nil
		}
		return missingCells(groupTypesForCount(col.GroupCount, col.GroupTypes)), nil
	default:
		return []AnalyseCell{stringAnalyseCell("")}, nil
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
			groups, err := submatchGroups(raw, groupTypesForCount(p.GroupCount, p.GroupTypes))
			if err != nil {
				return nil, fmt.Errorf("pattern %q in %q: %w", p.Name, rel, err)
			}
			out[p.Name] = groups
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

func submatchGroups(raw [][]string, kinds []AnalyseValueKind) ([][]AnalyseCell, error) {
	out := make([][]AnalyseCell, 0, len(raw))
	for _, m := range raw {
		row := make([]AnalyseCell, 0, len(kinds))
		for i, kind := range kinds {
			text := ""
			if i+1 < len(m) {
				text = m[i+1]
			}
			cell, err := analyseCellFromCapture(text, kind)
			if err != nil {
				return nil, err
			}
			row = append(row, cell)
		}
		out = append(out, row)
	}
	return out, nil
}

func groupTypesForCount(count int, groupTypes []AnalyseValueKind) []AnalyseValueKind {
	out := make([]AnalyseValueKind, count)
	for i := range out {
		out[i] = analyseValueString
		if i < len(groupTypes) && groupTypes[i] != "" {
			out[i] = groupTypes[i]
		}
	}
	return out
}

func missingCells(kinds []AnalyseValueKind) []AnalyseCell {
	out := make([]AnalyseCell, len(kinds))
	for i, kind := range kinds {
		out[i] = missingAnalyseCell(kind)
	}
	return out
}

func analyseCellsToCSVRows(rows [][]AnalyseCell) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, len(row))
		for i, cell := range row {
			values[i] = cell.CSVString()
		}
		out = append(out, values)
	}
	return out
}
