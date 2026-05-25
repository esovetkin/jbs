package run

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workdir"
)

func validateWorkDirNamespaces(manifest Manifest, fileSubs map[string][]FileSubstitutionPlan) error {
	for _, work := range manifest.Work {
		used := workdir.ReservedEntries()
		for _, dep := range work.Deps {
			kind := fmt.Sprintf("dependency link for step %q", dep.Step)
			if err := claimWorkDirEntry(used, dep.Link, kind, work); err != nil {
				return err
			}
		}
		for _, spec := range fileSubs[work.Step] {
			kind := fmt.Sprintf("fsub output for template %q", spec.SourcePath)
			if err := claimWorkDirEntry(used, spec.DestName, kind, work); err != nil {
				return err
			}
		}
	}
	return nil
}

func claimWorkDirEntry(used map[string]string, name, kind string, work ManifestWork) error {
	if name == "" {
		return fmt.Errorf("step %q row %s has empty work-directory entry for %s", work.Step, rowDir(work.Row), kind)
	}
	if prev, ok := used[name]; ok {
		return fmt.Errorf(
			"step %q row %s work-directory entry %q for %s collides with %s",
			work.Step,
			rowDir(work.Row),
			name,
			kind,
			prev,
		)
	}
	used[name] = kind
	return nil
}
