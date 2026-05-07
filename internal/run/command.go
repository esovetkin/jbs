package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
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
	ctx, stop := withSignals(ctx)
	defer stop()
	progress := NewProgress(opts.Stdout)
	final := NewScheduler(store, progress).Run(ctx)
	progress.Close(final)
	message := finalMessage(final)
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
		return fmt.Errorf("benchmark %s", final)
	}
	return nil
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
		return fmt.Errorf("cannot continue %s: source hash does not match", runDir)
	}
	manifest, err := LoadManifest(filepath.Join(runDir, "manifest.json"))
	if err != nil {
		return err
	}
	if manifest.SourceHash != plan.Manifest.SourceHash {
		return fmt.Errorf("cannot continue %s: manifest source hash does not match", runDir)
	}
	bodies := plan.Bodies
	store := OpenStore(runDir, manifest, bodies)
	if err := store.NormalizeStaleRunning(); err != nil {
		return err
	}
	if err := store.MarkRootRunning(); err != nil {
		return err
	}
	ctx, stop := withSignals(ctx)
	defer stop()
	progress := NewProgress(opts.Stdout)
	final := NewScheduler(store, progress).Run(ctx)
	progress.Close(final)
	message := finalMessage(final)
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
		return fmt.Errorf("benchmark %s", final)
	}
	return nil
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
