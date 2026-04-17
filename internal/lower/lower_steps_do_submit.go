// lower `do` and `submit` blocks to JUBE `step` entries
//
// handle step dependencies, resolve `submit`'s `use` parameter set,
// rewrite shell references when needed, emit submit parameterset
package lower

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"jbs/internal/ast"
	"jbs/internal/eval"
)

func (ctx *lowerContext) lowerDo(block ast.DoBlock) Step {
	inherits := make([]string, 0)
	inheritVars := make([]string, 0)
	if plan := ctx.res.StepScopeByName[block.Name]; plan != nil && len(plan.InheritedSteps) > 0 {
		inherits = append(inherits, plan.InheritedSteps...)
		inheritVars = slices.Sorted(maps.Keys(plan.Inherited))
	}
	step := Step{
		Name:       block.Name,
		MaxAsync:   block.MaxAsync,
		Procs:      block.Procs,
		Iterations: block.Iterations,
		Meta: StepMeta{
			Kind:          StepKindDo,
			Source:        block.Name,
			InheritsFrom:  inherits,
			InheritedVars: inheritVars,
		},
	}
	if len(block.After) > 0 {
		step.Depend = strings.Join(block.After, ",")
	}
	aliases := ctx.stepAliasMap(block.Name, false)
	resolution := ctx.resolveStepUsesForStep(block.Name, aliases)
	step.Use = resolution.Use
	ctx.stepSourceRows[block.Name] = cloneStringMap(resolution.SourceRows)

	body := normalizeRawLiteral(block.Body)
	body = rewriteShellRefs(body, aliases)
	step.Do = []interface{}{Literal(body)}
	return step
}

func (ctx *lowerContext) addSubmitParameterSet(block ast.SubmitBlock, aliases map[string]string) string {
	name := ctx.uniqueName(fmt.Sprintf("%s__submit_params", block.Name))
	params := make([]Parameter, 0)
	if spec := ctx.res.SubmitByName[block.Name]; spec != nil {
		for _, field := range spec.Values {
			if field.IsRaw {
				raw := normalizeRawLiteral(field.Raw)
				raw = rewriteShellRefs(raw, aliases)
				params = append(params, Parameter{
					Name:  field.Name,
					Mode:  "text",
					Value: Literal(raw),
				})
				continue
			}
			params = append(params, buildSubmitParameter(field.Name, field.Mode, field.Value, aliases, true))
		}
		for _, helper := range spec.Helpers {
			if helper.Aliased == "" {
				continue
			}
			params = append(params, buildSubmitParameter(helper.Aliased, helper.Mode, helper.Value, aliases, false))
		}
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		InitWith:  "platform.xml:systemParameter",
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubmitInit,
			Source: block.Name,
		},
	})
	ctx.names[name] = struct{}{}
	return name
}

func (ctx *lowerContext) lowerSubmit(block ast.SubmitBlock, submitSet string, aliases map[string]string) Step {
	inherits := make([]string, 0)
	inheritVars := make([]string, 0)
	if plan := ctx.res.StepScopeByName[block.Name]; plan != nil && len(plan.InheritedSteps) > 0 {
		inherits = append(inherits, plan.InheritedSteps...)
		inheritVars = slices.Sorted(maps.Keys(plan.Inherited))
	}
	step := Step{
		Name:       block.Name,
		MaxAsync:   block.MaxAsync,
		Procs:      block.Procs,
		Iterations: block.Iterations,
		Meta: StepMeta{
			Kind:          StepKindSubmit,
			Source:        block.Name,
			InheritsFrom:  inherits,
			InheritedVars: inheritVars,
		},
	}
	if len(block.After) > 0 {
		step.Depend = strings.Join(block.After, ",")
	}
	resolution := ctx.resolveStepUsesForStep(block.Name, aliases)
	ctx.stepSourceRows[block.Name] = cloneStringMap(resolution.SourceRows)
	use := append([]interface{}{}, resolution.Use...)
	use = append(use,
		submitSet,
		UseEntry{From: "platform.xml", Value: "jobfiles"},
		UseEntry{From: "platform.xml", Value: "executesub"},
		UseEntry{From: "platform.xml", Value: "executeset"},
	)
	step.Use = use
	step.Do = []interface{}{
		SubmitOperation{
			DoneFile:  "$done_file",
			ErrorFile: "$error_file",
			Command:   `${submit} --parsable ${submit_script} > run.jobid`,
		},
		`echo "true" > success`,
	}
	return step
}

// buildSubmitParameter converts one submit field/helper value into a JUBE parameter payload.
func buildSubmitParameter(name, mode string, value eval.Value, aliases map[string]string, withTypeInference bool) Parameter {
	parameter := Parameter{Name: name}
	if withTypeInference {
		if t := submitParameterType(name); t != "" {
			parameter.Type = t
		}
	}
	value = rewriteShellRefsInEvalValue(value, aliases)
	if mode != "" {
		parameter.Mode = mode
		if mode == "python" {
			parameter.Value = SingleQuoted(asString(value))
		} else {
			parameter.Value = asString(value)
		}
		return parameter
	}
	switch value.Kind {
	case eval.KindList, eval.KindTuple, eval.KindNull:
		parameter.Value = pythonLiteral(value)
	default:
		parameter.Value = templateValue(value)
	}
	return parameter
}

func submitParameterType(name string) string {
	switch name {
	case "nodes", "tasks", "threadspertask":
		return "int"
	default:
		return ""
	}
}
