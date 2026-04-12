// implement the `jbs fmt`
//
// parse source into AST and rewrite the code in a canonical layout.
// Handle here bash blocks inside `do` and `submit`.
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
	normalized := normalizeLineEndings(src)
	srcRunes := []rune(normalized)
	if len(prog.Stmts) == 0 {
		return formatSourceWithoutStatements(normalized)
	}
	ranges := collectStmtRanges(prog.Stmts, len(srcRunes))
	lines := make([]string, 0)
	var prev ast.Stmt
	cursor := 0
	for idx, rng := range ranges {
		allowInline := idx > 0 && !isLineStartOffset(srcRunes, cursor)
		trivia := extractTopLevelTrivia(sliceSourceRange(srcRunes, cursor, rng.Start), allowInline)
		if trivia.InlineSuffix != "" && len(lines) > 0 {
			lines[len(lines)-1] += trivia.InlineSuffix
		}
		if len(trivia.Lines) > 0 {
			lines = append(lines, trivia.Lines...)
		} else if idx > 0 {
			if !(isGlobal(prev) && isGlobal(rng.Stmt)) {
				lines = append(lines, "")
			}
		}
		lines = append(lines, formatStmt(rng.Stmt, srcRunes)...)
		prev = rng.Stmt
		cursor = rng.End
	}
	allowTrailingInline := len(ranges) > 0 && !isLineStartOffset(srcRunes, cursor)
	trailingTrivia := extractTopLevelTrivia(sliceSourceRange(srcRunes, cursor, len(srcRunes)), allowTrailingInline)
	if trailingTrivia.InlineSuffix != "" && len(lines) > 0 {
		lines[len(lines)-1] += trailingTrivia.InlineSuffix
	}
	if len(trailingTrivia.Lines) > 0 {
		lines = append(lines, trailingTrivia.Lines...)
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatSourceWithoutStatements(src string) string {
	lines := strings.Split(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	if len(lines) == 0 {
		return "\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

type stmtRange struct {
	Start int
	End   int
	Stmt  ast.Stmt
}

func collectStmtRanges(stmts []ast.Stmt, sourceLen int) []stmtRange {
	ranges := make([]stmtRange, 0, len(stmts))
	for _, stmt := range stmts {
		span := stmt.GetSpan()
		start, end := clampRange(span.Start.Offset, span.End.Offset, sourceLen)
		ranges = append(ranges, stmtRange{
			Start: start,
			End:   end,
			Stmt:  stmt,
		})
	}
	return ranges
}

func clampRange(start int, end int, size int) (int, int) {
	if size < 0 {
		size = 0
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > size {
		start = size
	}
	if end > size {
		end = size
	}
	if end < start {
		end = start
	}
	return start, end
}

func sliceSourceRange(srcRunes []rune, start int, end int) string {
	start, end = clampRange(start, end, len(srcRunes))
	if start >= end {
		return ""
	}
	return string(srcRunes[start:end])
}

type topLevelTrivia struct {
	InlineSuffix string
	Lines        []string
}

func extractTopLevelTrivia(segment string, allowInline bool) topLevelTrivia {
	if segment == "" {
		return topLevelTrivia{}
	}
	lines := splitSegmentLines(segment)
	if len(lines) == 0 {
		return topLevelTrivia{}
	}
	if strings.HasPrefix(segment, "\n") && len(lines) > 0 && lines[0] == "" {
		allowInline = false
		lines = lines[1:]
	}
	result := topLevelTrivia{
		Lines: make([]string, 0, len(lines)),
	}
	hasComment := false
	start := 0
	if allowInline && len(lines) > 0 && lines[0] != "" {
		if suffix, ok := parseCommentFragment(lines[0], false); ok {
			result.InlineSuffix = suffix
			hasComment = true
			start = 1
		}
	}
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "" {
			result.Lines = append(result.Lines, "")
			continue
		}
		commentLine, ok := parseCommentFragment(line, true)
		if ok {
			result.Lines = append(result.Lines, commentLine)
			hasComment = true
			continue
		}
		if isSeparatorOnly(line) {
			continue
		}
	}
	if !hasComment {
		return topLevelTrivia{}
	}
	return result
}

func splitSegmentLines(segment string) []string {
	lines := strings.Split(segment, "\n")
	if strings.HasSuffix(segment, "\n") && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func parseCommentFragment(line string, allowSemicolonPrefix bool) (string, bool) {
	idx := strings.IndexRune(line, '#')
	if idx < 0 {
		return "", false
	}
	prefix := line[:idx]
	if allowSemicolonPrefix {
		if !isWhitespaceOrSemicolon(prefix) {
			return "", false
		}
		if strings.Contains(prefix, ";") {
			return strings.TrimRight(line[idx:], " \t"), true
		}
		return strings.TrimRight(line, " \t"), true
	}
	if !isWhitespace(prefix) {
		return "", false
	}
	return strings.TrimRight(line, " \t"), true
}

func isSeparatorOnly(line string) bool {
	trimmed := strings.TrimLeft(line, " \t;")
	return trimmed == ""
}

func isWhitespace(text string) bool {
	for _, r := range text {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func isWhitespaceOrSemicolon(text string) bool {
	for _, r := range text {
		if r != ' ' && r != '\t' && r != ';' {
			return false
		}
	}
	return true
}

func isLineStartOffset(srcRunes []rune, offset int) bool {
	if offset <= 0 {
		return true
	}
	if offset > len(srcRunes) {
		offset = len(srcRunes)
	}
	return srcRunes[offset-1] == '\n'
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
	op := string(g.Op)
	if op == "" {
		op = string(ast.AssignEq)
	}
	return []string{g.Name + " " + op + " " + exprText}
}

func formatParamBlock(p ast.ParamBlock) []string {
	lines := renderBlockHeader("param", p.Name, nil, nil, p.WithItems, nil, nil, nil, p.Header)
	lines = append(lines, "{")
	body := normalizeBody(p.BodyRaw, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatDoBlock(d ast.DoBlock) []string {
	lines := renderBlockHeader("do", d.Name, d.After, nil, d.WithItems, d.MaxAsync, d.Procs, d.Iterations, d.Header)
	lines = append(lines, "{")
	body := normalizeBody(d.Body, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatSubmitBlock(s ast.SubmitBlock, srcRunes []rune) []string {
	lines := renderBlockHeader("submit", s.Name, s.After, s.UseNames, s.WithItems, s.MaxAsync, s.Procs, s.Iterations, s.Header)
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
	lines := renderBlockHeader("let", l.Name, nil, nil, nil, nil, nil, nil, l.Header)
	lines = append(lines, "{")
	body := normalizeBody(l.BodyRaw, bodyIndent)
	lines = append(lines, body...)
	lines = append(lines, "}")
	return lines
}

func formatAnalyseBlock(a ast.AnalyseBlock) []string {
	lines := renderBlockHeader("analyse", a.StepName, nil, nil, a.WithItems, nil, nil, nil, a.Header)
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
		op := string(f.Op)
		if op == "" {
			op = string(ast.AssignEq)
		}
		lines = append(lines, bodyIndent+f.Name+" "+op+" "+exprText)
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

type headerClauseKind int

const (
	headerClauseAfter headerClauseKind = iota
	headerClauseUse
	headerClauseWith
	headerClauseOptions
)

type renderedHeaderClause struct {
	Kind headerClauseKind
	Text string
}

type headerCommentBucket struct {
	Before []string
	Inline string
}

func renderBlockHeader(kind, name string, after []string, useNames []string, with []ast.WithItem, maxAsync *int, procs *int, iterations *int, header []ast.HeaderElem) []string {
	lines := []string{kind + " " + name}
	clauses := buildRenderedHeaderClauses(after, useNames, with, maxAsync, procs, iterations)
	if len(header) == 0 {
		for _, clause := range clauses {
			lines = append(lines, clauseIndent+clause.Text)
		}
		return lines
	}

	buckets, trailing := collectHeaderCommentBuckets(header)
	for _, clause := range clauses {
		bucket := buckets[clause.Kind]
		if bucket != nil {
			for _, text := range bucket.Before {
				lines = append(lines, renderHeaderCommentLine(text))
			}
		}
		line := clauseIndent + clause.Text
		if bucket != nil && bucket.Inline != "" {
			line += "  " + bucket.Inline
		}
		lines = append(lines, line)
	}
	for _, text := range trailing {
		lines = append(lines, renderHeaderCommentLine(text))
	}
	return lines
}

func buildRenderedHeaderClauses(after []string, useNames []string, with []ast.WithItem, maxAsync *int, procs *int, iterations *int) []renderedHeaderClause {
	clauses := make([]renderedHeaderClause, 0, 4)
	if len(after) > 0 {
		clauses = append(clauses, renderedHeaderClause{
			Kind: headerClauseAfter,
			Text: "after " + strings.Join(after, ", "),
		})
	}
	if len(useNames) > 0 {
		clauses = append(clauses, renderedHeaderClause{
			Kind: headerClauseUse,
			Text: "use " + strings.Join(useNames, ", "),
		})
	}
	if len(with) > 0 {
		clauses = append(clauses, renderedHeaderClause{
			Kind: headerClauseWith,
			Text: "with " + renderWithClause(with),
		})
	}
	if optionLine := renderStepOptionClause(maxAsync, procs, iterations); optionLine != "" {
		clauses = append(clauses, renderedHeaderClause{
			Kind: headerClauseOptions,
			Text: optionLine,
		})
	}
	return clauses
}

func collectHeaderCommentBuckets(header []ast.HeaderElem) (map[headerClauseKind]*headerCommentBucket, []string) {
	buckets := map[headerClauseKind]*headerCommentBucket{
		headerClauseAfter:   {},
		headerClauseUse:     {},
		headerClauseWith:    {},
		headerClauseOptions: {},
	}
	pending := make([]string, 0)

	appendPending := func(kind headerClauseKind) {
		if len(pending) == 0 {
			return
		}
		buckets[kind].Before = append(buckets[kind].Before, pending...)
		pending = pending[:0]
	}

	for _, elem := range header {
		switch elem.Kind {
		case ast.HeaderElemBlank:
			pending = append(pending, "")
		case ast.HeaderElemComment:
			if elem.Comment != nil {
				pending = append(pending, "# "+strings.TrimSpace(elem.Comment.Text))
			} else {
				pending = append(pending, "#")
			}
		case ast.HeaderElemAfter, ast.HeaderElemUse, ast.HeaderElemWith, ast.HeaderElemOption:
			kind := toHeaderClauseKind(elem.Kind)
			if elem.Inline != nil && buckets[kind].Inline != "" {
				buckets[kind].Before = append(buckets[kind].Before, buckets[kind].Inline)
				buckets[kind].Inline = ""
			}
			appendPending(kind)
			if elem.Inline != nil {
				inline := "# " + strings.TrimSpace(elem.Inline.Text)
				buckets[kind].Inline = strings.TrimSpace(inline)
			}
		default:
			text := strings.TrimSpace(elem.Text)
			if text != "" {
				pending = append(pending, text)
			}
			if elem.Inline != nil {
				pending = append(pending, "# "+strings.TrimSpace(elem.Inline.Text))
			}
		}
	}

	trailing := make([]string, len(pending))
	copy(trailing, pending)
	return buckets, trailing
}

func toHeaderClauseKind(kind ast.HeaderElemKind) headerClauseKind {
	switch kind {
	case ast.HeaderElemAfter:
		return headerClauseAfter
	case ast.HeaderElemUse:
		return headerClauseUse
	case ast.HeaderElemWith:
		return headerClauseWith
	default:
		return headerClauseOptions
	}
}

func renderHeaderCommentLine(text string) string {
	if text == "" {
		return ""
	}
	return clauseIndent + text
}

func renderStepOptionClause(maxAsync *int, procs *int, iterations *int) string {
	parts := make([]string, 0, 3)
	if maxAsync != nil {
		parts = append(parts, "max_async="+strconv.Itoa(*maxAsync))
	}
	if procs != nil {
		parts = append(parts, "procs="+strconv.Itoa(*procs))
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
		trimmedLeft := strings.TrimLeft(line, " \t")
		effectiveDepth := depth
		if startsWithGroupingCloser(trimmedLeft) && effectiveDepth > 0 {
			effectiveDepth--
		}
		prefix := indent
		if prevContinues {
			prefix += continuationIndent
		}
		if effectiveDepth > 0 {
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

func startsWithGroupingCloser(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case ')', ']', '}':
		return true
	default:
		return false
	}
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
