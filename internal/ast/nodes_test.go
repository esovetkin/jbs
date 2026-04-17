package ast

import (
	"testing"

	"jbs/internal/diag"
)

var (
	_ Node     = Program{}
	_ Node     = Comment{}
	_ Node     = HeaderElem{}
	_ Node     = UseSource{}
	_ Node     = WithItem{}
	_ Node     = Assignment{}
	_ Node     = AnalyseAssign{}
	_ Node     = AnalyseColumn{}
	_ Node     = SubmitField{}
	_ Stmt     = UseStmt{}
	_ Stmt     = GlobalAssign{}
	_ Stmt     = AnalyseBlock{}
	_ Stmt     = DoBlock{}
	_ Stmt     = SubmitBlock{}
	_ Expr     = IdentExpr{}
	_ Expr     = QualifiedIdentExpr{}
	_ Expr     = IndexExpr{}
	_ Expr     = StringExpr{}
	_ Expr     = NumberExpr{}
	_ Expr     = BoolExpr{}
	_ Expr     = ListExpr{}
	_ Expr     = TupleExpr{}
	_ Expr     = ConvertExpr{}
	_ Expr     = CallExpr{}
	_ Expr     = AliasExpr{}
	_ Expr     = UnaryExpr{}
	_ Expr     = BinaryExpr{}
	_ Expr     = CompareExpr{}
	_ Expr     = ConditionalExpr{}
	_ Expr     = ModeExpr{}
	_ CombExpr = CombIdent{}
	_ CombExpr = CombBinary{}
)

func testSpan(n int) diag.Span {
	return diag.NewSpan(
		"nodes.jbs",
		diag.NewPos(n*10, n, 1),
		diag.NewPos(n*10+4, n, 5),
	)
}

func TestPlainNodeGetSpan(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want diag.Span
	}{
		{name: "Program", node: Program{Span: testSpan(1)}, want: testSpan(1)},
		{name: "Comment", node: Comment{Span: testSpan(2)}, want: testSpan(2)},
		{name: "HeaderElem", node: HeaderElem{Span: testSpan(3)}, want: testSpan(3)},
		{name: "UseSource", node: UseSource{Span: testSpan(4)}, want: testSpan(4)},
		{name: "WithItem", node: WithItem{Span: testSpan(5)}, want: testSpan(5)},
		{name: "Assignment", node: Assignment{Span: testSpan(6)}, want: testSpan(6)},
		{name: "AnalyseAssign", node: AnalyseAssign{Span: testSpan(7)}, want: testSpan(7)},
		{name: "AnalyseColumn", node: AnalyseColumn{Span: testSpan(8)}, want: testSpan(8)},
		{name: "SubmitField", node: SubmitField{Span: testSpan(9)}, want: testSpan(9)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.node.GetSpan(); got != tc.want {
				t.Fatalf("GetSpan() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestStmtNodes(t *testing.T) {
	useStmt := UseStmt{Span: testSpan(10)}
	globalAssign := GlobalAssign{Span: testSpan(11)}
	analyseBlock := AnalyseBlock{Span: testSpan(12)}
	doBlock := DoBlock{Span: testSpan(13)}
	submitBlock := SubmitBlock{Span: testSpan(14)}

	tests := []struct {
		name string
		call func()
		node Node
		want diag.Span
	}{
		{name: "UseStmt", call: func() { useStmt.stmtNode() }, node: useStmt, want: testSpan(10)},
		{name: "GlobalAssign", call: func() { globalAssign.stmtNode() }, node: globalAssign, want: testSpan(11)},
		{name: "AnalyseBlock", call: func() { analyseBlock.stmtNode() }, node: analyseBlock, want: testSpan(12)},
		{name: "DoBlock", call: func() { doBlock.stmtNode() }, node: doBlock, want: testSpan(13)},
		{name: "SubmitBlock", call: func() { submitBlock.stmtNode() }, node: submitBlock, want: testSpan(14)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.call()
			if got := tc.node.GetSpan(); got != tc.want {
				t.Fatalf("GetSpan() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestExprNodes(t *testing.T) {
	ident := IdentExpr{Span: testSpan(15)}
	qualified := QualifiedIdentExpr{Span: testSpan(16)}
	index := IndexExpr{Span: testSpan(17)}
	str := StringExpr{Span: testSpan(18)}
	number := NumberExpr{Span: testSpan(19)}
	boolean := BoolExpr{Span: testSpan(20)}
	list := ListExpr{Span: testSpan(21)}
	tuple := TupleExpr{Span: testSpan(22)}
	convert := ConvertExpr{Span: testSpan(23)}
	call := CallExpr{Span: testSpan(24)}
	alias := AliasExpr{Span: testSpan(25)}
	unary := UnaryExpr{Span: testSpan(26)}
	binary := BinaryExpr{Span: testSpan(27)}
	compare := CompareExpr{Span: testSpan(28)}
	conditional := ConditionalExpr{Span: testSpan(29)}
	mode := ModeExpr{Span: testSpan(30)}

	tests := []struct {
		name string
		call func()
		node Node
		want diag.Span
	}{
		{name: "IdentExpr", call: func() { ident.exprNode() }, node: ident, want: testSpan(15)},
		{name: "QualifiedIdentExpr", call: func() { qualified.exprNode() }, node: qualified, want: testSpan(16)},
		{name: "IndexExpr", call: func() { index.exprNode() }, node: index, want: testSpan(17)},
		{name: "StringExpr", call: func() { str.exprNode() }, node: str, want: testSpan(18)},
		{name: "NumberExpr", call: func() { number.exprNode() }, node: number, want: testSpan(19)},
		{name: "BoolExpr", call: func() { boolean.exprNode() }, node: boolean, want: testSpan(20)},
		{name: "ListExpr", call: func() { list.exprNode() }, node: list, want: testSpan(21)},
		{name: "TupleExpr", call: func() { tuple.exprNode() }, node: tuple, want: testSpan(22)},
		{name: "ConvertExpr", call: func() { convert.exprNode() }, node: convert, want: testSpan(23)},
		{name: "CallExpr", call: func() { call.exprNode() }, node: call, want: testSpan(24)},
		{name: "AliasExpr", call: func() { alias.exprNode() }, node: alias, want: testSpan(25)},
		{name: "UnaryExpr", call: func() { unary.exprNode() }, node: unary, want: testSpan(26)},
		{name: "BinaryExpr", call: func() { binary.exprNode() }, node: binary, want: testSpan(27)},
		{name: "CompareExpr", call: func() { compare.exprNode() }, node: compare, want: testSpan(28)},
		{name: "ConditionalExpr", call: func() { conditional.exprNode() }, node: conditional, want: testSpan(29)},
		{name: "ModeExpr", call: func() { mode.exprNode() }, node: mode, want: testSpan(30)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.call()
			if got := tc.node.GetSpan(); got != tc.want {
				t.Fatalf("GetSpan() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCombNodes(t *testing.T) {
	ident := CombIdent{Span: testSpan(31)}
	binary := CombBinary{Span: testSpan(32)}

	tests := []struct {
		name string
		call func()
		node Node
		want diag.Span
	}{
		{name: "CombIdent", call: func() { ident.combNode() }, node: ident, want: testSpan(31)},
		{name: "CombBinary", call: func() { binary.combNode() }, node: binary, want: testSpan(32)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.call()
			if got := tc.node.GetSpan(); got != tc.want {
				t.Fatalf("GetSpan() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
