package run

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func ShowStatus(ctx context.Context, opts Options) error {
	_ = ctx
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	prepared, err := openLatestStoresForInspection(suite)
	if err != nil {
		return err
	}
	return printStatusForStores(opts.Stdout, prepared)
}

func LsAnalyse(ctx context.Context, opts Options) error {
	_ = ctx
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	prepared, err := openLatestStoresForInspection(suite)
	if err != nil {
		return err
	}
	return printAnalyseOutputsForStores(opts.Stdout, prepared)
}

func Tree(ctx context.Context, opts Options) error {
	_ = ctx
	diags := &diag.Diagnostics{}
	suite, err := buildRuntimeSuitePlan(opts, diags)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return fmt.Errorf("failed to build runtime workplan")
	}
	return printTreeForPlans(opts.Stdout, suite.Plans)
}

func openLatestStoresForInspection(suite runtimeSuitePlan) ([]preparedStore, error) {
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

func printStatusForStores(w io.Writer, prepared []preparedStore) error {
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

type labelledAnalyseSummaries struct {
	label     string
	summaries []AnalyseOutputSummary
}

func printAnalyseOutputsForStores(w io.Writer, prepared []preparedStore) error {
	if w == nil {
		return nil
	}
	sections := make([]labelledAnalyseSummaries, 0, len(prepared))
	for _, item := range prepared {
		summaries, err := BuildAnalyseOutputSummaries(item.Store)
		if err != nil {
			return err
		}
		if len(summaries) == 0 {
			continue
		}
		sections = append(sections, labelledAnalyseSummaries{
			label:     item.Plan.ComponentName,
			summaries: summaries,
		})
	}
	multi := len(prepared) > 1
	for i, section := range sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if multi {
			fmt.Fprintf(w, "[%s]\n", section.label)
		}
		PrintAnalyseOutputSummaries(w, section.summaries)
	}
	return nil
}

func printTreeForPlans(w io.Writer, plans []runtimePlan) error {
	if w == nil {
		return nil
	}
	multi := len(plans) > 1
	for i, plan := range plans {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if multi {
			fmt.Fprintf(w, "[%s]\n", plan.ComponentName)
		}
		PrintJobTreeSummary(w, BuildJobTreeSummary(plan.Manifest))
	}
	return nil
}
