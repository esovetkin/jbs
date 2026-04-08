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
	if plan := ctx.res.StepImportByName[block.Name]; plan != nil && len(plan.InheritedSteps) > 0 {
		inherits = append(inherits, plan.InheritedSteps...)
		inheritVars = slices.Sorted(maps.Keys(plan.Inherited))
	}
	step := Step{
		Name:       block.Name,
		MaxAsync:   block.MaxAsync,
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

			param := Parameter{Name: field.Name}
			if t := submitParameterType(field.Name); t != "" {
				param.Type = t
			}
			value := rewriteShellRefsInEvalValue(field.Value, aliases)
			if field.Mode != "" {
				param.Mode = field.Mode
				if field.Mode == "python" {
					param.Value = SingleQuoted(asString(value))
				} else {
					param.Value = asString(value)
				}
			} else {
				switch value.Kind {
				case eval.KindList, eval.KindNull:
					param.Value = pythonLiteral(value)
				default:
					param.Value = templateValue(value)
				}
			}
			params = append(params, param)
		}
		for _, helper := range spec.Helpers {
			if helper.Aliased == "" {
				continue
			}
			param := Parameter{Name: helper.Aliased}
			value := rewriteShellRefsInEvalValue(helper.Value, aliases)
			if helper.Mode != "" {
				param.Mode = helper.Mode
				if helper.Mode == "python" {
					param.Value = SingleQuoted(asString(value))
				} else {
					param.Value = asString(value)
				}
			} else {
				switch value.Kind {
				case eval.KindList, eval.KindNull:
					param.Value = pythonLiteral(value)
				default:
					param.Value = templateValue(value)
				}
			}
			params = append(params, param)
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
	if plan := ctx.res.StepImportByName[block.Name]; plan != nil && len(plan.InheritedSteps) > 0 {
		inherits = append(inherits, plan.InheritedSteps...)
		inheritVars = slices.Sorted(maps.Keys(plan.Inherited))
	}
	step := Step{
		Name:       block.Name,
		MaxAsync:   block.MaxAsync,
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

func submitParameterType(name string) string {
	switch name {
	case "nodes", "tasks", "threadspertask":
		return "int"
	default:
		return ""
	}
}
