package eval

import (
	"fmt"
	"math"
	"math/bits"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalUnary(op string, v Value, at diag.Span, diags *diag.Diagnostics, ctx *evalCtx) Value {
	if op == "!" {
		return evalLogicalNot(v, at, diags)
	}
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

func evalLogicalNot(v Value, at diag.Span, diags *diag.Diagnostics) Value {
	if isSequence(v) {
		out := make([]Value, 0, len(v.L))
		castWarned := false
		for _, item := range v.L {
			b, casted := truthy(item)
			if casted && !castWarned {
				castWarned = true
				diags.AddWarning(diag.CodeW101, "logical '!' cast non-boolean values via truthiness", at, "use explicit boolean expressions to avoid implicit casts")
			}
			out = append(out, Bool(!b))
		}
		return List(out)
	}
	b, casted := truthy(v)
	if casted {
		diags.AddWarning(diag.CodeW101, "logical '!' cast non-boolean value via truthiness", at, "use explicit boolean expressions to avoid implicit casts")
	}
	return Bool(!b)
}

func evalLogicalBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	if !isSequence(l) && !isSequence(r) {
		lb, lcast := truthy(l)
		rb, rcast := truthy(r)
		if lcast || rcast {
			diags.AddWarning(diag.CodeW101, fmt.Sprintf("logical '%s' cast non-boolean values via truthiness", op), at, "use explicit boolean expressions to avoid implicit casts")
		}
		if op == "&" {
			return Bool(lb && rb)
		}
		return Bool(lb || rb)
	}

	ls := ToSeries(l)
	rs := ToSeries(r)
	if len(ls) == 0 || len(rs) == 0 {
		return List(nil)
	}
	n := len(ls)
	if len(rs) > n {
		n = len(rs)
	}
	if len(ls) != len(rs) {
		diags.AddWarning(
			diag.CodeW101,
			fmt.Sprintf("length mismatch in logical '%s': left=%d right=%d; cyclic broadcast to length %d", op, len(ls), len(rs), n),
			at,
			"align lengths to avoid cyclic broadcast",
		)
	}
	out := make([]Value, 0, n)
	castWarned := false
	for i := 0; i < n; i++ {
		lb, lcast := truthy(ls[i%len(ls)])
		rb, rcast := truthy(rs[i%len(rs)])
		if (lcast || rcast) && !castWarned {
			castWarned = true
			diags.AddWarning(diag.CodeW101, fmt.Sprintf("logical '%s' cast non-boolean values via truthiness", op), at, "use explicit boolean expressions to avoid implicit casts")
		}
		if op == "&" {
			out = append(out, Bool(lb && rb))
		} else {
			out = append(out, Bool(lb || rb))
		}
	}
	return List(out)
}

func evalBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if op == "&" || op == "|" {
		return evalLogicalBinary(op, l, r, at, diags)
	}
	if l.Kind == KindFunction || r.Kind == KindFunction {
		diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' does not accept function values", op), at, "call the function first or remove it from the arithmetic expression")
		return Null()
	}
	if l.Kind == KindDict || r.Kind == KindDict {
		if op != "+" || l.Kind != KindDict || r.Kind != KindDict {
			diags.AddError(diag.CodeE106, "dictionary '+' requires dictionary operands", at, "use dict + dict")
			return Null()
		}
		return mergeDicts(l, r)
	}

	if IsComb(l) || IsComb(r) {
		switch op {
		case "+", "*":
			leftRows := combRowsFromValue(l, at)
			rightRows := combRowsFromValue(r, at)
			opNode := ast.CombBinary{Op: op, OpSpan: at, Span: at}
			if op == "+" {
				return combValueFromRows(zipRows(leftRows, rightRows, opNode, diags))
			}
			return combValueFromRows(productRows(leftRows, rightRows, opNode, diags))
		default:
			diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' is not supported for table values", op), at, "use zip(), product(), select(), or filter() with table values")
			return Null()
		}
	}

	if opts.GlobalAssignmentTupleArithmetic && (IsTuple(l) || IsTuple(r)) {
		return evalParamTupleBinary(op, l, r, at, diags)
	}

	if isSequence(l) || isSequence(r) {
		return evalVectorBinary(op, l, r, at, diags, opts, ctx)
	}
	if l.Kind == KindString || r.Kind == KindString {
		switch op {
		case "+":
			return String(l.String() + r.String())
		case "*":
			if l.Kind == KindString {
				return evalStringRepeat(l, r, at, diags)
			}
			return evalStringRepeat(r, l, at, diags)
		default:
			diags.AddError(diag.CodeE105, fmt.Sprintf("operator '%s' is not supported for strings", op), at, "use '+' for concatenation or '*' for repetition")
			return Null()
		}
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

		total, repeatCount, ok := checkedRepeatSize(len(l.L), r.I, diag.CodeE106, "tuple repetition", at, diags)
		if !ok {
			return Null()
		}
		if total == 0 {
			return Tuple(nil)
		}

		items := make([]Value, 0, total)
		for i := 0; i < repeatCount; i++ {
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
	if isSequence(l) || isSequence(r) {
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
			out = append(out, evalCompare(op, ls[i%len(ls)], rs[i%len(rs)], at, diags))
		}
		return List(out)
	}
	if l.Kind == KindFunction || r.Kind == KindFunction {
		diags.AddError(diag.CodeE110, fmt.Sprintf("comparison '%s' does not accept function values", op), at, "call the function first or compare non-function values")
		return Bool(false)
	}

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
