package sema

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func compileLetBlock(block ast.LetBlock, globals map[string]eval.Value, diags *diag.Diagnostics) *LetNamespace {
	env := make(map[string]eval.Value, len(globals)+len(block.Assignments)+8)
	for k, v := range globals {
		env[k] = v
	}

	out := &LetNamespace{
		Name:    block.Name,
		Vars:    make(map[string]eval.Value, len(block.Assignments)),
		Modes:   make(map[string]string, len(block.Assignments)),
		Origins: make(map[string]diag.Span, len(block.Assignments)),
		Span:    block.Span,
	}

	seen := make(map[string]diag.Span, len(block.Assignments))
	for _, asn := range block.Assignments {
		if prev, exists := seen[asn.Name]; exists {
			diags.AddError(
				diag.CodeE401,
				fmt.Sprintf("duplicate variable '%s' in let block '%s'", asn.Name, block.Name),
				asn.Span,
				"use unique variable names within a let block",
				diag.RelatedSpan{Message: "first definition", Span: prev},
			)
			continue
		}
		seen[asn.Name] = asn.Span
		warnModeExprInCollections(asn.Expr, diags)
		mode, inner, isModeExpr := unwrapModeExpr(asn.Expr)
		expr := asn.Expr
		if isModeExpr {
			expr = inner
		}
		v := eval.EvalExpr(expr, env, diags)
		if isModeExpr {
			v = coerceModeValue(mode, v, asn.Span, diags)
			out.Modes[asn.Name] = mode
		} else {
			delete(out.Modes, asn.Name)
		}
		if v.Kind == eval.KindList {
			diags.AddError(
				diag.CodeE403,
				fmt.Sprintf("let variable '%s' must be scalar", asn.Name),
				asn.Span,
				"use string/int/float/bool or shell()/python() scalar values",
			)
			continue
		}
		out.Vars[asn.Name] = v
		out.Origins[asn.Name] = asn.Span
		env[asn.Name] = v
	}

	return out
}
