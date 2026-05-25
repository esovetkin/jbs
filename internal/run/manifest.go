package run

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/runtimevar"
)

func analyseTableName(prefix, runID, stepName string) string {
	return prefix + "_" + runID + "_" + stepName
}

func manifestAnalyseTablePrefix(manifest Manifest) string {
	if manifest.AnalyseTablePrefix != "" {
		return manifest.AnalyseTablePrefix
	}
	return manifest.BenchmarkName
}

func finalizeRunManifest(manifest Manifest, runID string) (Manifest, error) {
	if runID == "" {
		return Manifest{}, fmt.Errorf("run_id is empty")
	}
	manifest.RunID = runID
	if manifest.AnalyseDatabasePath == "" {
		return manifest, nil
	}

	seen := make(map[string]struct{})
	for i := range manifest.Steps {
		step := &manifest.Steps[i]
		if step.AnalyseTable == "" {
			continue
		}
		table := analyseTableName(manifestAnalyseTablePrefix(manifest), runID, step.Name)
		if _, ok := seen[table]; ok {
			return Manifest{}, fmt.Errorf("duplicate analyse table name %q", table)
		}
		seen[table] = struct{}{}
		step.AnalyseTable = table
	}
	return manifest, nil
}

func validateRunManifest(manifest Manifest) error {
	if manifest.WorkLimit < 0 {
		return fmt.Errorf("manifest work_limit must be non-negative")
	}
	if manifest.RunID == "" {
		return fmt.Errorf("manifest is missing run_id")
	}
	for _, work := range manifest.Work {
		for name := range work.Values {
			if reason, ok := runtimevar.ReservedName(name); ok {
				return fmt.Errorf("manifest work value %q is reserved for JBS runtime metadata (%s)", name, reason)
			}
		}
	}
	if manifest.AnalyseDatabasePath == "" {
		return nil
	}
	for _, step := range manifest.Steps {
		if step.AnalyseTable == "" {
			continue
		}
		want := analyseTableName(manifestAnalyseTablePrefix(manifest), manifest.RunID, step.Name)
		if step.AnalyseTable != want {
			return fmt.Errorf("manifest analyse table for step %q is %q, want %q", step.Name, step.AnalyseTable, want)
		}
	}
	return nil
}
