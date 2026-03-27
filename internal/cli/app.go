package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"jbs/internal/diag"
	"jbs/internal/emit"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/sema"
)

func Run(args []string, stdout, stderr io.Writer) int {
	flags, err := ParseFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, UsageText())
		return 2
	}
	if flags.Help {
		fmt.Fprintln(stdout, UsageText())
		return 0
	}
	if flags.Input == "" {
		printGlobals(stdout)
		return 0
	}

	src, err := os.ReadFile(flags.Input)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read input file %q: %v\n", flags.Input, err)
		return 1
	}

	diags := &diag.Diagnostics{}
	prog := parser.Parse(flags.Input, string(src), diags)
	res := sema.Analyze(prog, lower.BuiltinGlobalValues(), diags)
	doc := lower.ToJUBEYAML(res, lower.Options{InputPath: flags.Input}, diags)

	if len(diags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnostics(*diags, string(src)))
	}
	if diags.HasErrors() {
		return 1
	}
	if flags.Check {
		return 0
	}

	outBytes, err := emit.YAML(doc)
	if err != nil {
		fmt.Fprintf(stderr, "failed to encode YAML: %v\n", err)
		return 1
	}

	if flags.Output == "-" {
		_, err = stdout.Write(outBytes)
	} else {
		err = os.WriteFile(flags.Output, outBytes, 0o644)
	}
	if err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return 1
	}
	return 0
}

func formatDiagnostics(diags diag.Diagnostics, source string) string {
	if len(diags.Items) == 0 {
		return ""
	}
	lines := strings.Split(source, "\n")
	var b strings.Builder
	for _, item := range diags.Items {
		fmt.Fprintf(&b, "%s %s %s\n", strings.ToUpper(string(item.Severity)), item.Code, item.Span.String())
		b.WriteString(item.Message)
		b.WriteByte('\n')
		if excerpt := sourceExcerpt(lines, item.Span); excerpt != "" {
			b.WriteString(excerpt)
			b.WriteByte('\n')
		}
		if item.Hint != "" {
			b.WriteString("Hint: ")
			b.WriteString(item.Hint)
			b.WriteByte('\n')
		}
		for _, rel := range item.Related {
			fmt.Fprintf(&b, "Related: %s (%s)\n", rel.Message, rel.Span.String())
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func sourceExcerpt(lines []string, span diag.Span) string {
	if span.IsZero() || span.Start.Line <= 0 || span.Start.Line > len(lines) {
		return ""
	}
	startLine := span.Start.Line
	endLine := span.End.Line
	if endLine < startLine {
		endLine = startLine
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if endLine-startLine > 2 {
		endLine = startLine + 2
	}

	var b strings.Builder
	for lineNo := startLine; lineNo <= endLine; lineNo++ {
		fmt.Fprintf(&b, "  %4d | %s\n", lineNo, lines[lineNo-1])
		if lineNo == startLine {
			col := span.Start.Column
			if col < 1 {
				col = 1
			}
			endCol := col + 1
			if span.End.Line == startLine && span.End.Column > col {
				endCol = span.End.Column
			}
			width := endCol - col
			if width < 1 {
				width = 1
			}
			fmt.Fprintf(&b, "       | %s%s\n", strings.Repeat(" ", col-1), strings.Repeat("^", width))
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func printGlobals(out io.Writer) {
	fmt.Fprintln(out, "Built-in jbs_* variables")
	for _, spec := range lower.BuiltinGlobals() {
		target := "-"
		if spec.Target != "" {
			target = spec.Target
		}
		mode := "-"
		if spec.Mode != "" {
			mode = spec.Mode
		}
		fmt.Fprintf(out, "- %s\n", spec.Name)
		fmt.Fprintf(out, "  default: %s\n", spec.DefaultExpr)
		fmt.Fprintf(out, "  mode: %s\n", mode)
		fmt.Fprintf(out, "  maps_to: %s\n", target)
		if spec.Description != "" {
			fmt.Fprintf(out, "  note: %s\n", spec.Description)
		}
	}
}
