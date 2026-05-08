package format

import (
	"strconv"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
)

func formatExprLines(expr ast.Expr, srcRunes []rune) []string {
	if expr == nil {
		return nil
	}
	if !needsStructuredExprFormatting(expr) {
		text := strings.TrimSpace(spanText(srcRunes, expr.GetSpan()))
		if text != "" {
			return []string{text}
		}
		inline := formatExprInline(expr, srcRunes)
		if inline == "" {
			return nil
		}
		return []string{inline}
	}
	switch e := expr.(type) {
	case ast.FunctionExpr:
		return formatFunctionExprLines(e, srcRunes)
	case ast.CallExpr:
		return formatCallExprLines(e, srcRunes)
	default:
		return []string{formatExprInline(expr, srcRunes)}
	}
}

func needsStructuredExprFormatting(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case ast.FunctionExpr:
		return true
	case ast.CallExpr:
		if needsStructuredExprFormatting(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if arg.Name != "" || needsStructuredExprFormatting(arg.Expr) {
				return true
			}
		}
		return false
	case ast.ListExpr:
		for _, item := range e.Items {
			if needsStructuredExprFormatting(item) {
				return true
			}
		}
	case ast.TupleExpr:
		for _, item := range e.Items {
			if needsStructuredExprFormatting(item) {
				return true
			}
		}
	case ast.IndexExpr:
		if needsStructuredExprFormatting(e.Base) {
			return true
		}
		for _, item := range e.Items {
			if needsStructuredExprFormatting(item) {
				return true
			}
		}
	case ast.MemberExpr:
		return needsStructuredExprFormatting(e.Base)
	case ast.AliasExpr:
		return needsStructuredExprFormatting(e.Expr)
	case ast.UnaryExpr:
		return needsStructuredExprFormatting(e.Expr)
	case ast.BinaryExpr:
		return needsStructuredExprFormatting(e.Left) || needsStructuredExprFormatting(e.Right)
	case ast.CompareExpr:
		return needsStructuredExprFormatting(e.Left) || needsStructuredExprFormatting(e.Right)
	case ast.ConditionalExpr:
		return needsStructuredExprFormatting(e.Then) || needsStructuredExprFormatting(e.Cond) || needsStructuredExprFormatting(e.Else)
	}
	return false
}

type exprPrec int

const (
	precLowest exprPrec = iota
	precConditional
	precPipe
	precAmp
	precCompare
	precAdd
	precMul
	precUnary
	precPostfix
	precPrimary
)

type exprSide int

const (
	sideNone exprSide = iota
	sideLeft
	sideRight
	sideUnary
	sideContainer
	sideConditionalThen
	sideConditionalCond
	sideConditionalElse
)

func formatExprInline(expr ast.Expr, srcRunes []rune) string {
	return formatExprInlinePrec(expr, srcRunes, precLowest, sideNone)
}

func formatExprInlinePrec(expr ast.Expr, srcRunes []rune, parent exprPrec, side exprSide) string {
	if expr == nil {
		return ""
	}
	if !needsStructuredExprFormatting(expr) {
		if text := sourceExprText(expr, srcRunes); text != "" {
			return text
		}
	}
	text := formatExprRebuilt(expr, srcRunes)
	return parenthesizeIfNeeded(text, expr, parent, side)
}

func sourceExprText(expr ast.Expr, srcRunes []rune) string {
	if expr == nil {
		return ""
	}
	return strings.TrimSpace(spanText(srcRunes, expr.GetSpan()))
}

func formatExprRebuilt(expr ast.Expr, srcRunes []rune) string {
	switch e := expr.(type) {
	case ast.IdentExpr:
		return e.Name
	case ast.QualifiedIdentExpr:
		if e.Namespace == "" {
			return e.Name
		}
		return e.Namespace + "." + e.Name
	case ast.MemberExpr:
		base := formatExprInlinePrec(e.Base, srcRunes, precPostfix, sideLeft)
		return base + "." + e.Name
	case ast.IndexExpr:
		items := make([]string, 0, len(e.Items))
		for _, item := range e.Items {
			items = append(items, formatExprInlinePrec(item, srcRunes, precConditional, sideContainer))
		}
		base := formatExprInlinePrec(e.Base, srcRunes, precPostfix, sideLeft)
		return base + "[" + strings.Join(items, ", ") + "]"
	case ast.StringExpr:
		return strconv.Quote(e.Value)
	case ast.NumberExpr:
		if e.Raw != "" {
			return e.Raw
		}
		if e.Int {
			return strconv.FormatInt(e.IntValue, 10)
		}
		return strconv.FormatFloat(e.FloatValue, 'g', -1, 64)
	case ast.BoolExpr:
		if e.Value {
			return "true"
		}
		return "false"
	case ast.ListExpr:
		items := make([]string, 0, len(e.Items))
		for _, item := range e.Items {
			items = append(items, formatExprInlinePrec(item, srcRunes, precConditional, sideContainer))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case ast.TupleExpr:
		items := make([]string, 0, len(e.Items))
		for _, item := range e.Items {
			items = append(items, formatExprInlinePrec(item, srcRunes, precConditional, sideContainer))
		}
		switch len(items) {
		case 0:
			return "()"
		case 1:
			return "(" + items[0] + ",)"
		default:
			return "(" + strings.Join(items, ", ") + ")"
		}
	case ast.CallExpr:
		lines := formatCallExprLines(e, srcRunes)
		return flattenFormattedLines(lines)
	case ast.FunctionExpr:
		lines := formatFunctionExprLines(e, srcRunes)
		return flattenFormattedLines(lines)
	case ast.AliasExpr:
		return formatExprInlinePrec(e.Expr, srcRunes, precPostfix, sideLeft) + " as " + e.Alias
	case ast.UnaryExpr:
		return e.Op + formatExprInlinePrec(e.Expr, srcRunes, precUnary, sideUnary)
	case ast.BinaryExpr:
		prec := exprPrecedence(e)
		left := formatExprInlinePrec(e.Left, srcRunes, prec, sideLeft)
		right := formatExprInlinePrec(e.Right, srcRunes, prec, sideRight)
		return left + " " + e.Op + " " + right
	case ast.CompareExpr:
		prec := exprPrecedence(e)
		left := formatExprInlinePrec(e.Left, srcRunes, prec, sideLeft)
		right := formatExprInlinePrec(e.Right, srcRunes, prec, sideRight)
		return left + " " + e.Op + " " + right
	case ast.ConditionalExpr:
		thenText := formatExprInlinePrec(e.Then, srcRunes, precConditional, sideConditionalThen)
		condText := formatExprInlinePrec(e.Cond, srcRunes, precConditional, sideConditionalCond)
		elseText := formatExprInlinePrec(e.Else, srcRunes, precConditional, sideConditionalElse)
		return thenText + " if " + condText + " else " + elseText
	default:
		return sourceExprText(expr, srcRunes)
	}
}

func exprPrecedence(expr ast.Expr) exprPrec {
	switch e := expr.(type) {
	case ast.ConditionalExpr:
		return precConditional
	case ast.BinaryExpr:
		switch e.Op {
		case "|":
			return precPipe
		case "&":
			return precAmp
		case "+", "-":
			return precAdd
		case "*", "/", "%":
			return precMul
		default:
			return precLowest
		}
	case ast.CompareExpr:
		return precCompare
	case ast.UnaryExpr:
		return precUnary
	case ast.CallExpr, ast.IndexExpr, ast.MemberExpr, ast.AliasExpr, ast.QualifiedIdentExpr:
		return precPostfix
	default:
		return precPrimary
	}
}

func parenthesizeIfNeeded(text string, expr ast.Expr, parent exprPrec, side exprSide) string {
	child := exprPrecedence(expr)
	if child < parent {
		return "(" + text + ")"
	}
	if child == parent && needsSamePrecedenceParens(expr, side) {
		return "(" + text + ")"
	}
	return text
}

func needsSamePrecedenceParens(expr ast.Expr, side exprSide) bool {
	switch expr.(type) {
	case ast.BinaryExpr:
		return side == sideRight
	case ast.CompareExpr:
		return side == sideLeft || side == sideRight
	case ast.ConditionalExpr:
		return side == sideContainer || side == sideConditionalThen || side == sideConditionalCond
	default:
		return false
	}
}

func formatFunctionExprLines(fn ast.FunctionExpr, srcRunes []rune) []string {
	params := make([]string, 0, len(fn.Params))
	for _, param := range fn.Params {
		text := param.Name
		if param.Default != nil {
			text += " = " + formatExprInline(param.Default, srcRunes)
		}
		params = append(params, text)
	}
	lines := []string{"function(" + strings.Join(params, ", ") + ") {"}
	for _, stmt := range fn.Body {
		lines = append(lines, indentLines(formatFuncBodyStmtLines(stmt, srcRunes), continuationIndent)...)
	}
	lines = append(lines, "}")
	return lines
}

func formatFuncBodyStmtLines(stmt ast.FuncBodyStmt, srcRunes []rune) []string {
	switch s := stmt.(type) {
	case ast.LocalAssignStmt:
		op := string(s.Op)
		if op == "" {
			op = string(ast.AssignEq)
		}
		exprLines := formatExprLines(s.Expr, srcRunes)
		if len(exprLines) == 0 {
			exprLines = []string{`""`}
		}
		return prefixFormattedLines("", s.Name+" "+op+" ", exprLines)
	case ast.ReturnStmt:
		exprLines := formatExprLines(s.Expr, srcRunes)
		if len(exprLines) == 0 {
			exprLines = []string{`""`}
		}
		return prefixFormattedLines("", "return ", exprLines)
	case ast.ExprStmt:
		return formatExprLines(s.Expr, srcRunes)
	case ast.FuncIfStmt:
		return formatFuncIfStmtLines(s, srcRunes)
	case ast.FuncForStmt:
		return formatFuncForStmtLines(s, srcRunes)
	case ast.FuncWhileStmt:
		return formatFuncWhileStmtLines(s, srcRunes)
	case ast.BreakStmt:
		return []string{"break"}
	case ast.ContinueStmt:
		return []string{"continue"}
	default:
		return nil
	}
}

func formatFuncIfStmtLines(stmt ast.FuncIfStmt, srcRunes []rune) []string {
	condLines := formatExprLines(stmt.Cond, srcRunes)
	if len(condLines) == 0 {
		condLines = []string{`true`}
		if stmt.Cond != nil {
			condLines = []string{strings.TrimSpace(spanText(srcRunes, stmt.Cond.GetSpan()))}
		}
	}
	lines := prefixFormattedLines("", "if ", condLines)
	lines[len(lines)-1] += " {"
	for _, child := range stmt.Then {
		lines = append(lines, indentLines(formatFuncBodyStmtLines(child, srcRunes), continuationIndent)...)
	}
	for _, branch := range stmt.Elifs {
		lines = append(lines, formatFuncElifBranchLines(branch, srcRunes)...)
	}
	if len(stmt.Else) == 0 {
		lines = append(lines, "}")
		return lines
	}
	lines = append(lines, "} else {")
	for _, child := range stmt.Else {
		lines = append(lines, indentLines(formatFuncBodyStmtLines(child, srcRunes), continuationIndent)...)
	}
	lines = append(lines, "}")
	return lines
}

func formatFuncElifBranchLines(branch ast.FuncElifBranch, srcRunes []rune) []string {
	condLines := formatExprLines(branch.Cond, srcRunes)
	if len(condLines) == 0 {
		condLines = []string{"true"}
		if branch.Cond != nil {
			condLines = []string{strings.TrimSpace(spanText(srcRunes, branch.Cond.GetSpan()))}
		}
	}
	lines := prefixFormattedLines("", "} elif ", condLines)
	lines[len(lines)-1] += " {"
	for _, child := range branch.Body {
		lines = append(lines, indentLines(formatFuncBodyStmtLines(child, srcRunes), continuationIndent)...)
	}
	return lines
}

func formatFuncForStmtLines(stmt ast.FuncForStmt, srcRunes []rune) []string {
	exprLines := formatExprLines(stmt.Iterable, srcRunes)
	if len(exprLines) == 0 {
		exprLines = []string{`[]`}
	}
	lines := prefixFormattedLines("", "for "+stmt.Target+" in ", exprLines)
	lines[len(lines)-1] += " {"
	for _, child := range stmt.Body {
		lines = append(lines, indentLines(formatFuncBodyStmtLines(child, srcRunes), continuationIndent)...)
	}
	lines = append(lines, "}")
	return lines
}

func formatFuncWhileStmtLines(stmt ast.FuncWhileStmt, srcRunes []rune) []string {
	condLines := formatExprLines(stmt.Cond, srcRunes)
	if len(condLines) == 0 {
		condLines = []string{"true"}
	}
	lines := prefixFormattedLines("", "while ", condLines)
	lines[len(lines)-1] += " {"
	for _, child := range stmt.Body {
		lines = append(lines, indentLines(formatFuncBodyStmtLines(child, srcRunes), continuationIndent)...)
	}
	lines = append(lines, "}")
	return lines
}

func formatCallExprLines(call ast.CallExpr, srcRunes []rune) []string {
	calleeLines := formatExprLines(call.Callee, srcRunes)
	if len(calleeLines) == 0 {
		calleeLines = []string{""}
	}
	if len(calleeLines) == 1 {
		calleeLines[0] = formatExprInlinePrec(call.Callee, srcRunes, precPostfix, sideLeft)
	}
	args := make([][]string, 0, len(call.Args))
	multilineArgs := false
	for _, arg := range call.Args {
		lines := formatCallArgLines(arg, srcRunes)
		if len(lines) > 1 {
			multilineArgs = true
		}
		args = append(args, lines)
	}
	if !multilineArgs {
		parts := make([]string, 0, len(args))
		for _, lines := range args {
			parts = append(parts, flattenFormattedLines(lines))
		}
		out := append([]string(nil), calleeLines...)
		out[len(out)-1] += "(" + strings.Join(parts, ", ") + ")"
		return out
	}
	out := append([]string(nil), calleeLines...)
	if len(args) == 0 {
		out[len(out)-1] += "()"
		return out
	}
	out[len(out)-1] += "("
	for i, argLines := range args {
		indented := indentLines(argLines, continuationIndent)
		if len(indented) == 0 {
			indented = []string{continuationIndent}
		}
		if i < len(args)-1 {
			indented[len(indented)-1] += ","
		}
		out = append(out, indented...)
	}
	out = append(out, ")")
	return out
}

func formatCallArgLines(arg ast.CallArg, srcRunes []rune) []string {
	exprLines := formatExprLines(arg.Expr, srcRunes)
	if len(exprLines) == 0 {
		exprLines = []string{""}
	}
	if arg.Name == "" {
		return exprLines
	}
	return prefixFormattedLines("", arg.Name+" = ", exprLines)
}
