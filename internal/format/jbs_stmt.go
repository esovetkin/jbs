package format

import (
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
)

func isGlobal(stmt ast.Stmt) bool {
	if stmt == nil {
		return false
	}
	_, ok := stmt.(ast.GlobalAssign)
	return ok
}

func formatStmt(stmt ast.Stmt, srcRunes []rune) []formattedLine {
	switch s := stmt.(type) {
	case ast.GlobalAssign:
		return plainLines(formatGlobalAssign(s, srcRunes))
	case ast.ExprStmt:
		return plainLines(formatExprStmt(s, srcRunes))
	case ast.IfStmt:
		return formatIfStmt(s, srcRunes)
	case ast.ForStmt:
		return formatForStmt(s, srcRunes)
	case ast.WhileStmt:
		return formatWhileStmt(s, srcRunes)
	case ast.BreakStmt:
		return []formattedLine{plainLine("break")}
	case ast.ContinueStmt:
		return []formattedLine{plainLine("continue")}
	case ast.UseStmt:
		return plainLines(formatUseStmt(s))
	case ast.DoBlock:
		return formatDoBlock(s)
	case ast.AnalyseBlock:
		return plainLines(formatAnalyseBlock(s))
	default:
		return nil
	}
}

func formatIfStmt(stmt ast.IfStmt, srcRunes []rune) []formattedLine {
	condLines := formatExprLines(stmt.Cond, srcRunes)
	if len(condLines) == 0 {
		condLines = []string{`true`}
		if stmt.Cond != nil {
			condLines = []string{strings.TrimSpace(spanText(srcRunes, stmt.Cond.GetSpan()))}
		}
	}
	lines := plainLines(prefixFormattedLines("", "if ", condLines))
	lines[len(lines)-1].Text += " {"
	for _, child := range stmt.Then {
		lines = append(lines, indentFormattedLines(formatStmt(child, srcRunes), continuationIndent)...)
	}
	for _, branch := range stmt.Elifs {
		lines = append(lines, formatElifBranch(branch, srcRunes)...)
	}
	if len(stmt.Else) == 0 {
		lines = append(lines, plainLine("}"))
		return lines
	}
	lines = append(lines, plainLine("} else {"))
	for _, child := range stmt.Else {
		lines = append(lines, indentFormattedLines(formatStmt(child, srcRunes), continuationIndent)...)
	}
	lines = append(lines, plainLine("}"))
	return lines
}

func formatElifBranch(branch ast.ElifBranch, srcRunes []rune) []formattedLine {
	condLines := formatExprLines(branch.Cond, srcRunes)
	if len(condLines) == 0 {
		condLines = []string{"true"}
		if branch.Cond != nil {
			condLines = []string{strings.TrimSpace(spanText(srcRunes, branch.Cond.GetSpan()))}
		}
	}
	lines := plainLines(prefixFormattedLines("", "} elif ", condLines))
	lines[len(lines)-1].Text += " {"
	for _, child := range branch.Body {
		lines = append(lines, indentFormattedLines(formatStmt(child, srcRunes), continuationIndent)...)
	}
	return lines
}

func formatForStmt(stmt ast.ForStmt, srcRunes []rune) []formattedLine {
	exprLines := formatExprLines(stmt.Iterable, srcRunes)
	if len(exprLines) == 0 {
		exprLines = []string{`[]`}
	}
	lines := plainLines(prefixFormattedLines("", "for "+stmt.Target+" in ", exprLines))
	lines[len(lines)-1].Text += " {"
	for _, child := range stmt.Body {
		lines = append(lines, indentFormattedLines(formatStmt(child, srcRunes), continuationIndent)...)
	}
	lines = append(lines, plainLine("}"))
	return lines
}

func formatWhileStmt(stmt ast.WhileStmt, srcRunes []rune) []formattedLine {
	condLines := formatExprLines(stmt.Cond, srcRunes)
	if len(condLines) == 0 {
		condLines = []string{"true"}
	}
	lines := plainLines(prefixFormattedLines("", "while ", condLines))
	lines[len(lines)-1].Text += " {"
	for _, child := range stmt.Body {
		lines = append(lines, indentFormattedLines(formatStmt(child, srcRunes), continuationIndent)...)
	}
	lines = append(lines, plainLine("}"))
	return lines
}

func formatExprStmt(stmt ast.ExprStmt, srcRunes []rune) []string {
	if stmt.Expr == nil {
		exprText := strings.TrimSpace(spanText(srcRunes, stmt.Span))
		if exprText == "" {
			return nil
		}
		return []string{exprText}
	}
	lines := formatExprLines(stmt.Expr, srcRunes)
	if len(lines) == 0 {
		return nil
	}
	return lines
}

func formatGlobalAssign(g ast.GlobalAssign, srcRunes []rune) []string {
	op := string(g.Op)
	if op == "" {
		op = string(ast.AssignEq)
	}
	exprLines := formatExprLines(g.Expr, srcRunes)
	if len(exprLines) == 0 {
		exprLines = []string{`""`}
	}
	return prefixFormattedLines("", g.Name+" "+op+" ", exprLines)
}

func formatDoBlock(d ast.DoBlock) []formattedLine {
	lines := plainLines(renderBlockHeader("do", d.Name, d.After, d.WithItems, d.NProc, d.Header))
	lines = append(lines, plainLine("{"))
	lines = append(lines, preserveRawBodyLines(d.Body)...)
	lines = append(lines, plainLine("}"))
	return lines
}

func formatAnalyseBlock(a ast.AnalyseBlock) []string {
	lines := renderBlockHeader("analyse", a.StepName, nil, a.WithItems, nil, a.Header)
	lines = append(lines, "{")
	body := normalizeBody(a.BodyRaw, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func preserveRawBodyLines(raw string) []formattedLine {
	body := rawBodyForCanonicalBraces(normalizeLineEndings(raw))
	if body == "" {
		return nil
	}
	parts := strings.Split(body, "\n")
	out := make([]formattedLine, 0, len(parts))
	for _, part := range parts {
		out = append(out, rawLine(part))
	}
	return out
}

func rawBodyForCanonicalBraces(raw string) string {
	if raw == "" || raw == "\n" {
		return ""
	}
	if strings.HasPrefix(raw, "\n") && strings.HasSuffix(raw, "\n") {
		return raw[1 : len(raw)-1]
	}
	if strings.HasPrefix(raw, "\n") {
		return raw[1:]
	}
	if strings.HasSuffix(raw, "\n") {
		return raw[:len(raw)-1]
	}
	return raw
}

func formatUseStmt(u ast.UseStmt) []string {
	if len(u.Names) == 0 {
		if u.Source.Kind == ast.UseSourcePath {
			alias := u.Alias
			if alias == "" {
				alias = "module"
			}
			return []string{`use "` + u.Source.Value + `" as ` + alias}
		}
		return []string{"use " + u.Source.Value}
	}
	target := u.Source.Value
	if u.Source.Kind == ast.UseSourcePath {
		target = `"` + target + `"`
	}
	return []string{"use " + strings.Join(u.Names, ", ") + " from " + target}
}
