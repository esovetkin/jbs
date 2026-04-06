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
	if !isShellVarStart(fragment[2]) {
		return "", 0, false
	}
	j := 3
	for j < len(fragment) && isShellVarChar(fragment[j]) {
		j++
	}
	if j >= len(fragment) || fragment[j] != '}' {
		return "", 0, false
	}
	name := fragment[2:j]
	alias := name
	if mapped, ok := aliases[name]; ok && mapped != "" {
		alias = mapped
	}
	return "${" + alias + "}", j + 1, true
}

func isShellVarStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isShellVarChar(ch byte) bool {
	return isShellVarStart(ch) || (ch >= '0' && ch <= '9')
}
