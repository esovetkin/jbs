package sema

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/planutil"
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
					if prev.Source != origin.Source {
						reportConflict(name, prev, origin, def.Span, "inherited")
					}
					continue
				}
				inherited[name] = origin
			}
		}

		resolver := BindingResolver{
			Bindings:   snapshotBindings(res, def.Snapshot),
			Globals:    snapshotGlobals(res, def.Snapshot),
			Namespaces: snapshotNamespaces(res, def.Snapshot),
		}
		expandedWith, _ := resolver.ExpandWithItems(def.WithItems, ResolveOptions{Context: ImportIntoStep})

		explicitDelta := make([]ScopeImport, 0)
		selected := make(map[string]VisibleBinding)
		for _, expanded := range expandedWith {
			kept := make([]ExpandedWithVar, 0, len(expanded.Vars))
			sourceObj := snapshotBindings(res, def.Snapshot)[expanded.Source]
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
					Source:    expanded.Source,
					Span:      originSpan,
				}
				if prev, exists := inherited[name]; exists {
					if prev.Source != current.Source {
						reportConflict(name, prev, current, expanded.Span, "explicit_vs_inherited")
					}
					continue
				}
				if prev, exists := selected[name]; exists {
					if prev.Source != current.Source {
						reportConflict(name, prev, current, expanded.Span, "explicit")
						continue
					}
					continue
				}
				selected[name] = current
				kept = append(kept, v)
			}
			if len(kept) == 0 {
				continue
			}
			if expanded.Full && len(kept) == len(expanded.Vars) {
				explicitDelta = append(explicitDelta, ScopeImport{
					Source: expanded.Source,
					Full:   true,
					Span:   expanded.Span,
				})
				continue
			}
			for _, keptVar := range kept {
				explicitDelta = append(explicitDelta, ScopeImport{
					Source:    expanded.Source,
					Visible:   keptVar.Visible,
					SourceVar: keptVar.SourceVar,
					Span:      expanded.Span,
				})
			}
		}
		effective := make(map[string]VisibleBinding, len(inherited)+len(selected))
		for name, origin := range inherited {
			effective[name] = origin
		}
		for name, origin := range selected {
			if prev, exists := effective[name]; exists && prev.Source != origin.Source {
				reportConflict(name, prev, origin, def.Span, "effective")
				continue
			}
			effective[name] = origin
		}
		plans[stepName] = &StepScopePlan{
			StepName:       stepName,
			Inherited:      inherited,
			ExplicitDelta:  explicitDelta,
			Effective:      effective,
			InheritedSteps: inheritedSteps,
		}
	}
	res.StepScopeByName = plans
}

func stepScopeConflictKey(binding VisibleBinding) string {
	if binding.ViaStep != "" {
		return "after:" + binding.ViaStep + ":" + binding.Source
	}
	return "with:" + binding.Source
}

func stepScopeConflictRelated(binding VisibleBinding) string {
	if binding.ViaStep != "" {
		return fmt.Sprintf("visible via `after %s`", binding.ViaStep)
	}
	return fmt.Sprintf("imported via `with %s`", binding.Source)
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
				right.Source,
				left.ViaStep,
			),
			"rename the imported variable at the source or avoid importing the same visible name twice"
	case right.ViaStep != "":
		return fmt.Sprintf(
				"conflicting variable '%s' for step '%s': `with` import from '%s' collides with name inherited via `after %s`",
				variable,
				stepName,
				left.Source,
				right.ViaStep,
			),
			"rename the imported variable at the source or avoid importing the same visible name twice"
	default:
		return fmt.Sprintf(
				"conflicting variable '%s' for step '%s': imported via `with` from '%s' and '%s'",
				variable,
				stepName,
				left.Source,
				right.Source,
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
		for _, node := range res.Submits {
			if node.Name != stepName {
				continue
			}
			defs[node.Name] = stepDefinition{
				Name:      node.Name,
				After:     append([]string(nil), node.After...),
				WithItems: append([]ast.WithItem(nil), node.WithItems...),
				Span:      node.Span,
				Snapshot:  snapshotForSubmitBlock(res, node),
			}
			order = append(order, node.Name)
			break
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
