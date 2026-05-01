package parser

import "strings"

type StructuralScanState struct {
	BraceDepth   int
	ParenDepth   int
	BracketDepth int
	InSingle     bool
	InDouble     bool
	Escaped      bool
	LineContinue bool
}

func (s StructuralScanState) NeedsMoreInput() bool {
	return s.BraceDepth > 0 ||
		s.ParenDepth > 0 ||
		s.BracketDepth > 0 ||
		s.InSingle ||
		s.InDouble ||
		s.LineContinue
}

func ScanStructuralState(src string) StructuralScanState {
	runes := []rune(src)
	state := StructuralScanState{}
	mode := blockScanCode
	escaped := false
	lineStart := 0

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch mode {
		case blockScanLineComment:
			if r == '\n' {
				mode = blockScanCode
				lineStart = i + 1
			}
		case blockScanSingleQuote:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				mode = blockScanCode
			}
		case blockScanDoubleQuote:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				mode = blockScanCode
			}
		default:
			switch r {
			case '#':
				mode = blockScanLineComment
			case '\'':
				mode = blockScanSingleQuote
				escaped = false
			case '"':
				mode = blockScanDoubleQuote
				escaped = false
			case '{':
				state.BraceDepth++
			case '}':
				if state.BraceDepth > 0 {
					state.BraceDepth--
				}
			case '(':
				state.ParenDepth++
			case ')':
				if state.ParenDepth > 0 {
					state.ParenDepth--
				}
			case '[':
				state.BracketDepth++
			case ']':
				if state.BracketDepth > 0 {
					state.BracketDepth--
				}
			case '\n':
				lineStart = i + 1
			}
		}
	}

	state.InSingle = mode == blockScanSingleQuote
	state.InDouble = mode == blockScanDoubleQuote
	state.Escaped = escaped
	if mode == blockScanCode {
		state.LineContinue = hasTrailingBackslashContinuationRunes(runes[lineStart:])
	}
	return state
}

func scanTopLevelStatementOffsets(src []rune, start int) (int, int) {
	if start >= len(src) {
		return start, start
	}
	state := StructuralScanState{}
	mode := blockScanCode
	escaped := false
	lineStart := start

	for i := start; i < len(src); i++ {
		r := src[i]
		switch mode {
		case blockScanLineComment:
			if r == '\n' {
				mode = blockScanCode
				lineStart = i + 1
				if state.BraceDepth == 0 && state.ParenDepth == 0 && state.BracketDepth == 0 {
					return i, i + 1
				}
			}
		case blockScanSingleQuote:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				mode = blockScanCode
			}
		case blockScanDoubleQuote:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				mode = blockScanCode
			}
		default:
			switch r {
			case '#':
				if state.BraceDepth == 0 && state.ParenDepth == 0 && state.BracketDepth == 0 {
					next := i
					for next < len(src) && src[next] != '\n' {
						next++
					}
					if next < len(src) && src[next] == '\n' {
						next++
					}
					return i, next
				}
				mode = blockScanLineComment
			case '\'':
				mode = blockScanSingleQuote
				escaped = false
			case '"':
				mode = blockScanDoubleQuote
				escaped = false
			case '{':
				state.BraceDepth++
			case '}':
				if state.BraceDepth > 0 {
					state.BraceDepth--
				} else {
					return i, i
				}
			case '(':
				state.ParenDepth++
			case ')':
				if state.ParenDepth > 0 {
					state.ParenDepth--
				}
			case '[':
				state.BracketDepth++
			case ']':
				if state.BracketDepth > 0 {
					state.BracketDepth--
				}
			case ';':
				if state.BraceDepth == 0 && state.ParenDepth == 0 && state.BracketDepth == 0 {
					return i, i + 1
				}
			case '\n':
				continues := hasTrailingBackslashContinuationRunes(src[lineStart:i])
				lineStart = i + 1
				if continues {
					continue
				}
				if state.BraceDepth == 0 && state.ParenDepth == 0 && state.BracketDepth == 0 {
					return i, i + 1
				}
			}
		}
	}

	return len(src), len(src)
}

func hasTrailingBackslashContinuationRunes(line []rune) bool {
	trimmed := strings.TrimRight(string(line), " \t\r")
	if trimmed == "" {
		return false
	}
	n := 0
	for i := len(trimmed) - 1; i >= 0 && trimmed[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}
