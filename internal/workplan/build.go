package workplan

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

type state struct {
	ID         WorkID
	Values     map[string]eval.Value
	SourceRows map[sema.BindingVersionKey][]int
	Parents    []WorkID
}

type sourceGroup struct {
	Source        string
	SourceKey     sema.BindingVersionKey
	DisplaySource string
	Vars          []sourceVar
	Full          bool
	Span          diag.Span
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
		groups := groupExplicitDeltaBySource(plan, res.BindingsByName)
		states := expandStep(parents, groups, res.BindingsByName, step.Span, diags)
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

func groupExplicitDeltaBySource(plan *sema.StepScopePlan, sources map[string]*sema.GlobalBinding) []sourceGroup {
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
			sourceKey := sema.BindingVersionKeyForSource(sources, source)
			g = &sourceGroup{Source: source, SourceKey: sourceKey, DisplaySource: sourceKey.Display(), Vars: make([]sourceVar, 0), Span: item.Span}
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

func expandStep(parents []state, groups []sourceGroup, sources map[string]*sema.GlobalBinding, at diag.Span, diags *diag.Diagnostics) []state {
	if len(parents) == 0 {
		return nil
	}
	states := cloneStateSlice(parents)
	for _, group := range groups {
		next := make([]state, 0)
		for _, st := range states {
			choices := buildChoices(st, group, sources)
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

func buildChoices(st state, group sourceGroup, sources map[string]*sema.GlobalBinding) []sourceChoice {
	src := sources[group.Source]
	if src == nil {
		return nil
	}
	rowCount := planutil.SourceRowCount(src.Order, src.Vars)
	if rowCount == 0 {
		rowCount = 1
	}

	vars := group.Vars
	if group.Full && len(vars) == 0 {
		for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
			vars = append(vars, sourceVar{Visible: name, SourceVar: name})
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

	sourceKey := sourceKeyForGroup(group, sources)
	allowedRows, constrained := st.SourceRows[sourceKey]
	if constrained {
		filtered := make([]int, 0, len(allowedRows))
		for _, idx := range allowedRows {
			if idx < 0 || idx >= rowCount {
				continue
			}
			filtered = append(filtered, idx)
		}
		allowedRows = filtered
		if len(allowedRows) == 0 {
			return nil
		}
	} else {
		allowedRows = planutil.SequentialIndices(rowCount)
	}

	projected := planutil.BuildProjectedRowGroups(allowedRows, visibleNames, valuesByName, group.Full, valueKey)
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
	for sourceKey, rows := range b.SourceRows {
		if existing, ok := out.SourceRows[sourceKey]; ok {
			if !slices.Equal(existing, rows) {
				diags.AddError(
					diag.CodeE501,
					fmt.Sprintf("conflicting inherited row context for source '%s'", sourceKey.Display()),
					at,
					"ensure dependencies constrain the same source consistently",
				)
				return state{}, false
			}
			continue
		}
		out.SourceRows[sourceKey] = slices.Clone(rows)
	}
	return out, true
}

func mergeWithChoice(st state, group sourceGroup, choice sourceChoice, at diag.Span, diags *diag.Diagnostics) (state, bool) {
	out := cloneState(st)
	sourceKey := group.SourceKey
	if sourceKey == (sema.BindingVersionKey{}) {
		source := displaySourceForGroup(group)
		sourceKey = sema.BindingVersionKey{Public: source, Version: source}
	}
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
	out.SourceRows[sourceKey] = slices.Clone(choice.Rows)
	return out, true
}

func emptyState() state {
	return state{
		Values:     make(map[string]eval.Value),
		SourceRows: make(map[sema.BindingVersionKey][]int),
	}
}

func cloneState(st state) state {
	out := emptyState()
	out.ID = st.ID
	out.Parents = append([]WorkID(nil), st.Parents...)
	for name, value := range st.Values {
		out.Values[name] = value
	}
	for sourceKey, rows := range st.SourceRows {
		out.SourceRows[sourceKey] = slices.Clone(rows)
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

func cloneSourceRows(in map[sema.BindingVersionKey][]int) map[sema.BindingVersionKey][]int {
	out := make(map[sema.BindingVersionKey][]int, len(in))
	for k, v := range in {
		out[k] = slices.Clone(v)
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
