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
}

func buildStepImportPlans(res *Result, diags *diag.Diagnostics) {
	defs, order := collectStepDefinitions(res)
	plans := make(map[string]*StepImportPlan, len(defs))
	for _, stepName := range planutil.TopoStepOrder(stepDefinitionDeps(defs), order) {
		def, ok := defs[stepName]
		if !ok {
			continue
		}
		reported := make(map[string]struct{})
		reportConflict := func(name string, left VarOrigin, right VarOrigin, at diag.Span, relation string) {
			a := left.Paramset
			b := right.Paramset
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
			diags.AddError(
				diag.CodeE214,
				fmt.Sprintf(
					"conflicting variable '%s' for step '%s' from parametersets '%s' and '%s'",
					name,
					stepName,
					left.Paramset,
					right.Paramset,
				),
				at,
				"import each variable name from only one source parameterset",
				diag.RelatedSpan{Message: "first conflicting source", Span: left.Span},
				diag.RelatedSpan{Message: "second conflicting source", Span: right.Span},
			)
		}

		inherited := make(map[string]VarOrigin)
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
				if prev, exists := inherited[name]; exists {
					if prev.Paramset != origin.Paramset {
						reportConflict(name, prev, origin, def.Span, "inherited")
					}
					continue
				}
				inherited[name] = origin
			}
		}

		resolver := WithResolver{
			Params:  res.ParamByName,
			Lets:    res.LetByName,
			Sources: res.ImportSourceByName,
		}
		expandedWith, _ := resolver.ExpandWithItems(def.WithItems, WithResolveOptions{
			AllowParam:                true,
			AllowLet:                  true,
			EnableMixedSourceFallback: true,
			DetectAmbiguousSource:     true,
		})

		explicitDelta := make([]PlannedImport, 0)
		selected := make(map[string]VarOrigin)
		for _, expanded := range expandedWith {
			kept := make([]ExpandedWithVar, 0, len(expanded.Vars))
			sourceObj := res.ImportSourceByName[expanded.Source]
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
				current := VarOrigin{
					Name:      name,
					SourceVar: v.SourceVar,
					Paramset:  expanded.Source,
					Kind:      expanded.Kind,
					Span:      originSpan,
				}
				if prev, exists := inherited[name]; exists {
					if prev.Paramset != current.Paramset {
						reportConflict(name, prev, current, expanded.Span, "explicit_vs_inherited")
					}
					continue
				}
				if prev, exists := selected[name]; exists {
					if prev.Paramset != current.Paramset {
						// Explicit-with conflicts are already diagnosed by validateWithItems.
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
				explicitDelta = append(explicitDelta, PlannedImport{
					Source: expanded.Source,
					Kind:   expanded.Kind,
					Full:   true,
					Span:   expanded.Span,
				})
				continue
			}
			for _, keptVar := range kept {
				explicitDelta = append(explicitDelta, PlannedImport{
					Source:    expanded.Source,
					Kind:      expanded.Kind,
					Visible:   keptVar.Visible,
					SourceVar: keptVar.SourceVar,
					Span:      expanded.Span,
				})
			}
		}
		effective := make(map[string]VarOrigin, len(inherited)+len(selected))
		for name, origin := range inherited {
			effective[name] = origin
		}
		for name, origin := range selected {
			if prev, exists := effective[name]; exists {
				if prev.Paramset != origin.Paramset {
					reportConflict(name, prev, origin, def.Span, "effective")
					continue
				}
			}
			effective[name] = origin
		}

		plans[stepName] = &StepImportPlan{
			StepName:       stepName,
			Inherited:      inherited,
			ExplicitDelta:  explicitDelta,
			Effective:      effective,
			InheritedSteps: inheritedSteps,
		}
	}
	res.StepImportByName = plans
}

func collectStepDefinitions(res *Result) (map[string]stepDefinition, []string) {
	defs := make(map[string]stepDefinition)
	order := make([]string, 0)
	for _, stmt := range res.Program.Stmts {
		switch node := stmt.(type) {
		case ast.DoBlock:
			if _, exists := defs[node.Name]; exists {
				continue
			}
			defs[node.Name] = stepDefinition{
				Name:      node.Name,
				After:     append([]string(nil), node.After...),
				WithItems: append([]ast.WithItem(nil), node.WithItems...),
				Span:      node.Span,
			}
			order = append(order, node.Name)
		case ast.SubmitBlock:
			if _, exists := defs[node.Name]; exists {
				continue
			}
			defs[node.Name] = stepDefinition{
				Name:      node.Name,
				After:     append([]string(nil), node.After...),
				WithItems: append([]ast.WithItem(nil), node.WithItems...),
				Span:      node.Span,
			}
			order = append(order, node.Name)
		}
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
