package format

import (
	"strings"
)

type formattedLine struct {
	Text                  string
	PreserveTrailingSpace bool
}

func plainLine(text string) formattedLine {
	return formattedLine{Text: text}
}

func rawLine(text string) formattedLine {
	return formattedLine{Text: text, PreserveTrailingSpace: true}
}

func plainLines(lines []string) []formattedLine {
	out := make([]formattedLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, plainLine(line))
	}
	return out
}

func formattedLineTexts(lines []formattedLine) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.Text)
	}
	return out
}

// JBS normalizes source formatting from syntax only.
// Semantic validation happens in CLI analysis flow.

func prefixFormattedLines(baseIndent string, firstPrefix string, lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	out = append(out, baseIndent+firstPrefix+lines[0])
	for i := 1; i < len(lines); i++ {
		out = append(out, baseIndent+lines[i])
	}
	return out
}

func indentLines(lines []string, indent string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+line)
	}
	return out
}

func indentFormattedLines(lines []formattedLine, indent string) []formattedLine {
	if len(lines) == 0 {
		return nil
	}
	out := make([]formattedLine, 0, len(lines))
	for _, line := range lines {
		if line.Text != "" {
			line.Text = indent + line.Text
		}
		out = append(out, line)
	}
	return out
}

func flattenFormattedLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	flat := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		flat = append(flat, trimmed)
	}
	return strings.Join(flat, " ")
}

func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
