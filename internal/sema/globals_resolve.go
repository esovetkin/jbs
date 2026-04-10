package sema

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func resolveTopLevelGlobals(prog ast.Program, defaults map[string]eval.Value, diags *diag.Diagnostics) GlobalState {
	values := make(map[string]eval.Value, len(defaults))
	for k, v := range defaults {
		values[k] = v
	}
	modes := make(map[string]string)
	spans := make(map[string]diag.Span)
	known := make(map[string]struct{}, len(defaults))
	for name := range defaults {
		known[name] = struct{}{}
	}

	for _, stmt := range prog.Stmts {
		assign, ok := stmt.(ast.GlobalAssign)
		if !ok {
			continue
		}
		if _, ok := known[assign.Name]; !ok {
			diags.AddError(
				diag.CodeE300,
				fmt.Sprintf("unknown global variable '%s'", assign.Name),
				assign.Span,
				"use `jbs help globals` to list supported globals",
			)
			continue
		}
		effectiveExpr := assignmentExpr(assign.Name, assign.Op, assign.Expr, assign.Span)
		warnModeExprInCollections(effectiveExpr, diags)
		if prev, exists := spans[assign.Name]; exists {
			diags.AddWarning(
				diag.CodeW300,
				fmt.Sprintf("global variable '%s' reassigned; last value wins", assign.Name),
				assign.Span,
				"remove duplicate assignments to avoid ambiguity",
				diag.RelatedSpan{Message: "previous assignment", Span: prev},
			)
		}
		if assign.Name == "jbs_name" || assign.Name == "jbs_outpath" {
			if _, isMode := assign.Expr.(ast.ModeExpr); isMode {
				diags.AddError(
					diag.CodeE303,
					fmt.Sprintf("%s must be a simple string, not shell()/python()", assign.Name),
					assign.Span,
					"assign a plain string literal",
				)
				continue
			}
			if _, ok := assign.Expr.(ast.StringExpr); !ok {
				code := diag.CodeE301
				if assign.Name == "jbs_outpath" {
					code = diag.CodeE302
				}
				diags.AddError(
					code,
					fmt.Sprintf("%s must be a simple string literal", assign.Name),
					assign.Span,
					"assign a plain quoted string",
				)
				continue
			}
			v := eval.EvalExpr(assign.Expr, values, diags)
			if v.Kind != eval.KindString {
				code := diag.CodeE301
				if assign.Name == "jbs_outpath" {
					code = diag.CodeE302
				}
				diags.AddError(
					code,
					fmt.Sprintf("%s must be a simple string literal", assign.Name),
					assign.Span,
					"assign a plain quoted string",
				)
				continue
			}
			values[assign.Name] = v
			delete(modes, assign.Name)
			spans[assign.Name] = assign.Span
			continue
		}

		mode, inner, isModeExpr := unwrapModeExpr(effectiveExpr)
		expr := effectiveExpr
		if isModeExpr {
			expr = inner
		}
		v := eval.EvalExpr(expr, values, diags)
		if isModeExpr {
			v = coerceModeValue(mode, v, assign.Span, diags)
			modes[assign.Name] = mode
		} else {
			delete(modes, assign.Name)
		}
		if !isScalarGlobalValue(v) {
			diags.AddError(
				diag.CodeE304,
				fmt.Sprintf("global variable '%s' must be scalar; tuples/lists are not allowed", assign.Name),
				assign.Span,
				"use string/int/float/bool or shell()/python() scalar values",
			)
			continue
		}
		values[assign.Name] = v
		spans[assign.Name] = assign.Span
	}

	return GlobalState{
		Values: values,
		Modes:  modes,
		Spans:  spans,
	}
}

func isScalarGlobalValue(v eval.Value) bool {
	switch v.Kind {
	case eval.KindString, eval.KindInt, eval.KindFloat, eval.KindBool, eval.KindNull:
		return true
	default:
		return false
	}
}

func hasNestedList(v eval.Value) bool {
	if v.Kind != eval.KindList && v.Kind != eval.KindTuple {
		return false
	}
	for _, item := range v.L {
		if item.Kind == eval.KindList || item.Kind == eval.KindTuple {
			return true
		}
		if hasNestedList(item) {
			return true
		}
	}
	return false
}
