package workplan

import (
	"fmt"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

type state struct {
	ID         WorkID
	Values     map[string]eval.Value
	SourceRows map[sema.BindingVersionKey][]SourceRowConstraint
	Parents    []WorkID
}

type sourceGroup struct {
	ItemID           int
	Source           string
	SourceKey        sema.BindingVersionKey
	DisplaySource    string
	Vars             []sourceVar
	ValuesByName     map[string][]eval.Value
	ProjectionByName map[string][]eval.ProjectionKey
	RowCount         int
	Full             bool
	Span             diag.Span
}

type sourceVar struct {
	Visible   string
	SourceVar string
}

type sourceChoice struct {
	Rows   []int
	Values map[string]eval.Value
}

type stepDef struct {
	Name  string
	Kind  string
	After []string
	NProc int
	Body  string
	Span  diag.Span
}

func Build(res *sema.Result, diags *diag.Diagnostics) Plan {
	steps := collectStepsInResultOrder(res)
	defs := make(map[string]stepDef, len(steps))
	preferred := make([]string, 0, len(steps))
	for _, step := range steps {
		defs[step.Name] = step
		preferred = append(preferred, step.Name)
	}

	statesByStep := make(map[string][]state, len(steps))
	outSteps := make([]Step, 0, len(steps))
	outWork := make([]WorkPackage, 0)

	for _, stepName := range planutil.TopoStepOrder(stepDeps(defs), preferred) {
		step := defs[stepName]
		plan := res.StepScopeByName[stepName]
		parents := inheritParentStates(step.After, statesByStep, step.Span, diags)
		groups := groupExplicitDeltaByItem(plan, res.BindingsByName, res.BindingsByKey)
		states := expandStep(parents, groups, res.BindingsByName, res.BindingsByKey, step.Span, diags)
		for i := range states {
			states[i].ID = WorkID{Step: step.Name, Row: i}
		}
		statesByStep[stepName] = states
		outSteps = append(outSteps, Step{
			Name:  step.Name,
			Kind:  step.Kind,
			After: append([]string(nil), step.After...),
			NProc: step.NProc,
			Body:  step.Body,
			Span:  step.Span,
		})
		for _, st := range states {
			outWork = append(outWork, WorkPackage{
				ID:         st.ID,
				StepName:   step.Name,
				StepKind:   step.Kind,
				Values:     cloneValues(st.Values),
				SourceRows: cloneSourceRows(st.SourceRows),
				Deps:       append([]WorkID(nil), st.Parents...),
				Span:       step.Span,
			})
		}
	}

	return Plan{Steps: outSteps, Work: outWork}
}

func collectStepsInResultOrder(res *sema.Result) []stepDef {
	out := make([]stepDef, 0, len(res.StepOrder))
	for _, name := range res.StepOrder {
		for _, n := range res.DoBlocks {
			if n.Name == name {
				nproc := 0
				if n.NProc != nil {
					nproc = *n.NProc
				}
				out = append(out, stepDef{Name: n.Name, Kind: "do", After: append([]string(nil), n.After...), NProc: nproc, Body: n.Body, Span: n.Span})
				goto next
			}
		}
	next:
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

func inheritParentStates(after []string, byStep map[string][]state, at diag.Span, diags *diag.Diagnostics) []state {
	deps := uniqueStrings(after)
	if len(deps) == 0 {
		return []state{emptyState()}
	}

	combined := []state{emptyState()}
	for _, dep := range deps {
		depStates := byStep[dep]
		if len(depStates) == 0 {
			return nil
		}
		next := make([]state, 0, len(combined)*len(depStates))
		for _, base := range combined {
			for _, add := range depStates {
				merged, ok := mergeParentStates(base, add, at, diags)
				if !ok {
					continue
				}
				merged.Parents = append(merged.Parents, add.ID)
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

func groupExplicitDeltaByItem(plan *sema.StepScopePlan, sources map[string]*sema.GlobalBinding, sourcesByKey map[sema.BindingVersionKey]*sema.GlobalBinding) []sourceGroup {
	if plan == nil {
		return nil
	}
	if len(plan.Expansions) > 0 {
		out := make([]sourceGroup, 0, len(plan.Expansions))
		for _, expansion := range plan.Expansions {
			vars := make([]sourceVar, 0, len(expansion.Vars))
			for _, v := range expansion.Vars {
				sourceVarName := v.SourceVar
				if sourceVarName == "" {
					sourceVarName = v.Visible
				}
				vars = append(vars, sourceVar{Visible: v.Visible, SourceVar: sourceVarName})
			}
			rowCount := expansion.RowCount
			if rowCount == 0 {
				rowCount = sourceRowCountFromVars(expansion.VarsByName)
			}
			out = append(out, sourceGroup{
				ItemID:           expansion.ItemID,
				Source:           expansion.Source,
				SourceKey:        expansion.SourceKey,
				DisplaySource:    expansion.DisplaySource,
				Vars:             vars,
				ValuesByName:     cloneSeriesMap(expansion.VarsByName),
				ProjectionByName: cloneProjectionMap(expansion.ProjectionByName),
				RowCount:         rowCount,
				Full:             expansion.Full,
				Span:             expansion.Span,
			})
		}
		return out
	}

	type fallbackGroupKey struct {
		itemID    int
		source    string
		sourceKey sema.BindingVersionKey
	}
	order := make([]fallbackGroupKey, 0)
	byItem := make(map[fallbackGroupKey]*sourceGroup)
	for _, item := range plan.ExplicitDelta {
		source := item.Source
		if source == "" {
			continue
		}
		sourceKey := item.SourceKey
		if sourceKey == (sema.BindingVersionKey{}) {
			sourceKey = sema.BindingVersionKeyForSource(sources, source)
		}
		key := fallbackGroupKey{itemID: item.ItemID, source: source, sourceKey: sourceKey}
		g, ok := byItem[key]
		if !ok {
			g = &sourceGroup{ItemID: item.ItemID, Source: source, SourceKey: sourceKey, DisplaySource: sourceKey.Display(), Vars: make([]sourceVar, 0), Span: item.Span}
			byItem[key] = g
			order = append(order, key)
		}
		if g.Span.IsZero() {
			g.Span = item.Span
		}
		if item.Full {
			g.Full = true
			if src := bindingForGroup(*g, sources, sourcesByKey); src != nil {
				for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
					if slices.ContainsFunc(g.Vars, func(v sourceVar) bool { return v.Visible == name }) {
						continue
					}
					g.Vars = append(g.Vars, sourceVar{Visible: name, SourceVar: name})
				}
				g.ValuesByName = cloneSeriesMap(src.Vars)
				g.ProjectionByName = projectionByNameForBinding(src)
				g.RowCount = planutil.SourceRowCount(src.Order, src.Vars)
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
	for _, key := range order {
		if g := byItem[key]; g != nil {
			if g.ValuesByName == nil {
				g.ValuesByName = valuesByNameForGroup(*g, sources, sourcesByKey)
				g.RowCount = sourceRowCountFromVars(g.ValuesByName)
			}
			if g.ProjectionByName == nil {
				g.ProjectionByName = projectionByNameForGroup(*g, sources, sourcesByKey)
			}
			out = append(out, *g)
		}
	}
	return out
}

func expandStep(parents []state, groups []sourceGroup, sources map[string]*sema.GlobalBinding, sourcesByKey map[sema.BindingVersionKey]*sema.GlobalBinding, at diag.Span, diags *diag.Diagnostics) []state {
	if len(parents) == 0 {
		return nil
	}
	states := cloneStateSlice(parents)
	for _, group := range groups {
		next := make([]state, 0)
		for _, st := range states {
			choices := buildChoices(st, group, sources, sourcesByKey)
			for _, choice := range choices {
				merged, ok := mergeWithChoice(st, group, choice, at, diags)
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

func buildChoices(st state, group sourceGroup, sources map[string]*sema.GlobalBinding, sourcesByKey map[sema.BindingVersionKey]*sema.GlobalBinding) []sourceChoice {
	vars := group.Vars
	if group.Full && len(vars) == 0 {
		if src := bindingForGroup(group, sources, sourcesByKey); src != nil {
			for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
				vars = append(vars, sourceVar{Visible: name, SourceVar: name})
			}
		} else {
			for _, name := range sortedSeriesNames(group.ValuesByName) {
				vars = append(vars, sourceVar{Visible: name, SourceVar: name})
			}
		}
	}
	group.Vars = vars
	if group.ValuesByName == nil {
		group.ValuesByName = valuesByNameForGroup(group, sources, sourcesByKey)
	}
	if group.ProjectionByName == nil {
		group.ProjectionByName = projectionByNameForGroup(group, sources, sourcesByKey)
	}
	rowCount := group.RowCount
	if rowCount == 0 {
		rowCount = sourceRowCountFromVars(group.ValuesByName)
	}
	if rowCount == 0 && group.Full && len(vars) == 0 {
		rowCount = 1
	}
	if rowCount == 0 {
		return nil
	}

	valuesByName := make(map[string][]eval.Value, len(vars))
	projectionByName := make(map[string][]eval.ProjectionKey, len(vars))
	visibleNames := make([]string, 0, len(vars))
	for _, v := range vars {
		sourceVarName := v.SourceVar
		if sourceVarName == "" {
			sourceVarName = v.Visible
		}
		valuesByName[v.Visible] = planutil.ExpandValues(group.ValuesByName[sourceVarName], rowCount)
		projectionByName[v.Visible] = planutil.ExpandProjectionKeys(group.ProjectionByName[sourceVarName], rowCount)
		visibleNames = append(visibleNames, v.Visible)
	}

	sourceKey := sourceKeyForGroup(group, sources)
	allowedRows := allowedRowsForSource(st.SourceRows[sourceKey], rowCount)
	if len(allowedRows) == 0 {
		return nil
	}

	projected := planutil.BuildProjectedRowGroups(allowedRows, visibleNames, projectionByName, group.Full)
	choices := make([]sourceChoice, 0, len(projected))
	for _, grp := range projected {
		vals := make(map[string]eval.Value, len(visibleNames))
		for _, name := range visibleNames {
			vals[name] = valueAt(valuesByName[name], grp.Rep)
		}
		choices = append(choices, sourceChoice{
			Rows:   slices.Clone(grp.Rows),
			Values: vals,
		})
	}
	return choices
}

func sourceKeyForGroup(group sourceGroup, sources map[string]*sema.GlobalBinding) sema.BindingVersionKey {
	if group.SourceKey != (sema.BindingVersionKey{}) {
		return group.SourceKey
	}
	if sources != nil {
		return sema.BindingVersionKeyForSource(sources, group.Source)
	}
	display := displaySourceForGroup(group)
	return sema.BindingVersionKey{Public: display, Version: display}
}

func displaySourceForGroup(group sourceGroup) string {
	if group.DisplaySource != "" {
		return group.DisplaySource
	}
	if display := group.SourceKey.Display(); display != "" {
		return display
	}
	return group.Source
}

func valueAt(series []eval.Value, idx int) eval.Value {
	if idx < 0 || idx >= len(series) {
		return eval.Null()
	}
	return series[idx]
}

func allowedRowsForSource(constraints []SourceRowConstraint, rowCount int) []int {
	if rowCount <= 0 {
		return nil
	}
	allowed := make([]bool, rowCount)
	for i := range allowed {
		allowed[i] = true
	}
	constrained := false
	for _, constraint := range constraints {
		if !constraint.Inherited {
			continue
		}
		constrained = true
		next := make([]bool, rowCount)
		for _, row := range constraint.Rows {
			if row >= 0 && row < rowCount {
				next[row] = true
			}
		}
		for i := range allowed {
			allowed[i] = allowed[i] && next[i]
		}
	}
	if !constrained {
		return planutil.SequentialIndices(rowCount)
	}
	out := make([]int, 0, rowCount)
	for i, ok := range allowed {
		if ok {
			out = append(out, i)
		}
	}
	return out
}

func visibleNamesForGroup(group sourceGroup) []string {
	out := make([]string, 0, len(group.Vars))
	for _, v := range group.Vars {
		out = append(out, v.Visible)
	}
	return out
}

func mergeParentStates(a, b state, at diag.Span, diags *diag.Diagnostics) (state, bool) {
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
				return state{}, false
			}
			continue
		}
		out.Values[name] = value
	}
	for sourceKey, constraints := range b.SourceRows {
		for _, constraint := range constraints {
			constraint.Inherited = true
			constraint.Rows = slices.Clone(constraint.Rows)
			constraint.Vars = slices.Clone(constraint.Vars)
			out.SourceRows[sourceKey] = append(out.SourceRows[sourceKey], constraint)
		}
	}
	return out, true
}

func mergeWithChoice(st state, group sourceGroup, choice sourceChoice, at diag.Span, diags *diag.Diagnostics) (state, bool) {
	out := cloneState(st)
	sourceKey := sourceKeyForGroup(group, nil)
	source := sourceKey.Display()
	for name, value := range choice.Values {
		if existing, ok := out.Values[name]; ok {
			if !eval.Equal(existing, value) {
				diags.AddError(
					diag.CodeE502,
					fmt.Sprintf("conflicting value for '%s' while expanding source '%s'", name, source),
					at,
					"check with-clause imports and inherited variables for conflicts",
				)
				return state{}, false
			}
			continue
		}
		out.Values[name] = value
	}
	out.SourceRows[sourceKey] = append(out.SourceRows[sourceKey], SourceRowConstraint{
		ItemID: group.ItemID,
		Vars:   visibleNamesForGroup(group),
		Rows:   slices.Clone(choice.Rows),
	})
	return out, true
}

func emptyState() state {
	return state{
		Values:     make(map[string]eval.Value),
		SourceRows: make(map[sema.BindingVersionKey][]SourceRowConstraint),
	}
}

func cloneState(st state) state {
	out := emptyState()
	out.ID = st.ID
	out.Parents = append([]WorkID(nil), st.Parents...)
	for name, value := range st.Values {
		out.Values[name] = value
	}
	for sourceKey, constraints := range st.SourceRows {
		out.SourceRows[sourceKey] = cloneConstraints(constraints)
	}
	return out
}

func cloneStateSlice(states []state) []state {
	out := make([]state, 0, len(states))
	for _, st := range states {
		out = append(out, cloneState(st))
	}
	return out
}

func cloneValues(in map[string]eval.Value) map[string]eval.Value {
	out := make(map[string]eval.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSeriesMap(in map[string][]eval.Value) map[string][]eval.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]eval.Value, len(in))
	for k, v := range in {
		out[k] = eval.CloneValues(v)
	}
	return out
}

func cloneProjectionMap(in map[string][]eval.ProjectionKey) map[string][]eval.ProjectionKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]eval.ProjectionKey, len(in))
	for k, v := range in {
		out[k] = append([]eval.ProjectionKey(nil), v...)
	}
	return out
}

func valuesByNameForGroup(group sourceGroup, sources map[string]*sema.GlobalBinding, sourcesByKey map[sema.BindingVersionKey]*sema.GlobalBinding) map[string][]eval.Value {
	src := bindingForGroup(group, sources, sourcesByKey)
	if src == nil {
		return nil
	}
	out := make(map[string][]eval.Value, len(group.Vars))
	for _, v := range group.Vars {
		sourceVar := v.SourceVar
		if sourceVar == "" {
			sourceVar = v.Visible
		}
		if values, ok := src.Vars[sourceVar]; ok {
			out[sourceVar] = eval.CloneValues(values)
		}
	}
	return out
}

func projectionByNameForGroup(group sourceGroup, sources map[string]*sema.GlobalBinding, sourcesByKey map[sema.BindingVersionKey]*sema.GlobalBinding) map[string][]eval.ProjectionKey {
	src := bindingForGroup(group, sources, sourcesByKey)
	if src == nil {
		return nil
	}
	all := projectionByNameForBinding(src)
	out := make(map[string][]eval.ProjectionKey, len(group.Vars))
	for _, v := range group.Vars {
		sourceVar := v.SourceVar
		if sourceVar == "" {
			sourceVar = v.Visible
		}
		if values, ok := all[sourceVar]; ok {
			out[sourceVar] = append([]eval.ProjectionKey(nil), values...)
		}
	}
	return out
}

func projectionByNameForBinding(src *sema.GlobalBinding) map[string][]eval.ProjectionKey {
	if src == nil {
		return nil
	}
	order := src.Order
	if len(order) == 0 && eval.IsComb(src.Value) {
		order = eval.CombNames(src.Value)
	}
	out := make(map[string][]eval.ProjectionKey, len(order))
	if eval.IsComb(src.Value) {
		for _, name := range order {
			if keys, ok := eval.CombColumnProjections(src.Value, name); ok {
				out[name] = keys
			}
		}
		return out
	}
	for rowIndex, row := range src.Rows {
		for _, name := range order {
			cell, ok := row.Values[name]
			if !ok {
				continue
			}
			key := cell.Projection
			if !key.Valid() {
				key = eval.ProjectionFallbackKey(rowIndex)
			}
			out[name] = append(out[name], key)
		}
	}
	return out
}

func bindingForGroup(group sourceGroup, sources map[string]*sema.GlobalBinding, sourcesByKey map[sema.BindingVersionKey]*sema.GlobalBinding) *sema.GlobalBinding {
	if group.SourceKey != (sema.BindingVersionKey{}) {
		if src := sourcesByKey[group.SourceKey]; src != nil {
			return src
		}
	}
	if sources == nil {
		return nil
	}
	return sources[group.Source]
}

func sourceRowCountFromVars(vars map[string][]eval.Value) int {
	rowCount := 0
	for _, values := range vars {
		if len(values) > rowCount {
			rowCount = len(values)
		}
	}
	return rowCount
}

func sortedSeriesNames(vars map[string][]eval.Value) []string {
	out := make([]string, 0, len(vars))
	for name := range vars {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func cloneConstraints(in []SourceRowConstraint) []SourceRowConstraint {
	out := make([]SourceRowConstraint, len(in))
	for i, constraint := range in {
		out[i] = constraint
		out[i].Vars = slices.Clone(constraint.Vars)
		out[i].Rows = slices.Clone(constraint.Rows)
	}
	return out
}

func cloneSourceRows(in map[sema.BindingVersionKey][]SourceRowConstraint) map[sema.BindingVersionKey][]SourceRowConstraint {
	out := make(map[sema.BindingVersionKey][]SourceRowConstraint, len(in))
	for k, v := range in {
		out[k] = cloneConstraints(v)
	}
	return out
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
