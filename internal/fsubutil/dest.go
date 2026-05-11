package fsubutil

import (
	"path/filepath"
	"strings"
)

func DestName(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	dest := filepath.Base(filepath.Clean(path))
	if dest == "" || dest == "." || dest == ".." {
		return ""
	}
	return dest
}
