package sema

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/planutil"
)

type stepDefinition struct {
	Name      string
	After     []string
	WithItems []ast.WithItem
	Span      diag.Span
	Snapshot  *ScopeSnapshot
}

func buildStepScopePlans(res *Result, diags *diag.Diagnostics) {
	defs, order := collectStepDefinitions(res)
	plans := make(map[string]*StepScopePlan, len(defs))
	for _, stepName := range planutil.TopoStepOrder(stepDefinitionDeps(defs), order) {
		def := defs[stepName]
		reported := make(map[string]struct{})
		reportConflict := func(name string, left VisibleBinding, right VisibleBinding, at diag.Span, relation string) {
			a := stepScopeConflictKey(left)
			b := stepScopeConflictKey(right)
			if a == b {
				return
			}
			if a > b {
				a, b = b, a
			}
			key := name + "|" + a + "|" + b + "|" + relation
			if _, exists := reported[key]; exists {
				return
			}
			reported[key] = struct{}{}
			message, hint := stepScopeConflictMessage(stepName, name, left, right)
			diags.AddError(
				diag.CodeE214,
				message,
				at,
				hint,
				diag.RelatedSpan{Message: stepScopeConflictRelated(left), Span: left.Span},
				diag.RelatedSpan{Message: stepScopeConflictRelated(right), Span: right.Span},
			)
		}

		inherited := make(map[string]VisibleBinding)
		inheritedValues := make(map[string][]eval.Value)
		inheritedSteps := make([]string, 0, len(def.After))
		seenStep := make(map[string]struct{}, len(def.After))
		for _, dep := range def.After {
			if _, exists := seenStep[dep]; !exists {
				seenStep[dep] = struct{}{}
				inheritedSteps = append(inheritedSteps, dep)
			}
			depPlan := plans[dep]
			if depPlan == nil {
				continue
			}
			for name, origin := range depPlan.Effective {
				origin.ViaStep = dep
				if prev, exists := inherited[name]; exists {
					if !sameVisibleBinding(prev, origin) {
						reportConflict(name, prev, origin, def.Span, "inherited")
					}
					continue
				}
				inherited[name] = origin
				if depPlan.EffectiveValues != nil {
					if values, ok := depPlan.EffectiveValues[name]; ok {
						inheritedValues[name] = eval.CloneValues(values)
					}
				}
			}
		}

		resolver := BindingResolver{
			Bindings:   snapshotBindings(res, def.Snapshot),
			Globals:    snapshotGlobals(res, def.Snapshot),
			Namespaces: snapshotNamespaces(res, def.Snapshot),
		}
		expandedWith := resolver.ResolveDoWithItems(def.WithItems, diags)

		explicitDelta := make([]ScopeImport, 0)
		selected := make(map[string]VisibleBinding)
		selectedValues := make(map[string][]eval.Value)
		keptExpansions := make([]WithExpansion, 0, len(expandedWith))
		for _, expanded := range expandedWith {
			kept := make([]ExpandedWithVar, 0, len(expanded.Vars))
			sourceObj := bindingByKey(snapshotBindingsByKey(res, def.Snapshot), expanded.SourceKey)
			if sourceObj == nil {
				sourceObj = snapshotBindings(res, def.Snapshot)[expanded.Source]
			}
			sourceDisplay := expanded.DisplaySource
			if sourceDisplay == "" {
				sourceDisplay = expanded.SourceKey.Display()
			}
			if sourceDisplay == "" {
				sourceDisplay = expanded.Source
			}
			full := expanded.Full
			for _, v := range expanded.Vars {
				name := v.Visible
				originSpan := expanded.Span
				if sourceObj != nil {
					sourceVar := v.SourceVar
					if sourceVar == "" {
						sourceVar = name
					}
					if origin, ok := sourceObj.Origins[sourceVar]; ok && !origin.IsZero() {
						originSpan = origin
					}
				}
				current := VisibleBinding{
					Name:      name,
					SourceVar: v.SourceVar,
					Source:    sourceDisplay,
					SourceKey: expanded.SourceKey,
					Span:      originSpan,
				}
				if prev, exists := inherited[name]; exists {
					if !sameVisibleBinding(prev, current) {
						reportConflict(name, prev, current, expanded.Span, "explicit_vs_inherited")
					} else {
						full = false
					}
					continue
				}
				if prev, exists := selected[name]; exists {
					if !sameVisibleBinding(prev, current) {
						reportConflict(name, prev, current, expanded.Span, "explicit")
						continue
					}
					continue
				}
				selected[name] = current
				sourceVar := v.SourceVar
				if sourceVar == "" {
					sourceVar = name
				}
				if values, ok := expanded.VarsByName[sourceVar]; ok {
					selectedValues[name] = eval.CloneValues(values)
				}
				kept = append(kept, v)
			}
			if len(kept) == 0 {
				continue
			}
			nextExpansion := expanded
			nextExpansion.Source = sourceDisplay
			nextExpansion.DisplaySource = sourceDisplay
			nextExpansion.Vars = kept
			if len(kept) != len(expanded.Vars) {
				full = false
				nextExpansion.VarsByName = filterExpansionVars(nextExpansion.VarsByName, kept)
			}
			nextExpansion.Full = full
			keptExpansions = append(keptExpansions, nextExpansion)
			if full && len(kept) == len(expanded.Vars) {
				explicitDelta = append(explicitDelta, ScopeImport{
					ItemID:    expanded.ItemID,
					Source:    sourceDisplay,
					SourceKey: expanded.SourceKey,
					Full:      true,
					Span:      expanded.Span,
				})
				continue
			}
			for _, keptVar := range kept {
				explicitDelta = append(explicitDelta, ScopeImport{
					ItemID:    expanded.ItemID,
					Source:    sourceDisplay,
					SourceKey: expanded.SourceKey,
					Visible:   keptVar.Visible,
					SourceVar: keptVar.SourceVar,
					Span:      expanded.Span,
				})
			}
		}
		effective := make(map[string]VisibleBinding, len(inherited)+len(selected))
		effectiveValues := make(map[string][]eval.Value, len(inheritedValues)+len(selectedValues))
		for name, origin := range inherited {
			effective[name] = origin
			if values, ok := inheritedValues[name]; ok {
				effectiveValues[name] = eval.CloneValues(values)
			}
		}
		for name, origin := range selected {
			if prev, exists := effective[name]; exists && !sameVisibleBinding(prev, origin) {
				reportConflict(name, prev, origin, def.Span, "effective")
				continue
			}
			effective[name] = origin
			if values, ok := selectedValues[name]; ok {
				effectiveValues[name] = eval.CloneValues(values)
			}
		}
		plans[stepName] = &StepScopePlan{
			StepName:        stepName,
			Inherited:       inherited,
			ExplicitDelta:   explicitDelta,
			Effective:       effective,
			EffectiveValues: effectiveValues,
			InheritedSteps:  inheritedSteps,
			Expansions:      keptExpansions,
		}
	}
	res.StepScopeByName = plans
}

func sameSourceVersion(a, b VisibleBinding) bool {
	if a.SourceKey != (BindingVersionKey{}) || b.SourceKey != (BindingVersionKey{}) {
		return a.SourceKey == b.SourceKey
	}
	return a.Source == b.Source
}

func sameVisibleBinding(a, b VisibleBinding) bool {
	if !sameSourceVersion(a, b) {
		return false
	}
	return visibleBindingSourceVar(a) == visibleBindingSourceVar(b)
}

func visibleBindingSourceVar(binding VisibleBinding) string {
	if binding.SourceVar != "" {
		return binding.SourceVar
	}
	return binding.Name
}

func filterExpansionVars(vars map[string][]eval.Value, kept []ExpandedWithVar) map[string][]eval.Value {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string][]eval.Value, len(kept))
	for _, v := range kept {
		sourceVar := v.SourceVar
		if sourceVar == "" {
			sourceVar = v.Visible
		}
		if values, ok := vars[sourceVar]; ok {
			out[sourceVar] = eval.CloneValues(values)
		}
	}
	return out
}

func stepScopeConflictKey(binding VisibleBinding) string {
	source := binding.SourceKey.Display()
	if binding.SourceKey.Version != "" {
		source += "@" + binding.SourceKey.Version
	}
	if source == "" {
		source = binding.Source
	}
	if binding.ViaStep != "" {
		return "after:" + binding.ViaStep + ":" + source + ":" + visibleBindingSourceVar(binding)
	}
	return "with:" + source + ":" + visibleBindingSourceVar(binding)
}

func stepScopeConflictRelated(binding VisibleBinding) string {
	if binding.ViaStep != "" {
		return fmt.Sprintf("visible via `after %s`", binding.ViaStep)
	}
	return fmt.Sprintf("imported via `with %s`", visibleSourceDisplay(binding))
}

func visibleSourceDisplay(binding VisibleBinding) string {
	if display := binding.SourceKey.Display(); display != "" {
		return display
	}
	return binding.Source
}

func stepScopeConflictMessage(stepName, variable string, left VisibleBinding, right VisibleBinding) (string, string) {
	switch {
	case left.ViaStep != "" && right.ViaStep != "":
		return fmt.Sprintf(
				"conflicting variable '%s' for step '%s': inherited via `after` from predecessor steps '%s' and '%s'",
				variable,
				stepName,
				left.ViaStep,
				right.ViaStep,
			),
			"ensure only one predecessor makes this visible name available"
	case left.ViaStep != "":
		return fmt.Sprintf(
				"conflicting variable '%s' for step '%s': `with` import from '%s' collides with name inherited via `after %s`",
				variable,
				stepName,
				visibleSourceDisplay(right),
				left.ViaStep,
			),
			"rename the imported variable at the source or avoid importing the same visible name twice"
	case right.ViaStep != "":
		return fmt.Sprintf(
				"conflicting variable '%s' for step '%s': `with` import from '%s' collides with name inherited via `after %s`",
				variable,
				stepName,
				visibleSourceDisplay(left),
				right.ViaStep,
			),
			"rename the imported variable at the source or avoid importing the same visible name twice"
	default:
		return fmt.Sprintf(
				"conflicting variable '%s' for step '%s': imported via `with` from '%s' and '%s'",
				variable,
				stepName,
				visibleSourceDisplay(left),
				visibleSourceDisplay(right),
			),
			"import each variable name from only one source"
	}
}

func collectStepDefinitions(res *Result) (map[string]stepDefinition, []string) {
	defs := make(map[string]stepDefinition)
	order := make([]string, 0, len(res.StepOrder))
	for _, stepName := range res.StepOrder {
		for _, node := range res.DoBlocks {
			if node.Name != stepName {
				continue
			}
			defs[node.Name] = stepDefinition{
				Name:      node.Name,
				After:     append([]string(nil), node.After...),
				WithItems: append([]ast.WithItem(nil), node.WithItems...),
				Span:      node.Span,
				Snapshot:  snapshotForDoBlock(res, node),
			}
			order = append(order, node.Name)
			goto nextStep
		}
	nextStep:
	}
	return defs, order
}

func stepDefinitionDeps(defs map[string]stepDefinition) map[string][]string {
	out := make(map[string][]string, len(defs))
	for name, def := range defs {
		out[name] = append([]string(nil), def.After...)
	}
	return out
}
