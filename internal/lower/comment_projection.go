package lower

import (
	"strings"

	"jbs/internal/ast"
	"jbs/internal/sema"
)

func projectSourceComments(res *sema.Result) []CommentProjection {
	if res == nil {
		return nil
	}
	out := make([]CommentProjection, 0)
	for _, stmt := range res.Program.Stmts {
		switch node := stmt.(type) {
		case ast.DoBlock:
			out = append(out, projectHeaderComments("do", node.Name, node.Header)...)
		case ast.SubmitBlock:
			out = append(out, projectHeaderComments("submit", node.Name, node.Header)...)
		case ast.AnalyseBlock:
			out = append(out, projectHeaderComments("analyse", node.StepName, node.Header)...)
		}
	}
	return out
}

func projectHeaderComments(kind string, name string, header []ast.HeaderElem) []CommentProjection {
	base := kind + ":" + name + ".header"
	out := make([]CommentProjection, 0)
	for _, elem := range header {
		switch elem.Kind {
		case ast.HeaderElemComment:
			if elem.Comment != nil && strings.TrimSpace(elem.Comment.Text) != "" {
				out = append(out, CommentProjection{
					Target: base,
					Text:   strings.TrimSpace(elem.Comment.Text),
				})
			}
		case ast.HeaderElemAfter, ast.HeaderElemUse, ast.HeaderElemWith, ast.HeaderElemOption:
			if elem.Inline != nil && strings.TrimSpace(elem.Inline.Text) != "" {
				out = append(out, CommentProjection{
					Target: base + "." + headerElemLabel(elem.Kind),
					Text:   strings.TrimSpace(elem.Inline.Text),
				})
			}
		}
	}
	return out
}

func headerElemLabel(kind ast.HeaderElemKind) string {
	switch kind {
	case ast.HeaderElemAfter:
		return "after"
	case ast.HeaderElemUse:
		return "use"
	case ast.HeaderElemWith:
		return "with"
	case ast.HeaderElemOption:
		return "options"
	default:
		return "header"
	}
}
