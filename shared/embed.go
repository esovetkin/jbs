package shared

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.jbs
var embedded embed.FS

func normalizeName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	if !strings.HasSuffix(n, ".jbs") {
		n += ".jbs"
	}
	return n
}

func List() ([]string, error) {
	entries, err := fs.ReadDir(embedded, ".")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".jbs") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func Has(name string) bool {
	n := normalizeName(name)
	if n == "" {
		return false
	}
	_, err := fs.Stat(embedded, n)
	return err == nil
}

func Read(name string) (string, error) {
	n := normalizeName(name)
	if n == "" {
		return "", fmt.Errorf("empty embedded file name")
	}
	data, err := embedded.ReadFile(n)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
