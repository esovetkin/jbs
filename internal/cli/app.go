package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"jbs/internal/diag"
	"jbs/internal/emit"
	jbsformat "jbs/internal/format"
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
		if flags.HelpGlobals {
			printGlobalsHelp(stdout)
		} else if flags.HelpTemplate {
			printTemplateHelp(stdout)
		} else {
			fmt.Fprintln(stdout, UsageText())
		}
		return 0
	}
	if flags.Fmt {
		return runFmt(flags.Input, stdout, stderr)
	}
	if flags.Input == "" {
		fmt.Fprintln(stderr, "missing input file")
		fmt.Fprintln(stderr, UsageText())
		return 2
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

func runFmt(path string, stdout, stderr io.Writer) int {
	_ = stdout

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "failed to read input file %q: %v\n", path, err)
		return 1
	}
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(stderr, "failed to stat input file %q: %v\n", path, err)
		return 1
	}

	diags := &diag.Diagnostics{}
	formatted, err := jbsformat.JBS(path, string(src), diags)
	if err != nil {
		fmt.Fprintf(stderr, "failed to format %q: %v\n", path, err)
		return 1
	}
	if len(diags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnostics(*diags, string(src)))
	}
	if diags.HasErrors() {
		return 1
	}
	if formatted == string(src) {
		return 0
	}
	if err := writeFileAtomic(path, []byte(formatted), info.Mode().Perm()); err != nil {
		fmt.Fprintf(stderr, "failed to write formatted file %q: %v\n", path, err)
		return 1
	}
	return 0
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".jbsfmt-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
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

func printGlobalsHelp(out io.Writer) {
	fmt.Fprintln(out, "# JBS global defaults")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "# Top-level assignments only (outside param/do/submit blocks).")
	fmt.Fprintln(out, "# Unknown globals are compile errors.")
	fmt.Fprintln(out, "# jbs_name and jbs_outpath must be plain string literals.")
	fmt.Fprintln(out, "# Other globals accept scalar values or shell(\"...\") / python(\"...\").")
	fmt.Fprintln(out, "")
	for _, spec := range lower.BuiltinGlobals() {
		target := "root"
		if spec.Target != "" {
			target = "submit:" + spec.Target
		} else if spec.Name == "jbs_outpath" {
			target = "root:outpath"
		} else if spec.Name == "jbs_name" {
			target = "root:name"
		}
		mode := "-"
		if spec.Mode != "" {
			mode = spec.Mode
		}
		note := spec.Description
		if note == "" {
			note = "No description."
		}
		fmt.Fprintf(out, "# %s maps_to: %s. mode: %s\n", note, target, mode)
		fmt.Fprintln(out, globalAssignmentLine(spec))
		fmt.Fprintln(out, "")
	}
}

func printTemplateHelp(out io.Writer) {
	_, _ = io.WriteString(out, `# `+"set global variables, see `jbs help globals`"+`
jbs_name="jbs_benchmark"
jbs_outpath="out"

# e.g. 'param test_cases'
param <name>
{
    # Define a parameter set.
    # Use "+" for direct sums, and "*" for outer products.
    #
    # For example,
    # a = (1, 2, 3)
    # b = ("one", "two")
    # a + b # yields [(1,"one"),(2,"two"),(3,"one")] (and a warning as length don't match)
    #
    # a * b # yields [(1,"one"),(1,"two"),(2,"one"), ...]
    #
    # The last line "returns" the parameter set, which means that ${a}
    # and ${b} variables can be used with whatever this parameter set is used
}

# e.g. 'do setup_environment with b from my'
do <name> after <step_name> with <parameter_set>, <variable> from <parameter_set>
{
    # bash code executed on the login node within ${jube_benchmark_home}
    #
    # For example,
    # echo ${b}
}

# submit my_submitjob after setup_environment with test_cases
submit <name> after <name0> with <paramset> {
    account = "atmlaml"
    preprocess = {
        # optional
    }
    executable = "/bin/bash"
    args_exec = "-lc hostname"
}
`)
}

func globalAssignmentLine(spec lower.GlobalSpec) string {
	expr := spec.DefaultExpr
	switch spec.Mode {
	case "python":
		return fmt.Sprintf("%s = python(%s)", spec.Name, strconv.Quote(expr))
	case "shell":
		return fmt.Sprintf("%s = shell(%s)", spec.Name, strconv.Quote(expr))
	}

	switch spec.Name {
	case "jbs_tasks":
		if strings.HasPrefix(expr, "$") && len(expr) > 1 {
			return fmt.Sprintf("%s = %s", spec.Name, expr[1:])
		}
		return fmt.Sprintf("%s = %s", spec.Name, expr)
	case "jbs_threadspertask", "jbs_nnodes":
		return fmt.Sprintf("%s = %s", spec.Name, expr)
	default:
		return fmt.Sprintf("%s = %s", spec.Name, strconv.Quote(expr))
	}
}
