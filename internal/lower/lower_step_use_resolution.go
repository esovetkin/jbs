// apply alias-emitted variable names for submit collisions/helpers
//
// carry forward source-row context across dependencies, and reports
// inherited-row conflicts.
package lower

import (
	"fmt"
	"slices"

	"jbs/internal/diag"
	"jbs/internal/planutil"
	"jbs/internal/sema"
)

type stepUseResolution struct {
	Use        []interface{}
	SourceRows map[sourceRowKey]sourceRowContext
}

type subsetVarSpec struct {
	Visible   string
	SourceVar string
	Emitted   string
}

func (ctx *lowerContext) resolveStepUsesForStep(stepName string, aliases map[string]string) stepUseResolution {
	inheritedSteps := make([]string, 0)
	if plan := ctx.res.StepScopeByName[stepName]; plan != nil {
		inheritedSteps = append(inheritedSteps, plan.InheritedSteps...)
		return ctx.resolveStepUses(stepName, inheritedSteps, plan.ExplicitDelta, aliases)
	}
	return ctx.resolveStepUses(stepName, inheritedSteps, nil, aliases)
}

func (ctx *lowerContext) resolveStepUses(stepName string, inheritedSteps []string, items []sema.ScopeImport, aliases map[string]string) stepUseResolution {
	uses := make([]interface{}, 0)
	grouped := make(map[string][]subsetVarSpec)
	groupedFull := make(map[string]bool)
	groupOrder := make([]string, 0)
	seenDirect := make(map[string]struct{})
	sourceRows := ctx.inheritedRowsForStep(stepName, inheritedSteps)
	bindings := ctx.res.BindingsByName

	for _, item := range items {
		sourceKey := ctx.sourceRowKeyForSource(item.Source)
		if item.Full {
			if src := bindings[item.Source]; src != nil {
				if src.Shape == sema.BindingTable && sourceRows[sourceKey].VarName == "" && !sourceNeedsAlias(src, aliases) {
					ctx.ensureSourceParameterSet(item.Source)
					if _, seen := seenDirect[item.Source]; !seen {
						seenDirect[item.Source] = struct{}{}
						uses = append(uses, item.Source)
					}
					continue
				}
				if _, ok := grouped[item.Source]; !ok {
					grouped[item.Source] = make([]subsetVarSpec, 0)
					groupOrder = append(groupOrder, item.Source)
				}
				groupedFull[item.Source] = true
				for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
					if slices.ContainsFunc(grouped[item.Source], func(v subsetVarSpec) bool { return v.Visible == name }) {
						continue
					}
					emitted := name
					if alias, ok := aliases[name]; ok && alias != "" {
						emitted = alias
					}
					grouped[item.Source] = append(grouped[item.Source], subsetVarSpec{
						Visible:   name,
						SourceVar: name,
						Emitted:   emitted,
					})
				}
			}
			continue
		}
		if _, ok := grouped[item.Source]; !ok {
			grouped[item.Source] = make([]subsetVarSpec, 0)
			groupOrder = append(groupOrder, item.Source)
		}
		if !slices.ContainsFunc(grouped[item.Source], func(v subsetVarSpec) bool { return v.Visible == item.Visible }) {
			sourceVar := item.SourceVar
			if sourceVar == "" {
				sourceVar = item.Visible
			}
			emitted := item.Visible
			if alias, ok := aliases[item.Visible]; ok && alias != "" {
				emitted = alias
			}
			grouped[item.Source] = append(grouped[item.Source], subsetVarSpec{
				Visible:   item.Visible,
				SourceVar: sourceVar,
				Emitted:   emitted,
			})
		}
	}

	for _, source := range groupOrder {
		sourceKey := ctx.sourceRowKeyForSource(source)
		src := bindings[source]
		if scalarImportCanUseConstantSubset(src, grouped[source]) {
			subset, rowContext := ctx.ensureScalarLetSubsetParameterSetForStep(stepName, source, grouped[source])
			if subset != "" {
				uses = append(uses, subset)
			}
			if rowContext.VarName != "" {
				sourceRows[sourceKey] = rowContext
			}
			continue
		}
		subset, rowContext := ctx.ensureSubsetParameterSetForStep(stepName, source, grouped[source], groupedFull[source], sourceRows[sourceKey])
		if subset != "" {
			uses = append(uses, subset)
		}
		if rowContext.VarName != "" {
			sourceRows[sourceKey] = rowContext
		}
	}
	return stepUseResolution{
		Use:        uses,
		SourceRows: sourceRows,
	}
}

func (ctx *lowerContext) sourceRowKeyForSource(source string) sourceRowKey {
	if ctx == nil || ctx.res == nil {
		return sourceRowKey{Public: source, Version: source}
	}
	key := sema.BindingVersionKeyForSource(ctx.res.BindingsByName, source)
	return sourceRowKey{Public: key.Public, Version: key.Version}
}

func (ctx *lowerContext) stepAliasMap(stepName string, forSubmit bool) map[string]string {
	if !forSubmit {
		return map[string]string{}
	}
	plan := ctx.res.StepScopeByName[stepName]
	if plan == nil {
		return map[string]string{}
	}
	out := make(map[string]string)
	for name, origin := range plan.Effective {
		if origin.Source == "" {
			continue
		}
		if sema.IsSubmitKey(name) {
			out[name] = escapedAliasPrefix + name
		}
	}
	return out
}

func (ctx *lowerContext) submitValueAliasMap(stepName string) map[string]string {
	out := ctx.stepAliasMap(stepName, true)
	spec := ctx.res.SubmitByName[stepName]
	if spec == nil {
		return out
	}
	for _, helper := range spec.Helpers {
		if helper.Original == "" || helper.Aliased == "" {
			continue
		}
		out[helper.Original] = helper.Aliased
	}
	return out
}

func sourceNeedsAlias(src *sema.GlobalBinding, aliases map[string]string) bool {
	if src == nil || len(aliases) == 0 {
		return false
	}
	for _, name := range planutil.SourceVarNames(src.Order, src.Vars) {
		if _, ok := aliases[name]; ok {
			return true
		}
	}
	return false
}

func scalarImportCanUseConstantSubset(src *sema.GlobalBinding, vars []subsetVarSpec) bool {
	if src == nil || src.Shape != sema.BindingScalar {
		return false
	}
	return selectedSourceRowCount(src, vars) <= 1
}

func selectedSourceRowCount(src *sema.GlobalBinding, vars []subsetVarSpec) int {
	if src == nil {
		return 0
	}
	if len(vars) == 0 {
		return planutil.SourceRowCount(src.Order, src.Vars)
	}
	rowCount := 0
	for _, variable := range vars {
		sourceVar := variable.SourceVar
		if sourceVar == "" {
			sourceVar = variable.Visible
		}
		if n := len(src.Vars[sourceVar]); n > rowCount {
			rowCount = n
		}
	}
	return rowCount
}

func (ctx *lowerContext) inheritedRowsForStep(stepName string, inheritedSteps []string) map[sourceRowKey]sourceRowContext {
	out := make(map[sourceRowKey]sourceRowContext)
	conflicts := make(map[sourceRowKey]struct{})
	for _, dep := range inheritedSteps {
		depRows := ctx.stepSourceRows[dep]
		if len(depRows) == 0 {
			continue
		}
		for sourceKey, rowContext := range depRows {
			if rowContext.VarName == "" {
				continue
			}
			if prev, exists := out[sourceKey]; exists && !equalSourceRowContext(prev, rowContext) {
				if _, reported := conflicts[sourceKey]; !reported {
					ctx.diags.AddError(
						diag.CodeE232,
						fmt.Sprintf("conflicting inherited row context for source '%s' in step '%s'", sourceKey.display(), stepName),
						ctx.stepSpan(stepName),
						"ensure dependencies constrain the same source consistently",
						diag.RelatedSpan{Message: fmt.Sprintf("dependency '%s'", dep), Span: ctx.stepSpan(dep)},
					)
				}
				conflicts[sourceKey] = struct{}{}
				delete(out, sourceKey)
				continue
			}
			if _, bad := conflicts[sourceKey]; bad {
				continue
			}
			out[sourceKey] = cloneSourceRowContext(rowContext)
		}
	}
	return out
}

func (ctx *lowerContext) stepSpan(stepName string) diag.Span {
	for _, block := range ctx.res.DoBlocks {
		if block.Name == stepName {
			return block.Span
		}
	}
	for _, block := range ctx.res.Submits {
		if block.Name == stepName {
			return block.Span
		}
	}
	return diag.Span{}
}

func equalSourceRowContext(a, b sourceRowContext) bool {
	return a.VarName == b.VarName && slices.Equal(a.Groups, b.Groups)
}

func cloneSourceRowContext(in sourceRowContext) sourceRowContext {
	return sourceRowContext{
		VarName: in.VarName,
		Groups:  slices.Clone(in.Groups),
	}
}

func cloneSourceRowContextMap(src map[sourceRowKey]sourceRowContext) map[sourceRowKey]sourceRowContext {
	if src == nil {
		return nil
	}
	out := make(map[sourceRowKey]sourceRowContext, len(src))
	for key, value := range src {
		out[key] = cloneSourceRowContext(value)
	}
	return out
}
