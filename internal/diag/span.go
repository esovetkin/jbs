// define diagnostic message locations across source files
package diag

import "fmt"

type Position struct {
	Offset int
	Line   int
	Column int
}

type Span struct {
	File  string
	Start Position
	End   Position
}

func NewPos(offset, line, column int) Position {
	return Position{Offset: offset, Line: line, Column: column}
}

func NewSpan(file string, start, end Position) Span {
	return Span{File: file, Start: start, End: end}
}

func (s Span) IsZero() bool {
	return s.Start.Line == 0 && s.Start.Column == 0 && s.End.Line == 0 && s.End.Column == 0
}

func (s Span) String() string {
	if s.IsZero() {
		if s.File == "" {
			return "<unknown>"
		}
		return s.File
	}
	file := s.File
	if file == "" {
		file = "<input>"
	}
	return fmt.Sprintf("%s:%d:%d", file, s.Start.Line, s.Start.Column)
}

func Merge(a, b Span) Span {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.File == "" {
		a.File = b.File
	}
	start := a.Start
	if b.Start.Offset < start.Offset {
		start = b.Start
	}
	end := a.End
	if b.End.Offset > end.Offset {
		end = b.End
	}
	return Span{File: a.File, Start: start, End: end}
}
