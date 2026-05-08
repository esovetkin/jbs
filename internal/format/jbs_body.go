package format

import (
	"strings"
	"unicode"
)

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
