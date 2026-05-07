package imports

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func hasDiagCode(diags *diag.Diagnostics, code string) bool {
	if diags == nil {
		return false
	}
	for _, item := range diags.Items {
		if item.Code == code {
			return true
		}
	}
	return false
}
