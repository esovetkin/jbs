package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"jbs/internal/diag"
)

const diagnosticsDocPath = "docs/diagnostics.md"

func main() {
	os.Exit(run(os.Args[1:], diagnosticsDocPath, os.Stderr))
}

func run(args []string, outputPath string, stderr io.Writer) int {
	fs := flag.NewFlagSet("gendiagdocs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	check := fs.Bool("check", false, "verify diagnostics docs are up to date")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, "usage: gendiagdocs [-check]")
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: gendiagdocs [-check]")
		return 2
	}

	rendered := renderCatalog()
	if *check {
		current, err := os.ReadFile(outputPath)
		if err != nil {
			fmt.Fprintf(stderr, "failed to read %s: %v\n", outputPath, err)
			return 1
		}
		if !bytes.Equal(current, rendered) {
			fmt.Fprintf(stderr, "%s is out of date; run `go run ./cmd/gendiagdocs`\n", outputPath)
			return 1
		}
		return 0
	}

	if err := os.WriteFile(outputPath, rendered, 0o644); err != nil {
		fmt.Fprintf(stderr, "failed to write %s: %v\n", outputPath, err)
		return 1
	}
	return 0
}

func renderCatalog() []byte {
	type groupKey struct {
		Severity diag.Severity
		Owner    string
		Summary  string
	}

	groups := map[groupKey][]diag.Code{}
	for code, meta := range diag.Catalog {
		key := groupKey{
			Severity: meta.Severity,
			Owner:    meta.Owner,
			Summary:  meta.Summary,
		}
		groups[key] = append(groups[key], code)
	}

	keys := slices.Collect(maps.Keys(groups))
	slices.SortFunc(keys, func(a, b groupKey) int {
		if c := strings.Compare(string(a.Severity), string(b.Severity)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Owner, b.Owner); c != 0 {
			return c
		}
		return strings.Compare(a.Summary, b.Summary)
	})

	var b bytes.Buffer
	b.WriteString("# Diagnostics Catalog\n\n")
	b.WriteString("> Generated from `internal/diag/codes.go`. Do not edit manually.\n\n")
	b.WriteString("| Severity | Owner | Summary | Codes |\n")
	b.WriteString("|----------|-------|---------|-------|\n")

	for _, key := range keys {
		codes := groups[key]
		slices.Sort(codes)
		codeNames := make([]string, 0, len(codes))
		for _, code := range codes {
			codeNames = append(codeNames, string(code))
		}
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %s |\n",
			escapeTableCell(string(key.Severity)),
			escapeTableCell(key.Owner),
			escapeTableCell(key.Summary),
			escapeTableCell(strings.Join(codeNames, ", ")),
		)
	}

	return b.Bytes()
}

func escapeTableCell(value string) string {
	escaped := strings.ReplaceAll(value, "|", `\|`)
	return strings.ReplaceAll(escaped, "\n", "<br>")
}
