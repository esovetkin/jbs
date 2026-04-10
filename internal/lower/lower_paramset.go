package lower

import (
	"fmt"
	"strconv"
	"strings"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
	"jbs/internal/sema"
)

func lowerParamset(ps *sema.Paramset, diags *diag.Diagnostics) ParameterSet {
	out := ParameterSet{
		Name:      ps.Name,
		Parameter: make([]Parameter, 0),
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindParam,
			Source: ps.Name,
		},
	}

	rowCount := len(ps.Rows)
	if rowCount == 0 {
		for _, name := range ps.Order {
			if n := len(ps.Vars[name]); n > rowCount {
				rowCount = n
			}
		}
	}
	if rowCount == 0 {
		diags.AddError(
			diag.CodeE230,
			fmt.Sprintf("parameterset '%s' evaluates to zero rows", ps.Name),
			ps.Block.Span,
			"ensure final expression yields at least one row",
		)
		rowCount = 1
	}

	valuesByName := make(map[string][]eval.Value, len(ps.Order))
	for _, name := range ps.Order {
		valuesByName[name] = valuesFor(ps, name, rowCount)
	}

	indices := planutil.SequentialIndices(rowCount)
	out.Parameter = lowerIndexedParameters(ps.Order, valuesByName, ps.Modes, indices, indexVariableName(ps.Name), func(name string) diag.Span {
		return originFor(ps, name)
	}, diags)
	return out
}

func lowerIndexedParameters(
	order []string,
	valuesByName map[string][]eval.Value,
	modes map[string]string,
	indices []int,
	idxName string,
	origin func(name string) diag.Span,
	diags *diag.Diagnostics,
) []Parameter {
	if len(indices) == 0 {
		indices = []int{0}
	}
	if idxName == "" {
		idxName = indexVariableName("set")
	}
	idxRef := "$" + idxName

	params := make([]Parameter, 0, len(order)+1)
	params = append(params, Parameter{
		Name:  idxName,
		Type:  "int",
		Mode:  "text",
		Value: joinIntIndices(indices),
	})
	params = append(params, lowerIndexedPayloadParameters(order, valuesByName, modes, indices, idxRef, origin, diags)...)
	return params
}

func lowerIndexedPayloadParameters(
	order []string,
	valuesByName map[string][]eval.Value,
	modes map[string]string,
	indices []int,
	idxRef string,
	origin func(name string) diag.Span,
	diags *diag.Diagnostics,
) []Parameter {
	params := make([]Parameter, 0, len(order))
	for _, name := range order {
		fullValues := valuesByName[name]
		selectedValues := pickValuesAtIndices(fullValues, indices)
		if len(fullValues) == 0 {
			fullValues = []eval.Value{eval.Null()}
		}
		if len(selectedValues) == 0 {
			selectedValues = []eval.Value{fullValues[0]}
		}

		if mode := modes[name]; mode != "" {
			param := Parameter{Name: name, Mode: mode}
			switch mode {
			case "python":
				if allEqualValues(selectedValues) {
					param.Value = SingleQuoted(asString(selectedValues[0]))
				} else {
					param.Value = SingleQuoted(pythonIndexExpr(fullValues, idxRef))
				}
			case "shell":
				if !allEqualValues(selectedValues) {
					diags.AddError(
						diag.CodeE231,
						fmt.Sprintf("%s(...) parameter '%s' cannot vary across indexed rows", mode, name),
						origin(name),
						"use a single expression value for mode-declared parameters",
					)
				}
				param.Value = asString(selectedValues[0])
			default:
				param.Value = asString(selectedValues[0])
			}
			params = append(params, param)
			continue
		}

		params = append(params, Parameter{
			Name:  name,
			Mode:  "python",
			Value: SingleQuoted(pythonIndexExpr(fullValues, idxRef)),
		})
	}
	return params
}

func lowerContextualPayloadParameters(
	order []string,
	valuesByName map[string][]eval.Value,
	modes map[string]string,
	idxRef string,
	origin func(name string) diag.Span,
	diags *diag.Diagnostics,
) []Parameter {
	params := make([]Parameter, 0, len(order))
	for _, name := range order {
		fullValues := valuesByName[name]
		if len(fullValues) == 0 {
			fullValues = []eval.Value{eval.Null()}
		}
		if mode := modes[name]; mode != "" {
			param := Parameter{Name: name, Mode: mode}
			switch mode {
			case "python":
				if allEqualValues(fullValues) {
					param.Value = SingleQuoted(asString(fullValues[0]))
				} else {
					param.Value = SingleQuoted(pythonIndexExpr(fullValues, idxRef))
				}
			case "shell":
				if !allEqualValues(fullValues) {
					diags.AddError(
						diag.CodeE231,
						fmt.Sprintf("%s(...) parameter '%s' cannot vary across indexed rows", mode, name),
						origin(name),
						"use a single expression value for mode-declared parameters",
					)
				}
				param.Value = asString(fullValues[0])
			default:
				param.Value = asString(fullValues[0])
			}
			params = append(params, param)
			continue
		}

		params = append(params, Parameter{
			Name:  name,
			Mode:  "python",
			Value: SingleQuoted(pythonIndexExpr(fullValues, idxRef)),
		})
	}
	return params
}

func joinIntIndices(indices []int) string {
	if len(indices) == 0 {
		return ""
	}
	out := make([]string, len(indices))
	for i, idx := range indices {
		out[i] = strconv.Itoa(idx)
	}
	return strings.Join(out, ",")
}

func pickValuesAtIndices(values []eval.Value, indices []int) []eval.Value {
	if len(indices) == 0 {
		return nil
	}
	out := make([]eval.Value, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(values) {
			out = append(out, values[idx])
			continue
		}
		out = append(out, eval.Null())
	}
	return out
}

func originFor(ps *sema.Paramset, name string) diag.Span {
	if s, ok := ps.Origins[name]; ok {
		return s
	}
	return ps.Block.Span
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
		for range rowCount {
			values = append(values, eval.Null())
		}
		return values
	}
	values = values[:0]
	for i := range rowCount {
		values = append(values, base[i%len(base)])
	}
	return values
}

func allEqualValues(values []eval.Value) bool {
	if len(values) <= 1 {
		return true
	}
	first := values[0]
	for i := 1; i < len(values); i++ {
		if !eval.Equal(first, values[i]) {
			return false
		}
	}
	return true
}

func asString(v eval.Value) string {
	if v.Kind == eval.KindString {
		return v.S
	}
	return v.String()
}

func templateValue(v eval.Value) string {
	switch v.Kind {
	case eval.KindInt:
		return strconv.FormatInt(v.I, 10)
	case eval.KindFloat:
		return strconv.FormatFloat(v.F, 'g', -1, 64)
	case eval.KindString:
		return v.S
	case eval.KindBool:
		if v.B {
			return "true"
		}
		return "false"
	default:
		return pythonLiteral(v)
	}
}

func pythonIndexExpr(values []eval.Value, indexVar string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, pythonLiteral(value))
	}
	return "[" + strings.Join(parts, ",") + "][" + indexVar + "]"
}

func pythonStringMapLookupExpr(keys []int, values []string, varName string) string {
	parts := make([]string, 0, len(keys))
	for i := range keys {
		key := strconv.Quote(strconv.Itoa(keys[i]))
		value := ""
		if i < len(values) {
			value = values[i]
		}
		parts = append(parts, key+":"+strconv.Quote(value))
	}
	return "{" + strings.Join(parts, ",") + "}" + "[\"${" + varName + "}\"]"
}

func pythonLiteral(v eval.Value) string {
	switch v.Kind {
	case eval.KindNull:
		return "None"
	case eval.KindInt:
		return strconv.FormatInt(v.I, 10)
	case eval.KindFloat:
		return strconv.FormatFloat(v.F, 'g', -1, 64)
	case eval.KindString:
		return strconv.Quote(v.S)
	case eval.KindBool:
		if v.B {
			return "True"
		}
		return "False"
	case eval.KindList:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, pythonLiteral(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case eval.KindTuple:
		parts := make([]string, 0, len(v.L))
		for _, item := range v.L {
			parts = append(parts, pythonLiteral(item))
		}
		if len(parts) == 1 {
			return "(" + parts[0] + ",)"
		}
		return "(" + strings.Join(parts, ",") + ")"
	default:
		return strconv.Quote(v.String())
	}
}
