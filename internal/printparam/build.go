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
	candidateColumns := collectQualifiedColumns(res.BindingsByName)

	rows := make([]Row, 0)
	usedCols := make(map[string]struct{})
	for _, work := range plan.Work {
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
			key := displayColumnKey(res.BindingsByName, origin.Source, sourceVar)
			vals[key] = value.String()
			usedCols[key] = struct{}{}
		}
		rows = append(rows, Row{
			StepKind: work.StepKind,
			StepName: work.StepName,
			Values:   vals,
		})
	}

	columns := filterColumnsByUsage(candidateColumns, usedCols)
	columns = pruneHeaderOnlyColumns(columns, rows)
	return Table{Columns: columns, Rows: rows}
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

func collectQualifiedColumns(bindings map[string]*sema.GlobalBinding) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, sourceName := range slices.Sorted(maps.Keys(bindings)) {
		src := bindings[sourceName]
		if src == nil {
			continue
		}
		for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
			key := displayColumnKey(bindings, src.Name, name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out
}

func displaySourceName(bindings map[string]*sema.GlobalBinding, source string) string {
	if binding := bindings[source]; binding != nil && binding.PublicName != "" {
		return binding.PublicName
	}
	return source
}

func displayColumnKey(bindings map[string]*sema.GlobalBinding, source, sourceVar string) string {
	sourceDisplay := displaySourceName(bindings, source)
	if scalarIdentityColumn(bindings[source], sourceDisplay, sourceVar) {
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
