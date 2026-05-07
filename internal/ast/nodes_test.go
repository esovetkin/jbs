package ast

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

var (
	_ Node         = Program{}
	_ Node         = Comment{}
	_ Node         = HeaderElem{}
	_ Node         = UseSource{}
	_ Node         = WithItem{}
	_ Node         = Assignment{}
	_ Node         = AnalyseAssign{}
	_ Node         = AnalyseColumn{}
	_ Node         = CallArg{}
	_ Node         = FuncParam{}
	_ Node         = LocalAssignStmt{}
	_ Node         = ReturnStmt{}
	_ Node         = FuncIfStmt{}
	_ Node         = FuncForStmt{}
	_ Node         = FuncWhileStmt{}
	_ Stmt         = UseStmt{}
	_ Stmt         = GlobalAssign{}
	_ Stmt         = ExprStmt{}
	_ Stmt         = IfStmt{}
	_ Stmt         = ForStmt{}
	_ Stmt         = WhileStmt{}
	_ Stmt         = BreakStmt{}
	_ Stmt         = ContinueStmt{}
	_ Stmt         = AnalyseBlock{}
	_ Stmt         = DoBlock{}
	_ Expr         = IdentExpr{}
	_ Expr         = QualifiedIdentExpr{}
	_ Expr         = MemberExpr{}
	_ Expr         = IndexExpr{}
	_ Expr         = StringExpr{}
	_ Expr         = NumberExpr{}
	_ Expr         = BoolExpr{}
	_ Expr         = ListExpr{}
	_ Expr         = TupleExpr{}
	_ Expr         = CallExpr{}
	_ Expr         = FunctionExpr{}
	_ Expr         = AliasExpr{}
	_ Expr         = UnaryExpr{}
	_ Expr         = BinaryExpr{}
	_ Expr         = CompareExpr{}
	_ Expr         = ConditionalExpr{}
	_ FuncBodyStmt = ExprStmt{}
	_ FuncBodyStmt = LocalAssignStmt{}
	_ FuncBodyStmt = ReturnStmt{}
	_ FuncBodyStmt = FuncIfStmt{}
	_ FuncBodyStmt = FuncForStmt{}
	_ FuncBodyStmt = FuncWhileStmt{}
	_ FuncBodyStmt = BreakStmt{}
	_ FuncBodyStmt = ContinueStmt{}
	_ CombExpr     = CombIdent{}
	_ CombExpr     = CombBinary{}
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
		{name: "CallArg", node: CallArg{Span: testSpan(10)}, want: testSpan(10)},
		{name: "FuncParam", node: FuncParam{Span: testSpan(11)}, want: testSpan(11)},
		{name: "LocalAssignStmt", node: LocalAssignStmt{Span: testSpan(12)}, want: testSpan(12)},
		{name: "ReturnStmt", node: ReturnStmt{Span: testSpan(13)}, want: testSpan(13)},
		{name: "FuncIfStmt", node: FuncIfStmt{Span: testSpan(14)}, want: testSpan(14)},
		{name: "FuncForStmt", node: FuncForStmt{Span: testSpan(15)}, want: testSpan(15)},
		{name: "FuncWhileStmt", node: FuncWhileStmt{Span: testSpan(16)}, want: testSpan(16)},
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
	useStmt := UseStmt{Span: testSpan(20)}
	globalAssign := GlobalAssign{Span: testSpan(21)}
	exprStmt := ExprStmt{Span: testSpan(22)}
	ifStmt := IfStmt{Span: testSpan(23)}
	forStmt := ForStmt{Span: testSpan(24)}
	whileStmt := WhileStmt{Span: testSpan(25)}
	breakStmt := BreakStmt{Span: testSpan(26)}
	continueStmt := ContinueStmt{Span: testSpan(27)}
	analyseBlock := AnalyseBlock{Span: testSpan(28)}
	doBlock := DoBlock{Span: testSpan(29)}

	tests := []struct {
		name string
		call func()
		node Node
		want diag.Span
	}{
		{name: "UseStmt", call: func() { useStmt.stmtNode() }, node: useStmt, want: testSpan(20)},
		{name: "GlobalAssign", call: func() { globalAssign.stmtNode() }, node: globalAssign, want: testSpan(21)},
		{name: "ExprStmt", call: func() { exprStmt.stmtNode(); exprStmt.funcBodyStmtNode() }, node: exprStmt, want: testSpan(22)},
		{name: "IfStmt", call: func() { ifStmt.stmtNode() }, node: ifStmt, want: testSpan(23)},
		{name: "ForStmt", call: func() { forStmt.stmtNode() }, node: forStmt, want: testSpan(24)},
		{name: "WhileStmt", call: func() { whileStmt.stmtNode() }, node: whileStmt, want: testSpan(25)},
		{name: "BreakStmt", call: func() { breakStmt.stmtNode(); breakStmt.funcBodyStmtNode() }, node: breakStmt, want: testSpan(26)},
		{name: "ContinueStmt", call: func() { continueStmt.stmtNode(); continueStmt.funcBodyStmtNode() }, node: continueStmt, want: testSpan(27)},
		{name: "AnalyseBlock", call: func() { analyseBlock.stmtNode() }, node: analyseBlock, want: testSpan(28)},
		{name: "DoBlock", call: func() { doBlock.stmtNode() }, node: doBlock, want: testSpan(29)},
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
	ident := IdentExpr{Span: testSpan(30)}
	qualified := QualifiedIdentExpr{Span: testSpan(31)}
	member := MemberExpr{Span: testSpan(32)}
	index := IndexExpr{Span: testSpan(33)}
	str := StringExpr{Span: testSpan(34)}
	number := NumberExpr{Span: testSpan(35)}
	boolean := BoolExpr{Span: testSpan(36)}
	list := ListExpr{Span: testSpan(37)}
	tuple := TupleExpr{Span: testSpan(38)}
	call := CallExpr{Span: testSpan(40)}
	function := FunctionExpr{Span: testSpan(41)}
	alias := AliasExpr{Span: testSpan(42)}
	unary := UnaryExpr{Span: testSpan(43)}
	binary := BinaryExpr{Span: testSpan(44)}
	compare := CompareExpr{Span: testSpan(45)}
	conditional := ConditionalExpr{Span: testSpan(46)}

	tests := []struct {
		name string
		call func()
		node Node
		want diag.Span
	}{
		{name: "IdentExpr", call: func() { ident.exprNode() }, node: ident, want: testSpan(30)},
		{name: "QualifiedIdentExpr", call: func() { qualified.exprNode() }, node: qualified, want: testSpan(31)},
		{name: "MemberExpr", call: func() { member.exprNode() }, node: member, want: testSpan(32)},
		{name: "IndexExpr", call: func() { index.exprNode() }, node: index, want: testSpan(33)},
		{name: "StringExpr", call: func() { str.exprNode() }, node: str, want: testSpan(34)},
		{name: "NumberExpr", call: func() { number.exprNode() }, node: number, want: testSpan(35)},
		{name: "BoolExpr", call: func() { boolean.exprNode() }, node: boolean, want: testSpan(36)},
		{name: "ListExpr", call: func() { list.exprNode() }, node: list, want: testSpan(37)},
		{name: "TupleExpr", call: func() { tuple.exprNode() }, node: tuple, want: testSpan(38)},
		{name: "CallExpr", call: func() { call.exprNode() }, node: call, want: testSpan(40)},
		{name: "FunctionExpr", call: func() { function.exprNode() }, node: function, want: testSpan(41)},
		{name: "AliasExpr", call: func() { alias.exprNode() }, node: alias, want: testSpan(42)},
		{name: "UnaryExpr", call: func() { unary.exprNode() }, node: unary, want: testSpan(43)},
		{name: "BinaryExpr", call: func() { binary.exprNode() }, node: binary, want: testSpan(44)},
		{name: "CompareExpr", call: func() { compare.exprNode() }, node: compare, want: testSpan(45)},
		{name: "ConditionalExpr", call: func() { conditional.exprNode() }, node: conditional, want: testSpan(46)},
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

func TestFuncBodyStmtNodes(t *testing.T) {
	assign := LocalAssignStmt{Span: testSpan(50)}
	ret := ReturnStmt{Span: testSpan(51)}
	expr := ExprStmt{Span: testSpan(52)}

	tests := []struct {
		name string
		call func()
		node Node
		want diag.Span
	}{
		{name: "LocalAssignStmt", call: func() { assign.funcBodyStmtNode() }, node: assign, want: testSpan(50)},
		{name: "ReturnStmt", call: func() { ret.funcBodyStmtNode() }, node: ret, want: testSpan(51)},
		{name: "ExprStmt", call: func() { expr.funcBodyStmtNode() }, node: expr, want: testSpan(52)},
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

func TestCallArgHelpers(t *testing.T) {
	first := NumberExpr{Span: testSpan(60)}
	second := IdentExpr{Name: "x", Span: testSpan(61)}
	args := PosCallArgs(first, second)
	if len(args) != 2 {
		t.Fatalf("expected 2 positional args, got %#v", args)
	}
	if args[0].Name != "" || args[1].Name != "" {
		t.Fatalf("expected positional args to have empty names, got %#v", args)
	}
	if args[0].Expr != first || args[1].Expr != second {
		t.Fatalf("unexpected positional arg payloads: %#v", args)
	}
	if args[0].Span != testSpan(60) || args[1].Span != testSpan(61) {
		t.Fatalf("unexpected positional arg spans: %#v", args)
	}

	named := CallArg{Name: "value", Expr: second, Span: testSpan(62)}
	if named.Name != "value" || named.Expr != second || named.GetSpan() != testSpan(62) {
		t.Fatalf("unexpected named call arg: %#v", named)
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
