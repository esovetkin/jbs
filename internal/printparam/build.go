package printparam

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
	"jbs/internal/sema"
)

type stepDef struct {
	Name  string
	Kind  string
	After []string
	Span  diag.Span
}

type wpState struct {
	Values     map[string]eval.Value
	SourceRows map[string][]int
}

type sourceGroup struct {
	Source string
	Kind   sema.SourceKind
	Vars   []sourceVar
	Full   bool
	Span   diag.Span
}

type sourceVar struct {
	Visible   string
	SourceVar string
}

type sourceChoice struct {
	Rows   []int
	Values map[string]eval.Value
}

func Build(res *sema.Result, diags *diag.Diagnostics) Table {
	candidateColumns := collectQualifiedColumns(res.ImportSourceByName)
	steps := collectStepsInProgramOrder(res.Program)
	defs := make(map[string]stepDef, len(steps))
	preferred := make([]string, 0, len(steps))
	for _, step := range steps {
		defs[step.Name] = step
		preferred = append(preferred, step.Name)
	}

	statesByStep := make(map[string][]wpState, len(steps))
	for _, stepName := range planutil.TopoStepOrder(stepDeps(defs), preferred) {
		step := defs[stepName]
		plan := res.StepImportByName[stepName]
		parents := inheritParentStates(step.After, statesByStep, step.Span, diags)
		groups := groupExplicitDeltaBySource(plan, res.ImportSourceByName)
		statesByStep[stepName] = expandStep(parents, groups, res.ImportSourceByName, step.Span, diags)
	}

	rows := make([]Row, 0)
	usedCols := make(map[string]struct{})
	for _, step := range steps {
		plan := res.StepImportByName[step.Name]
		states := statesByStep[step.Name]
		for _, state := range states {
			vals := make(map[string]string)
			for name, value := range state.Values {
				if plan == nil {
					continue
				}
				origin, ok := plan.Effective[name]
				if !ok || origin.Paramset == "" {
					continue
				}
				sourceVar := name
				if origin.SourceVar != "" {
					sourceVar = origin.SourceVar
				}
				key := origin.Paramset + "." + sourceVar
				vals[key] = value.String()
				usedCols[key] = struct{}{}
			}
			rows = append(rows, Row{
				StepKind: step.Kind,
				StepName: step.Name,
				Values:   vals,
			})
		}
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

func collectQualifiedColumns(sources map[string]*sema.ImportSource) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, sourceName := range slices.Sorted(maps.Keys(sources)) {
		src := sources[sourceName]
		if src == nil {
			continue
		}
		for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
			key := src.Name + "." + name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	return out
}

func collectStepsInProgramOrder(prog ast.Program) []stepDef {
	out := make([]stepDef, 0)
	for _, stmt := range prog.Stmts {
		switch n := stmt.(type) {
		case ast.DoBlock:
			out = append(out, stepDef{
				Name:  n.Name,
				Kind:  "do",
				After: append([]string(nil), n.After...),
				Span:  n.Span,
			})
		case ast.SubmitBlock:
			out = append(out, stepDef{
				Name:  n.Name,
				Kind:  "submit",
				After: append([]string(nil), n.After...),
				Span:  n.Span,
			})
		}
	}
	return out
}

func stepDeps(defs map[string]stepDef) map[string][]string {
	out := make(map[string][]string, len(defs))
	for name, def := range defs {
		out[name] = append([]string(nil), def.After...)
	}
	return out
}

func inheritParentStates(after []string, byStep map[string][]wpState, at diag.Span, diags *diag.Diagnostics) []wpState {
	deps := uniqueStrings(after)
	if len(deps) == 0 {
		return []wpState{emptyState()}
	}

	combined := []wpState{emptyState()}
	for _, dep := range deps {
		depStates := byStep[dep]
		if len(depStates) == 0 {
			return nil
		}
		next := make([]wpState, 0, len(combined)*len(depStates))
		for _, base := range combined {
			for _, add := range depStates {
				merged, ok := mergeParentStates(base, add, at, diags)
				if !ok {
					continue
				}
				next = append(next, merged)
			}
		}
		combined = next
		if len(combined) == 0 {
			return nil
		}
	}
	return combined
}

func groupExplicitDeltaBySource(plan *sema.StepImportPlan, sources map[string]*sema.ImportSource) []sourceGroup {
	if plan == nil {
		return nil
	}
	order := make([]string, 0)
	bySource := make(map[string]*sourceGroup)
	for _, item := range plan.ExplicitDelta {
		source := item.Source
		if source == "" {
			continue
		}
		g, ok := bySource[source]
		if !ok {
			g = &sourceGroup{Source: source, Kind: item.Kind, Vars: make([]sourceVar, 0), Span: item.Span}
			bySource[source] = g
			order = append(order, source)
		}
		if g.Span.IsZero() {
			g.Span = item.Span
		}
		if item.Full {
			g.Full = true
			if src := sources[source]; src != nil {
				for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
					if slices.ContainsFunc(g.Vars, func(v sourceVar) bool { return v.Visible == name }) {
						continue
					}
					g.Vars = append(g.Vars, sourceVar{Visible: name, SourceVar: name})
				}
			}
			continue
		}
		visible := item.Visible
		if visible == "" {
			visible = item.SourceVar
		}
		sourceVarName := item.SourceVar
		if sourceVarName == "" {
			sourceVarName = visible
		}
		if !slices.ContainsFunc(g.Vars, func(v sourceVar) bool { return v.Visible == visible }) {
			g.Vars = append(g.Vars, sourceVar{Visible: visible, SourceVar: sourceVarName})
		}
	}

	out := make([]sourceGroup, 0, len(order))
	for _, source := range order {
		if g := bySource[source]; g != nil {
			out = append(out, *g)
		}
	}
	return out
}

func expandStep(parents []wpState, groups []sourceGroup, sources map[string]*sema.ImportSource, at diag.Span, diags *diag.Diagnostics) []wpState {
	if len(parents) == 0 {
		return nil
	}
	states := cloneStateSlice(parents)
	for _, group := range groups {
		next := make([]wpState, 0)
		for _, state := range states {
			choices := buildChoices(state, group, sources)
			for _, choice := range choices {
				merged, ok := mergeWithChoice(state, group.Source, choice, at, diags)
				if !ok {
					continue
				}
				next = append(next, merged)
			}
		}
		states = next
		if len(states) == 0 {
			return nil
		}
	}
	return states
}

func buildChoices(state wpState, group sourceGroup, sources map[string]*sema.ImportSource) []sourceChoice {
	src := sources[group.Source]
	if src == nil {
		return nil
	}
	rowCount := planutil.SourceRowCount(src.Order, src.Vars)
	if rowCount == 0 {
		rowCount = 1
	}

	vars := group.Vars
	if group.Full {
		if len(vars) == 0 {
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				vars = append(vars, sourceVar{Visible: name, SourceVar: name})
			}
		}
	}

	valuesByName := make(map[string][]eval.Value, len(vars))
	visibleNames := make([]string, 0, len(vars))
	for _, v := range vars {
		sourceVarName := v.SourceVar
		if sourceVarName == "" {
			sourceVarName = v.Visible
		}
		valuesByName[v.Visible] = planutil.ExpandValues(src.Vars[sourceVarName], rowCount)
		visibleNames = append(visibleNames, v.Visible)
	}

	if rows, ok := state.SourceRows[group.Source]; ok && len(rows) > 0 {
		choices := make([]sourceChoice, 0, len(rows))
		for _, idx := range rows {
			if idx < 0 || idx >= rowCount {
				continue
			}
			vals := make(map[string]eval.Value, len(visibleNames))
			for _, name := range visibleNames {
				series := valuesByName[name]
				value := eval.Null()
				if idx < len(series) {
					value = series[idx]
				}
				vals[name] = value
			}
			choices = append(choices, sourceChoice{
				Rows:   []int{idx},
				Values: vals,
			})
		}
		return choices
	}

	if group.Full {
		choices := make([]sourceChoice, 0, rowCount)
		for idx := 0; idx < rowCount; idx++ {
			vals := make(map[string]eval.Value, len(visibleNames))
			for _, name := range visibleNames {
				series := valuesByName[name]
				value := eval.Null()
				if idx < len(series) {
					value = series[idx]
				}
				vals[name] = value
			}
			choices = append(choices, sourceChoice{
				Rows:   []int{idx},
				Values: vals,
			})
		}
		return choices
	}

	groups := planutil.BuildRowGroups(visibleNames, valuesByName, rowCount, valueKey)
	choices := make([]sourceChoice, 0, len(groups))
	for _, grp := range groups {
		vals := make(map[string]eval.Value, len(visibleNames))
		for _, name := range visibleNames {
			series := valuesByName[name]
			value := eval.Null()
			if grp.Rep >= 0 && grp.Rep < len(series) {
				value = series[grp.Rep]
			}
			vals[name] = value
		}
		choices = append(choices, sourceChoice{
			Rows:   slices.Clone(grp.Rows),
			Values: vals,
		})
	}
	return choices
}

func mergeParentStates(a, b wpState, at diag.Span, diags *diag.Diagnostics) (wpState, bool) {
	out := cloneState(a)
	for name, value := range b.Values {
		if existing, ok := out.Values[name]; ok {
			if !eval.Equal(existing, value) {
				diags.AddError(
					diag.CodeE500,
					fmt.Sprintf("conflicting inherited value for '%s'", name),
					at,
					"ensure dependencies inherit compatible parameter values",
				)
				return wpState{}, false
			}
			continue
		}
		out.Values[name] = value
	}
	for source, rows := range b.SourceRows {
		if existing, ok := out.SourceRows[source]; ok {
			if !slices.Equal(existing, rows) {
				diags.AddError(
					diag.CodeE501,
					fmt.Sprintf("conflicting inherited row context for source '%s'", source),
					at,
					"ensure dependencies constrain the same source consistently",
				)
				return wpState{}, false
			}
			continue
		}
		out.SourceRows[source] = slices.Clone(rows)
	}
	return out, true
}

func mergeWithChoice(state wpState, source string, choice sourceChoice, at diag.Span, diags *diag.Diagnostics) (wpState, bool) {
	out := cloneState(state)
	for name, value := range choice.Values {
		if existing, ok := out.Values[name]; ok {
			if !eval.Equal(existing, value) {
				diags.AddError(
					diag.CodeE502,
					fmt.Sprintf("conflicting value for '%s' while expanding source '%s'", name, source),
					at,
					"check with-clause imports and inherited variables for conflicts",
				)
				return wpState{}, false
			}
			continue
		}
		out.Values[name] = value
	}
	out.SourceRows[source] = slices.Clone(choice.Rows)
	return out, true
}

func emptyState() wpState {
	return wpState{
		Values:     make(map[string]eval.Value),
		SourceRows: make(map[string][]int),
	}
}

func cloneState(state wpState) wpState {
	out := emptyState()
	for name, value := range state.Values {
		out.Values[name] = value
	}
	for source, rows := range state.SourceRows {
		out.SourceRows[source] = slices.Clone(rows)
	}
	return out
}

func cloneStateSlice(states []wpState) []wpState {
	out := make([]wpState, 0, len(states))
	for _, state := range states {
		out = append(out, cloneState(state))
	}
	return out
}

func valueKey(v eval.Value) string {
	switch v.Kind {
	case eval.KindNull:
		return "null"
	case eval.KindInt:
		return "int:" + strconv.FormatInt(v.I, 10)
	case eval.KindFloat:
		return "float:" + strconv.FormatFloat(v.F, 'g', -1, 64)
	case eval.KindString:
		return "str:" + strconv.Quote(v.S)
	case eval.KindBool:
		if v.B {
			return "bool:true"
		}
		return "bool:false"
	case eval.KindList:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, valueKey(item))
		}
		return "list:[" + strings.Join(parts, ",") + "]"
	case eval.KindTuple:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, valueKey(item))
		}
		return "tuple:(" + strings.Join(parts, ",") + ")"
	default:
		return "other:" + v.String()
	}
}

func uniqueStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
