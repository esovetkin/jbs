package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	helpdocs "jbs/docs"
	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/emit"
	"jbs/internal/eval"
	jbsformat "jbs/internal/format"
	"jbs/internal/imports"
	"jbs/internal/lower"
	"jbs/internal/parser"
	"jbs/internal/printparam"
	jbsrepl "jbs/internal/repl"
	"jbs/internal/sema"
	"jbs/shared"
)

// analysisBundle contains the entire parsed AST, corresponding
// sources (plural due to imports), and compiled program
type analysisBundle struct {
	Program ast.Program
	Sources map[string]string
	Result  *sema.Result
}

var runReplFn = runRepl

func Run(args []string, stdout, stderr io.Writer) int {
	flags, err := ParseFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, UsageText())
		return 2
	}
	if flags.Repl {
		return runReplFn(stdout, stderr)
	}
	if flags.Help {
		if flags.HelpTopic == "" {
			fmt.Fprintln(stdout, UsageText())
			return 0
		}
		if err := printHelpTopic(stdout, flags.HelpTopic); err != nil {
			fmt.Fprintf(stderr, "failed to print help for %q: %v\n", flags.HelpTopic, err)
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

func runRepl(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "failed to determine working directory: %v\n", err)
		return 1
	}
	check := func(source string) (string, bool, error) {
		diags := &diag.Diagnostics{}
		bundle, err := analyzeSource("<repl>", source, cwd, diags)
		if err != nil {
			return "", true, err
		}
		diagText := ""
		errorDiags := filterDiagnosticsBySeverity(diags, diag.SeverityError)
		if len(errorDiags.Items) > 0 {
			diagText = formatDiagnosticsWithSources(errorDiags, bundle.Sources, bundle.Program.File)
		}
		return diagText, diags.HasErrors(), nil
	}
	yaml := func(source string) (string, string, bool, error) {
		diags := &diag.Diagnostics{}
		bundle, err := analyzeSource("<repl>", source, cwd, diags)
		if err != nil {
			return "", "", true, err
		}
		doc := lower.ToJUBEYAML(bundle.Result, diags)
		diagText := ""
		errorDiags := filterDiagnosticsBySeverity(diags, diag.SeverityError)
		if len(errorDiags.Items) > 0 {
			diagText = formatDiagnosticsWithSources(errorDiags, bundle.Sources, bundle.Program.File)
		}
		if diags.HasErrors() {
			return "", diagText, true, nil
		}
		out, err := emit.YAML(doc)
		if err != nil {
			return "", diagText, true, err
		}
		return string(out), diagText, false, nil
	}
	inspect := func(source, name string) (string, bool, error) {
		diags := &diag.Diagnostics{}
		bundle, err := analyzeSource("<repl>", source, cwd, diags)
		if err != nil {
			return "", false, err
		}
		if gv := bundle.Result.GlobalVarByName[name]; gv != nil {
			return formatReplValue(gv.Value), true, nil
		}
		return "", false, nil
	}
	evalExpr := func(source, exprText string) (string, string, bool, bool, error) {
		return evaluateReplExpression(cwd, source, exprText)
	}
	return jbsrepl.Run(jbsrepl.Options{
		Stdout:   stdout,
		Stderr:   stderr,
		Cwd:      cwd,
		Check:    check,
		YAML:     yaml,
		Inspect:  inspect,
		EvalExpr: evalExpr,
	})
}

func evaluateReplExpression(cwd, source, exprText string) (string, string, bool, bool, error) {
	diags := &diag.Diagnostics{}
	bundle, err := analyzeSource("<repl>", source, cwd, diags)
	if err != nil {
		return "", "", true, true, err
	}
	sourceErrs := filterDiagnosticsBySeverity(diags, diag.SeverityError)
	if len(sourceErrs.Items) > 0 {
		return "", formatDiagnosticsWithSources(sourceErrs, bundle.Sources, bundle.Program.File), true, true, nil
	}

	exprDiags := &diag.Diagnostics{}
	expr, handled := parser.ParseStandaloneExpr("<repl-expr>", exprText, diag.NewPos(0, 1, 1), exprDiags)
	if !handled {
		return "", "", false, false, nil
	}
	exprErrs := filterDiagnosticsBySeverity(exprDiags, diag.SeverityError)
	if len(exprErrs.Items) > 0 {
		return "", formatDiagnostics(exprErrs, exprText), true, true, nil
	}

	env := lower.BuiltinGlobalValues()
	for name, gv := range bundle.Result.GlobalVarByName {
		if gv == nil {
			continue
		}
		env[name] = gv.Value
	}
	evalDiags := &diag.Diagnostics{}
	value := eval.EvalExprWithOptions(expr, env, evalDiags, eval.ExprOptions{
		GlobalAssignmentTupleArithmetic: true,
		Context:                         eval.EvalCtxBindingAssign,
	})
	evalErrs := filterDiagnosticsBySeverity(evalDiags, diag.SeverityError)
	if len(evalErrs.Items) > 0 {
		return "", formatDiagnostics(evalErrs, exprText), true, true, nil
	}
	return formatReplValue(value), "", true, false, nil
}

const replMaxPreviewItems = 3

func filterDiagnosticsBySeverity(diags *diag.Diagnostics, severity diag.Severity) diag.Diagnostics {
	out := diag.Diagnostics{
		Items: make([]diag.Diagnostic, 0, len(diags.Items)),
	}
	for _, item := range diags.Items {
		if item.Severity == severity {
			out.Items = append(out.Items, item)
		}
	}
	return out
}

func formatReplValue(v eval.Value) string {
	switch v.Kind {
	case eval.KindList:
		return formatReplSequence("[", "]", v.L)
	case eval.KindTuple:
		return formatReplSequence("(", ")", v.L)
	case eval.KindComb:
		return formatReplComb(v.C)
	default:
		return v.String()
	}
}

func formatReplSequence(open, close string, items []eval.Value) string {
	limit := len(items)
	if limit > replMaxPreviewItems {
		limit = replMaxPreviewItems
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, formatReplInlineValue(items[i]))
	}
	if len(items) > limit {
		parts = append(parts, "...")
	}
	return open + strings.Join(parts, ", ") + close
}

func formatReplInlineValue(v eval.Value) string {
	switch v.Kind {
	case eval.KindList:
		return formatReplSequence("[", "]", v.L)
	case eval.KindTuple:
		return formatReplSequence("(", ")", v.L)
	case eval.KindComb:
		return formatReplComb(v.C)
	case eval.KindString:
		return strconv.Quote(v.S)
	default:
		return v.String()
	}
}

func formatReplComb(c *eval.Comb) string {
	if c == nil {
		return "comb(rows=0, cols=[], head=[])"
	}
	cols := slices.Clone(c.Order)
	if len(cols) == 0 {
		colSet := make(map[string]struct{})
		for _, row := range c.Rows {
			for name := range row.Values {
				colSet[name] = struct{}{}
			}
		}
		cols = make([]string, 0, len(colSet))
		for name := range colSet {
			cols = append(cols, name)
		}
		slices.Sort(cols)
	}

	headLimit := len(c.Rows)
	if headLimit > replMaxPreviewItems {
		headLimit = replMaxPreviewItems
	}
	headRows := make([]string, 0, headLimit+1)
	for i := 0; i < headLimit; i++ {
		row := c.Rows[i]
		cells := make([]string, 0, len(cols))
		for _, col := range cols {
			cell, ok := row.Values[col]
			if !ok {
				continue
			}
			cells = append(cells, col+":"+formatReplInlineValue(cell.Value))
		}
		headRows = append(headRows, "{"+strings.Join(cells, ", ")+"}")
	}
	if len(c.Rows) > headLimit {
		headRows = append(headRows, "...")
	}
	return "comb(rows=" + strconv.Itoa(len(c.Rows)) +
		", cols=[" + strings.Join(cols, ", ") +
		"], head=[" + strings.Join(headRows, ", ") + "])"
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
	res := sema.AnalyzeWithImports(loadRes, lower.BuiltinGlobalValues(), diags)
	prog := ast.Program{}
	if info := loadRes.Modules[loadRes.Entry.ID]; info != nil {
		prog = info.Program
	}
	return &analysisBundle{
		Program: prog,
		Sources: loadRes.Sources,
		Result:  res,
	}, nil
}

func analyzeSource(file string, source string, cwd string, diags *diag.Diagnostics) (*analysisBundle, error) {
	loadRes, err := imports.LoadAndExpandSource(file, source, cwd, cwd, diags)
	if err != nil {
		return nil, err
	}
	res := sema.AnalyzeWithImports(loadRes, lower.BuiltinGlobalValues(), diags)
	prog := ast.Program{}
	if info := loadRes.Modules[loadRes.Entry.ID]; info != nil {
		prog = info.Program
	}
	return &analysisBundle{
		Program: prog,
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

func printHelpTopic(out io.Writer, topic string) error {
	page, err := helpdocs.Page(topic)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, page)
	return err
}
