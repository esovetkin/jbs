package repl

import (
	"slices"
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

type symbolCompleter struct {
	builtins []string
	globals  []string
}

func newSymbolCompleter(builtins []string, globals []string) *symbolCompleter {
	c := &symbolCompleter{builtins: sortedUniqueStrings(builtins)}
	c.SetGlobals(globals)
	return c
}

func defaultBuiltinCompletionNames() []string {
	return eval.BuiltinSymbolNames()
}

func (c *symbolCompleter) SetGlobals(names []string) {
	if c == nil {
		return
	}
	c.globals = sortedUniqueStrings(names)
}

func (c *symbolCompleter) Do(line []rune, pos int) ([][]rune, int) {
	prefix, ok := completionPrefix(line, pos)
	if !ok {
		return nil, 0
	}
	return matchingSymbolSuffixes(c.symbols(), prefix), len([]rune(prefix))
}

func (c *symbolCompleter) symbols() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.builtins)+len(c.globals))
	names = append(names, c.builtins...)
	names = append(names, c.globals...)
	slices.Sort(names)
	return slices.Compact(names)
}

func completionPrefix(line []rune, pos int) (string, bool) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(line) {
		pos = len(line)
	}
	if insideStringOrComment(line[:pos]) {
		return "", false
	}

	start := pos
	for start > 0 && isIdentContinue(line[start-1]) {
		start--
	}
	if start > 0 && line[start-1] == '.' {
		return "", false
	}
	prefix := string(line[start:pos])
	if prefix != "" {
		runes := []rune(prefix)
		if !isIdentStart(runes[0]) {
			return "", false
		}
	}
	return prefix, true
}

func insideStringOrComment(line []rune) bool {
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
			return true
		}
		if r == '\'' || r == '"' {
			quote = r
		}
	}
	return quote != 0
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentContinue(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func matchingSymbolSuffixes(symbols []string, prefix string) [][]rune {
	out := make([][]rune, 0)
	for _, symbol := range symbols {
		if strings.HasPrefix(symbol, prefix) {
			out = append(out, []rune(symbol[len(prefix):]))
		}
	}
	return out
}

func sortedUniqueStrings(in []string) []string {
	out := cloneStrings(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func cloneStrings(in []string) []string {
	return append([]string(nil), in...)
}
