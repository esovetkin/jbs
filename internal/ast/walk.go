package ast

type WalkAction uint8

const (
	WalkContinue WalkAction = iota
	WalkSkipChildren
	WalkStop
)

type WalkCallbacks struct {
	Expr         func(Expr) WalkAction
	FuncBodyStmt func(FuncBodyStmt) WalkAction
}

func WalkExpr(expr Expr, callbacks WalkCallbacks) bool {
	return walkExpr(expr, callbacks)
}

func WalkFuncBody(body []FuncBodyStmt, callbacks WalkCallbacks) bool {
	for _, stmt := range body {
		if !walkFuncBodyStmt(stmt, callbacks) {
			return false
		}
	}
	return true
}

func walkExpr(expr Expr, callbacks WalkCallbacks) bool {
	if expr == nil {
		return true
	}
	if callbacks.Expr != nil {
		switch callbacks.Expr(expr) {
		case WalkStop:
			return false
		case WalkSkipChildren:
			return true
		}
	}

	switch node := expr.(type) {
	case IdentExpr, QualifiedIdentExpr, StringExpr, NumberExpr, BoolExpr:
		return true
	case MemberExpr:
		return walkExpr(node.Base, callbacks)
	case ListExpr:
		return walkExprList(node.Items, callbacks)
	case TupleExpr:
		return walkExprList(node.Items, callbacks)
	case DictExpr:
		for _, entry := range node.Entries {
			if !walkExpr(entry.Key, callbacks) || !walkExpr(entry.Value, callbacks) {
				return false
			}
		}
		return true
	case CallExpr:
		if !walkExpr(node.Callee, callbacks) {
			return false
		}
		for _, arg := range node.Args {
			if !walkExpr(arg.Expr, callbacks) {
				return false
			}
		}
		return true
	case FunctionExpr:
		for _, param := range node.Params {
			if !walkExpr(param.Default, callbacks) {
				return false
			}
		}
		return WalkFuncBody(node.Body, callbacks)
	case AliasExpr:
		return walkExpr(node.Expr, callbacks)
	case IndexExpr:
		if !walkExpr(node.Base, callbacks) {
			return false
		}
		return walkExprList(node.Items, callbacks)
	case UnaryExpr:
		return walkExpr(node.Expr, callbacks)
	case BinaryExpr:
		return walkExpr(node.Left, callbacks) && walkExpr(node.Right, callbacks)
	case CompareExpr:
		return walkExpr(node.Left, callbacks) && walkExpr(node.Right, callbacks)
	case ConditionalExpr:
		return walkExpr(node.Then, callbacks) &&
			walkExpr(node.Cond, callbacks) &&
			walkExpr(node.Else, callbacks)
	}
	return true
}

func walkExprList(exprs []Expr, callbacks WalkCallbacks) bool {
	for _, expr := range exprs {
		if !walkExpr(expr, callbacks) {
			return false
		}
	}
	return true
}

func walkFuncBodyStmt(stmt FuncBodyStmt, callbacks WalkCallbacks) bool {
	if stmt == nil {
		return true
	}
	if callbacks.FuncBodyStmt != nil {
		switch callbacks.FuncBodyStmt(stmt) {
		case WalkStop:
			return false
		case WalkSkipChildren:
			return true
		}
	}

	switch node := stmt.(type) {
	case LocalAssignStmt:
		return walkExpr(node.Expr, callbacks)
	case ReturnStmt:
		return walkExpr(node.Expr, callbacks)
	case ExprStmt:
		return walkExpr(node.Expr, callbacks)
	case FuncIfStmt:
		if !walkExpr(node.Cond, callbacks) || !WalkFuncBody(node.Then, callbacks) {
			return false
		}
		for _, branch := range node.Elifs {
			if !walkExpr(branch.Cond, callbacks) || !WalkFuncBody(branch.Body, callbacks) {
				return false
			}
		}
		return WalkFuncBody(node.Else, callbacks)
	case FuncForStmt:
		return walkExpr(node.Iterable, callbacks) && WalkFuncBody(node.Body, callbacks)
	case FuncWhileStmt:
		return walkExpr(node.Cond, callbacks) && WalkFuncBody(node.Body, callbacks)
	case BreakStmt, ContinueStmt:
		return true
	}
	return true
}
