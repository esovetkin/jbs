package run

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func Stats(ctx context.Context, opts Options) error {
	_ = ctx
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	prepared, err := openLatestStoresForStats(suite)
	if err != nil {
		return err
	}
	return printStatsForStores(opts.Stdout, prepared)
}

func openLatestStoresForStats(suite runtimeSuitePlan) ([]preparedStore, error) {
	prepared := make([]preparedStore, 0, len(suite.Plans))
	for _, plan := range suite.Plans {
		runDir, err := latestRunDir(plan.RootDir)
		if err != nil {
			if suite.Configured && suite.SelectedName == "" {
				return nil, fmt.Errorf("cannot inspect benchmark %q: %w; use --benchmark to inspect one component", plan.ComponentName, err)
			}
			return nil, err
		}
		manifest, err := LoadManifest(filepath.Join(runDir, "manifest.json"))
		if err != nil {
			return nil, err
		}
		if err := validateRunManifest(manifest); err != nil {
			return nil, fmt.Errorf("cannot inspect %s: %w", runDir, err)
		}
		prepared = append(prepared, preparedStore{Plan: plan, Store: NewStore(runDir, manifest, plan.Bodies)})
	}
	return prepared, nil
}

func printStatsForStores(w io.Writer, prepared []preparedStore) error {
	if w == nil {
		return nil
	}
	multi := len(prepared) > 1
	for i, item := range prepared {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if multi {
			fmt.Fprintf(w, "[%s]\n", item.Plan.ComponentName)
		}
		summary, err := BuildStatusSummary(item.Store)
		if err != nil {
			return err
		}
		PrintStatusSummary(w, summary)
		if len(summary.FailedWork) > 0 {
			fmt.Fprintln(w)
			PrintFailedWorkDirectories(w, summary.FailedWork)
		}
	}
	return nil
}
