package run

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type StatusCounts struct {
	Finished    int
	Error       int
	Blocked     int
	NotStarted  int
	Running     int
	Interrupted int
}

type StatusSummaryRow struct {
	Label  string
	Counts StatusCounts
}

type StepTreeRow struct {
	Name  string
	Label string
}

type JobTreeRow struct {
	Label string
	Count int
}

type JobTreeSummary struct {
	Rows  []JobTreeRow
	Total int
}

type FailedWorkDirectory struct {
	Step string
	Row  int
	Path string
}

type RunStatusSummary struct {
	RunDir     string
	Rows       []StatusSummaryRow
	Total      StatusCounts
	FailedWork []FailedWorkDirectory
}

func BuildStatusSummary(store *Store) (RunStatusSummary, error) {
	byStep := make(map[string]StatusCounts, len(store.Manifest.Steps))
	for _, step := range store.Manifest.Steps {
		byStep[step.Name] = StatusCounts{}
	}
	var total StatusCounts
	failed := make([]FailedWorkDirectory, 0)
	for _, work := range store.Manifest.Work {
		status, err := store.LoadWorkStatus(work)
		if err != nil {
			return RunStatusSummary{}, fmt.Errorf("read %s/%s status: %w", work.Step, work.Dir, err)
		}
		counts := byStep[work.Step]
		addStatusCount(&counts, status.Status)
		byStep[work.Step] = counts
		addStatusCount(&total, status.Status)
		if status.Status == StatusError {
			failed = append(failed, FailedWorkDirectory{
				Step: work.Step,
				Row:  work.Row,
				Path: store.WorkDir(work),
			})
		}
	}
	return RunStatusSummary{
		RunDir:     store.RunDir,
		Rows:       buildStatusSummaryRows(store.Manifest, byStep),
		Total:      total,
		FailedWork: failed,
	}, nil
}

func addStatusCount(c *StatusCounts, status Status) {
	switch status {
	case StatusFinished:
		c.Finished++
	case StatusError:
		c.Error++
	case StatusBlocked:
		c.Blocked++
	case StatusNotStarted:
		c.NotStarted++
	case StatusRunning:
		c.Running++
	case StatusInterrupted:
		c.Interrupted++
	}
}

func PrintStatusSummary(w io.Writer, summary RunStatusSummary) {
	if w == nil {
		return
	}
	rows := []alignedTableRow{
		alignedData("step", "FINISHED", "ERROR", "BLOCKED", "NOTSTARTED", "RUNNING", "INTERRUPTED"),
		alignedSeparator(),
	}
	for _, row := range summary.Rows {
		rows = append(rows, alignedData(statusSummaryCells(row.Label, row.Counts)...))
	}
	rows = append(rows,
		alignedSeparator(),
		alignedData(statusSummaryCells("total:", summary.Total)...),
	)
	writeAlignedTable(w, rows, numericColumns(1, 6))
}

func BuildJobTreeSummary(manifest Manifest) JobTreeSummary {
	byStep := make(map[string]int, len(manifest.Steps))
	for _, step := range manifest.Steps {
		byStep[step.Name] = 0
	}
	for _, work := range manifest.Work {
		byStep[work.Step]++
	}
	treeRows := buildStepTreeRows(manifest)
	rows := make([]JobTreeRow, 0, len(treeRows))
	for _, row := range treeRows {
		rows = append(rows, JobTreeRow{Label: row.Label, Count: byStep[row.Name]})
	}
	return JobTreeSummary{Rows: rows, Total: len(manifest.Work)}
}

func PrintJobTreeSummary(w io.Writer, summary JobTreeSummary) {
	if w == nil {
		return
	}
	rows := []alignedTableRow{
		alignedData("step", "#"),
		alignedSeparator(),
	}
	for _, row := range summary.Rows {
		rows = append(rows, alignedData(row.Label, strconv.Itoa(row.Count)))
	}
	rows = append(rows,
		alignedSeparator(),
		alignedData("total:", strconv.Itoa(summary.Total)),
	)
	writeAlignedTable(w, rows, numericColumns(1, 1))
}

func PrintFailedWorkDirectories(w io.Writer, failed []FailedWorkDirectory) {
	if w == nil || len(failed) == 0 {
		return
	}
	fmt.Fprintln(w, "failed workpackage directories:")
	for _, item := range failed {
		fmt.Fprintln(w, item.Path)
	}
}

func statusSummaryCells(label string, counts StatusCounts) []string {
	return []string{
		label,
		strconv.Itoa(counts.Finished),
		strconv.Itoa(counts.Error),
		strconv.Itoa(counts.Blocked),
		strconv.Itoa(counts.NotStarted),
		strconv.Itoa(counts.Running),
		strconv.Itoa(counts.Interrupted),
	}
}

func buildStatusSummaryRows(manifest Manifest, counts map[string]StatusCounts) []StatusSummaryRow {
	treeRows := buildStepTreeRows(manifest)
	rows := make([]StatusSummaryRow, 0, len(treeRows))
	for _, row := range treeRows {
		rows = append(rows, StatusSummaryRow{Label: row.Label, Counts: counts[row.Name]})
	}
	return rows
}

func buildStepTreeRows(manifest Manifest) []StepTreeRow {
	parents, children := stepDependencyGraph(manifest)
	roots := make([]string, 0)
	for _, step := range manifest.Steps {
		if len(parents[step.Name]) == 0 {
			roots = append(roots, step.Name)
		}
	}
	if len(roots) == 0 {
		for _, step := range manifest.Steps {
			roots = append(roots, step.Name)
		}
	}
	return renderStepTreeRows(manifest, children, roots)
}

func stepDependencyGraph(manifest Manifest) (map[string][]string, map[string][]string) {
	known := make(map[string]bool, len(manifest.Steps))
	order := make(map[string]int, len(manifest.Steps))
	for i, step := range manifest.Steps {
		known[step.Name] = true
		order[step.Name] = i
	}

	parents := make(map[string][]string)
	children := make(map[string][]string)
	seen := make(map[string]struct{})
	for _, work := range manifest.Work {
		if !known[work.Step] {
			continue
		}
		for _, dep := range work.Deps {
			if !known[dep.Step] || dep.Step == work.Step {
				continue
			}
			edge := dep.Step + "\x00" + work.Step
			if _, ok := seen[edge]; ok {
				continue
			}
			seen[edge] = struct{}{}
			parents[work.Step] = append(parents[work.Step], dep.Step)
			children[dep.Step] = append(children[dep.Step], work.Step)
		}
	}

	sortByStepOrder := func(values []string) {
		sort.SliceStable(values, func(i, j int) bool {
			return order[values[i]] < order[values[j]]
		})
	}
	for step := range parents {
		sortByStepOrder(parents[step])
	}
	for step := range children {
		sortByStepOrder(children[step])
	}
	return parents, children
}

func renderStepTreeRows(manifest Manifest, children map[string][]string, roots []string) []StepTreeRow {
	rows := make([]StepTreeRow, 0, len(manifest.Steps))
	seen := make(map[string]bool, len(manifest.Steps))
	var walk func(string, string, bool)
	walk = func(step, prefix string, last bool) {
		if seen[step] {
			return
		}
		branch := "├── "
		nextPrefix := prefix + "│   "
		if last {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		rows = append(rows, StepTreeRow{Name: step, Label: prefix + branch + step})
		seen[step] = true

		visible := make([]string, 0, len(children[step]))
		for _, child := range children[step] {
			if !seen[child] {
				visible = append(visible, child)
			}
		}
		for i, child := range visible {
			walk(child, nextPrefix, i == len(visible)-1)
		}
	}

	remainingRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		if !seen[root] {
			remainingRoots = append(remainingRoots, root)
		}
	}
	for i, root := range remainingRoots {
		walk(root, "", i == len(remainingRoots)-1)
	}
	for _, step := range manifest.Steps {
		if !seen[step.Name] {
			walk(step.Name, "", true)
		}
	}
	return rows
}

type alignedTableRow struct {
	cells     []string
	separator bool
}

func alignedData(cells ...string) alignedTableRow {
	return alignedTableRow{cells: cells}
}

func alignedSeparator() alignedTableRow {
	return alignedTableRow{separator: true}
}

func numericColumns(first, last int) map[int]bool {
	out := make(map[int]bool, last-first+1)
	for i := first; i <= last; i++ {
		out[i] = true
	}
	return out
}

func writeAlignedTable(w io.Writer, rows []alignedTableRow, right map[int]bool) {
	widths := tableColumnWidths(rows)
	for _, row := range rows {
		if row.separator {
			writeAlignedSeparator(w, widths)
			continue
		}
		writeAlignedDataRow(w, row.cells, widths, right)
	}
}

func tableColumnWidths(rows []alignedTableRow) []int {
	cols := 0
	for _, row := range rows {
		if row.separator {
			continue
		}
		if len(row.cells) > cols {
			cols = len(row.cells)
		}
	}
	widths := make([]int, cols)
	for _, row := range rows {
		if row.separator {
			continue
		}
		for i, cell := range row.cells {
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

func writeAlignedSeparator(w io.Writer, widths []int) {
	io.WriteString(w, "|")
	for _, width := range widths {
		io.WriteString(w, strings.Repeat("-", width+2))
		io.WriteString(w, "|")
	}
	io.WriteString(w, "\n")
}

func writeAlignedDataRow(w io.Writer, cells []string, widths []int, right map[int]bool) {
	io.WriteString(w, "|")
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		io.WriteString(w, " ")
		if right[i] {
			io.WriteString(w, padLeftRunes(cell, width))
		} else {
			io.WriteString(w, padRightRunes(cell, width))
		}
		io.WriteString(w, " |")
	}
	io.WriteString(w, "\n")
}

func padLeftRunes(s string, width int) string {
	if utf8.RuneCountInString(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-utf8.RuneCountInString(s)) + s
}

func padRightRunes(s string, width int) string {
	if utf8.RuneCountInString(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-utf8.RuneCountInString(s))
}
