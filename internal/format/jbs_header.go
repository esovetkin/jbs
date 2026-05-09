package format

import (
	"strconv"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
)

type headerClauseKind int

const (
	headerClauseAfter headerClauseKind = iota
	headerClauseWith
	headerClauseFSub
	headerClauseOptions
)

type renderedHeaderClause struct {
	Kind  headerClauseKind
	Lines []string
}

type headerCommentBucket struct {
	Before []string
	Inline string
}

func renderBlockHeader(kind, name string, after []string, with []ast.WithItem, nproc *int, fsubs []ast.FileSubstitution, header []ast.HeaderElem, srcRunes []rune) []string {
	lines := []string{kind + " " + name}
	clauses := buildRenderedHeaderClauses(after, with, nproc, fsubs, srcRunes)
	if len(header) == 0 {
		for _, clause := range clauses {
			lines = append(lines, renderHeaderClauseLines(clause, nil)...)
		}
		return lines
	}

	buckets, trailing := collectHeaderCommentBuckets(header)
	for _, clause := range clauses {
		bucket := buckets[clause.Kind]
		if bucket != nil {
			for _, text := range bucket.Before {
				lines = append(lines, renderHeaderCommentLine(text))
			}
		}
		lines = append(lines, renderHeaderClauseLines(clause, bucket)...)
	}
	for _, text := range trailing {
		lines = append(lines, renderHeaderCommentLine(text))
	}
	return lines
}

func renderHeaderClauseLines(clause renderedHeaderClause, bucket *headerCommentBucket) []string {
	lines := make([]string, 0, len(clause.Lines))
	for i, text := range clause.Lines {
		line := clauseIndent + text
		if i == 0 && bucket != nil && bucket.Inline != "" {
			line += "  " + bucket.Inline
		}
		lines = append(lines, line)
	}
	return lines
}

func buildRenderedHeaderClauses(after []string, with []ast.WithItem, nproc *int, fsubs []ast.FileSubstitution, srcRunes []rune) []renderedHeaderClause {
	clauses := make([]renderedHeaderClause, 0, 4)
	if len(after) > 0 {
		clauses = append(clauses, renderedHeaderClause{
			Kind:  headerClauseAfter,
			Lines: []string{"after " + strings.Join(after, ", ")},
		})
	}
	if len(with) > 0 {
		clauses = append(clauses, renderedHeaderClause{
			Kind:  headerClauseWith,
			Lines: []string{"with " + renderWithClause(with)},
		})
	}
	for _, fsub := range fsubs {
		clauses = append(clauses, renderedHeaderClause{
			Kind:  headerClauseFSub,
			Lines: renderFSubClause(fsub, srcRunes),
		})
	}
	if optionLine := renderStepOptionClause(nproc); optionLine != "" {
		clauses = append(clauses, renderedHeaderClause{
			Kind:  headerClauseOptions,
			Lines: []string{optionLine},
		})
	}
	return clauses
}

func collectHeaderCommentBuckets(header []ast.HeaderElem) (map[headerClauseKind]*headerCommentBucket, []string) {
	buckets := map[headerClauseKind]*headerCommentBucket{
		headerClauseAfter:   {},
		headerClauseWith:    {},
		headerClauseFSub:    {},
		headerClauseOptions: {},
	}
	pending := make([]string, 0)

	appendPending := func(kind headerClauseKind) {
		if len(pending) == 0 {
			return
		}
		buckets[kind].Before = append(buckets[kind].Before, pending...)
		pending = pending[:0]
	}

	for _, elem := range header {
		switch elem.Kind {
		case ast.HeaderElemBlank:
			pending = append(pending, "")
		case ast.HeaderElemComment:
			if elem.Comment != nil {
				pending = append(pending, "# "+strings.TrimSpace(elem.Comment.Text))
			} else {
				pending = append(pending, "#")
			}
		case ast.HeaderElemAfter, ast.HeaderElemWith, ast.HeaderElemFSub, ast.HeaderElemOption:
			kind := toHeaderClauseKind(elem.Kind)
			if elem.Inline != nil && buckets[kind].Inline != "" {
				buckets[kind].Before = append(buckets[kind].Before, buckets[kind].Inline)
				buckets[kind].Inline = ""
			}
			appendPending(kind)
			if elem.Inline != nil {
				inline := "# " + strings.TrimSpace(elem.Inline.Text)
				buckets[kind].Inline = strings.TrimSpace(inline)
			}
		default:
			text := strings.TrimSpace(elem.Text)
			if text != "" {
				pending = append(pending, text)
			}
			if elem.Inline != nil {
				pending = append(pending, "# "+strings.TrimSpace(elem.Inline.Text))
			}
		}
	}

	trailing := make([]string, len(pending))
	copy(trailing, pending)
	return buckets, trailing
}

func toHeaderClauseKind(kind ast.HeaderElemKind) headerClauseKind {
	switch kind {
	case ast.HeaderElemAfter:
		return headerClauseAfter
	case ast.HeaderElemWith:
		return headerClauseWith
	case ast.HeaderElemFSub:
		return headerClauseFSub
	default:
		return headerClauseOptions
	}
}

func renderHeaderCommentLine(text string) string {
	if text == "" {
		return ""
	}
	return clauseIndent + text
}

func renderStepOptionClause(nproc *int) string {
	parts := make([]string, 0, 1)
	if nproc != nil {
		parts = append(parts, "nproc "+strconv.Itoa(*nproc))
	}
	return strings.Join(parts, " ")
}

func renderWithClause(items []ast.WithItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if len(item.Selectors) == 0 {
			parts = append(parts, item.Source)
			continue
		}
		parts = append(parts, item.Source+"["+strings.Join(item.Selectors, ",")+"]")
	}
	return strings.Join(parts, ", ")
}

func renderFSubClause(fsub ast.FileSubstitution, srcRunes []rune) []string {
	lines := []string{"fsub " + strconv.Quote(fsub.Path) + " {"}
	for _, rule := range fsub.Rules {
		value := formatExprInline(rule.Expr, srcRunes)
		if value == "" && rule.Expr != nil {
			value = strings.TrimSpace(spanText(srcRunes, rule.Expr.GetSpan()))
		}
		if value == "" {
			value = `""`
		}
		lines = append(lines, bodyIndent+strconv.Quote(rule.Pattern)+": "+value+",")
	}
	lines = append(lines, "}")
	return lines
}
