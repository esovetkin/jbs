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
	SourceRows map[string]sourceRowContext
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
		sourceID := ctx.sourceIdentity(item.Source)
		if item.Full {
			if src := bindings[item.Source]; src != nil {
				if src.Shape == sema.BindingTable && sourceRows[sourceID].VarName == "" && !sourceNeedsAlias(src, aliases) {
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
		sourceID := ctx.sourceIdentity(source)
		src := bindings[source]
		if src != nil && src.Shape == sema.BindingScalar {
			subset, rowContext := ctx.ensureScalarLetSubsetParameterSetForStep(stepName, source, grouped[source])
			if subset != "" {
				uses = append(uses, subset)
			}
			if rowContext.VarName != "" {
				sourceRows[sourceID] = rowContext
			}
			continue
		}
		subset, rowContext := ctx.ensureSubsetParameterSetForStep(stepName, source, grouped[source], groupedFull[source], sourceRows[sourceID])
		if subset != "" {
			uses = append(uses, subset)
		}
		if rowContext.VarName != "" {
			sourceRows[sourceID] = rowContext
		}
	}
	return stepUseResolution{
		Use:        uses,
		SourceRows: sourceRows,
	}
}

func (ctx *lowerContext) sourceIdentity(source string) string {
	if ctx == nil || ctx.res == nil {
		return source
	}
	if binding := ctx.res.BindingsByName[source]; binding != nil && binding.PublicName != "" {
		return binding.PublicName
	}
	return source
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

func (ctx *lowerContext) inheritedRowsForStep(stepName string, inheritedSteps []string) map[string]sourceRowContext {
	out := make(map[string]sourceRowContext)
	conflicts := make(map[string]struct{})
	for _, dep := range inheritedSteps {
		depRows := ctx.stepSourceRows[dep]
		if len(depRows) == 0 {
			continue
		}
		for source, rowContext := range depRows {
			if rowContext.VarName == "" {
				continue
			}
			if prev, exists := out[source]; exists && !equalSourceRowContext(prev, rowContext) {
				if _, reported := conflicts[source]; !reported {
					ctx.diags.AddError(
						diag.CodeE232,
						fmt.Sprintf("conflicting inherited row context for source '%s' in step '%s'", source, stepName),
						ctx.stepSpan(stepName),
						"ensure dependencies constrain the same source consistently",
						diag.RelatedSpan{Message: fmt.Sprintf("dependency '%s'", dep), Span: ctx.stepSpan(dep)},
					)
				}
				conflicts[source] = struct{}{}
				delete(out, source)
				continue
			}
			if _, bad := conflicts[source]; bad {
				continue
			}
			out[source] = cloneSourceRowContext(rowContext)
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

func cloneSourceRowContextMap(src map[string]sourceRowContext) map[string]sourceRowContext {
	if src == nil {
		return nil
	}
	out := make(map[string]sourceRowContext, len(src))
	for key, value := range src {
		out[key] = cloneSourceRowContext(value)
	}
	return out
}
