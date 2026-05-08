package sema

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func namespaceHead(name string) string {
	head, _, ok := strings.Cut(name, ".")
	if !ok {
		return ""
	}
	return head
}

func isBuiltinGlobalName(name string) bool {
	return name == "jbs_name" || name == "jbs_nproc"
}

func bindingDisplayName(binding *GlobalBinding) string {
	if binding == nil {
		return ""
	}
	if binding.PublicName != "" {
		return binding.PublicName
	}
	return binding.Name
}

func globalStepBlockKey(step globalInputStep) string {
	switch step.Kind {
	case globalInputDo:
		if step.DoBlock != nil {
			return doBlockSnapshotKey(*step.DoBlock)
		}
	case globalInputAnalyse:
		if step.AnalyseBlock != nil {
			return analyseBlockSnapshotKey(*step.AnalyseBlock)
		}
	}
	return ""
}

func doBlockSnapshotKey(block ast.DoBlock) string {
	return blockSnapshotKey("do", block.Name, block.Span)
}

func analyseBlockSnapshotKey(block ast.AnalyseBlock) string {
	return blockSnapshotKey("analyse", block.StepName, block.Span)
}

func blockSnapshotKey(kind string, name string, span diag.Span) string {
	return kind + "|" + name + "|" + span.File + "|" + fmt.Sprint(span.Start.Offset)
}

func errorCount(diags *diag.Diagnostics) int {
	if diags == nil {
		return 0
	}
	count := 0
	for _, item := range diags.Items {
		if item.Severity == diag.SeverityError {
			count++
		}
	}
	return count
}

func exprSpan(expr ast.Expr) diag.Span {
	if expr == nil {
		return diag.Span{}
	}
	return expr.GetSpan()
}

func uniqueSortedNamesExcept(names []string, except string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" || name == except {
			continue
		}
		seen[name] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(seen))
}

func globalStepSpan(step globalInputStep) diag.Span {
	switch step.Kind {
	case globalInputAssign:
		if step.Assign != nil {
			return step.Assign.Span
		}
	case globalInputExpr:
		if step.ExprStmt != nil {
			return step.ExprStmt.Span
		}
	case globalInputIf:
		if step.IfStmt != nil {
			return step.IfStmt.Span
		}
	case globalInputFor:
		if step.ForStmt != nil {
			return step.ForStmt.Span
		}
	case globalInputWhile:
		if step.WhileStmt != nil {
			return step.WhileStmt.Span
		}
	case globalInputBreak:
		if step.BreakStmt != nil {
			return step.BreakStmt.Span
		}
	case globalInputContinue:
		if step.ContinueStmt != nil {
			return step.ContinueStmt.Span
		}
	case globalInputProjectedImport:
		if step.Import != nil {
			return step.Import.Span
		}
	case globalInputDo:
		if step.DoBlock != nil {
			return step.DoBlock.Span
		}
	case globalInputAnalyse:
		if step.AnalyseBlock != nil {
			return step.AnalyseBlock.Span
		}
	}
	return diag.Span{}
}
