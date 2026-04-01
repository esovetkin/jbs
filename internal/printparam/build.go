package printparam

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
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
	Vars   []string
	Full   bool
	Span   diag.Span
}

type sourceChoice struct {
	Rows   []int
	Values map[string]eval.Value
}

type rowGroup struct {
	Rep  int
	Rows []int
}

func Build(res *sema.Result, diags *diag.Diagnostics) Table {
	columns := collectQualifiedColumns(res.Paramsets)
	steps := collectStepsInProgramOrder(res.Program)
	defs := make(map[string]stepDef, len(steps))
	preferred := make([]string, 0, len(steps))
	for _, step := range steps {
		defs[step.Name] = step
		preferred = append(preferred, step.Name)
	}

	statesByStep := make(map[string][]wpState, len(steps))
	for _, stepName := range topoStepOrder(defs, preferred) {
		step := defs[stepName]
		plan := res.StepImportByName[stepName]
		parents := inheritParentStates(step.After, statesByStep, step.Span, diags)
		groups := groupExplicitDeltaBySource(plan, res.ParamByName)
		statesByStep[stepName] = expandStep(parents, groups, res.ParamByName, step.Span, diags)
	}

	rows := make([]Row, 0)
	for _, step := range steps {
		plan := res.StepImportByName[step.Name]
		states := statesByStep[step.Name]
		for _, state := range states {
			vals := make(map[string]string)
			for name, value := range state.Values {
				originParam := ""
				if plan != nil {
					if origin, ok := plan.Effective[name]; ok {
						originParam = origin.Paramset
					}
				}
				if originParam == "" {
					continue
				}
				vals[originParam+"."+name] = value.String()
			}
			rows = append(rows, Row{
				StepKind: step.Kind,
				StepName: step.Name,
				Values:   vals,
			})
		}
	}

	return Table{Columns: columns, Rows: rows}
}

func collectQualifiedColumns(paramsets []*sema.Paramset) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, ps := range paramsets {
		if ps == nil {
			continue
		}
		for _, name := range exposedVarNames(ps) {
			key := ps.Name + "." + name
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

func topoStepOrder(defs map[string]stepDef, preferred []string) []string {
	state := make(map[string]int, len(defs))
	order := make([]string, 0, len(defs))
	var visit func(string)
	visit = func(name string) {
		if state[name] == 2 {
			return
		}
		if state[name] == 1 {
			return
		}
		def, ok := defs[name]
		if !ok {
			return
		}
		state[name] = 1
		for _, dep := range def.After {
			if _, ok := defs[dep]; ok {
				visit(dep)
			}
		}
		state[name] = 2
		order = append(order, name)
	}

	for _, name := range preferred {
		visit(name)
	}
	extra := make([]string, 0, len(defs))
	for name := range defs {
		extra = append(extra, name)
	}
	sort.Strings(extra)
	for _, name := range extra {
		visit(name)
	}
	return order
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

func groupExplicitDeltaBySource(plan *sema.StepImportPlan, params map[string]*sema.Paramset) []sourceGroup {
	if plan == nil {
		return nil
	}
	order := make([]string, 0)
	bySource := make(map[string]*sourceGroup)
	for _, item := range plan.ExplicitDelta {
		source := item.From
		full := false
		if source == "" {
			source = item.Name
			full = true
		}
		if source == "" {
			continue
		}
		g, ok := bySource[source]
		if !ok {
			g = &sourceGroup{Source: source, Vars: make([]string, 0), Span: item.Span}
			bySource[source] = g
			order = append(order, source)
		}
		if g.Span.IsZero() {
			g.Span = item.Span
		}
		if full {
			g.Full = true
			if ps := params[source]; ps != nil {
				g.Vars = exposedVarNames(ps)
			}
			continue
		}
		if g.Full {
			continue
		}
		if !containsString(g.Vars, item.Name) {
			g.Vars = append(g.Vars, item.Name)
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

func expandStep(parents []wpState, groups []sourceGroup, params map[string]*sema.Paramset, at diag.Span, diags *diag.Diagnostics) []wpState {
	if len(parents) == 0 {
		return nil
	}
	states := cloneStateSlice(parents)
	for _, group := range groups {
		next := make([]wpState, 0)
		for _, state := range states {
			choices := buildChoices(state, group, params)
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

func buildChoices(state wpState, group sourceGroup, params map[string]*sema.Paramset) []sourceChoice {
	ps := params[group.Source]
	if ps == nil {
		return nil
	}
	rowCount := sourceRowCount(ps)
	if rowCount == 0 {
		rowCount = 1
	}

	vars := group.Vars
	if group.Full {
		if len(vars) == 0 {
			vars = exposedVarNames(ps)
		}
	}

	valuesByName := make(map[string][]eval.Value, len(vars))
	for _, name := range vars {
		valuesByName[name] = valuesFor(ps, name, rowCount)
	}

	if rows, ok := state.SourceRows[group.Source]; ok && len(rows) > 0 {
		choices := make([]sourceChoice, 0, len(rows))
		for _, idx := range rows {
			if idx < 0 || idx >= rowCount {
				continue
			}
			vals := make(map[string]eval.Value, len(vars))
			for _, name := range vars {
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
			vals := make(map[string]eval.Value, len(vars))
			for _, name := range vars {
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

	groups := buildRowGroups(vars, valuesByName, rowCount)
	choices := make([]sourceChoice, 0, len(groups))
	for _, grp := range groups {
		vals := make(map[string]eval.Value, len(vars))
		for _, name := range vars {
			series := valuesByName[name]
			value := eval.Null()
			if grp.Rep >= 0 && grp.Rep < len(series) {
				value = series[grp.Rep]
			}
			vals[name] = value
		}
		choices = append(choices, sourceChoice{
			Rows:   copyIntSlice(grp.Rows),
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
					"E500",
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
			if !equalIntSlices(existing, rows) {
				diags.AddError(
					"E501",
					fmt.Sprintf("conflicting inherited row context for source '%s'", source),
					at,
					"ensure dependencies constrain the same source consistently",
				)
				return wpState{}, false
			}
			continue
		}
		out.SourceRows[source] = copyIntSlice(rows)
	}
	return out, true
}

func mergeWithChoice(state wpState, source string, choice sourceChoice, at diag.Span, diags *diag.Diagnostics) (wpState, bool) {
	out := cloneState(state)
	for name, value := range choice.Values {
		if existing, ok := out.Values[name]; ok {
			if !eval.Equal(existing, value) {
				diags.AddError(
					"E502",
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
	out.SourceRows[source] = copyIntSlice(choice.Rows)
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
		out.SourceRows[source] = copyIntSlice(rows)
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

func sourceRowCount(ps *sema.Paramset) int {
	if ps == nil {
		return 0
	}
	if n := len(ps.Rows); n > 0 {
		return n
	}
	rowCount := 0
	for _, name := range ps.Order {
		if n := len(ps.Vars[name]); n > rowCount {
			rowCount = n
		}
	}
	return rowCount
}

func valuesFor(ps *sema.Paramset, name string, rowCount int) []eval.Value {
	values := make([]eval.Value, 0, rowCount)
	if len(ps.Rows) > 0 {
		for _, row := range ps.Rows {
			if cell, ok := row.Values[name]; ok {
				values = append(values, cell.Value)
			}
		}
		if len(values) == rowCount {
			return values
		}
	}

	base := ps.Vars[name]
	if len(base) == 0 {
		for i := 0; i < rowCount; i++ {
			values = append(values, eval.Null())
		}
		return values
	}
	values = values[:0]
	for i := 0; i < rowCount; i++ {
		values = append(values, base[i%len(base)])
	}
	return values
}

func buildRowGroups(vars []string, valuesByName map[string][]eval.Value, rowCount int) []rowGroup {
	if rowCount <= 0 {
		return nil
	}
	if len(vars) == 0 {
		return []rowGroup{{Rep: 0, Rows: sequentialIndices(rowCount)}}
	}
	indexByKey := make(map[string]int)
	groups := make([]rowGroup, 0, rowCount)
	for row := 0; row < rowCount; row++ {
		key := tupleKeyAt(vars, valuesByName, row)
		if idx, ok := indexByKey[key]; ok {
			groups[idx].Rows = append(groups[idx].Rows, row)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, rowGroup{Rep: row, Rows: []int{row}})
	}
	return groups
}

func tupleKeyAt(vars []string, valuesByName map[string][]eval.Value, row int) string {
	var b strings.Builder
	for _, name := range vars {
		values := valuesByName[name]
		value := eval.Null()
		if row >= 0 && row < len(values) {
			value = values[row]
		}
		lit := valueKey(value)
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(strconv.Itoa(len(lit)))
		b.WriteByte(':')
		b.WriteString(lit)
		b.WriteByte('|')
	}
	return b.String()
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
	default:
		return "other:" + v.String()
	}
}

func exposedVarNames(ps *sema.Paramset) []string {
	if ps == nil {
		return nil
	}
	if len(ps.Order) > 0 {
		out := make([]string, len(ps.Order))
		copy(out, ps.Order)
		return out
	}
	names := make([]string, 0, len(ps.Vars))
	for name := range ps.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sequentialIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = i
	}
	return out
}

func copyIntSlice(values []int) []int {
	out := make([]int, len(values))
	copy(out, values)
	return out
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
