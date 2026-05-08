package format

import (
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

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
