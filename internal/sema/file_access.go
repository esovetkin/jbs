package sema

import (
	"path/filepath"
	"strings"

	"jbs/internal/diag"
	"jbs/internal/eval"
)

func baseDirForProgramFile(file string) string {
	file = strings.TrimSpace(file)
	if file == "" {
		return ""
	}
	if strings.HasPrefix(file, "<") && strings.HasSuffix(file, ">") {
		return ""
	}
	return filepath.Dir(file)
}

func fileAccessForSpan(baseDirByFile map[string]string, span diag.Span) *eval.FileAccess {
	if len(baseDirByFile) == 0 {
		return nil
	}
	file := strings.TrimSpace(span.File)
	if file != "" {
		if baseDir := strings.TrimSpace(baseDirByFile[file]); baseDir != "" {
			return &eval.FileAccess{BaseDir: baseDir}
		}
		if baseDir := baseDirForProgramFile(file); baseDir != "" {
			return &eval.FileAccess{BaseDir: baseDir}
		}
	}
	if len(baseDirByFile) == 1 {
		for _, baseDir := range baseDirByFile {
			if strings.TrimSpace(baseDir) != "" {
				return &eval.FileAccess{BaseDir: baseDir}
			}
		}
	}
	return nil
}
