package sema

import (
	"fmt"
	"regexp"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/fsubutil"
)

func validateFileSubstitutions(res *Result, diags *diag.Diagnostics) {
	if res == nil {
		return
	}
	for _, block := range res.DoBlocks {
		if len(block.FSubs) == 0 {
			continue
		}
		visible := visibleNamesForStep(res.StepScopeByName[block.Name])
		seenDest := make(map[string]diag.Span, len(block.FSubs))
		for _, fsub := range block.FSubs {
			dest := fsubutil.DestName(fsub.Path)
			if dest == "" {
				diags.AddError(diag.CodeE220, "fsub path must name a file", fsub.PathSpan, "use a path with a filename")
			} else if prev, ok := seenDest[dest]; ok {
				diags.AddError(
					diag.CodeE220,
					fmt.Sprintf("duplicate fsub destination %q in step %q", dest, block.Name),
					fsub.PathSpan,
					"write each substituted template to a distinct filename",
					diag.RelatedSpan{Message: "first destination", Span: prev},
				)
			} else {
				seenDest[dest] = fsub.PathSpan
			}
			for _, rule := range fsub.Rules {
				if _, err := regexp.Compile(rule.Pattern); err != nil {
					diags.AddError(
						diag.CodeE220,
						fmt.Sprintf("invalid fsub regex %q: %v", rule.Pattern, err),
						rule.PatternSpan,
						"fix the regular expression",
					)
				}
				validateFSubExprRefs(block.Name, visible, rule.Expr, diags)
			}
		}
	}
}

func visibleNamesForStep(plan *StepScopePlan) map[string]struct{} {
	out := make(map[string]struct{})
	if plan == nil {
		return out
	}
	for name := range plan.Effective {
		out[name] = struct{}{}
	}
	return out
}

func validateFSubExprRefs(stepName string, visible map[string]struct{}, expr ast.Expr, diags *diag.Diagnostics) {
	for _, ref := range collectExprIdentRefs(expr) {
		if eval.IsBuiltinCallName(ref.Name) || eval.IsBuiltinConstantName(ref.Name) {
			continue
		}
		if _, ok := visible[ref.Name]; ok {
			continue
		}
		diags.AddError(
			diag.CodeE220,
			fmt.Sprintf("fsub expression in step %q references variable %q that is not visible", stepName, ref.Name),
			ref.Span,
			"import the variable with `with` or inherit it with `after`",
		)
	}
}
