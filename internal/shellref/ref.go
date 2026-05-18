package shellref

import (
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type Ref struct {
	Name string
	Span diag.Span
}

type bracedVarRef struct {
	Name    string
	NameEnd int
	End     int
}

type scanState uint8

const (
	scanCode scanState = iota
	scanSingleQuote
	scanDoubleQuote
	scanComment
)

func Collect(text string, base diag.Position, file string) []Ref {
	runes := []rune(text)
	refs := make([]Ref, 0)
	line := base.Line
	col := base.Column
	off := base.Offset
	i := 0
	state := scanCode

	advance := func() {
		if i >= len(runes) {
			return
		}
		r := runes[i]
		i++
		off++
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	advanceN := func(target int) {
		for i < target {
			advance()
		}
	}
	appendRef := func(name string, start diag.Position) {
		end := diag.NewPos(off, line, col)
		refs = append(refs, Ref{
			Name: name,
			Span: diag.NewSpan(file, start, end),
		})
	}
	parseExpansion := func(start diag.Position) {
		dollarIdx := i
		if dollarIdx+1 < len(runes) && runes[dollarIdx+1] == '{' {
			ref, ok := parseBracedVarRef(runes, dollarIdx+2)
			if ok {
				advanceN(ref.End + 1)
				appendRef(ref.Name, start)
				refs = append(refs, collectNestedBracedRefs(runes, dollarIdx, ref.NameEnd, ref.End, start, file)...)
				return
			}
			advance()
			return
		}
		if end, ok := parseBareVarName(runes, i+1); ok {
			name := string(runes[i+1 : end])
			advanceN(end)
			appendRef(name, start)
			return
		}
		advance()
	}

	for i < len(runes) {
		switch state {
		case scanCode:
			curr := runes[i]
			if curr == '\'' {
				advance()
				state = scanSingleQuote
				continue
			}
			if curr == '"' {
				advance()
				state = scanDoubleQuote
				continue
			}
			if curr == '#' && isCommentStart(runes, i) {
				advance()
				state = scanComment
				continue
			}
			if curr == '$' && !isEscapedDollar(runes, i) {
				start := diag.NewPos(off, line, col)
				parseExpansion(start)
				continue
			}
			advance()
		case scanSingleQuote:
			if runes[i] == '\'' {
				advance()
				state = scanCode
				continue
			}
			advance()
		case scanDoubleQuote:
			curr := runes[i]
			if curr == '\\' {
				advance()
				if i < len(runes) {
					advance()
				}
				continue
			}
			if curr == '"' {
				advance()
				state = scanCode
				continue
			}
			if curr == '$' && !isEscapedDollar(runes, i) {
				start := diag.NewPos(off, line, col)
				parseExpansion(start)
				continue
			}
			advance()
		case scanComment:
			if runes[i] == '\n' {
				advance()
				state = scanCode
				continue
			}
			advance()
		default:
			advance()
			continue
		}
	}
	return refs
}

func Names(text string) []string {
	refs := Collect(text, diag.Position{}, "")
	out := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		if _, ok := seen[ref.Name]; ok {
			continue
		}
		seen[ref.Name] = struct{}{}
		out = append(out, ref.Name)
	}
	return out
}

func isEscapedDollar(runes []rune, idx int) bool {
	count := 0
	for i := idx - 1; i >= 0; i-- {
		if runes[i] != '\\' {
			break
		}
		count++
	}
	return count%2 == 1
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsDigit(r) || isIdentStart(r)
}

func parseBareVarName(runes []rune, start int) (int, bool) {
	j := start
	if j >= len(runes) || !isIdentStart(runes[j]) {
		return 0, false
	}
	j++
	for j < len(runes) && isIdentPart(runes[j]) {
		j++
	}
	return j, true
}

func parseBracedVarRef(runes []rune, start int) (bracedVarRef, bool) {
	j := start
	if j >= len(runes) {
		return bracedVarRef{}, false
	}
	if runes[j] == '#' || runes[j] == '!' {
		j++
	}
	nameStart := j
	nameEnd, ok := parseBareVarName(runes, j)
	if !ok {
		return bracedVarRef{}, false
	}
	ref := bracedVarRef{
		Name:    string(runes[nameStart:nameEnd]),
		NameEnd: nameEnd,
	}
	j = nameEnd
	depth := 1
	for j < len(runes) {
		switch runes[j] {
		case '\\':
			j += 2
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				ref.End = j
				return ref, true
			}
		}
		j++
	}
	return bracedVarRef{}, false
}

func collectNestedBracedRefs(runes []rune, dollarIdx, suffixStart, suffixEnd int, dollarPos diag.Position, file string) []Ref {
	if suffixStart >= suffixEnd {
		return nil
	}
	base := positionAfter(dollarPos, runes, dollarIdx, suffixStart)
	return Collect(string(runes[suffixStart:suffixEnd]), base, file)
}

func positionAfter(pos diag.Position, runes []rune, from, to int) diag.Position {
	for idx := from; idx < to && idx < len(runes); idx++ {
		pos.Offset++
		if runes[idx] == '\n' {
			pos.Line++
			pos.Column = 1
		} else {
			pos.Column++
		}
	}
	return pos
}

func isCommentStart(runes []rune, idx int) bool {
	if idx < 0 || idx >= len(runes) || runes[idx] != '#' {
		return false
	}
	if idx == 0 {
		return true
	}
	return isShellCommentBoundary(runes[idx-1])
}

func isShellCommentBoundary(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	switch r {
	case ';', '|', '&', '(', ')', '{', '}':
		return true
	default:
		return false
	}
}
