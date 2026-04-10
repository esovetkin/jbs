package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"jbs/internal/diag"
)

func TestRenderCatalogIncludesAllCodesGroupedWithoutDuplication(t *testing.T) {
	content := string(renderCatalog())
	lines := strings.Split(content, "\n")

	var gotCodes []string
	metaSeen := map[string]bool{}
	for _, line := range lines {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Severity |") || strings.HasPrefix(line, "|----------|") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			t.Fatalf("malformed diagnostics row: %q", line)
		}
		metaKey := fmt.Sprintf("%s|%s|%s", strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3]))
		if metaSeen[metaKey] {
			t.Fatalf("duplicate grouped row for metadata %q", metaKey)
		}
		metaSeen[metaKey] = true

		codesCell := strings.TrimSpace(parts[4])
		codes := strings.Split(codesCell, ", ")
		sorted := append([]string(nil), codes...)
		slices.Sort(sorted)
		if !slices.Equal(codes, sorted) {
			t.Fatalf("codes in grouped row must be sorted: got=%v", codes)
		}
		gotCodes = append(gotCodes, codes...)
	}

	wantCodes := make([]string, 0, len(diag.Catalog))
	for _, code := range slices.Sorted(maps.Keys(diag.Catalog)) {
		wantCodes = append(wantCodes, string(code))
	}

	counts := map[string]int{}
	for _, code := range gotCodes {
		counts[code]++
	}
	if len(gotCodes) != len(wantCodes) {
		t.Fatalf("unexpected total number of codes: want=%d got=%d", len(wantCodes), len(gotCodes))
	}
	for _, code := range wantCodes {
		if counts[code] != 1 {
			t.Fatalf("expected code %s exactly once, got count=%d", code, counts[code])
		}
	}
}

func TestRunCheckModePassesAndFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.md")
	content := renderCatalog()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write diagnostics file: %v", err)
	}

	var stderr bytes.Buffer
	if code := run([]string{"-check"}, path, &stderr); code != 0 {
		t.Fatalf("expected check pass, got code=%d stderr=%q", code, stderr.String())
	}

	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale diagnostics file: %v", err)
	}
	stderr.Reset()
	if code := run([]string{"-check"}, path, &stderr); code != 1 {
		t.Fatalf("expected check failure, got code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "out of date") {
		t.Fatalf("expected out-of-date message, got %q", stderr.String())
	}
}

func TestRunWritesDiagnosticsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.md")
	var stderr bytes.Buffer
	if code := run(nil, path, &stderr); code != 0 {
		t.Fatalf("expected write success, got code=%d stderr=%q", code, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated diagnostics file: %v", err)
	}
	if !bytes.Equal(got, renderCatalog()) {
		t.Fatalf("generated file content mismatch")
	}
}

func TestRenderCatalogHeaderSnapshot(t *testing.T) {
	content := string(renderCatalog())
	lines := strings.Split(content, "\n")
	wantPrefix := []string{
		"# Diagnostics Catalog",
		"",
		"> Generated from `internal/diag/codes.go`. Do not edit manually.",
		"",
		"| Severity | Owner | Summary | Codes |",
		"|----------|-------|---------|-------|",
	}
	if len(lines) < len(wantPrefix) {
		t.Fatalf("rendered output is shorter than expected: %d", len(lines))
	}
	for i := range wantPrefix {
		if lines[i] != wantPrefix[i] {
			t.Fatalf("header line %d mismatch: want %q got %q", i+1, wantPrefix[i], lines[i])
		}
	}
}
