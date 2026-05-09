package parser

import (
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func parseHeaderElements(file string, raw string, start diag.Position) []ast.HeaderElem {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	if raw == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	out := make([]ast.HeaderElem, 0, len(lines))
	pos := start
	fsubDepth := 0
	for idx, line := range lines {
		lineStart := pos
		lineEnd := advancePosition(lineStart, line)
		span := diag.NewSpan(file, lineStart, lineEnd)

		trimmed := strings.TrimSpace(line)
		code, commentText, hasComment := splitLineComment(line)
		codeTrimmed := strings.TrimSpace(code)
		if fsubDepth > 0 {
			fsubDepth += headerBraceDelta(code)
			if fsubDepth < 0 {
				fsubDepth = 0
			}
		} else if trimmed == "" {
			out = append(out, ast.HeaderElem{
				Kind: ast.HeaderElemBlank,
				Text: "",
				Span: span,
			})
		} else {
			if codeTrimmed == "" {
				comment := ast.Comment{
					Text: strings.TrimSpace(commentText),
					Span: span,
				}
				out = append(out, ast.HeaderElem{
					Kind:    ast.HeaderElemComment,
					Comment: &comment,
					Span:    span,
				})
			} else {
				elem := ast.HeaderElem{
					Kind: classifyHeaderElemKind(codeTrimmed),
					Text: codeTrimmed,
					Span: span,
				}
				if hasComment {
					comment := ast.Comment{
						Text: strings.TrimSpace(commentText),
						Span: span,
					}
					elem.Inline = &comment
				}
				out = append(out, elem)
				if elem.Kind == ast.HeaderElemFSub {
					fsubDepth += headerBraceDelta(code)
					if fsubDepth < 0 {
						fsubDepth = 0
					}
				}
			}
		}

		if idx < len(lines)-1 {
			pos = diag.NewPos(lineEnd.Offset+1, lineEnd.Line+1, 1)
		} else {
			pos = lineEnd
		}
	}

	return trimHeaderBlankEdges(out)
}

func advancePosition(start diag.Position, text string) diag.Position {
	line := start.Line
	col := start.Column
	offset := start.Offset
	for _, r := range text {
		offset++
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return diag.NewPos(offset, line, col)
}

func trimHeaderBlankEdges(in []ast.HeaderElem) []ast.HeaderElem {
	start := 0
	for start < len(in) && in[start].Kind == ast.HeaderElemBlank {
		start++
	}
	end := len(in)
	for end > start && in[end-1].Kind == ast.HeaderElemBlank {
		end--
	}
	if start >= end {
		return nil
	}
	out := make([]ast.HeaderElem, end-start)
	copy(out, in[start:end])
	return out
}

func splitLineComment(line string) (code string, comment string, hasComment bool) {
	inSingle := false
	inDouble := false
	escaped := false
	for idx, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if inSingle {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inDouble = false
			}
			continue
		}
		if r == '\'' {
			inSingle = true
			continue
		}
		if r == '"' {
			inDouble = true
			continue
		}
		if r == '#' {
			return line[:idx], line[idx+1:], true
		}
	}
	return line, "", false
}

func headerBraceDelta(line string) int {
	inSingle := false
	inDouble := false
	escaped := false
	delta := 0
	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if inSingle {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inDouble = false
			}
			continue
		}
		switch r {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

func classifyHeaderElemKind(code string) ast.HeaderElemKind {
	if hasKeywordPrefix(code, "after") {
		return ast.HeaderElemAfter
	}
	if hasKeywordPrefix(code, "with") {
		return ast.HeaderElemWith
	}
	if hasKeywordPrefix(code, "fsub") {
		return ast.HeaderElemFSub
	}
	if isDoHeaderOptionLine(code) {
		return ast.HeaderElemOption
	}
	return ast.HeaderElemUnknown
}

func hasKeywordPrefix(text string, keyword string) bool {
	if !strings.HasPrefix(text, keyword) {
		return false
	}
	if len(text) == len(keyword) {
		return true
	}
	r := rune(text[len(keyword)])
	return unicode.IsSpace(r)
}

func isDoHeaderOptionLine(text string) bool {
	if !hasKeywordPrefix(text, "nproc") {
		return false
	}
	return strings.TrimSpace(text[len("nproc"):]) != ""
}
