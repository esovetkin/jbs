// evaluate assignment-level JBS expressions
//
// i.e. in
// ```jbs
// a = [1,2,3] * 2   # [2,4,6]
// b = ("a","b") * 2 # ("a","b","a","b")
// a */+ b
// ```
//
// implement the `a` and `b` assignments. It also handles operations
// with literals/unary/binary/compare/conditional/calls
package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type EvalContext int

const (
	EvalCtxDefault EvalContext = iota
	EvalCtxBindingAssign
	EvalCtxScalarGlobalAssign
	EvalCtxAnalyseAssign
)

type ExprOptions struct {
	GlobalAssignmentTupleArithmetic bool
	Context                         EvalContext
	Names                           *NameCatalog
	Files                           *FileAccess
	Frame                           *Frame
	MaxFunctionCallDepth            int
	Print                           PrintSink
	PrintIndex                      int
	NextPrintSeq                    func() int
	ShellRunner                     ShellRunner
	ShellUse                        func(ShellUseEvent)
	Environ                         func() []string
	DeleteName                      DeleteNameFunc
}

type DeleteNameFunc func(name string, at diag.Span, diags *diag.Diagnostics) bool

const MaxLoopIterations = 1000000
const MaxFunctionCallDepth = 10000

func EvalExpr(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics) Value {
	return EvalExprWithOptions(expr, env, diags, ExprOptions{})
}

func EvalExprWithOptions(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions) Value {
	frame := opts.Frame
	if frame == nil {
		frame = NewRootFrame(env)
	}
	return evalExprWithCtx(expr, env, diags, opts, newEvalCtx(frame))
}

func EvalBoolCondition(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions) (bool, bool) {
	return EvalBoolConditionFor("if", expr, env, diags, opts)
}

func EvalBoolConditionFor(kind string, expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions) (bool, bool) {
	frame := opts.Frame
	if frame == nil {
		frame = NewRootFrame(env)
	}
	return evalBoolConditionWithCtx(kind, expr, env, diags, opts, newEvalCtx(frame))
}

type evalAbortState struct {
	recursionLimitHit bool
}

type evalCtx struct {
	overflowWarned map[string]struct{}
	frame          *Frame
	callDepth      int
	abort          *evalAbortState
}

func newEvalCtx(frame *Frame) *evalCtx {
	return &evalCtx{
		overflowWarned: make(map[string]struct{}),
		frame:          frame,
		abort:          &evalAbortState{},
	}
}

func functionCallDepthLimit(opts ExprOptions) int {
	if opts.MaxFunctionCallDepth > 0 {
		return opts.MaxFunctionCallDepth
	}
	return MaxFunctionCallDepth
}

func (ctx *evalCtx) recursionLimitHit() bool {
	return ctx != nil && ctx.abort != nil && ctx.abort.recursionLimitHit
}

func (ctx *evalCtx) markRecursionLimitHit() {
	if ctx == nil {
		return
	}
	if ctx.abort == nil {
		ctx.abort = &evalAbortState{}
	}
	ctx.abort.recursionLimitHit = true
}

func (ctx *evalCtx) enterFunctionCall(frame *Frame) *evalCtx {
	next := ctx.withFrame(frame)
	next.callDepth++
	return next
}

func evalBoolConditionWithCtx(_ string, expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) (bool, bool) {
	beforeErrors := diagErrorCount(diags)
	value := evalExprWithCtx(expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return false, false
	}
	if diagErrorCount(diags) > beforeErrors {
		return false, false
	}
	b, _ := truthy(value)
	return b, true
}

func evalExprWithCtx(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if ctx.recursionLimitHit() {
		return Null()
	}
	if expr == nil {
		return Null()
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if v, ok := lookupValue(e.Name, env, e.Span, diags, ctx); ok {
			return v
		}
		return Null()
	case ast.QualifiedIdentExpr:
		if ns, found, assigned := lookupLocalOrCapturedValue(e.Namespace, env, e.Span, diags, ctx); found {
			if !assigned {
				diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", e.Namespace), e.Span, "assign the local before reading it")
				return Null()
			}
			if IsComb(ns) {
				col, exists := CombColumn(ns, e.Name)
				if !exists {
					diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s.%s'", e.Namespace, e.Name), e.Span, "import or define the variable before use")
					return Null()
				}
				if len(col) == 1 {
					return col[0]
				}
				return List(col)
			}
			diags.AddError(diag.CodeE106, fmt.Sprintf("qualified access '%s.%s' requires a table namespace", e.Namespace, e.Name), e.Span, "use qualified access only on table values in expressions")
			return Null()
		}
		key := e.Namespace + "." + e.Name
		if v, found, assigned := lookupLocalOrCapturedValue(key, env, e.Span, diags, ctx); found {
			if !assigned {
				diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", key), e.Span, "assign the local before reading it")
				return Null()
			}
			return v
		}
		diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", key), e.Span, "import or define the variable before use")
		return Null()
	case ast.MemberExpr:
		base := evalExprWithCtx(e.Base, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		if !IsComb(base) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("member access '.%s' requires a table base", e.Name), e.Span, "use member access only on table values")
			return Null()
		}
		col, ok := CombColumn(base, e.Name)
		if !ok {
			diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", e.Name), e.Span, "select an existing table column")
			return Null()
		}
		if len(col) == 1 {
			return col[0]
		}
		return List(col)
	case ast.StringExpr:
		return String(e.Value)
	case ast.NumberExpr:
		if e.Int {
			return Int(e.IntValue)
		}
		return Float(e.FloatValue)
	case ast.BoolExpr:
		return Bool(e.Value)
	case ast.ListExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, evalExprWithCtx(it, env, diags, opts, ctx))
			if ctx.recursionLimitHit() {
				return Null()
			}
		}
		return List(items)
	case ast.TupleExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, evalExprWithCtx(it, env, diags, opts, ctx))
			if ctx.recursionLimitHit() {
				return Null()
			}
		}
		return Tuple(items)
	case ast.DictExpr:
		return evalDictExpr(e, env, diags, opts, ctx)
	case ast.RangeExpr:
		start := evalExprWithCtx(e.Start, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		stop := evalExprWithCtx(e.Stop, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		args := []Value{start, stop}
		if e.Step != nil {
			step := evalExprWithCtx(e.Step, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return Null()
			}
			args = append(args, step)
		}
		return evalRangeCall(args, e.Span, diags)
	case ast.FunctionExpr:
		value := newFunctionValue(e, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		return value
	case ast.AliasExpr:
		diags.AddError(diag.CodeE106, "alias expression is only allowed in table-valued assignment operands", e.Span, "replace it with table(name = expr) or a named table operation")
		return Null()
	case ast.CallExpr:
		value := evalCall(e.Callee, e.Args, env, e.Span, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		return value
	case ast.IndexExpr:
		base := evalExprWithCtx(e.Base, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		switch {
		case base.Kind == KindDict:
			return evalDictIndex(base, e.Items, env, e.Span, diags, opts, ctx)
		case base.Kind == KindList || base.Kind == KindTuple:
			return evalSequenceIndex(base, e.Items, env, e.Span, diags, opts, ctx)
		case IsComb(base):
			return evalCombIndex(base, e.Items, env, e.Span, diags, opts, ctx)
		default:
			diags.AddError(diag.CodeE106, "index expression requires a list, tuple, dictionary, or table base", e.Span, "use value[index], dict_value[key], or table_value[col]")
			return Null()
		}
	case ast.UnaryExpr:
		v := evalExprWithCtx(e.Expr, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		return evalUnary(e.Op, v, e.Span, diags, ctx)
	case ast.BinaryExpr:
		if opts.Context == EvalCtxBindingAssign && (e.Op == "+" || e.Op == "*") && binaryNeedsRelaxedCombEval(e) {
			value := evalRelaxedCombBinary(e, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return Null()
			}
			return value
		}
		l := evalExprWithCtx(e.Left, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		r := evalExprWithCtx(e.Right, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		if (e.Op == "+" || e.Op == "*") && (IsComb(l) || IsComb(r)) {
			opNode := ast.CombBinary{Op: e.Op, OpSpan: e.Span, Span: e.Span}
			leftRows := combRowsFromBinaryOperand(e.Left, l, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return Null()
			}
			rightRows := combRowsFromBinaryOperand(e.Right, r, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return Null()
			}
			if e.Op == "+" {
				return combValueFromRows(rowWiseMergeRows(leftRows, rightRows, opNode, diags))
			}
			return combValueFromRows(productRows(leftRows, rightRows, opNode, diags))
		}
		return evalBinary(e.Op, l, r, e.Span, diags, opts, ctx)
	case ast.CompareExpr:
		l := evalExprWithCtx(e.Left, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		r := evalExprWithCtx(e.Right, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		return evalCompare(e.Op, l, r, e.Span, diags)
	case ast.ConditionalExpr:
		cond, ok := evalBoolConditionWithCtx("conditional", e.Cond, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		if !ok {
			return Null()
		}
		if cond {
			value := evalExprWithCtx(e.Then, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return Null()
			}
			return value
		}
		value := evalExprWithCtx(e.Else, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		return value
	default:
		diags.AddError(diag.CodeE199, "unsupported expression node", expr.GetSpan(), "check expression syntax")
		return Null()
	}
}
