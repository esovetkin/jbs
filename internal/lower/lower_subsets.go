// build synthetic subset parameter sets used by steps
//
// build subset/index/rows helper parameters, handles direct vs
// inherited row selection modes
package lower

import (
	"strings"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

func (ctx *lowerContext) ensureSubsetParameterSetForStep(stepName, source string, vars []subsetVarSpec, full bool, inherited sourceRowContext) (string, sourceRowContext) {
	varKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		varKeys = append(varKeys, v.Visible+"="+v.SourceVar+"=>"+subsetEmittedName(v))
	}
	k := subsetKey{
		Step:          stepName,
		Source:        source,
		Vars:          strings.Join(varKeys, ","),
		Full:          full,
		InheritedRows: sourceRowContextKey(inherited),
	}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing.Name, cloneSourceRowContext(existing.RowContext)
	}

	src := ctx.res.BindingsByName[source]
	if src == nil {
		// Semantic analysis already reports unknown parameter set imports with
		// precise spans. Skip lower-stage duplicate diagnostics.
		return "", sourceRowContext{}
	}

	rowCount := planutil.SourceRowCount(src.Order, src.Vars)
	if rowCount == 0 {
		rowCount = 1
	}

	visibleNames := make([]string, 0, len(vars))
	emittedByVisible := make(map[string]string, len(vars))
	for _, v := range vars {
		visibleNames = append(visibleNames, v.Visible)
		emittedByVisible[v.Visible] = subsetEmittedName(v)
	}

	baseName := shortSubsetBaseName(stepName, source, visibleNames)
	name := ctx.uniqueName(baseName)
	suffix := strings.TrimPrefix(name, baseName)
	rowsVar := shortSubsetRowsName(stepName, source, visibleNames) + suffix
	idxName := shortSubsetIndexName(stepName, source, visibleNames) + suffix
	idxRef := "$" + idxName

	valuesByName := make(map[string][]eval.Value, len(vars))
	modeByName := make(map[string]string, len(vars))
	sourceVarByVisible := make(map[string]string, len(vars))
	for _, variable := range vars {
		sourceVar := variable.SourceVar
		if sourceVar == "" {
			sourceVar = variable.Visible
		}
		sourceVarByVisible[variable.Visible] = sourceVar
		valuesByName[variable.Visible] = planutil.ExpandValues(src.Vars[sourceVar], rowCount)
		if mode, ok := src.Modes[sourceVar]; ok {
			modeByName[variable.Visible] = mode
		}
	}

	params := make([]Parameter, 0, len(vars)+2)
	var rowContext sourceRowContext
	if inherited.VarName == "" {
		groups := planutil.BuildProjectedRowGroups(planutil.SequentialIndices(rowCount), visibleNames, valuesByName, full, pythonLiteral)
		repIndices := make([]int, 0, len(groups))
		rowGroupStrings := make([]string, 0, len(groups))
		for _, group := range groups {
			repIndices = append(repIndices, group.Rep)
			rowGroupStrings = append(rowGroupStrings, joinIntIndices(group.Rows))
		}
		if len(repIndices) == 0 {
			repIndices = []int{0}
			rowGroupStrings = []string{"0"}
		}
		params = append(params, Parameter{
			Name:  idxName,
			Type:  "int",
			Mode:  "text",
			Value: joinIntIndices(repIndices),
		})
		params = append(params, Parameter{
			Name: rowsVar,
			Mode: "python",
			// Keep row groups like "0,1" opaque at this stage so they are
			// transported as one value across step dependencies.
			Separator: ReservedSeparator,
			Value:     SingleQuoted(pythonStringMapLookupExpr(repIndices, rowGroupStrings, idxName)),
		})
		payload := lowerIndexedPayloadParameters(visibleNames, valuesByName, modeByName, repIndices, idxRef, func(varName string) diag.Span {
			sourceVar := sourceVarByVisible[varName]
			if span, ok := src.Origins[sourceVar]; ok {
				return span
			}
			return src.Span
		}, ctx.diags)
		params = append(params, applyEmittedNames(payload, emittedByVisible)...)
		rowContext = sourceRowContext{
			VarName: rowsVar,
			Groups:  rowGroupStrings,
		}
	} else {
		parentGroups, parentValues, repIndices, rowGroupStrings := buildInheritedProjectedGroups(inherited.Groups, visibleNames, valuesByName, rowCount, full)
		params = append(params, Parameter{
			Name:      idxName,
			Type:      "int",
			Mode:      "python",
			Separator: ",",
			Value:     SingleQuoted(pythonStringLookupExpr(parentGroups, parentValues, inherited.VarName)),
		})
		params = append(params, Parameter{
			Name:      rowsVar,
			Mode:      "python",
			Separator: ReservedSeparator,
			Value:     SingleQuoted(pythonStringMapLookupExpr(repIndices, rowGroupStrings, idxName)),
		})
		payload := lowerIndexedPayloadParameters(visibleNames, valuesByName, modeByName, repIndices, idxRef, func(varName string) diag.Span {
			sourceVar := sourceVarByVisible[varName]
			if span, ok := src.Origins[sourceVar]; ok {
				return span
			}
			return src.Span
		}, ctx.diags)
		params = append(params, applyEmittedNames(payload, emittedByVisible)...)
		rowContext = sourceRowContext{
			VarName: rowsVar,
			Groups:  rowGroupStrings,
		}
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubset,
			Source: source,
			Step:   stepName,
		},
	})
	ctx.names[name] = struct{}{}
	ctx.subsetNames[k] = subsetInfo{Name: name, RowContext: rowContext}
	return name, cloneSourceRowContext(rowContext)
}

func (ctx *lowerContext) ensureScalarLetSubsetParameterSetForStep(stepName, source string, vars []subsetVarSpec) (string, sourceRowContext) {
	varKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		varKeys = append(varKeys, v.Visible+"="+v.SourceVar+"=>"+subsetEmittedName(v))
	}
	k := subsetKey{
		Step:          stepName,
		Source:        source,
		Vars:          strings.Join(varKeys, ","),
		Full:          false,
		InheritedRows: "",
	}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing.Name, cloneSourceRowContext(existing.RowContext)
	}

	src := ctx.res.BindingsByName[source]
	if src == nil {
		return "", sourceRowContext{}
	}

	visibleNames := make([]string, 0, len(vars))
	for _, v := range vars {
		visibleNames = append(visibleNames, v.Visible)
	}
	baseName := shortSubsetBaseName(stepName, source, visibleNames)
	name := ctx.uniqueName(baseName)

	params := make([]Parameter, 0, len(vars))
	for _, variable := range vars {
		sourceVar := variable.SourceVar
		if sourceVar == "" {
			sourceVar = variable.Visible
		}
		vals := planutil.ExpandValues(src.Vars[sourceVar], 1)
		value := eval.Null()
		if len(vals) > 0 {
			value = vals[0]
		}
		parameter := Parameter{Name: subsetEmittedName(variable)}
		if mode, ok := src.Modes[sourceVar]; ok && mode != "" {
			parameter.Mode = mode
			switch mode {
			case "python":
				parameter.Value = SingleQuoted(asString(value))
			default:
				parameter.Value = asString(value)
			}
		} else {
			parameter.Mode = "text"
			parameter.Value = asString(value)
		}
		applyScalarStringSeparator(&parameter, value)
		params = append(params, parameter)
	}

	ctx.doc.ParameterSet = append(ctx.doc.ParameterSet, ParameterSet{
		Name:      name,
		Parameter: params,
		Meta: ParameterSetMeta{
			Kind:   ParameterSetKindSubset,
			Source: source,
			Step:   stepName,
		},
	})
	ctx.names[name] = struct{}{}
	ctx.subsetNames[k] = subsetInfo{Name: name}
	return name, sourceRowContext{}
}

func sourceRowContextKey(in sourceRowContext) string {
	if in.VarName == "" {
		return ""
	}
	return in.VarName + "|" + strings.Join(in.Groups, ReservedSeparator)
}

func buildInheritedProjectedGroups(parentGroups []string, visibleNames []string, valuesByName map[string][]eval.Value, rowCount int, full bool) ([]string, []string, []int, []string) {
	parentValues := make([]string, 0, len(parentGroups))
	repIndices := make([]int, 0)
	rowGroupStrings := make([]string, 0)
	for _, parentGroup := range parentGroups {
		allowedRows := make([]int, 0)
		for _, idx := range parseIntIndices(parentGroup) {
			if idx < 0 || idx >= rowCount {
				continue
			}
			allowedRows = append(allowedRows, idx)
		}
		projected := planutil.BuildProjectedRowGroups(allowedRows, visibleNames, valuesByName, full, pythonLiteral)
		parentRepIndices := make([]int, 0, len(projected))
		for _, group := range projected {
			parentRepIndices = append(parentRepIndices, group.Rep)
			repIndices = append(repIndices, group.Rep)
			rowGroupStrings = append(rowGroupStrings, joinIntIndices(group.Rows))
		}
		parentValues = append(parentValues, joinIntIndices(parentRepIndices))
	}
	return append([]string(nil), parentGroups...), parentValues, repIndices, rowGroupStrings
}
