// construct data needed for `jbs param`
package printparam

import (
	"maps"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/workplan"
)

func Build(res *sema.Result, diags *diag.Diagnostics) Table {
	plan := workplan.Build(res, diags)
	return BuildFromWorkPlans(res, []ComponentPlan{{WorkPlan: plan}}, false)
}

func BuildFromWorkPlan(res *sema.Result, plan workplan.Plan) Table {
	return BuildFromWorkPlans(res, []ComponentPlan{{WorkPlan: plan}}, false)
}

func BuildFromWorkPlans(res *sema.Result, plans []ComponentPlan, includeBenchmark bool) Table {
	candidateColumns := collectQualifiedColumns(res.Bindings)

	rows := make([]Row, 0)
	usedCols := make(map[string]struct{})
	for _, component := range plans {
		for _, work := range component.WorkPlan.Work {
			scopePlan := res.StepScopeByName[work.StepName]
			vals := make(map[string]string)
			for name, value := range work.Values {
				if scopePlan == nil {
					continue
				}
				origin, ok := scopePlan.Effective[name]
				if !ok || origin.Source == "" {
					continue
				}
				sourceVar := name
				if origin.SourceVar != "" {
					sourceVar = origin.SourceVar
				}
				binding := res.BindingsByKey[origin.SourceKey]
				if binding == nil {
					binding = res.BindingsByName[origin.Source]
				}
				sourceDisplay := origin.SourceKey.Display()
				if sourceDisplay == "" {
					sourceDisplay = origin.Source
				}
				key := displayColumnKey(binding, sourceDisplay, sourceVar)
				vals[key] = value.String()
				usedCols[key] = struct{}{}
			}
			rows = append(rows, Row{
				Benchmark: component.Name,
				StepKind:  work.StepKind,
				StepName:  work.StepName,
				Values:    vals,
			})
		}
	}

	columns := filterColumnsByUsage(candidateColumns, usedCols)
	columns = pruneHeaderOnlyColumns(columns, rows)
	return Table{Columns: columns, Rows: rows, BenchmarkColumn: includeBenchmark}
}

func filterColumnsByUsage(candidates []string, used map[string]struct{}) []string {
	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := used[candidate]; !ok {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	extra := make([]string, 0)
	for _, key := range slices.Sorted(maps.Keys(used)) {
		if _, ok := seen[key]; ok {
			continue
		}
		extra = append(extra, key)
	}
	out = append(out, extra...)
	return out
}

func pruneHeaderOnlyColumns(cols []string, rows []Row) []string {
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		present := false
		for _, row := range rows {
			// Check key presence, not row.Values[col] value; empty string can be valid data.
			if _, ok := row.Values[col]; ok {
				present = true
				break
			}
		}
		if present {
			out = append(out, col)
		}
	}
	return out
}

func collectQualifiedColumns(bindings []*sema.GlobalBinding) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, src := range bindings {
		if src == nil {
			continue
		}
		sourceDisplay := src.PublicName
		if sourceDisplay == "" {
			sourceDisplay = src.Name
		}
		for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
			key := displayColumnKey(src, sourceDisplay, name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out
}

func displayColumnKey(binding *sema.GlobalBinding, sourceDisplay, sourceVar string) string {
	if scalarIdentityColumn(binding, sourceDisplay, sourceVar) {
		return sourceDisplay
	}
	return sourceDisplay + "." + sourceVar
}

func scalarIdentityColumn(binding *sema.GlobalBinding, sourceDisplay, sourceVar string) bool {
	if binding == nil || binding.Shape != sema.BindingScalar || sourceVar == "" {
		return false
	}
	names := planutil.SourceVarNames(binding.Order, binding.Vars)
	if len(names) != 1 || names[0] != sourceVar {
		return false
	}
	return sourceDisplay == sourceVar || strings.HasSuffix(sourceDisplay, "."+sourceVar)
}
