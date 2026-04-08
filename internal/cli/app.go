package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	helpdocs "jbs/docs"
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/emit"
	jbsformat "jbs/internal/format"
	"jbs/internal/imports"
	"jbs/internal/lower"
	"jbs/internal/printparam"
	"jbs/internal/sema"
	"jbs/shared"
)

type analysisBundle struct {
	Program ast.Program
	Sources map[string]string
	Result  *sema.Result
}

func Run(args []string, stdout, stderr io.Writer) int {
	flags, err := ParseFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, UsageText())
		return 2
	}
	if flags.Help {
		topic := helpTopic(flags)
		if topic == "" {
			fmt.Fprintln(stdout, UsageText())
			return 0
		}
		if err := printHelpTopic(stdout, topic); err != nil {
			fmt.Fprintf(stderr, "failed to print help for %q: %v\n", topic, err)
			return 1
		}
		return 0
	}
	if flags.Fmt {
		return runFmt(flags.Input, flags.FmtStrict, stdout, stderr)
	}
	if flags.Embed {
		return runEmbed(flags.EmbedName, stdout, stderr)
	}
	if flags.PrintParam {
		return runPrintParam(flags, stdout, stderr)
	}
	if flags.Input == "" {
		fmt.Fprintln(stderr, "missing input file")
		fmt.Fprintln(stderr, UsageText())
		return 2
	}

	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(flags.Input, diags)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load input %q: %v\n", flags.Input, err)
		return 1
	}
	doc := lower.ToJUBEYAML(bundle.Result, diags)

	if len(diags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnosticsWithSources(*diags, bundle.Sources, bundle.Program.File))
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

func runPrintParam(flags Flags, stdout, stderr io.Writer) int {
	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(flags.Input, diags)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load input %q: %v\n", flags.Input, err)
		return 1
	}
	if diags.HasErrors() {
		fmt.Fprintln(stderr, formatDiagnosticsWithSources(*diags, bundle.Sources, bundle.Program.File))
		return 1
	}

	table := printparam.Build(bundle.Result, diags)
	if len(diags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnosticsWithSources(*diags, bundle.Sources, bundle.Program.File))
	}
	if diags.HasErrors() {
		return 1
	}

	out, err := printparam.Render(table, printparam.RenderType(flags.PrintType))
	if err != nil {
		fmt.Fprintf(stderr, "failed to render printparam output: %v\n", err)
		return 1
	}

	if flags.Output == "-" {
		_, err = io.WriteString(stdout, out)
	} else {
		err = os.WriteFile(flags.Output, []byte(out), 0o644)
	}
	if err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return 1
	}
	return 0
}

func runFmt(path string, strict bool, stdout, stderr io.Writer) int {
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

	if strict {
		diags := &diag.Diagnostics{}
		bundle, err := analyzeInput(path, diags)
		if err != nil {
			fmt.Fprintf(stderr, "failed to load input %q: %v\n", path, err)
			return 1
		}
		if len(diags.Items) > 0 {
			fmt.Fprintln(stderr, formatDiagnosticsWithSources(*diags, bundle.Sources, bundle.Program.File))
		}
		if diags.HasErrors() {
			return 1
		}
	}

	formatDiags := &diag.Diagnostics{}
	formatted, err := jbsformat.JBS(path, string(src), formatDiags)
	if err != nil {
		fmt.Fprintf(stderr, "failed to format %q: %v\n", path, err)
		return 1
	}
	if len(formatDiags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnostics(*formatDiags, string(src)))
	}
	if formatDiags.HasErrors() {
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

func analyzeInput(path string, diags *diag.Diagnostics) (*analysisBundle, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("determine working directory: %w", err)
	}
	loadRes, err := imports.LoadAndExpand(path, cwd, diags)
	if err != nil {
		return nil, err
	}
	res := sema.Analyze(loadRes.Program, lower.BuiltinGlobalValues(), diags)
	return &analysisBundle{
		Program: loadRes.Program,
		Sources: loadRes.Sources,
		Result:  res,
	}, nil
}

func runEmbed(name string, stdout, stderr io.Writer) int {
	if strings.TrimSpace(name) == "" {
		files, err := shared.List()
		if err != nil {
			fmt.Fprintf(stderr, "failed to list embedded files: %v\n", err)
			return 1
		}
		for _, file := range files {
			fmt.Fprintln(stdout, file)
		}
		return 0
	}
	text, err := shared.Read(name)
	if err != nil {
		fmt.Fprintf(stderr, "unknown embedded file %q\n", name)
		return 1
	}
	_, err = io.WriteString(stdout, text)
	if err != nil {
		fmt.Fprintf(stderr, "failed to write embedded content: %v\n", err)
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
	defaultFile := "<input>"
	if len(diags.Items) > 0 && diags.Items[0].Span.File != "" {
		defaultFile = diags.Items[0].Span.File
	}
	sourceByFile := map[string]string{defaultFile: source}
	return formatDiagnosticsWithSources(diags, sourceByFile, defaultFile)
}

func formatDiagnosticsWithSources(diags diag.Diagnostics, sources map[string]string, defaultFile string) string {
	if len(diags.Items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, item := range diags.Items {
		fmt.Fprintf(&b, "%s %s %s\n", strings.ToUpper(string(item.Severity)), item.Code, item.Span.String())
		b.WriteString(item.Message)
		b.WriteByte('\n')
		file := item.Span.File
		if file == "" {
			file = defaultFile
		}
		source := sources[file]
		if source == "" && defaultFile != "" {
			source = sources[defaultFile]
		}
		lines := strings.Split(source, "\n")
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

func helpTopic(flags Flags) string {
	switch {
	case flags.HelpGlobals:
		return "globals"
	case flags.HelpAnalyse:
		return "analyse"
	case flags.HelpDo:
		return "do"
	case flags.HelpLet:
		return "let"
	case flags.HelpParam:
		return "param"
	case flags.HelpSubmit:
		return "submit"
	case flags.HelpUse:
		return "use"
	default:
		return ""
	}
}

func printHelpTopic(out io.Writer, topic string) error {
	page, err := helpdocs.Page(topic)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, page)
	return err
}
