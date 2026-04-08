package format

import (
	"strconv"
	"strings"
	"unicode"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/parser"
)

const (
	clauseIndent       = "        "
	bodyIndent         = "        "
	continuationIndent = "    "
)

// JBS normalizes source formatting from syntax only.
// Semantic validation happens in CLI analysis flow.
func JBS(file string, src string, diags *diag.Diagnostics) (string, error) {
	prog := parser.Parse(file, src, diags)
	if diags.HasErrors() {
		return "", nil
	}
	return formatProgram(prog, src), nil
}

func formatProgram(prog ast.Program, src string) string {
	srcRunes := []rune(normalizeLineEndings(src))
	lines := make([]string, 0)
	var prev ast.Stmt
	for _, stmt := range prog.Stmts {
		if len(lines) > 0 {
			if !(isGlobal(prev) && isGlobal(stmt)) {
				lines = append(lines, "")
			}
		}
		lines = append(lines, formatStmt(stmt, srcRunes)...)
		prev = stmt
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n") + "\n"
}

func isGlobal(stmt ast.Stmt) bool {
	if stmt == nil {
		return false
	}
	_, ok := stmt.(ast.GlobalAssign)
	return ok
}

func formatStmt(stmt ast.Stmt, srcRunes []rune) []string {
	switch s := stmt.(type) {
	case ast.GlobalAssign:
		return formatGlobalAssign(s, srcRunes)
	case ast.UseStmt:
		return formatUseStmt(s)
	case ast.ParamBlock:
		return formatParamBlock(s)
	case ast.DoBlock:
		return formatDoBlock(s)
	case ast.SubmitBlock:
		return formatSubmitBlock(s, srcRunes)
	case ast.LetBlock:
		return formatLetBlock(s)
	case ast.AnalyseBlock:
		return formatAnalyseBlock(s)
	default:
		return nil
	}
}

func formatGlobalAssign(g ast.GlobalAssign, srcRunes []rune) []string {
	exprText := ""
	if g.Expr != nil {
		exprText = strings.TrimSpace(spanText(srcRunes, g.Expr.GetSpan()))
	}
	if exprText == "" {
		exprText = "\"\""
	}
	return []string{g.Name + " = " + exprText}
}

func formatParamBlock(p ast.ParamBlock) []string {
	lines := renderBlockHeader("param", p.Name, nil, nil, p.WithItems, nil, nil)
	lines = append(lines, "{")
	body := normalizeBody(p.BodyRaw, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatDoBlock(d ast.DoBlock) []string {
	lines := renderBlockHeader("do", d.Name, d.After, nil, d.WithItems, d.MaxAsync, d.Iterations)
	lines = append(lines, "{")
	body := normalizeBody(d.Body, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatSubmitBlock(s ast.SubmitBlock, srcRunes []rune) []string {
	lines := renderBlockHeader("submit", s.Name, s.After, s.UseNames, s.WithItems, s.MaxAsync, s.Iterations)
	lines = append(lines, "{")
	body := normalizeSubmitBody(s.BodyRaw, bodyIndent)
	if len(body) == 0 && len(s.Fields) > 0 {
		body = renderSubmitFields(s.Fields, srcRunes)
	}
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatLetBlock(l ast.LetBlock) []string {
	lines := renderBlockHeader("let", l.Name, nil, nil, nil, nil, nil)
	lines = append(lines, "{")
	body := normalizeBody(l.BodyRaw, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatAnalyseBlock(a ast.AnalyseBlock) []string {
	lines := renderBlockHeader("analyse", a.StepName, nil, nil, a.WithItems, nil, nil)
	lines = append(lines, "{")
	body := normalizeBody(a.BodyRaw, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func renderSubmitFields(fields []ast.SubmitField, srcRunes []rune) []string {
	lines := make([]string, 0, len(fields)*2)
	for _, f := range fields {
		if f.IsRaw {
			lines = append(lines, bodyIndent+f.Name+" = {")
			raw := normalizeBody(f.Raw, bodyIndent+bodyIndent)
			lines = append(lines, raw...)
			lines = append(lines, bodyIndent+"}")
			continue
		}
		exprText := ""
		if f.Expr != nil {
			exprText = strings.TrimSpace(spanText(srcRunes, f.Expr.GetSpan()))
		}
		if exprText == "" {
			exprText = "\"\""
		}
		lines = append(lines, bodyIndent+f.Name+" = "+exprText)
	}
	return lines
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

func renderBlockHeader(kind, name string, after []string, useNames []string, with []ast.WithItem, maxAsync *int, iterations *int) []string {
	lines := []string{kind + " " + name}
	if len(after) > 0 {
		lines = append(lines, clauseIndent+"after "+strings.Join(after, ", "))
	}
	if len(useNames) > 0 {
		lines = append(lines, clauseIndent+"use "+strings.Join(useNames, ", "))
	}
	if len(with) > 0 {
		lines = append(lines, clauseIndent+"with "+renderWithClause(with))
	}
	if optionLine := renderStepOptionClause(maxAsync, iterations); optionLine != "" {
		lines = append(lines, clauseIndent+optionLine)
	}
	return lines
}

func renderStepOptionClause(maxAsync *int, iterations *int) string {
	parts := make([]string, 0, 2)
	if maxAsync != nil {
		parts = append(parts, "max_async="+strconv.Itoa(*maxAsync))
	}
	if iterations != nil {
		parts = append(parts, "iterations="+strconv.Itoa(*iterations))
	}
	return strings.Join(parts, " ")
}

func renderWithClause(items []ast.WithItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.From == "" {
			parts = append(parts, item.Name)
			continue
		}
		parts = append(parts, item.Name+" from "+item.From)
	}
	return strings.Join(parts, ", ")
}

func normalizeBody(raw string, indent string) []string {
	lines := prepareBodyLines(raw)
	return renderGenericBody(lines, indent)
}

func prepareBodyLines(raw string) []string {
	raw = normalizeLineEndings(raw)
	lines := strings.Split(raw, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return nil
	}
	trimmed := lines[start:end]

	minIndent := -1
	for _, line := range trimmed {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indentCount := leadingIndent(line)
		if minIndent < 0 || indentCount < minIndent {
			minIndent = indentCount
		}
	}
	if minIndent < 0 {
		return nil
	}

	out := make([]string, 0, len(trimmed))
	for _, line := range trimmed {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		value := dropIndent(line, minIndent)
		value = strings.TrimRight(value, " \t")
		out = append(out, value)
	}
	return rebaseInlineBodyIndent(out)
}

func renderGenericBody(lines []string, indent string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	depth := 0
	prevContinues := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			prevContinues = false
			continue
		}
		trimmedLeft := line
		if depth == 0 {
			trimmedLeft = strings.TrimLeft(trimmedLeft, " \t")
		}
		prefix := indent
		if prevContinues {
			prefix += continuationIndent
		}
		out = append(out, prefix+trimmedLeft)
		open, close := countGroupingDelimsOutsideQuotes(trimmedLeft)
		depth += open - close
		if depth < 0 {
			depth = 0
		}
		prevContinues = endsWithLineContinuation(trimmedLeft)
	}
	return out
}

func rebaseInlineBodyIndent(lines []string) []string {
	first := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		first = i
		break
	}
	if first < 0 || leadingIndent(lines[first]) > 0 {
		return lines
	}

	minRest := -1
	for i := first + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := leadingIndent(line)
		if n == 0 {
			return lines
		}
		if minRest < 0 || n < minRest {
			minRest = n
		}
	}
	if minRest <= 0 {
		return lines
	}

	out := make([]string, len(lines))
	copy(out, lines)
	for i := first + 1; i < len(out); i++ {
		if strings.TrimSpace(out[i]) == "" {
			continue
		}
		out[i] = dropIndent(out[i], minRest)
	}
	return out
}

func normalizeSubmitBody(raw string, indent string) []string {
	lines := prepareBodyLines(raw)
	return renderSubmitTopLevelBody(lines, indent)
}

func renderSubmitTopLevelBody(lines []string, indent string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	depth := 0
	prevContinues := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			prevContinues = false
			continue
		}
		trimmedLeft := strings.TrimLeft(line, " \t")
		indentDepth := depth
		if strings.HasPrefix(trimmedLeft, "}") && indentDepth > 0 {
			indentDepth--
		}
		if indentDepth == 0 {
			trimmedLeft = canonicalizeTopLevelSubmitLine(trimmedLeft)
		}
		prefix := strings.Repeat(indent, 1+indentDepth)
		if prevContinues {
			prefix += continuationIndent
		}
		out = append(out, prefix+trimmedLeft)
		open, close := countBracesOutsideQuotes(trimmedLeft)
		depth += open - close
		if depth < 0 {
			depth = 0
		}
		prevContinues = endsWithLineContinuation(trimmedLeft)
	}
	return out
}

func endsWithLineContinuation(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	runes := []rune(line)
	semanticEnd := len(runes)
	var quote rune
	escaped := false
	for i, r := range runes {
		if quote != 0 {
			if quote == '"' {
				if escaped {
					escaped = false
					continue
				}
				if r == '\\' {
					escaped = true
					continue
				}
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' {
			semanticEnd = i
			break
		}
	}
	semantic := strings.TrimRight(string(runes[:semanticEnd]), " \t")
	if semantic == "" {
		return false
	}
	semanticRunes := []rune(semantic)
	return semanticRunes[len(semanticRunes)-1] == '\\'
}

func canonicalizeTopLevelSubmitLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return trimmed
	}
	eq := strings.Index(trimmed, "=")
	if eq <= 0 {
		return trimmed
	}
	left := strings.TrimSpace(trimmed[:eq])
	if !isIdent(left) {
		return trimmed
	}
	right := strings.TrimSpace(trimmed[eq+1:])
	return left + " = " + right
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !(unicode.IsLetter(r) || r == '_') {
				return false
			}
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return false
		}
	}
	return true
}

func countBracesOutsideQuotes(line string) (openCount int, closeCount int) {
	var quote rune
	escaped := false
	for _, r := range line {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if r == '{' {
			openCount++
			continue
		}
		if r == '}' {
			closeCount++
		}
	}
	return openCount, closeCount
}

func countGroupingDelimsOutsideQuotes(line string) (openCount int, closeCount int) {
	var quote rune
	escaped := false
	for _, r := range line {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '#' {
			break
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		switch r {
		case '(', '[', '{':
			openCount++
		case ')', ']', '}':
			closeCount++
		}
	}
	return openCount, closeCount
}

func leadingIndent(s string) int {
	count := 0
	for _, r := range s {
		if r == ' ' || r == '\t' {
			count++
			continue
		}
		break
	}
	return count
}

func dropIndent(s string, n int) string {
	if n <= 0 {
		return s
	}
	runes := []rune(s)
	i := 0
	for i < len(runes) && i < n {
		if runes[i] != ' ' && runes[i] != '\t' {
			break
		}
		i++
	}
	return string(runes[i:])
}

func spanText(srcRunes []rune, span diag.Span) string {
	start := span.Start.Offset
	end := span.End.Offset
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(srcRunes) {
		start = len(srcRunes)
	}
	if end > len(srcRunes) {
		end = len(srcRunes)
	}
	if start >= end {
		return ""
	}
	return string(srcRunes[start:end])
}

func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
