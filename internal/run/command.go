package run

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/valuefmt"
)

func Run(ctx context.Context, opts Options) error {
	diags := &diag.Diagnostics{}
	plan, err := buildRuntimePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	store, err := CreateRunDirectory(plan.Manifest.BenchmarkName, plan)
	if err != nil {
		return err
	}
	printEvents(opts.Stdout, opts.PrintEvents)
	ctx, stop := withSignals(ctx, nil)
	defer stop()
	progress := NewProgress(opts.Stdout)
	schedulerResult := NewScheduler(store, progress).Run(ctx)
	final := schedulerResult.Status
	progress.Close(final)
	message := schedulerResultMessage(schedulerResult)
	if final == StatusFinished {
		if err := RunAnalyses(store, plan.Analyses); err != nil {
			final = StatusError
			message = err.Error()
		}
	}
	if err := store.MarkRootFinal(final, message); err != nil {
		return err
	}
	if final == StatusFinished {
		printAnalyseTables(opts.Stdout, store)
	}
	if final != StatusFinished {
		if schedulerResult.Err != nil {
			return fmt.Errorf("benchmark %s: %w", final, schedulerResult.Err)
		}
		return fmt.Errorf("benchmark %s", final)
	}
	return nil
}

func DryRun(ctx context.Context, opts Options) error {
	_ = ctx
	diags := &diag.Diagnostics{}
	plan, err := buildRuntimePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	if _, err := CreateDryRunDirectory(plan.Manifest.BenchmarkName, plan); err != nil {
		return err
	}
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
	plan, err := buildRuntimePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	unlock, err := acquireExistingRootLock(plan.Manifest.BenchmarkName)
	if err != nil {
		return err
	}
	unlockOnce := sync.OnceFunc(unlock)
	defer unlockOnce()

	runDir, err := latestRunDir(plan.Manifest.BenchmarkName)
	if err != nil {
		return err
	}
	rootStatus, err := LoadRootStatus(filepath.Join(runDir, "status"))
	if err != nil {
		return fmt.Errorf("cannot continue incomplete run %s: %w", runDir, err)
	}
	if rootStatus.Status == StatusRunning {
		return fmt.Errorf("cannot continue %s: benchmark status is RUNNING", runDir)
	}
	if rootStatus.SourceHash != plan.Manifest.SourceHash {
		return sourceHashMismatchError(runDir, rootStatus.SourceHash, plan.Manifest.SourceHash, "root status")
	}
	manifest, err := LoadManifest(filepath.Join(runDir, "manifest.json"))
	if err != nil {
		return err
	}
	if err := validateRunManifest(manifest); err != nil {
		return fmt.Errorf("cannot continue %s: %w", runDir, err)
	}
	if manifest.SourceHash != plan.Manifest.SourceHash {
		return sourceHashMismatchError(runDir, manifest.SourceHash, plan.Manifest.SourceHash, "manifest")
	}
	bodies := plan.Bodies
	store := NewStore(runDir, manifest, bodies)
	if err := store.NormalizeStaleRunning(); err != nil {
		return err
	}
	if err := store.MarkRootRunning(); err != nil {
		return err
	}
	ctx, stop := withSignals(ctx, unlockOnce)
	defer stop()
	progress := NewProgress(opts.Stdout)
	schedulerResult := NewScheduler(store, progress).Run(ctx)
	final := schedulerResult.Status
	progress.Close(final)
	message := schedulerResultMessage(schedulerResult)
	if final == StatusFinished {
		if err := RunAnalyses(store, plan.Analyses); err != nil {
			final = StatusError
			message = err.Error()
		}
	}
	if err := store.MarkRootFinal(final, message); err != nil {
		return err
	}
	if final == StatusFinished {
		printAnalyseTables(opts.Stdout, store)
	}
	if final != StatusFinished {
		if schedulerResult.Err != nil {
			return fmt.Errorf("benchmark %s: %w", final, schedulerResult.Err)
		}
		return fmt.Errorf("benchmark %s", final)
	}
	return nil
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

func printAnalyseTables(w io.Writer, store *Store) {
	if w == nil {
		return
	}
	if store.Manifest.AnalyseDatabasePath != "" {
		printAnalyseDatabaseTables(w, store)
		return
	}
	printAnalyseCSVTables(w, store)
}

func printAnalyseCSVTables(w io.Writer, store *Store) {
	for _, step := range store.Manifest.Steps {
		if step.AnalyseCSV == "" {
			continue
		}
		path := filepath.Join(store.RunDir, step.Dir, step.AnalyseCSV)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "\n%s/analyse.csv\n", step.Name)
		if len(data) > 0 {
			w.Write(data)
			if data[len(data)-1] != '\n' {
				fmt.Fprintln(w)
			}
		}
	}
}

func printAnalyseDatabaseTables(w io.Writer, store *Store) {
	db, err := openAnalyseDB(store.Manifest.AnalyseDatabasePath)
	if err != nil {
		return
	}
	defer db.Close()

	for _, step := range store.Manifest.Steps {
		if step.AnalyseTable == "" {
			continue
		}
		header, rows, err := readAnalyseTable(db, step.AnalyseTable)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "\n%s:%s\n", store.Manifest.AnalyseDatabase, step.AnalyseTable)
		writeCSVRows(w, append([][]string{header}, rows...))
	}
}

func writeCSVRows(w io.Writer, rows [][]string) {
	cw := csv.NewWriter(w)
	cw.WriteAll(rows)
	cw.Flush()
}
