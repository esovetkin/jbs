package lower

import (
	"fmt"
	"maps"
	"slices"

	"jbs/internal/diag"
	"jbs/internal/planutil"
	"jbs/internal/sema"
)

type stepUseResolution struct {
	Use        []interface{}
	SourceRows map[string]string
}

type subsetVarSpec struct {
	Visible   string
	SourceVar string
	Emitted   string
}

func (ctx *lowerContext) resolveStepUsesForStep(stepName string, aliases map[string]string) stepUseResolution {
	inheritedSteps := make([]string, 0)
	if plan := ctx.res.StepImportByName[stepName]; plan != nil {
		inheritedSteps = append(inheritedSteps, plan.InheritedSteps...)
		return ctx.resolveStepUses(stepName, inheritedSteps, plan.ExplicitDelta, aliases)
	}
	return ctx.resolveStepUses(stepName, inheritedSteps, nil, aliases)
}

func (ctx *lowerContext) resolveStepUses(stepName string, inheritedSteps []string, items []sema.PlannedImport, aliases map[string]string) stepUseResolution {
	uses := make([]interface{}, 0)
	grouped := make(map[string][]subsetVarSpec)
	groupOrder := make([]string, 0)
	seenDirect := make(map[string]struct{})
	sourceRows := ctx.inheritedRowsForStep(stepName, inheritedSteps)
	sources := ctx.res.ImportSourceByName

	for _, item := range items {
		if item.Full {
			if src := sources[item.Source]; src != nil {
				if item.Kind == sema.SourceKindParam && sourceRows[item.Source] == "" && !sourceNeedsAlias(src, aliases) {
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
		src := sources[source]
		if src != nil && src.Kind == sema.SourceKindLet {
			subset, rowsVar := ctx.ensureScalarLetSubsetParameterSetForStep(stepName, source, grouped[source])
			if subset != "" {
				uses = append(uses, subset)
			}
			if rowsVar != "" {
				sourceRows[source] = rowsVar
			}
			continue
		}
		subset, rowsVar := ctx.ensureSubsetParameterSetForStep(stepName, source, grouped[source], sourceRows[source])
		if subset != "" {
			uses = append(uses, subset)
		}
		if rowsVar != "" {
			sourceRows[source] = rowsVar
		}
	}
	return stepUseResolution{
		Use:        uses,
		SourceRows: sourceRows,
	}
}

func (ctx *lowerContext) stepAliasMap(stepName string, forSubmit bool) map[string]string {
	if !forSubmit {
		return map[string]string{}
	}
	plan := ctx.res.StepImportByName[stepName]
	if plan == nil {
		return map[string]string{}
	}
	out := make(map[string]string)
	for name, origin := range plan.Effective {
		if origin.Kind != sema.SourceKindParam && origin.Kind != sema.SourceKindLet {
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

func sourceNeedsAlias(src *sema.ImportSource, aliases map[string]string) bool {
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

func (ctx *lowerContext) inheritedRowsForStep(stepName string, inheritedSteps []string) map[string]string {
	out := make(map[string]string)
	conflicts := make(map[string]struct{})
	for _, dep := range inheritedSteps {
		depRows := ctx.stepSourceRows[dep]
		if len(depRows) == 0 {
			continue
		}
		for source, rowsVar := range depRows {
			if rowsVar == "" {
				continue
			}
			if prev, exists := out[source]; exists && prev != rowsVar {
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
			out[source] = rowsVar
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

func cloneStringMap(src map[string]string) map[string]string {
	return maps.Clone(src)
}
