package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveHistoryPath(cwd string) (string, error) {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determine working directory: %w", err)
		}
		trimmed = wd
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Join(abs, ".jbs_history"), nil
}

func EnsureHistoryDir(historyPath string) error {
	dir := filepath.Dir(historyPath)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
