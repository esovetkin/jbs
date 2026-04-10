package eval

import (
	"fmt"
	"math"
	"math/bits"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

type ExprOptions struct {
	ParamAssignmentTupleArithmetic bool
}

func EvalExpr(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics) Value {
	return EvalExprWithOptions(expr, env, diags, ExprOptions{})
}

func EvalExprWithOptions(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions) Value {
	return evalExprWithCtx(expr, env, diags, opts, &evalCtx{overflowWarned: make(map[string]struct{})})
}

type evalCtx struct {
	overflowWarned map[string]struct{}
}

func evalExprWithCtx(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if expr == nil {
		return Null()
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if v, ok := env[e.Name]; ok {
			return v
		}
		diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", e.Name), e.Span, "import or define the variable before use")
		return Null()
	case ast.QualifiedIdentExpr:
		key := e.Namespace + "." + e.Name
		if v, ok := env[key]; ok {
			return v
		}
		diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", key), e.Span, "import or define the variable before use")
		return Null()
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
		}
		return List(items)
	case ast.TupleExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, evalExprWithCtx(it, env, diags, opts, ctx))
		}
		return Tuple(items)
	case ast.ConvertExpr:
		value := evalExprWithCtx(e.Expr, env, diags, opts, ctx)
		return evalConvert(e.Target, value)
	case ast.UnaryExpr:
		v := evalExprWithCtx(e.Expr, env, diags, opts, ctx)
		return evalUnary(e.Op, v, e.Span, diags, ctx)
	case ast.BinaryExpr:
		l := evalExprWithCtx(e.Left, env, diags, opts, ctx)
		r := evalExprWithCtx(e.Right, env, diags, opts, ctx)
		return evalBinary(e.Op, l, r, e.Span, diags, opts, ctx)
	case ast.CompareExpr:
		l := evalExprWithCtx(e.Left, env, diags, opts, ctx)
		r := evalExprWithCtx(e.Right, env, diags, opts, ctx)
		return evalCompare(e.Op, l, r, e.Span, diags)
	case ast.ConditionalExpr:
		c := evalExprWithCtx(e.Cond, env, diags, opts, ctx)
		if c.Kind != KindBool {
			diags.AddError(diag.CodeE102, "conditional requires boolean condition", e.Cond.GetSpan(), "ensure condition evaluates to true/false")
			return evalExprWithCtx(e.Then, env, diags, opts, ctx)
		}
		if c.B {
			return evalExprWithCtx(e.Then, env, diags, opts, ctx)
		}
		return evalExprWithCtx(e.Else, env, diags, opts, ctx)
	case ast.ModeExpr:
		return evalExprWithCtx(e.Expr, env, diags, opts, ctx)
	default:
		diags.AddError(diag.CodeE199, "unsupported expression node", expr.GetSpan(), "check expression syntax")
		return Null()
	}
}

func evalConvert(target string, value Value) Value {
	switch target {
	case "tuple":
		if isSequence(value) {
			return Tuple(slicesCloneValues(value.L))
		}
		return Tuple([]Value{value})
	case "list":
		if isSequence(value) {
			return List(slicesCloneValues(value.L))
		}
		return List([]Value{value})
	default:
		return value
	}
}

func evalUnary(op string, v Value, at diag.Span, diags *diag.Diagnostics, ctx *evalCtx) Value {
	if isSequence(v) {
		out := make([]Value, len(v.L))
		for i, it := range v.L {
			out[i] = evalUnary(op, it, at, diags, ctx)
		}
		return List(out)
	}
	if !isNumeric(v) {
		diags.AddError(diag.CodeE103, fmt.Sprintf("unary '%s' requires numeric value", op), at, "use int/float values")
		return Null()
	}
	if op == "+" {
		return v
	}
	if v.Kind == KindFloat {
		return Float(-v.F)
	}
	result, overflow := negInt64Checked(v.I)
	if overflow {
		ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("-%d wraps to %d", v.I, result))
	}
	return Int(result)
}

func evalBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if op == "and" || op == "or" {
		if l.Kind != KindBool || r.Kind != KindBool {
			diags.AddError(diag.CodeE104, fmt.Sprintf("'%s' requires boolean operands", op), at, "use boolean values with and/or")
			return Null()
		}
		if op == "and" {
			return Bool(l.B && r.B)
		}
		return Bool(l.B || r.B)
	}

	if opts.ParamAssignmentTupleArithmetic && (IsTuple(l) || IsTuple(r)) {
		return evalParamTupleBinary(op, l, r, at, diags)
	}

	if isSequence(l) || isSequence(r) {
		return evalVectorBinary(op, l, r, at, diags, opts, ctx)
	}
	if l.Kind == KindString || r.Kind == KindString {
		if op != "+" {
			diags.AddError(diag.CodeE105, fmt.Sprintf("operator '%s' is not supported for strings", op), at, "use '+' for string concatenation")
			return Null()
		}
		return String(l.String() + r.String())
	}
	if !isNumeric(l) || !isNumeric(r) {
		diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' requires numeric or string operands", op), at, "check operand types")
		return Null()
	}

	lf := toFloat(l)
	rf := toFloat(r)
	switch op {
	case "+":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf + rf)
		}
		result, overflow := addInt64Checked(l.I, r.I)
		if overflow {
			ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("%d + %d wraps to %d", l.I, r.I, result))
		}
		return Int(result)
	case "-":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf - rf)
		}
		result, overflow := subInt64Checked(l.I, r.I)
		if overflow {
			ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("%d - %d wraps to %d", l.I, r.I, result))
		}
		return Int(result)
	case "*":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf * rf)
		}
		result, overflow := mulInt64Checked(l.I, r.I)
		if overflow {
			ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("%d * %d wraps to %d", l.I, r.I, result))
		}
		return Int(result)
	case "/":
		if rf == 0 {
			diags.AddError(diag.CodeE107, "division by zero", at, "guard denominator")
			return Null()
		}
		return Float(lf / rf)
	case "%":
		if r.Kind == KindFloat || l.Kind == KindFloat {
			diags.AddError(diag.CodeE108, "modulo requires integer operands", at, "use int values with '%' operator")
			return Null()
		}
		if r.I == 0 {
			diags.AddError(diag.CodeE107, "modulo by zero", at, "guard denominator")
			return Null()
		}
		return Int(l.I % r.I)
	default:
		diags.AddError(diag.CodeE109, fmt.Sprintf("unknown operator '%s'", op), at, "use supported operators")
		return Null()
	}
}

func evalParamTupleBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch op {
	case "+":
		if !IsTuple(l) || !IsTuple(r) {
			diags.AddError(diag.CodeE106, "tuple '+' requires tuple operands on both sides", at, "use tuple + tuple")
			return Null()
		}
		items := make([]Value, 0, len(l.L)+len(r.L))
		items = append(items, l.L...)
		items = append(items, r.L...)
		return Tuple(items)
	case "*":
		if !IsTuple(l) || r.Kind != KindInt {
			diags.AddError(diag.CodeE106, "tuple '*' requires tuple * integer", at, "use tuple * non-negative integer")
			return Null()
		}
		if r.I < 0 {
			diags.AddError(diag.CodeE106, "tuple repetition count must be non-negative", at, "use an integer value >= 0")
			return Null()
		}
		if len(l.L) == 0 || r.I == 0 {
			return Tuple(nil)
		}
		items := make([]Value, 0, len(l.L)*int(r.I))
		for i := int64(0); i < r.I; i++ {
			items = append(items, l.L...)
		}
		return Tuple(items)
	default:
		diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' is not supported for tuple arithmetic", op), at, "use '+' for concatenation or '*' for repetition")
		return Null()
	}
}

func evalVectorBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	ls := ToSeries(l)
	rs := ToSeries(r)
	if len(ls) == 0 || len(rs) == 0 {
		return List(nil)
	}
	n := len(ls)
	if len(rs) > n {
		n = len(rs)
	}
	out := make([]Value, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, evalBinary(op, ls[i%len(ls)], rs[i%len(rs)], at, diags, opts, ctx))
	}
	return List(out)
}

func isSequence(v Value) bool {
	return v.Kind == KindList || v.Kind == KindTuple
}

func slicesCloneValues(v []Value) []Value {
	if len(v) == 0 {
		return nil
	}
	out := make([]Value, len(v))
	copy(out, v)
	return out
}

func addInt64Checked(a, b int64) (int64, bool) {
	result := a + b
	overflow := (a > 0 && b > 0 && result < 0) || (a < 0 && b < 0 && result >= 0)
	return result, overflow
}

func subInt64Checked(a, b int64) (int64, bool) {
	result := a - b
	overflow := (a >= 0 && b < 0 && result < 0) || (a < 0 && b > 0 && result >= 0)
	return result, overflow
}

func mulInt64Checked(a, b int64) (int64, bool) {
	result := a * b
	if a == 0 || b == 0 {
		return result, false
	}
	absA := absInt64ToUint64(a)
	absB := absInt64ToUint64(b)
	hi, lo := bits.Mul64(absA, absB)
	if hi != 0 {
		return result, true
	}
	negative := (a < 0) != (b < 0)
	if negative {
		return result, lo > (uint64(1) << 63)
	}
	return result, lo > uint64(math.MaxInt64)
}

func negInt64Checked(v int64) (int64, bool) {
	result := -v
	return result, v == math.MinInt64
}

func absInt64ToUint64(v int64) uint64 {
	if v >= 0 {
		return uint64(v)
	}
	if v == math.MinInt64 {
		return uint64(1) << 63
	}
	return uint64(-v)
}

func (c *evalCtx) warnIntOverflow(diags *diag.Diagnostics, op string, at diag.Span, detail string) {
	key := fmt.Sprintf("%s|%s|%d|%d", op, at.File, at.Start.Offset, at.End.Offset)
	if _, exists := c.overflowWarned[key]; exists {
		return
	}
	c.overflowWarned[key] = struct{}{}
	diags.AddWarning(
		diag.CodeW102,
		fmt.Sprintf("integer overflow in '%s': %s", op, detail),
		at,
		"use smaller values or switch to floating-point arithmetic",
	)
}

func evalCompare(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch op {
	case "==":
		return Bool(Equal(l, r))
	case "!=":
		return Bool(!Equal(l, r))
	}

	if l.Kind == KindString && r.Kind == KindString {
		switch op {
		case "<":
			return Bool(l.S < r.S)
		case "<=":
			return Bool(l.S <= r.S)
		case ">":
			return Bool(l.S > r.S)
		case ">=":
			return Bool(l.S >= r.S)
		}
	}
	if isNumeric(l) && isNumeric(r) {
		lf := toFloat(l)
		rf := toFloat(r)
		switch op {
		case "<":
			return Bool(lf < rf)
		case "<=":
			return Bool(lf <= rf)
		case ">":
			return Bool(lf > rf)
		case ">=":
			return Bool(lf >= rf)
		}
	}
	diags.AddError(diag.CodeE110, fmt.Sprintf("unsupported comparison '%s' for operand types", op), at, "compare compatible types")
	return Bool(false)
}
