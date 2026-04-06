package diag

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestDiagnosticCatalogWellFormed(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatalf("catalog must not be empty")
	}
	codeRE := regexp.MustCompile(`^[EW][0-9]{3}$`)
	for code, meta := range Catalog {
		raw := string(code)
		if !codeRE.MatchString(raw) {
			t.Fatalf("malformed diagnostic code: %q", raw)
		}
		if strings.HasPrefix(raw, "E") && meta.Severity != SeverityError {
			t.Fatalf("error code %s has non-error severity %q", raw, meta.Severity)
		}
		if strings.HasPrefix(raw, "W") && meta.Severity != SeverityWarning {
			t.Fatalf("warning code %s has non-warning severity %q", raw, meta.Severity)
		}
		if meta.Owner == "" {
			t.Fatalf("code %s has empty owner", raw)
		}
		if meta.Summary == "" {
			t.Fatalf("code %s has empty summary", raw)
		}
	}
}

func TestNoRawDiagnosticCodeLiteralsOutsideDiag(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to determine current file")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	internalDir := filepath.Join(root, "internal")
	rawCallRE := regexp.MustCompile(`Add(?:Error|Warning)\(\s*"(?:E|W)\d{3}"`)

	var offenders []string
	err := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		normalized := filepath.ToSlash(path)
		if strings.HasPrefix(normalized, filepath.ToSlash(filepath.Join(internalDir, "diag"))+"/") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if rawCallRE.Match(data) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("raw diagnostic code literals found outside internal/diag: %s", strings.Join(offenders, ", "))
	}
}
