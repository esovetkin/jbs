package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

const defaultHeadTailCount int64 = 5

func evalHeadTailValueCall(name string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) Value {
	bound, ok := bindBuiltinArgs(name, args, builtinSignature{
		Name: name,
		Params: []builtinParam{
			{Name: "values", Required: true},
			{Name: "n"},
		},
	}, at, diags)
	if !ok {
		return Null()
	}

	valuesArg := bound.ByName["values"]
	count := defaultHeadTailCount
	if nArg, ok := bound.ByName["n"]; ok {
		if nArg.Value.Kind != KindInt {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() n argument must be an integer", name), nArg.Span, "pass n as an integer value >= 0")
			return Null()
		}
		if nArg.Value.I < 0 {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() n argument must be non-negative", name), nArg.Span, "pass n as an integer value >= 0")
			return Null()
		}
		count = nArg.Value.I
	}

	return evalHeadTail(name, valuesArg.Value, count, valuesArg.Span, diags)
}

func evalHeadTail(name string, value Value, count int64, valueSpan diag.Span, diags *diag.Diagnostics) Value {
	switch value.Kind {
	case KindList, KindTuple:
		return headTailSequence(name, value, count)
	case KindComb:
		if !IsComb(value) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() received a malformed table value", name), valueSpan, "use a table value")
			return Null()
		}
		return headTailTable(name, value, count)
	default:
		diags.AddError(diag.CodeE106, fmt.Sprintf("%s() expects list/tuple/table as first argument", name), valueSpan, "pass a list, tuple, or table value")
		return Null()
	}
}

func headTailRange(name string, length int, count int64) (int, int) {
	n := int64(length)
	if count < n {
		n = count
	}
	end := int(n)
	start := 0
	if name == "tail" {
		start = length - end
		end = length
	}
	return start, end
}

func headTailSequence(name string, value Value, count int64) Value {
	start, end := headTailRange(name, len(value.L), count)
	out := CloneValues(value.L[start:end])
	if value.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

func headTailTable(name string, value Value, count int64) Value {
	start, end := headTailRange(name, len(value.C.Rows), count)
	return tableValueFromOrderedRows(value.C.Order, value.C.Rows[start:end])
}
