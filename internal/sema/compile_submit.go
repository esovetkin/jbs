package sema

import (
	"fmt"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/planutil"
)

func compileSubmitBlock(block ast.SubmitBlock, sources map[string]*ImportSource, globals map[string]eval.Value, effective map[string]VarOrigin, diags *diag.Diagnostics) *SubmitSpec {
	env := make(map[string]eval.Value, len(globals)+16)
	for k, v := range globals {
		env[k] = v
	}

	for name, origin := range effective {
		src := sources[origin.Paramset]
		if src == nil {
			continue
		}
		sourceVar := origin.SourceVar
		if sourceVar == "" {
			sourceVar = name
		}
		if vals, ok := src.Vars[sourceVar]; ok {
			env[name] = seriesAsValue(vals)
		}
	}

	spec := &SubmitSpec{
		Name:    block.Name,
		Values:  make([]SubmitValue, 0, len(block.Fields)+len(block.UseNames)*4),
		Helpers: make([]SubmitHelper, 0, len(block.UseNames)*4),
		Span:    block.Span,
	}
	resolved := make(map[string]SubmitValue, len(block.Fields)+len(block.UseNames)*4)
	order := make([]string, 0, len(block.Fields)+len(block.UseNames)*4)
	setValue := func(v SubmitValue) {
		if _, exists := resolved[v.Name]; !exists {
			order = append(order, v.Name)
		}
		resolved[v.Name] = v
	}
	resolvedHelpers := make(map[string]SubmitHelper, len(block.UseNames)*4)
	helperOrder := make([]string, 0, len(block.UseNames)*4)
	setHelper := func(h SubmitHelper) {
		if _, exists := resolvedHelpers[h.Original]; !exists {
			helperOrder = append(helperOrder, h.Original)
		}
		resolvedHelpers[h.Original] = h
	}
	type submitUseOrigin struct {
		useName string
		span    diag.Span
	}
	seenFromUse := make(map[string]submitUseOrigin, len(block.UseNames)*4)
	seenHelperFromUse := make(map[string]submitUseOrigin, len(block.UseNames)*4)
	helperAliasByOriginal := make(map[string]string, len(block.UseNames)*4)
	usedHelperAliases := make(map[string]struct{}, len(block.UseNames)*4)
	helperAlias := func(varName string) string {
		if alias, ok := helperAliasByOriginal[varName]; ok {
			return alias
		}
		base := submitHelperAlias(block.Name, varName)
		alias := base
		for i := 1; ; i++ {
			if _, exists := usedHelperAliases[alias]; !exists {
				usedHelperAliases[alias] = struct{}{}
				helperAliasByOriginal[varName] = alias
				return alias
			}
			alias = fmt.Sprintf("%s_%d", base, i)
		}
	}

	for _, useName := range block.UseNames {
		src := sources[useName]
		if src == nil {
			diags.AddError(
				diag.CodeE078,
				fmt.Sprintf("unknown submit use namespace '%s'", useName),
				block.Span,
				"use an existing let namespace in submit header use clause",
			)
			continue
		}
		if src.Kind != SourceKindLet {
			diags.AddError(
				diag.CodeE071,
				fmt.Sprintf("submit use source '%s' must be a let namespace", useName),
				block.Span,
				"use a let namespace in submit header use clause",
			)
			continue
		}
		for _, varName := range planutil.SourceVarNames(src.Order, src.Vars) {
			if _, ok := allowedSubmitKeys[varName]; !ok {
				vals := src.Vars[varName]
				value := eval.Null()
				if len(vals) > 0 {
					value = vals[0]
				}
				origin := src.Origins[varName]
				if origin.IsZero() {
					origin = src.Span
				}
				if prev, exists := seenHelperFromUse[varName]; exists && prev.useName != useName {
					diags.AddWarning(
						diag.CodeW072,
						fmt.Sprintf("submit helper '%s' is defined in multiple use namespaces ('%s', '%s'); last wins ('%s')", varName, prev.useName, useName, useName),
						origin,
						"merge defaults explicitly or keep one namespace per helper variable",
						diag.RelatedSpan{Message: "first definition", Span: prev.span},
					)
				}
				seenHelperFromUse[varName] = submitUseOrigin{
					useName: useName,
					span:    origin,
				}
				setHelper(SubmitHelper{
					Original: varName,
					Aliased:  helperAlias(varName),
					Mode:     src.Modes[varName],
					Value:    value,
					Span:     origin,
					UseName:  useName,
				})
				continue
			}
			if isRawSubmitKey(varName) {
				origin := src.Origins[varName]
				if origin.IsZero() {
					origin = src.Span
				}
				diags.AddWarning(
					diag.CodeW071,
					fmt.Sprintf("submit default '%s' from let '%s' is ignored (raw-block key)", varName, useName),
					origin,
					"set raw-block submit keys directly in submit body",
				)
				continue
			}
			vals := src.Vars[varName]
			value := eval.Null()
			if len(vals) > 0 {
				value = vals[0]
			}
			span := src.Origins[varName]
			if span.IsZero() {
				span = src.Span
			}
			if prev, exists := seenFromUse[varName]; exists && prev.useName != useName {
				diags.AddWarning(
					diag.CodeW072,
					fmt.Sprintf("submit default '%s' is defined in multiple use namespaces ('%s', '%s'); last wins ('%s')", varName, prev.useName, useName, useName),
					span,
					"merge defaults explicitly or keep one namespace per submit key",
					diag.RelatedSpan{Message: "first definition", Span: prev.span},
				)
			}
			seenFromUse[varName] = submitUseOrigin{
				useName: useName,
				span:    span,
			}
			setValue(SubmitValue{
				Name:  varName,
				Mode:  src.Modes[varName],
				Value: value,
				Span:  span,
			})
		}
	}

	seen := make(map[string]diag.Span)
	for _, field := range block.Fields {
		if _, ok := allowedSubmitKeys[field.Name]; !ok {
			diags.AddError(
				diag.CodeE072,
				fmt.Sprintf("unknown submit key '%s'", field.Name),
				field.Span,
				"use one of the allowed submit keys",
			)
			continue
		}
		if prev, exists := seen[field.Name]; exists {
			diags.AddError(
				diag.CodeE075,
				fmt.Sprintf("duplicate submit key '%s'", field.Name),
				field.Span,
				"set each submit key at most once",
				diag.RelatedSpan{Message: "first assignment", Span: prev},
			)
			continue
		}
		seen[field.Name] = field.Span

		if field.IsRaw {
			if !isRawSubmitKey(field.Name) {
				diags.AddError(
					diag.CodeE074,
					fmt.Sprintf("submit key '%s' does not accept raw blocks", field.Name),
					field.Span,
					"use an expression value for this key",
				)
				continue
			}
			setValue(SubmitValue{
				Name:  field.Name,
				Raw:   field.Raw,
				IsRaw: true,
				Span:  field.Span,
			})
			continue
		}

		if isRawSubmitKey(field.Name) {
			diags.AddError(
				diag.CodeE073,
				fmt.Sprintf("submit key '%s' must use a raw block", field.Name),
				field.Span,
				fmt.Sprintf("use syntax: %s = { ... }", field.Name),
			)
			continue
		}
		if field.Expr == nil {
			diags.AddError(
				diag.CodeE076,
				fmt.Sprintf("submit key '%s' is missing a value expression", field.Name),
				field.Span,
				"use syntax: key = expression",
			)
			continue
		}
		warnModeExprInCollections(field.Expr, diags)

		mode, inner, isModeExpr := unwrapModeExpr(field.Expr)
		expr := field.Expr
		if isModeExpr {
			expr = inner
		}
		value := eval.EvalExpr(expr, env, diags)
		if isModeExpr {
			value = coerceModeValue(mode, value, field.Span, diags)
		}
		if hasNestedList(value) {
			diags.AddError(
				diag.CodeE305,
				fmt.Sprintf("nested tuple/list value is not allowed for submit key '%s'", field.Name),
				field.Span,
				"use flat tuple/list values only",
			)
		}
		setValue(SubmitValue{
			Name:  field.Name,
			Mode:  mode,
			Value: value,
			Span:  field.Span,
		})
	}
	if _, hasTasks := resolved["tasks"]; !hasTasks {
		if nodes, hasNodes := resolved["nodes"]; hasNodes {
			setValue(SubmitValue{
				Name:  "tasks",
				Mode:  nodes.Mode,
				Value: nodes.Value,
				Span:  nodes.Span,
			})
		} else {
			setValue(SubmitValue{
				Name:  "tasks",
				Value: eval.String("$nodes"),
				Span:  block.Span,
			})
		}
	}
	accountEmptyOrMissing, accountSpan := submitKeyMissingOrEmpty(resolved, "account", block.Span)
	if accountEmptyOrMissing {
		diags.AddWarning(
			diag.CodeW073,
			"submit key 'account' is missing or empty",
			accountSpan,
			"set a non-empty account",
		)
	}
	queueEmptyOrMissing, queueSpan := submitKeyMissingOrEmpty(resolved, "queue", block.Span)
	if queueEmptyOrMissing {
		diags.AddWarning(
			diag.CodeW073,
			"submit key 'queue' is missing or empty",
			queueSpan,
			"set a non-empty queue",
		)
	}
	executableEmptyOrMissing, executableSpan := submitKeyMissingOrEmpty(resolved, "executable", block.Span)
	argsExecEmptyOrMissing, argsExecSpan := submitKeyMissingOrEmpty(resolved, "args_exec", block.Span)
	if executableEmptyOrMissing && argsExecEmptyOrMissing {
		starterEmptyOrMissing, _ := submitKeyMissingOrEmpty(resolved, "starter", block.Span)
		if !starterEmptyOrMissing {
			primary := argsExecSpan
			if primary.IsZero() {
				primary = executableSpan
			}
			related := []diag.RelatedSpan{}
			if !executableSpan.IsZero() && executableSpan != primary {
				related = append(related, diag.RelatedSpan{
					Message: "executable is missing or empty",
					Span:    executableSpan,
				})
			}
			if !argsExecSpan.IsZero() && argsExecSpan != primary {
				related = append(related, diag.RelatedSpan{
					Message: "args_exec is missing or empty",
					Span:    argsExecSpan,
				})
			}
			diags.AddWarning(
				diag.CodeW074,
				"submit keys 'executable' and 'args_exec' are both missing or empty",
				primary,
				"set at least one of executable or args_exec to a non-empty value",
				related...,
			)
		}
	}
	for _, name := range order {
		spec.Values = append(spec.Values, resolved[name])
	}
	for _, name := range helperOrder {
		spec.Helpers = append(spec.Helpers, resolvedHelpers[name])
	}
	return spec
}

func submitHelperAlias(stepName, varName string) string {
	return "_jk__" + sanitizeSubmitHelperPart(stepName) + "_" + sanitizeSubmitHelperPart(varName)
}

func sanitizeSubmitHelperPart(s string) string {
	if s == "" {
		return "x"
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

func submitValueHasEmptyString(v SubmitValue) bool {
	if v.IsRaw {
		return false
	}
	return evalValueHasEmptyString(v.Value)
}

func submitKeyMissingOrEmpty(resolved map[string]SubmitValue, key string, fallback diag.Span) (bool, diag.Span) {
	v, ok := resolved[key]
	if !ok {
		return true, fallback
	}
	return submitValueHasEmptyString(v), v.Span
}

func evalValueHasEmptyString(v eval.Value) bool {
	switch v.Kind {
	case eval.KindString:
		return v.S == ""
	case eval.KindList:
		if len(v.L) == 0 {
			return true
		}
		for _, item := range v.L {
			if item.Kind != eval.KindString || item.S != "" {
				return false
			}
		}
		return true
	}
	return false
}

func isRawSubmitKey(name string) bool {
	return name == "preprocess" || name == "postprocess"
}
