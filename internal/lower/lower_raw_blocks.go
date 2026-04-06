package lower

import "strings"

func normalizeRawLiteral(body string) string {
	trimmed := normalizeRawBlock(body)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, "\n") {
		return trimmed
	}
	return trimmed + "\n"
}

func normalizeRawBlock(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")

	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}

	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingIndent(line)
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent < 0 {
		minIndent = 0
	}

	for i, line := range lines {
		lines[i] = strings.TrimRight(stripIndent(line, minIndent), " \t")
	}
	return strings.Join(lines, "\n")
}

func leadingIndent(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' || r == '\t' {
			n++
			continue
		}
		break
	}
	return n
}

func stripIndent(s string, n int) string {
	if n <= 0 {
		return s
	}
	i := 0
	for _, r := range s {
		if i >= n {
			break
		}
		if r != ' ' && r != '\t' {
			break
		}
		i++
	}
	return s[i:]
}
