package run

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

func runAnalysesSQLite(store *Store, analyses map[string]AnalysePlan) error {
	return runAnalysesSQLiteWithOptions(store, analyses, AnalyseRunOptions{})
}

func runAnalysesSQLiteWithOptions(store *Store, analyses map[string]AnalysePlan, opts AnalyseRunOptions) error {
	dbPath := store.Manifest.AnalyseDatabasePath
	if dbPath == "" {
		return fmt.Errorf("analyse database path is empty")
	}
	if err := ensureAnalyseDatabaseParent(dbPath); err != nil {
		return err
	}
	db, err := openAnalyseDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, step := range store.Manifest.Steps {
		if step.AnalyseTable == "" {
			continue
		}
		plan, ok := analyses[step.Name]
		if !ok {
			return fmt.Errorf("missing analyse plan for step %q", step.Name)
		}
		header, err := analyseOutputHeader(plan, opts)
		if err != nil {
			return err
		}
		rows, err := collectStepAnalyseRows(store, step, plan, opts)
		if err != nil {
			return err
		}
		if err := replaceAnalyseTable(tx, step.AnalyseTable, header, analyseOutputColumnTypes(plan, opts), rows); err != nil {
			return fmt.Errorf("write analyse table %q: %w", step.Name, err)
		}
	}
	return tx.Commit()
}

func openAnalyseDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureAnalyseDatabaseParent(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func replaceAnalyseTable(tx *sql.Tx, table string, header []string, columnTypes []AnalyseValueKind, rows [][]AnalyseCell) error {
	quotedTable := quoteSQLiteIdent(table)
	if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + quotedTable); err != nil {
		return err
	}

	defs := make([]string, 0, len(header))
	for i, col := range header {
		kind := analyseValueString
		if i < len(columnTypes) && columnTypes[i] != "" {
			kind = columnTypes[i]
		}
		defs = append(defs, quoteSQLiteIdent(col)+` `+sqliteAnalyseType(kind))
	}
	if _, err := tx.Exec(`CREATE TABLE ` + quotedTable + ` (` + strings.Join(defs, ", ") + `)`); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	columns := make([]string, 0, len(header))
	placeholders := make([]string, 0, len(header))
	for _, col := range header {
		columns = append(columns, quoteSQLiteIdent(col))
		placeholders = append(placeholders, "?")
	}
	stmt, err := tx.Prepare(
		`INSERT INTO ` + quotedTable +
			` (` + strings.Join(columns, ", ") + `) VALUES (` +
			strings.Join(placeholders, ", ") + `)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	args := make([]any, len(header))
	for _, row := range rows {
		for i := range header {
			args[i] = nil
			if i < len(row) {
				args[i] = row[i].SQLiteValue()
			}
		}
		if _, err := stmt.Exec(args...); err != nil {
			return err
		}
	}
	return nil
}

func sqliteAnalyseType(kind AnalyseValueKind) string {
	switch kind {
	case analyseValueInt, analyseValueBool:
		return "INTEGER"
	case analyseValueFloat:
		return "REAL"
	default:
		return "TEXT"
	}
}

func readAnalyseTable(db *sql.DB, table string) ([]string, [][]string, error) {
	header, err := sqliteTableColumns(db, table)
	if err != nil {
		return nil, nil, err
	}
	if len(header) == 0 {
		return header, nil, nil
	}

	cols := make([]string, 0, len(header))
	for _, col := range header {
		cols = append(cols, quoteSQLiteIdent(col))
	}
	rows, err := db.Query(`SELECT ` + strings.Join(cols, ", ") + ` FROM ` + quoteSQLiteIdent(table) + ` ORDER BY rowid`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([][]string, 0)
	for rows.Next() {
		values := make([]any, len(header))
		dest := make([]any, len(header))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(header))
		for i, value := range values {
			row[i] = sqliteValueString(value)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return header, out, nil
}

func sqliteValueString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func sqliteTableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(`PRAGMA table_info(` + quoteSQLiteIdent(table) + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("analyse table %q does not exist", table)
	}
	return columns, nil
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
