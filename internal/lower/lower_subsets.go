package lower

import (
	"strings"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
	"jbs/internal/sema"
)

func (ctx *lowerContext) ensureSubsetParameterSetForStep(stepName, source string, vars []subsetVarSpec, inheritedRowsVar string) (string, string) {
	varKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		varKeys = append(varKeys, v.Visible+"="+v.SourceVar+"=>"+subsetEmittedName(v))
	}
	k := subsetKey{
		Step:          stepName,
		Source:        source,
		Vars:          strings.Join(varKeys, ","),
		InheritedRows: inheritedRowsVar,
	}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing.Name, existing.RowsVar
	}

	src := ctx.res.ImportSourceByName[source]
	if src == nil {
		// Semantic analysis already reports unknown parameter set imports with
		// precise spans. Skip lower-stage duplicate diagnostics.
		return "", ""
	}

	rowCount := sourceRowCountFromSource(src)
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
		valuesByName[variable.Visible] = sourceValuesFor(src, sourceVar, rowCount)
		if mode, ok := src.Modes[sourceVar]; ok {
			modeByName[variable.Visible] = mode
		}
	}

	params := make([]Parameter, 0, len(vars)+2)
	if inheritedRowsVar == "" {
		groups := planutil.BuildRowGroups(visibleNames, valuesByName, rowCount, pythonLiteral)
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
	} else {
		params = append(params, Parameter{
			Name: idxName,
			Type: "int",
			Mode: "text",
			// In inherited context we intentionally split grouped row IDs.
			Separator: ",",
			Value:     "$" + inheritedRowsVar,
		})
		params = append(params, Parameter{
			Name:  rowsVar,
			Mode:  "text",
			Value: "${" + idxName + "}",
		})
		payload := lowerContextualPayloadParameters(visibleNames, valuesByName, modeByName, idxRef, func(varName string) diag.Span {
			sourceVar := sourceVarByVisible[varName]
			if span, ok := src.Origins[sourceVar]; ok {
				return span
			}
			return src.Span
		}, ctx.diags)
		params = append(params, applyEmittedNames(payload, emittedByVisible)...)
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
	ctx.subsetNames[k] = subsetInfo{Name: name, RowsVar: rowsVar}
	return name, rowsVar
}

func (ctx *lowerContext) ensureScalarLetSubsetParameterSetForStep(stepName, source string, vars []subsetVarSpec) (string, string) {
	varKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		varKeys = append(varKeys, v.Visible+"="+v.SourceVar+"=>"+subsetEmittedName(v))
	}
	k := subsetKey{
		Step:          stepName,
		Source:        source,
		Vars:          strings.Join(varKeys, ","),
		InheritedRows: "",
	}
	if existing, ok := ctx.subsetNames[k]; ok {
		return existing.Name, existing.RowsVar
	}

	src := ctx.res.ImportSourceByName[source]
	if src == nil {
		return "", ""
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
		vals := sourceValuesFor(src, sourceVar, 1)
		value := eval.Null()
		if len(vals) > 0 {
			value = vals[0]
		}
		param := Parameter{Name: subsetEmittedName(variable)}
		if mode, ok := src.Modes[sourceVar]; ok && mode != "" {
			param.Mode = mode
			switch mode {
			case "python":
				param.Value = SingleQuoted(asString(value))
			default:
				param.Value = asString(value)
			}
		} else {
			param.Mode = "text"
			param.Value = asString(value)
		}
		params = append(params, param)
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
	ctx.subsetNames[k] = subsetInfo{Name: name, RowsVar: ""}
	return name, ""
}

func sourceRowCountFromSource(src *sema.ImportSource) int {
	if src == nil {
		return 0
	}
	return planutil.SourceRowCount(src.Order, src.Vars)
}

func sourceValuesFor(src *sema.ImportSource, name string, rowCount int) []eval.Value {
	if src == nil {
		return nil
	}
	return planutil.ExpandValues(src.Vars[name], rowCount)
}
