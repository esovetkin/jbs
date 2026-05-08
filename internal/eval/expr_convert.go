package eval

import (
	"math"
	"strconv"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalConvert(target string, value Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch target {
	case "tuple":
		return evalTupleCall([]Value{value}, at, diags)
	case "list":
		return evalListCall([]Value{value}, at, diags)
	case "int":
		return convertToInt(value, at, diags)
	case "float":
		return convertToFloat(value, at, diags)
	case "str":
		return convertToString(value)
	default:
		return value
	}
}

func evalUnaryConvertCall(name string, args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, name+"() expects exactly one argument", at, "use "+name+"(value)")
		return Null()
	}
	return evalConvert(name, args[0], at, diags)
}

func convertToInt(v Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch v.Kind {
	case KindInt:
		return v
	case KindFloat:
		if math.IsNaN(v.F) || math.IsInf(v.F, 0) || v.F < float64(math.MinInt64) || v.F > float64(math.MaxInt64) {
			diags.AddError(diag.CodeE106, "int() float must be finite and within 64-bit signed range", at, "use a finite float value within int64 range")
			return Null()
		}
		return Int(int64(v.F))
	case KindBool:
		if v.B {
			return Int(1)
		}
		return Int(0)
	case KindString:
		n, err := strconv.ParseInt(v.S, 10, 64)
		if err != nil {
			diags.AddError(diag.CodeE106, "int() string must be a base-10 integer", at, "use text such as '0', '-7', or '42'")
			return Null()
		}
		return Int(n)
	default:
		diags.AddError(diag.CodeE106, "int() expects int/float/bool/string value", at, "convert scalar values only")
		return Null()
	}
}

func convertToFloat(v Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch v.Kind {
	case KindFloat:
		return v
	case KindInt:
		return Float(float64(v.I))
	case KindBool:
		if v.B {
			return Float(1.0)
		}
		return Float(0.0)
	case KindString:
		f, err := strconv.ParseFloat(v.S, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			diags.AddError(diag.CodeE106, "float() string must be a finite decimal number", at, "use text such as '1', '1.5', or '1e3'")
			return Null()
		}
		return Float(f)
	default:
		diags.AddError(diag.CodeE106, "float() expects int/float/bool/string value", at, "convert scalar values only")
		return Null()
	}
}

func convertToString(v Value) Value {
	return String(v.String())
}
