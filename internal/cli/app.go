package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"syscall"

	helpdocs "gitlab.jsc.fz-juelich.de/sdlaml/jbs/docs"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/filewait"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/imports"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/printparam"
	jbsrepl "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/repl"
	jbsrun "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/run"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/valuefmt"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/version"
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
	if flags.Check {
		return checkInputSyntax(flags.Input, stdout, stderr)
	}
	if flags.Param {
		return runParam(flags, stdout, stderr)
	}
	if flags.FWait {
		return fwaitFiles(flags.FWaitPaths, flags.FWaitExitExisting, stdout, stderr)
	}
	if flags.Run {
		return runBenchmark(flags.Input, flags.NoStrict, flags.DryRun, flags.Weak, flags.Benchmark, stdout, stderr)
	}
	if flags.Continue {
		return continueBenchmark(flags.Input, flags.Benchmark, stdout, stderr)
	}
	if flags.Status {
		return statusBenchmark(flags.Input, flags.Benchmark, stdout, stderr)
	}
	if flags.Tree {
		return treeBenchmark(flags.Input, flags.Benchmark, stdout, stderr)
	}
	if flags.LsAnalyse {
		return listAnalyseBenchmark(flags.Input, flags.Benchmark, stdout, stderr)
	}
	if flags.Archive {
		return archiveBenchmark(flags.Input, stdout, stderr)
	}
	if flags.Input == "" {
		fmt.Fprintln(stderr, "missing input file")
		fmt.Fprintln(stderr, UsageText())
		return 2
	}
	return runBenchmark(flags.Input, flags.NoStrict, flags.DryRun, flags.Weak, flags.Benchmark, stdout, stderr)
}

func fwaitFiles(paths []string, exitExisting bool, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result, err := filewait.WaitAnyWithOptions(ctx, paths, filewait.Options{ExitIfExists: exitExisting})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, result.Path)
	return 0
}

func checkInputSyntax(path string, stdout, stderr io.Writer) int {
	_ = stdout
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load input %q: %v\n", path, err)
		return 1
	}
	source := string(data)
	diags := &diag.Diagnostics{}
	prog := parser.Parse(path, source, diags)
	sources := map[string]string{path: source}
	if len(diags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnosticsWithSources(*diags, sources, prog.File))
	}
	if diags.HasErrors() {
		return 1
	}
	return 0
}

func runBenchmark(path string, noStrict bool, dryRun bool, weak bool, benchmark string, stdout, stderr io.Writer) int {
	diags := &diag.Diagnostics{}
	bundle, err := analyzeInputWithOptions(path, sema.AnalyzeOptions{CollectPrints: true}, diags)
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
	opts := jbsrun.Options{
		Input:       path,
		Result:      bundle.Result,
		Sources:     bundle.Sources,
		ProgramFile: bundle.Program.File,
		Benchmark:   benchmark,
		NoStrict:    noStrict,
		Weak:        weak,
		PrintEvents: bundle.Result.PrintEvents,
		Stdout:      stdout,
		Stderr:      stderr,
	}
	var runErr error
	if dryRun {
		runErr = jbsrun.DryRun(context.Background(), opts)
	} else {
		runErr = jbsrun.Run(context.Background(), opts)
	}
	if runErr != nil {
		fmt.Fprintln(stderr, runErr)
		return 1
	}
	return 0
}

func continueBenchmark(path string, benchmark string, stdout, stderr io.Writer) int {
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
	if err := jbsrun.Continue(context.Background(), jbsrun.Options{
		Input:       path,
		Result:      bundle.Result,
		Sources:     bundle.Sources,
		ProgramFile: bundle.Program.File,
		Benchmark:   benchmark,
		Stdout:      stdout,
		Stderr:      stderr,
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func analyzeCommandInput(path string, stderr io.Writer) (*analysisBundle, bool) {
	diags := &diag.Diagnostics{}
	bundle, err := analyzeInput(path, diags)
	if err != nil {
		fmt.Fprintf(stderr, "failed to load input %q: %v\n", path, err)
		return nil, false
	}
	if len(diags.Items) > 0 {
		fmt.Fprintln(stderr, formatDiagnosticsWithSources(*diags, bundle.Sources, bundle.Program.File))
	}
	if diags.HasErrors() {
		return nil, false
	}
	return bundle, true
}

type benchmarkCommandInputKind int

const (
	benchmarkCommandInputSource benchmarkCommandInputKind = iota
	benchmarkCommandInputDirectory
)

func classifyBenchmarkCommandInput(path string) (benchmarkCommandInputKind, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return benchmarkCommandInputDirectory, nil
		}
		return benchmarkCommandInputSource, nil
	}
	if os.IsNotExist(err) {
		return benchmarkCommandInputSource, nil
	}
	return benchmarkCommandInputSource, fmt.Errorf("inspect input %q: %w", path, err)
}

func statusBenchmark(path string, benchmark string, stdout, stderr io.Writer) int {
	kind, err := classifyBenchmarkCommandInput(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if kind == benchmarkCommandInputDirectory {
		if err := jbsrun.ShowStatusForBenchmarkDir(context.Background(), jbsrun.BenchmarkDirOptions{
			Root:      path,
			Benchmark: benchmark,
			Stdout:    stdout,
			Stderr:    stderr,
		}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	bundle, ok := analyzeCommandInput(path, stderr)
	if !ok {
		return 1
	}
	if err := jbsrun.ShowStatus(context.Background(), jbsrun.Options{
		Input:       path,
		Result:      bundle.Result,
		Sources:     bundle.Sources,
		ProgramFile: bundle.Program.File,
		Benchmark:   benchmark,
		Stdout:      stdout,
		Stderr:      stderr,
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func treeBenchmark(path string, benchmark string, stdout, stderr io.Writer) int {
	bundle, ok := analyzeCommandInput(path, stderr)
	if !ok {
		return 1
	}
	if err := jbsrun.Tree(context.Background(), jbsrun.Options{
		Input:       path,
		Result:      bundle.Result,
		Sources:     bundle.Sources,
		ProgramFile: bundle.Program.File,
		Benchmark:   benchmark,
		Stdout:      stdout,
		Stderr:      stderr,
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func listAnalyseBenchmark(path string, benchmark string, stdout, stderr io.Writer) int {
	kind, err := classifyBenchmarkCommandInput(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if kind == benchmarkCommandInputDirectory {
		if err := jbsrun.LsAnalyseForBenchmarkDir(context.Background(), jbsrun.BenchmarkDirOptions{
			Root:      path,
			Benchmark: benchmark,
			Stdout:    stdout,
			Stderr:    stderr,
		}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	bundle, ok := analyzeCommandInput(path, stderr)
	if !ok {
		return 1
	}
	if err := jbsrun.LsAnalyse(context.Background(), jbsrun.Options{
		Input:       path,
		Result:      bundle.Result,
		Sources:     bundle.Sources,
		ProgramFile: bundle.Program.File,
		Benchmark:   benchmark,
		Stdout:      stdout,
		Stderr:      stderr,
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func archiveBenchmark(path string, stdout, stderr io.Writer) int {
	kind, err := classifyBenchmarkCommandInput(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if kind == benchmarkCommandInputDirectory {
		if err := jbsrun.ArchiveBenchmarkDir(context.Background(), jbsrun.BenchmarkDirOptions{
			Root:   path,
			Stdout: stdout,
			Stderr: stderr,
		}); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

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
	if err := jbsrun.Archive(context.Background(), jbsrun.Options{
		Input:       path,
		Result:      bundle.Result,
		ProgramFile: bundle.Program.File,
		Stdout:      stdout,
		Stderr:      stderr,
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runParam(flags Flags, stdout, stderr io.Writer) int {
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
		fmt.Fprintf(stderr, "failed to render param output: %v\n", err)
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

func runRepl(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "failed to determine working directory: %v\n", err)
		return 1
	}
	commit := func(source, chunk string) (jbsrepl.CommitResult, error) {
		return commitReplChunk(cwd, source, chunk)
	}
	return jbsrepl.Run(jbsrepl.Options{
		Stdout:                 stdout,
		Stderr:                 stderr,
		Cwd:                    cwd,
		BuildInfo:              version.Full(),
		Commit:                 commit,
		InitialCompletionNames: initialReplCompletionNames(),
	})
}

func initialReplCompletionNames() []string {
	defaults := sema.BuiltinGlobalValues()
	names := make([]string, 0, len(defaults))
	for name := range defaults {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func commitReplChunk(cwd, source, chunk string) (jbsrepl.CommitResult, error) {
	result := jbsrepl.CommitResult{
		Source:     source,
		ExprOutput: []string{},
	}
	candidate := appendReplChunk(source, chunk)
	diags := &diag.Diagnostics{}
	bundle, err := analyzeSourceWithOptions("<repl>", candidate, cwd, sema.AnalyzeOptions{CollectPrints: true}, diags)
	if err != nil {
		return result, err
	}
	errorDiags := filterDiagnosticsBySeverity(diags, diag.SeverityError)
	if len(errorDiags.Items) > 0 {
		result.DiagText = formatDiagnosticsWithSources(errorDiags, bundle.Sources, bundle.Program.File)
		result.HasErrors = true
		return result, nil
	}
	prevStmtCount := 0
	if strings.TrimSpace(source) != "" {
		prevDiags := &diag.Diagnostics{}
		prevBundle, err := analyzeSource("<repl>", source, cwd, prevDiags)
		if err != nil {
			return result, err
		}
		prevErrors := filterDiagnosticsBySeverity(prevDiags, diag.SeverityError)
		if len(prevErrors.Items) > 0 {
			result.DiagText = formatDiagnosticsWithSources(prevErrors, prevBundle.Sources, prevBundle.Program.File)
			result.HasErrors = true
			return result, nil
		}
		prevStmtCount = len(prevBundle.Program.Stmts)
	}
	events := make([]replOutputEvent, 0, len(bundle.Result.PrintEvents)+len(bundle.Result.TopLevelExprs))
	for _, event := range bundle.Result.PrintEvents {
		if event.Index < prevStmtCount {
			continue
		}
		events = append(events, replOutputEvent{
			Seq: event.Seq,
			Text: valuefmt.PrintLineWithOptions(event.Values, valuefmt.Options{
				NRow:  event.Options.NRow,
				Width: valuefmt.DefaultWidth,
			}),
		})
	}
	for _, expr := range bundle.Result.TopLevelExprs {
		if expr.Index < prevStmtCount || !expr.Echo {
			continue
		}
		events = append(events, replOutputEvent{
			Seq:  expr.Seq,
			Text: valuefmt.ReplValue(expr.Value),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Seq < events[j].Seq
	})
	for _, event := range events {
		result.ExprOutput = append(result.ExprOutput, event.Text)
	}
	result.Source = candidate
	result.CompletionNames = replCompletionNames(bundle.Result)
	return result, nil
}

func replCompletionNames(res *sema.Result) []string {
	if res == nil {
		return nil
	}
	names := make([]string, 0, len(res.Globals.Values))
	for name := range res.Globals.Values {
		if name == "" || strings.Contains(name, ".") {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

type replOutputEvent struct {
	Seq  int
	Text string
}

func appendReplChunk(accepted string, chunk string) string {
	if strings.TrimSpace(chunk) == "" {
		return accepted
	}
	if accepted == "" {
		return chunk
	}
	if strings.HasSuffix(accepted, "\n") {
		return accepted + chunk
	}
	return accepted + "\n" + chunk
}

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

func analyzeInput(path string, diags *diag.Diagnostics) (*analysisBundle, error) {
	return analyzeInputWithOptions(path, sema.AnalyzeOptions{}, diags)
}

func analyzeInputWithOptions(path string, opts sema.AnalyzeOptions, diags *diag.Diagnostics) (*analysisBundle, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("determine working directory: %w", err)
	}
	loadRes, err := imports.LoadAndExpand(path, cwd, diags)
	if err != nil {
		return nil, err
	}
	res := sema.AnalyzeWithImportsOptions(loadRes, sema.BuiltinGlobalValues(), opts, diags)
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
	return analyzeSourceWithOptions(file, source, cwd, sema.AnalyzeOptions{}, diags)
}

func analyzeSourceWithOptions(file string, source string, cwd string, opts sema.AnalyzeOptions, diags *diag.Diagnostics) (*analysisBundle, error) {
	loadRes, err := imports.LoadAndExpandSource(file, source, cwd, cwd, diags)
	if err != nil {
		return nil, err
	}
	res := sema.AnalyzeWithImportsOptions(loadRes, sema.BuiltinGlobalValues(), opts, diags)
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
