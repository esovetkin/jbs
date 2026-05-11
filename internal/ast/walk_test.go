package ast

import (
	"reflect"
	"testing"
)

func walkIdent(name string) IdentExpr {
	return IdentExpr{Name: name}
}

func collectWalkIdentNames(expr Expr) []string {
	var got []string
	WalkExpr(expr, WalkCallbacks{
		Expr: func(expr Expr) WalkAction {
			if ident, ok := expr.(IdentExpr); ok {
				got = append(got, ident.Name)
			}
			return WalkContinue
		},
	})
	return got
}

func TestWalkExprNil(t *testing.T) {
	called := false
	if !WalkExpr(nil, WalkCallbacks{Expr: func(Expr) WalkAction {
		called = true
		return WalkContinue
	}}) {
		t.Fatalf("nil expression walk stopped")
	}
	if called {
		t.Fatalf("nil expression called callback")
	}
}

func TestWalkExprPreOrder(t *testing.T) {
	expr := BinaryExpr{
		Left:  walkIdent("left"),
		Op:    "+",
		Right: walkIdent("right"),
	}
	var got []string
	WalkExpr(expr, WalkCallbacks{
		Expr: func(expr Expr) WalkAction {
			switch expr.(type) {
			case BinaryExpr:
				got = append(got, "binary")
			case IdentExpr:
				got = append(got, expr.(IdentExpr).Name)
			}
			return WalkContinue
		},
	})
	want := []string{"binary", "left", "right"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWalkExprVisitsChildExpressions(t *testing.T) {
	expr := ListExpr{Items: []Expr{
		MemberExpr{Base: walkIdent("member_base"), Name: "field"},
		ListExpr{Items: []Expr{walkIdent("list_item")}},
		TupleExpr{Items: []Expr{walkIdent("tuple_item")}},
		DictExpr{Entries: []DictEntryExpr{{
			Key:   walkIdent("dict_key"),
			Value: walkIdent("dict_value"),
		}}},
		CallExpr{
			Callee: walkIdent("callee"),
			Args:   []CallArg{PosCallArg(walkIdent("arg"))},
		},
		FunctionExpr{
			Params: []FuncParam{{Name: "p", Default: walkIdent("param_default")}},
			Body:   []FuncBodyStmt{ExprStmt{Expr: walkIdent("function_body")}},
		},
		AliasExpr{Expr: walkIdent("alias_expr"), Alias: "alias"},
		IndexExpr{Base: walkIdent("index_base"), Items: []Expr{walkIdent("index_item")}},
		UnaryExpr{Op: "-", Expr: walkIdent("unary_expr")},
		BinaryExpr{Left: walkIdent("binary_left"), Op: "+", Right: walkIdent("binary_right")},
		CompareExpr{Left: walkIdent("compare_left"), Op: "==", Right: walkIdent("compare_right")},
		ConditionalExpr{Then: walkIdent("cond_then"), Cond: walkIdent("cond_cond"), Else: walkIdent("cond_else")},
	}}

	got := collectWalkIdentNames(expr)
	want := []string{
		"member_base",
		"list_item",
		"tuple_item",
		"dict_key",
		"dict_value",
		"callee",
		"arg",
		"param_default",
		"function_body",
		"alias_expr",
		"index_base",
		"index_item",
		"unary_expr",
		"binary_left",
		"binary_right",
		"compare_left",
		"compare_right",
		"cond_then",
		"cond_cond",
		"cond_else",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWalkFuncBodyVisitsChildExpressions(t *testing.T) {
	body := []FuncBodyStmt{
		LocalAssignStmt{Name: "x", Expr: walkIdent("assign_expr")},
		ReturnStmt{Expr: walkIdent("return_expr")},
		ExprStmt{Expr: walkIdent("expr_stmt")},
		FuncIfStmt{
			Cond: walkIdent("if_cond"),
			Then: []FuncBodyStmt{ExprStmt{Expr: walkIdent("if_then")}},
			Elifs: []FuncElifBranch{{
				Cond: walkIdent("elif_cond"),
				Body: []FuncBodyStmt{ExprStmt{Expr: walkIdent("elif_body")}},
			}},
			Else: []FuncBodyStmt{ExprStmt{Expr: walkIdent("if_else")}},
		},
		FuncForStmt{
			Target:   "i",
			Iterable: walkIdent("for_iterable"),
			Body:     []FuncBodyStmt{ExprStmt{Expr: walkIdent("for_body")}},
		},
		FuncWhileStmt{
			Cond: walkIdent("while_cond"),
			Body: []FuncBodyStmt{ExprStmt{Expr: walkIdent("while_body")}},
		},
		BreakStmt{},
		ContinueStmt{},
	}

	var got []string
	ok := WalkFuncBody(body, WalkCallbacks{
		Expr: func(expr Expr) WalkAction {
			if ident, ok := expr.(IdentExpr); ok {
				got = append(got, ident.Name)
			}
			return WalkContinue
		},
	})
	if !ok {
		t.Fatalf("walk stopped unexpectedly")
	}
	want := []string{
		"assign_expr",
		"return_expr",
		"expr_stmt",
		"if_cond",
		"if_then",
		"elif_cond",
		"elif_body",
		"if_else",
		"for_iterable",
		"for_body",
		"while_cond",
		"while_body",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWalkSkipChildren(t *testing.T) {
	expr := ListExpr{Items: []Expr{walkIdent("child")}}
	var got []string
	WalkExpr(expr, WalkCallbacks{
		Expr: func(expr Expr) WalkAction {
			switch expr.(type) {
			case ListExpr:
				got = append(got, "list")
				return WalkSkipChildren
			case IdentExpr:
				got = append(got, expr.(IdentExpr).Name)
			}
			return WalkContinue
		},
	})
	want := []string{"list"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestWalkStop(t *testing.T) {
	expr := TupleExpr{Items: []Expr{walkIdent("first"), walkIdent("second")}}
	var got []string
	ok := WalkExpr(expr, WalkCallbacks{
		Expr: func(expr Expr) WalkAction {
			if ident, ok := expr.(IdentExpr); ok {
				got = append(got, ident.Name)
				return WalkStop
			}
			return WalkContinue
		},
	})
	if ok {
		t.Fatalf("walk did not report stop")
	}
	want := []string{"first"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	ok = WalkFuncBody([]FuncBodyStmt{ExprStmt{Expr: walkIdent("body")}}, WalkCallbacks{
		FuncBodyStmt: func(FuncBodyStmt) WalkAction {
			return WalkStop
		},
	})
	if ok {
		t.Fatalf("function body walk did not report stop")
	}
}
