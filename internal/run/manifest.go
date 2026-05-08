package run

import "fmt"

func analyseTableName(benchmarkName, runID, stepName string) string {
	return benchmarkName + "_" + runID + "_" + stepName
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
		table := analyseTableName(manifest.BenchmarkName, runID, step.Name)
		if _, ok := seen[table]; ok {
			return Manifest{}, fmt.Errorf("duplicate analyse table name %q", table)
		}
		seen[table] = struct{}{}
		step.AnalyseTable = table
	}
	return manifest, nil
}

func validateRunManifest(manifest Manifest) error {
	if manifest.RunID == "" {
		return fmt.Errorf("manifest is missing run_id")
	}
	if manifest.AnalyseDatabasePath == "" {
		return nil
	}
	for _, step := range manifest.Steps {
		if step.AnalyseTable == "" {
			continue
		}
		want := analyseTableName(manifest.BenchmarkName, manifest.RunID, step.Name)
		if step.AnalyseTable != want {
			return fmt.Errorf("manifest analyse table for step %q is %q, want %q", step.Name, step.AnalyseTable, want)
		}
	}
	return nil
}
