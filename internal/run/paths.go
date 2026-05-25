package run

import (
	"cmp"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/benchmarks"
)

var numericRunDir = regexp.MustCompile(`^\d+$`)

const minRunIDWidth = 6

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

func isRunDirName(name string) bool {
	return numericRunDir.MatchString(name)
}

func normalizeRunID(name string) string {
	name = strings.TrimLeft(name, "0")
	if name == "" {
		return "0"
	}
	return name
}

func compareRunIDNames(a, b string) int {
	an := normalizeRunID(a)
	bn := normalizeRunID(b)
	if len(an) != len(bn) {
		return cmp.Compare(len(an), len(bn))
	}
	if an != bn {
		return strings.Compare(an, bn)
	}
	return strings.Compare(a, b)
}

func nextRunIDAfter(name string) (string, error) {
	n, ok := new(big.Int).SetString(name, 10)
	if !ok {
		return "", fmt.Errorf("invalid run id %q", name)
	}
	n.Add(n, big.NewInt(1))
	out := n.String()
	if len(out) < minRunIDWidth {
		out = strings.Repeat("0", minRunIDWidth-len(out)) + out
	}
	return out, nil
}

func nextRunID(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "000000", nil
		}
		return "", err
	}
	maxName := ""
	for _, entry := range entries {
		if !entry.IsDir() || !isRunDirName(entry.Name()) {
			continue
		}
		if maxName == "" || compareRunIDNames(entry.Name(), maxName) > 0 {
			maxName = entry.Name()
		}
	}
	if maxName == "" {
		return "000000", nil
	}
	return nextRunIDAfter(maxName)
}

func latestRunDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	maxName := ""
	for _, entry := range entries {
		if !entry.IsDir() || !isRunDirName(entry.Name()) {
			continue
		}
		if maxName == "" || compareRunIDNames(entry.Name(), maxName) > 0 {
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
