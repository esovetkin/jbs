package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

func ShowStatusForBenchmarkDir(ctx context.Context, opts BenchmarkDirOptions) error {
	_ = ctx
	prepared, err := openLatestStoresForBenchmarkDir(opts.Root, opts.Benchmark)
	if err != nil {
		return err
	}
	return printStatusForStores(opts.Stdout, prepared)
}

func LsAnalyseForBenchmarkDir(ctx context.Context, opts BenchmarkDirOptions) error {
	_ = ctx
	prepared, err := openLatestStoresForBenchmarkDir(opts.Root, opts.Benchmark)
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

type discoveredBenchmarkRoot struct {
	rootDir string
	label   string
}

func openLatestStoresForBenchmarkDir(root string, selected string) ([]preparedStore, error) {
	roots, err := discoverBenchmarkRoots(root, selected)
	if err != nil {
		return nil, err
	}
	prepared := make([]preparedStore, 0, len(roots))
	for _, root := range roots {
		runDir, err := latestRunDir(root.rootDir)
		if err != nil {
			return nil, err
		}
		manifest, err := LoadManifest(filepath.Join(runDir, "manifest.json"))
		if err != nil {
			return nil, err
		}
		if err := validateRunManifest(manifest); err != nil {
			return nil, fmt.Errorf("cannot inspect %s: %w", runDir, err)
		}
		label := root.label
		if label == "" {
			label = labelFromManifestOrPath(manifest, root.rootDir)
		}
		prepared = append(prepared, preparedStore{
			Plan:  runtimePlan{RootDir: root.rootDir, ComponentName: label},
			Store: NewStore(runDir, manifest, nil),
		})
	}
	return prepared, nil
}

func discoverBenchmarkRoots(root string, selected string) ([]discoveredBenchmarkRoot, error) {
	root = filepath.Clean(root)
	manifest, _, directErr := latestManifestForRoot(root)
	if directErr == nil {
		if selected != "" && !manifestOrDirMatchesBenchmark(manifest, root, selected) {
			return nil, fmt.Errorf("benchmark directory %s does not match --benchmark %q", root, selected)
		}
		return []discoveredBenchmarkRoot{{
			rootDir: root,
			label:   labelFromManifestOrPath(manifest, root),
		}}, nil
	}
	if _, err := latestRunDir(root); err == nil {
		return nil, directErr
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	roots := make([]discoveredBenchmarkRoot, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || isRunDirName(entry.Name()) {
			continue
		}
		child := filepath.Join(root, entry.Name())
		manifest, _, err := latestManifestForRoot(child)
		if err != nil {
			if _, latestErr := latestRunDir(child); latestErr == nil {
				return nil, err
			}
			continue
		}
		if selected != "" && !manifestOrDirMatchesBenchmark(manifest, child, selected) {
			continue
		}
		roots = append(roots, discoveredBenchmarkRoot{
			rootDir: child,
			label:   labelFromManifestOrPath(manifest, child),
		})
	}
	slices.SortFunc(roots, func(a, b discoveredBenchmarkRoot) int {
		return strings.Compare(a.label, b.label)
	})
	if len(roots) == 0 {
		if selected != "" {
			return nil, fmt.Errorf("unknown benchmark %q in benchmark directory %s", selected, root)
		}
		return nil, fmt.Errorf("no run directories found in %s", root)
	}
	return roots, nil
}

func latestManifestForRoot(root string) (Manifest, string, error) {
	runDir, err := latestRunDir(root)
	if err != nil {
		return Manifest{}, "", err
	}
	manifest, err := LoadManifest(filepath.Join(runDir, "manifest.json"))
	if err != nil {
		return Manifest{}, "", err
	}
	if err := validateRunManifest(manifest); err != nil {
		return Manifest{}, "", fmt.Errorf("cannot inspect %s: %w", runDir, err)
	}
	return manifest, runDir, nil
}

func manifestOrDirMatchesBenchmark(manifest Manifest, root string, selected string) bool {
	if selected == "" {
		return true
	}
	if manifest.BenchmarkComponent == selected {
		return true
	}
	return filepath.Base(filepath.Clean(root)) == safePathComponent(selected)
}

func labelFromManifestOrPath(manifest Manifest, root string) string {
	if manifest.BenchmarkComponent != "" {
		return manifest.BenchmarkComponent
	}
	if manifest.BenchmarkName != "" {
		return manifest.BenchmarkName
	}
	return filepath.Base(filepath.Clean(root))
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
