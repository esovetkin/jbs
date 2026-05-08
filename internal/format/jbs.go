// implement the `jbs fmt`
//
// parse source into AST and rewrite the code in a canonical layout.
// Handle here bash blocks inside `do`.
package format

import (
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/parser"
)

const (
	clauseIndent       = "        "
	bodyIndent         = "        "
	continuationIndent = "    "
)

func JBS(file string, src string, diags *diag.Diagnostics) (string, error) {
	prog := parser.Parse(file, src, diags)
	if diags.HasErrors() {
		return "", nil
	}
	return formatProgram(prog, src), nil
}

func formatProgram(prog ast.Program, src string) string {
	normalized := normalizeLineEndings(src)
	srcRunes := []rune(normalized)
	if len(prog.Stmts) == 0 {
		return formatSourceWithoutStatements(normalized)
	}
	ranges := collectStmtRanges(prog.Stmts, len(srcRunes))
	lines := make([]formattedLine, 0)
	var prev ast.Stmt
	cursor := 0
	for idx, rng := range ranges {
		allowInline := idx > 0 && !isLineStartOffset(srcRunes, cursor)
		trivia := extractTopLevelTrivia(sliceSourceRange(srcRunes, cursor, rng.Start), allowInline)
		if trivia.InlineSuffix != "" && len(lines) > 0 {
			lines[len(lines)-1].Text += trivia.InlineSuffix
		}
		if len(trivia.Lines) > 0 {
			lines = append(lines, plainLines(trivia.Lines)...)
		} else if idx > 0 {
			if !(isGlobal(prev) && isGlobal(rng.Stmt)) {
				lines = append(lines, plainLine(""))
			}
		}
		lines = append(lines, formatStmt(rng.Stmt, srcRunes)...)
		prev = rng.Stmt
		cursor = rng.End
	}
	allowTrailingInline := len(ranges) > 0 && !isLineStartOffset(srcRunes, cursor)
	trailingTrivia := extractTopLevelTrivia(sliceSourceRange(srcRunes, cursor, len(srcRunes)), allowTrailingInline)
	if trailingTrivia.InlineSuffix != "" && len(lines) > 0 {
		lines[len(lines)-1].Text += trailingTrivia.InlineSuffix
	}
	if len(trailingTrivia.Lines) > 0 {
		lines = append(lines, plainLines(trailingTrivia.Lines)...)
	}
	for i := range lines {
		if !lines[i].PreserveTrailingSpace {
			lines[i].Text = strings.TrimRight(lines[i].Text, " \t")
		}
	}
	return strings.Join(formattedLineTexts(lines), "\n") + "\n"
}

func formatSourceWithoutStatements(src string) string {
	lines := strings.Split(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	if len(lines) == 0 {
		return "\n"
	}
	return strings.Join(lines, "\n") + "\n"
}
