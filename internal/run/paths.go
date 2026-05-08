package run

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/benchmarks"
)

var numericRunDir = regexp.MustCompile(`^\d{6}$`)

func safePathComponent(name string) string {
	return benchmarks.SafeComponent(name)
}

func stepDirName(name string, used map[string]struct{}) string {
	candidate := safePathComponent(name)
	if candidate == "" || candidate == "status" || candidate == "manifest.json" {
		candidate += "__step"
	}
	if candidate == "" || candidate == "__step" {
		candidate = "step"
	}
	base := candidate
	for i := 1; ; i++ {
		if _, exists := used[candidate]; !exists {
			break
		}
		candidate = fmt.Sprintf("%s__%d", base, i)
	}
	used[candidate] = struct{}{}
	return candidate
}

func rowDir(row int) string {
	return fmt.Sprintf("%06d", row)
}

func nextRunID(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "000000", nil
		}
		return "", err
	}
	maxID := -1
	for _, entry := range entries {
		if !entry.IsDir() || !numericRunDir.MatchString(entry.Name()) {
			continue
		}
		id, err := strconv.Atoi(entry.Name())
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return fmt.Sprintf("%06d", maxID+1), nil
}

func latestRunDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	maxID := -1
	maxName := ""
	for _, entry := range entries {
		if !entry.IsDir() || !numericRunDir.MatchString(entry.Name()) {
			continue
		}
		id, err := strconv.Atoi(entry.Name())
		if err == nil && id > maxID {
			maxID = id
			maxName = entry.Name()
		}
	}
	if maxName == "" {
		return "", fmt.Errorf("no run directories found in %s", root)
	}
	return filepath.Join(root, maxName), nil
}

func workKey(step string, row int) string {
	return step + "/" + rowDir(row)
}
