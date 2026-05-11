package run

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type AnalyseOutputSummary struct {
	Path string
	Rows int
	Cols int
}

func BuildAnalyseOutputSummaries(store *Store) ([]AnalyseOutputSummary, error) {
	if store.Manifest.AnalyseDatabasePath != "" {
		return buildSQLiteAnalyseOutputSummaries(store)
	}
	return buildCSVAnalyseOutputSummaries(store)
}

func PrintAnalyseOutputSummaries(w io.Writer, summaries []AnalyseOutputSummary) {
	if w == nil || len(summaries) == 0 {
		return
	}
	rows := []alignedTableRow{
		alignedData("analysis", "nrows", "ncols"),
		alignedSeparator(),
	}
	for _, summary := range summaries {
		rows = append(rows, alignedData(
			summary.Path,
			strconv.Itoa(summary.Rows),
			strconv.Itoa(summary.Cols),
		))
	}
	writeAlignedTable(w, rows, numericColumns(1, 2))
}

func buildCSVAnalyseOutputSummaries(store *Store) ([]AnalyseOutputSummary, error) {
	summaries := make([]AnalyseOutputSummary, 0)
	for _, step := range store.Manifest.Steps {
		if step.AnalyseCSV == "" {
			continue
		}
		path := filepath.Join(store.RunDir, step.Dir, step.AnalyseCSV)
		rows, cols, err := summarizeAnalyseCSV(path)
		if err != nil {
			return nil, fmt.Errorf("summarize analyse output %s: %w", path, err)
		}
		summaries = append(summaries, AnalyseOutputSummary{Path: path, Rows: rows, Cols: cols})
	}
	return summaries, nil
}

func buildSQLiteAnalyseOutputSummaries(store *Store) ([]AnalyseOutputSummary, error) {
	db, err := openAnalyseDB(store.Manifest.AnalyseDatabasePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	display := store.Manifest.AnalyseDatabase
	if display == "" {
		display = store.Manifest.AnalyseDatabasePath
	}
	summaries := make([]AnalyseOutputSummary, 0)
	for _, step := range store.Manifest.Steps {
		if step.AnalyseTable == "" {
			continue
		}
		rows, cols, err := summarizeAnalyseSQLiteTable(db, step.AnalyseTable)
		if err != nil {
			return nil, fmt.Errorf("summarize analyse table %s: %w", step.AnalyseTable, err)
		}
		summaries = append(summaries, AnalyseOutputSummary{
			Path: display + ":" + step.AnalyseTable,
			Rows: rows,
			Cols: cols,
		})
	}
	return summaries, nil
}

func summarizeAnalyseCSV(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err == io.EOF {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	rows := 0
	for {
		if _, err := r.Read(); err != nil {
			if err == io.EOF {
				break
			}
			return 0, 0, err
		}
		rows++
	}
	return rows, len(header), nil
}

func summarizeAnalyseSQLiteTable(db *sql.DB, table string) (int, int, error) {
	header, err := sqliteTableColumns(db, table)
	if err != nil {
		return 0, 0, err
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quoteSQLiteIdent(table)).Scan(&rows); err != nil {
		return 0, 0, err
	}
	return rows, len(header), nil
}
