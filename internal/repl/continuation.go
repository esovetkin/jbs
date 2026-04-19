package repl

import "strings"

type ContinuationState struct {
	BraceDepth   int
	ParenDepth   int
	BracketDepth int
	InSingle     bool
	InDouble     bool
	Escaped      bool
	LineContinue bool
}

func (s ContinuationState) NeedsMoreInput() bool {
	return s.BraceDepth > 0 ||
		s.InSingle ||
		s.InDouble ||
		s.LineContinue
}

type scannerMode uint8

const (
	modeCode scannerMode = iota
	modeSingle
	modeDouble
	modeComment
)

func ScanContinuationState(src string) ContinuationState {
	state := ContinuationState{}
	mode := modeCode
	escaped := false
	for _, r := range src {
		switch mode {
		case modeComment:
			if r == '\n' {
				mode = modeCode
			}
		case modeSingle:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				mode = modeCode
			}
		case modeDouble:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				mode = modeCode
			}
		default:
			switch r {
			case '#':
				mode = modeComment
			case '\'':
				mode = modeSingle
				escaped = false
			case '"':
				mode = modeDouble
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
			}
		}
	}

	state.InSingle = mode == modeSingle
	state.InDouble = mode == modeDouble
	state.Escaped = escaped
	if mode == modeCode {
		state.LineContinue = hasTrailingBackslashContinuation(src)
	}
	return state
}

func hasTrailingBackslashContinuation(src string) bool {
	lastLine := src
	if idx := strings.LastIndex(lastLine, "\n"); idx >= 0 {
		lastLine = lastLine[idx+1:]
	}
	codeLine, mode := stripLineComment(lastLine)
	if mode != modeCode {
		return false
	}
	trimmed := strings.TrimRight(codeLine, " \t\r")
	if trimmed == "" {
		return false
	}
	n := 0
	for i := len(trimmed) - 1; i >= 0 && trimmed[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

func stripLineComment(line string) (string, scannerMode) {
	mode := modeCode
	escaped := false
	runes := []rune(line)
	for i, r := range runes {
		switch mode {
		case modeSingle:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				mode = modeCode
			}
		case modeDouble:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				mode = modeCode
			}
		default:
			if r == '#' {
				return string(runes[:i]), modeCode
			}
			if r == '\'' {
				mode = modeSingle
				escaped = false
				continue
			}
			if r == '"' {
				mode = modeDouble
				escaped = false
				continue
			}
		}
	}
	return line, mode
}
