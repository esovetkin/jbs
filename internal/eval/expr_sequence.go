package eval

import (
	"math"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalRangeCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) < 1 || len(args) > 3 {
		diags.AddError(diag.CodeE106, "range() expects 1, 2, or 3 arguments", at, "use range(stop), range(start, stop), or range(start, stop, step)")
		return Null()
	}
	for _, arg := range args {
		if arg.Kind == KindNull {
			return Null()
		}
	}

	switch len(args) {
	case 1:
		if args[0].Kind != KindInt {
			diags.AddError(diag.CodeE106, "range() with 1 argument expects an integer", at, "use an integer stop value")
			return Null()
		}
		return evalRangeInt(0, args[0].I, 1, at, diags)
	case 2:
		if args[0].Kind != KindInt || args[1].Kind != KindInt {
			diags.AddError(diag.CodeE106, "range() with 2 arguments expects integers", at, "use integer start and stop values")
			return Null()
		}
		step := int64(1)
		if args[0].I > args[1].I {
			step = -1
		}
		return evalRangeInt(args[0].I, args[1].I, step, at, diags)
	default:
		if allRangeArgsInt(args) {
			return evalRangeInt(args[0].I, args[1].I, args[2].I, at, diags)
		}
		nums, ok := rangeArgsToFloat(args, at, diags)
		if !ok {
			return Null()
		}
		return evalRangeFloat(nums[0], nums[1], nums[2], at, diags)
	}
}

func allRangeArgsInt(args []Value) bool {
	for _, arg := range args {
		if arg.Kind != KindInt {
			return false
		}
	}
	return true
}

func rangeArgsToFloat(args []Value, at diag.Span, diags *diag.Diagnostics) ([3]float64, bool) {
	var nums [3]float64
	for i, arg := range args {
		switch arg.Kind {
		case KindInt:
			nums[i] = float64(arg.I)
		case KindFloat:
			nums[i] = arg.F
		default:
			diags.AddError(diag.CodeE106, "range() with 3 arguments expects numeric values", at, "use int or float arguments")
			return nums, false
		}
	}
	return nums, true
}

func evalRangeInt(start, stop, step int64, at diag.Span, diags *diag.Diagnostics) Value {
	if step == 0 {
		diags.AddError(diag.CodeE106, "range() step must not be zero", at, "use a non-zero step")
		return Null()
	}
	items := make([]Value, 0)
	for current := start; rangeIntContinues(current, stop, step); {
		if len(items) >= maxRangeOutputUnits {
			diags.AddError(diag.CodeE106, "range() result is too large", at, "use smaller bounds or a larger step")
			return Null()
		}
		items = append(items, Int(current))
		next, overflow := addInt64Checked(current, step)
		if overflow {
			diags.AddError(diag.CodeE106, "range() overflow while generating values", at, "use smaller bounds or step")
			return Null()
		}
		current = next
	}
	return List(items)
}

func rangeIntContinues(current, stop, step int64) bool {
	if step > 0 {
		return current < stop
	}
	return current > stop
}

func evalRangeFloat(start, stop, step float64, at diag.Span, diags *diag.Diagnostics) Value {
	if !finiteFloat(start) || !finiteFloat(stop) || !finiteFloat(step) {
		diags.AddError(diag.CodeE106, "range() expects finite numeric values", at, "use finite int/float bounds and step")
		return Null()
	}
	if step == 0 {
		diags.AddError(diag.CodeE106, "range() step must not be zero", at, "use a non-zero step")
		return Null()
	}
	items := make([]Value, 0)
	for current := start; rangeFloatContinues(current, stop, step); {
		if len(items) >= maxRangeOutputUnits {
			diags.AddError(diag.CodeE106, "range() result is too large", at, "use smaller bounds or a larger step")
			return Null()
		}
		items = append(items, Float(current))
		next := current + step
		if !finiteFloat(next) {
			diags.AddError(diag.CodeE106, "range() overflow while generating values", at, "use smaller bounds or step")
			return Null()
		}
		if (step > 0 && !(next > current)) || (step < 0 && !(next < current)) {
			diags.AddError(diag.CodeE106, "range() step is too small to make progress", at, "use a larger-magnitude step")
			return Null()
		}
		current = next
	}
	return List(items)
}

func finiteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func rangeFloatContinues(current, stop, step float64) bool {
	if step > 0 {
		return current < stop
	}
	return current > stop
}

func evalRevCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, "rev() expects exactly one list/tuple argument", at, "use rev(list_or_tuple_expr)")
		return Null()
	}
	value := args[0]
	if value.Kind == KindNull {
		return Null()
	}
	if value.Kind != KindList && value.Kind != KindTuple {
		diags.AddError(diag.CodeE106, "rev() expects a list or tuple argument", at, "use rev(list_or_tuple_expr)")
		return Null()
	}
	out := slicesCloneValues(value.L)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	if value.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

const maxRepeatOutputUnits = 1 << 20
const maxRangeOutputUnits = maxRepeatOutputUnits

func maxHostInt64() int64 {
	return int64(^uint(0) >> 1)
}

func checkedRepeatSize(elementSize int, count int64, code diag.Code, subject string, at diag.Span, diags *diag.Diagnostics) (int, int, bool) {
	if count < 0 {
		diags.AddError(code, subject+" count must be non-negative", at, "use an integer value >= 0")
		return 0, 0, false
	}

	maxInt := maxHostInt64()
	if count > maxInt {
		diags.AddError(code, subject+" count is too large", at, "use a smaller repeat count")
		return 0, 0, false
	}

	repeatCount := int(count)
	if elementSize == 0 || repeatCount == 0 {
		return 0, repeatCount, true
	}

	if int64(elementSize) > maxInt/count {
		diags.AddError(code, subject+" result is too large", at, "use a smaller repeat count")
		return 0, 0, false
	}

	total := elementSize * repeatCount
	if total > maxRepeatOutputUnits {
		diags.AddError(code, subject+" result is too large", at, "use a smaller repeat count")
		return 0, 0, false
	}

	return total, repeatCount, true
}

func evalStringRepeat(str Value, count Value, at diag.Span, diags *diag.Diagnostics) Value {
	if count.Kind != KindInt {
		diags.AddError(diag.CodeE105, "string '*' requires integer repeat count", at, "use string * int or int * string")
		return Null()
	}
	if count.I < 0 {
		diags.AddError(diag.CodeE105, "string repetition count must be non-negative", at, "use an integer value >= 0")
		return Null()
	}

	total, repeatCount, ok := checkedRepeatSize(len(str.S), count.I, diag.CodeE105, "string repetition", at, diags)
	if !ok {
		return Null()
	}
	if total == 0 {
		return String("")
	}

	return String(strings.Repeat(str.S, repeatCount))
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
