package lower

import (
	"strings"

	"jbs/internal/eval"
)

func rewriteShellRefsInEvalValue(v eval.Value, aliases map[string]string) eval.Value {
	if len(aliases) == 0 {
		return v
	}
	switch v.Kind {
	case eval.KindString:
		return eval.String(rewriteShellRefs(v.S, aliases))
	case eval.KindList:
		items := make([]eval.Value, len(v.L))
		for i := range v.L {
			items[i] = rewriteShellRefsInEvalValue(v.L[i], aliases)
		}
		return eval.List(items)
	default:
		return v
	}
}

func rewriteShellRefs(text string, aliases map[string]string) string {
	if text == "" || len(aliases) == 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		ch := text[i]
		if ch == '\\' && i+1 < len(text) && text[i+1] == '$' {
			out.WriteByte('\\')
			out.WriteByte('$')
			i += 2
			continue
		}
		if ch != '$' {
			out.WriteByte(ch)
			i++
			continue
		}
		if i+1 >= len(text) {
			out.WriteByte('$')
			i++
			continue
		}
		next := text[i+1]
		if next == '{' {
			if alias, consumed, ok := rewriteBracedShellRef(text[i:], aliases); ok {
				out.WriteString(alias)
				i += consumed
				continue
			}
			out.WriteByte('$')
			i++
			continue
		}
		if isShellVarStart(next) {
			j := i + 2
			for j < len(text) && isShellVarChar(text[j]) {
				j++
			}
			name := text[i+1 : j]
			if alias, ok := aliases[name]; ok && alias != "" {
				out.WriteByte('$')
				out.WriteString(alias)
			} else {
				out.WriteByte('$')
				out.WriteString(name)
			}
			i = j
			continue
		}
		out.WriteByte('$')
		i++
	}
	return out.String()
}

func rewriteBracedShellRef(fragment string, aliases map[string]string) (string, int, bool) {
	if len(fragment) < 4 || fragment[0] != '$' || fragment[1] != '{' {
		return "", 0, false
	}
	headStart, nameStart, nameEnd, ok := parseBracedShellRefHead(fragment)
	if !ok {
		return "", 0, false
	}
	closeIdx, ok := findMatchingBrace(fragment, nameEnd)
	if !ok {
		return "", 0, false
	}
	name := fragment[nameStart:nameEnd]
	alias := name
	if mapped, ok := aliases[name]; ok && mapped != "" {
		alias = mapped
	}
	rewritten := fragment[:headStart] + alias + fragment[nameEnd:closeIdx+1]
	return rewritten, closeIdx + 1, true
}

func parseBracedShellRefHead(fragment string) (headStart int, nameStart int, nameEnd int, ok bool) {
	i := 2
	if i < len(fragment) && (fragment[i] == '#' || fragment[i] == '!') {
		i++
	}
	if i >= len(fragment) || !isShellVarStart(fragment[i]) {
		return 0, 0, 0, false
	}
	start := i
	i++
	for i < len(fragment) && isShellVarChar(fragment[i]) {
		i++
	}
	return start, start, i, true
}

func findMatchingBrace(fragment string, start int) (int, bool) {
	depth := 1
	for i := start; i < len(fragment); i++ {
		switch fragment[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func isShellVarStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isShellVarChar(ch byte) bool {
	return isShellVarStart(ch) || (ch >= '0' && ch <= '9')
}
