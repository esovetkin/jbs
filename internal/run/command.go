package run

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/valuefmt"
)

func Run(ctx context.Context, opts Options) error {
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	prepared, err := createPreparedStores(suite.Plans)
	if err != nil {
		return err
	}
	printFileSubstitutionWarnings(opts.Stderr, prepared)
	printEvents(opts.Stdout, opts.PrintEvents)
	ctx, stop := withSignals(ctx, nil)
	defer stop()
	return runPreparedStores(ctx, opts, prepared, false)
}

func DryRun(ctx context.Context, opts Options) error {
	_ = ctx
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	prepared, err := createPreparedStores(suite.Plans)
	if err != nil {
		return err
	}
	printFileSubstitutionWarnings(opts.Stderr, prepared)
	printEvents(opts.Stdout, opts.PrintEvents)
	return nil
}

func printEvents(w io.Writer, events []sema.PrintEvent) {
	if w == nil || len(events) == 0 {
		return
	}
	ordered := append([]sema.PrintEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Seq < ordered[j].Seq
	})
	for _, event := range ordered {
		fmt.Fprintln(w, valuefmt.PrintLine(event.Values))
	}
}

func Continue(ctx context.Context, opts Options) error {
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	prepared, unlock, err := openContinuableStores(suite)
	if err != nil {
		return err
	}
	unlockOnce := sync.OnceFunc(unlock)
	defer unlockOnce()
	ctx, stop := withSignals(ctx, unlockOnce)
	defer stop()
	return runPreparedStores(ctx, opts, prepared, true)
}

type preparedStore struct {
	Plan     runtimePlan
	Store    *Store
	Warnings []FileSubstitutionWarning
}

type componentResult struct {
	Label string
	Store *Store
	Final Status
	Err   error
}

func createPreparedStores(plans []runtimePlan) ([]preparedStore, error) {
	prepared := make([]preparedStore, 0, len(plans))
	for _, plan := range plans {
		store, warnings, err := CreateRunDirectoryWithInitial(plan.RootDir, plan, StatusNotStarted)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, preparedStore{Plan: plan, Store: store, Warnings: warnings})
	}
	return prepared, nil
}

func printFileSubstitutionWarnings(w io.Writer, prepared []preparedStore) {
	if w == nil {
		return
	}
	for _, item := range prepared {
		for _, warning := range item.Warnings {
			fmt.Fprintf(w, "warning: fsub step %s row %s file %s pattern %q matched %d times; replaced all matches\n", warning.Step, rowDir(warning.Row), warning.DestName, warning.Pattern, warning.Matches)
		}
	}
}

func openContinuableStores(suite runtimeSuitePlan) ([]preparedStore, func(), error) {
	prepared := make([]preparedStore, 0, len(suite.Plans))
	unlocks := make([]func(), 0, len(suite.Plans))
	unlockAll := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, plan := range suite.Plans {
		unlock, err := acquireExistingRootLock(plan.RootDir)
		if err != nil {
			unlockAll()
			return nil, func() {}, err
		}
		unlocks = append(unlocks, unlock)
		runDir, err := latestRunDir(plan.RootDir)
		if err != nil {
			unlockAll()
			if suite.Configured && suite.SelectedName == "" {
				return nil, func() {}, fmt.Errorf("cannot continue benchmark %q: %w; use --benchmark to continue one component", plan.ComponentName, err)
			}
			return nil, func() {}, err
		}
		rootStatus, err := LoadRootStatus(filepath.Join(runDir, "status"))
		if err != nil {
			unlockAll()
			return nil, func() {}, fmt.Errorf("cannot continue incomplete run %s: %w", runDir, err)
		}
		if rootStatus.Status == StatusRunning {
			unlockAll()
			return nil, func() {}, fmt.Errorf("cannot continue %s: benchmark status is RUNNING", runDir)
		}
		manifest, err := LoadManifest(filepath.Join(runDir, "manifest.json"))
		if err != nil {
			unlockAll()
			return nil, func() {}, err
		}
		if err := validateRunManifest(manifest); err != nil {
			unlockAll()
			return nil, func() {}, fmt.Errorf("cannot continue %s: %w", runDir, err)
		}
		if err := validateTemplateHashes(runDir, manifest.TemplateHashes, plan.Manifest.TemplateHashes); err != nil {
			unlockAll()
			return nil, func() {}, err
		}
		if rootStatus.SourceHash != plan.Manifest.SourceHash {
			unlockAll()
			return nil, func() {}, sourceHashMismatchError(runDir, rootStatus.SourceHash, plan.Manifest.SourceHash, "root status")
		}
		if manifest.SourceHash != plan.Manifest.SourceHash {
			unlockAll()
			return nil, func() {}, sourceHashMismatchError(runDir, manifest.SourceHash, plan.Manifest.SourceHash, "manifest")
		}
		prepared = append(prepared, preparedStore{Plan: plan, Store: NewStore(runDir, manifest, plan.Bodies)})
	}
	return prepared, unlockAll, nil
}

func runPreparedStores(ctx context.Context, opts Options, prepared []preparedStore, continuing bool) error {
	results := make([]componentResult, 0, len(prepared))
	for i, item := range prepared {
		if ctx.Err() != nil {
			results = append(results, componentResult{Label: item.Plan.ComponentName, Store: item.Store, Final: StatusInterrupted, Err: ctx.Err()})
			break
		}
		if len(prepared) > 1 && opts.Stdout != nil {
			if i > 0 {
				fmt.Fprintln(opts.Stdout)
			}
			fmt.Fprintf(opts.Stdout, "[%s]\n", item.Plan.ComponentName)
		}
		result := runOneStore(ctx, item, continuing, opts.Stdout)
		results = append(results, result)
		if result.Final == StatusInterrupted {
			break
		}
	}
	printPostRunSummaries(opts.Stdout, opts.Stderr, results)
	return aggregateComponentResults(results)
}

func runOneStore(ctx context.Context, item preparedStore, continuing bool, progressWriter io.Writer) componentResult {
	label := item.Plan.ComponentName
	store := item.Store
	if continuing {
		if err := store.NormalizeStaleRunning(); err != nil {
			return componentResult{Label: label, Store: store, Final: StatusError, Err: err}
		}
	}
	if err := store.MarkRootRunning(); err != nil {
		return componentResult{Label: label, Store: store, Final: StatusError, Err: err}
	}
	progress := NewProgress(progressWriter)
	schedulerResult := NewScheduler(store, progress).Run(ctx)
	final := schedulerResult.Status
	progress.Close(final)
	message := schedulerResultMessage(schedulerResult)
	var runErr error
	if final == StatusFinished {
		if err := RunAnalyses(store, item.Plan.Analyses); err != nil {
			final = StatusError
			message = err.Error()
			runErr = err
		}
	}
	if err := store.MarkRootFinal(final, message); err != nil {
		return componentResult{Label: label, Store: store, Final: StatusError, Err: err}
	}
	if final == StatusFinished {
		return componentResult{Label: label, Store: store, Final: final}
	}
	if runErr != nil {
		return componentResult{Label: label, Store: store, Final: final, Err: runErr}
	}
	if schedulerResult.Err != nil {
		return componentResult{Label: label, Store: store, Final: final, Err: schedulerResult.Err}
	}
	return componentResult{Label: label, Store: store, Final: final}
}

func aggregateComponentResults(results []componentResult) error {
	messages := make([]string, 0)
	for _, result := range results {
		if result.Final == StatusFinished {
			continue
		}
		msg := fmt.Sprintf("%s: benchmark %s", result.Label, result.Final)
		if result.Err != nil {
			msg = fmt.Sprintf("%s: benchmark %s: %v", result.Label, result.Final, result.Err)
		}
		messages = append(messages, msg)
	}
	if len(messages) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(messages, "; "))
}

func sourceHashMismatchError(runDir string, stored, current string, source string) error {
	return fmt.Errorf("cannot continue %s: %s source hash does not match (stored %s, current %s); source identity includes loaded source path labels and contents, so continue with the same path used for jbs run", runDir, source, stored, current)
}

func schedulerResultMessage(result SchedulerResult) string {
	if result.Err != nil {
		return result.Err.Error()
	}
	return finalMessage(result.Status)
}

func finalMessage(final Status) string {
	switch final {
	case StatusError:
		return "one or more workpackages failed"
	case StatusInterrupted:
		return "run interrupted"
	default:
		return ""
	}
}

func printPostRunSummaries(stdout, stderr io.Writer, results []componentResult) {
	if stdout == nil {
		return
	}
	multi := len(results) > 1
	first := true
	for _, result := range results {
		if result.Store == nil {
			continue
		}
		if first {
			fmt.Fprintln(stdout)
			first = false
		} else {
			fmt.Fprintln(stdout)
		}
		if multi {
			fmt.Fprintf(stdout, "[%s]\n", result.Label)
		}
		summary, err := BuildStatusSummary(result.Store)
		if err != nil {
			printSummaryWarning(stderr, result.Label, "status", err)
			continue
		}
		PrintStatusSummary(stdout, summary)
		if result.Final != StatusFinished {
			continue
		}
		analyseSummaries, err := BuildAnalyseOutputSummaries(result.Store)
		if err != nil {
			printSummaryWarning(stderr, result.Label, "analyse", err)
			continue
		}
		if len(analyseSummaries) > 0 {
			fmt.Fprintln(stdout)
			PrintAnalyseOutputSummaries(stdout, analyseSummaries)
		}
	}
}

func printSummaryWarning(w io.Writer, label, kind string, err error) {
	if w == nil {
		return
	}
	if label == "" {
		fmt.Fprintf(w, "warning: failed to print %s summary: %v\n", kind, err)
		return
	}
	fmt.Fprintf(w, "warning: failed to print %s summary for %s: %v\n", kind, label, err)
}
