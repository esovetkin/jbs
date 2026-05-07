package run

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var numericRunDir = regexp.MustCompile(`^\d{6}$`)

func safePathComponent(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return ""
	}
	return out
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
