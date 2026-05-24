package eval

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"

func evalSetSeedValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	bound, ok := bindBuiltinArgs("setseed", args, builtinSignature{
		Name: "setseed",
		Params: []builtinParam{
			{Name: "seed", Required: true},
		},
	}, at, diags)
	if !ok {
		return Null()
	}

	seedArg := bound.ByName["seed"]
	if seedArg.Value.Kind != KindInt {
		diags.AddError(diag.CodeE106, "setseed() seed argument must be an integer", seedArg.Span, "pass an integer seed")
		return Null()
	}
	randomState(opts).SetSeed(seedArg.Value.I)
	return Null()
}

func evalSampleValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	bound, ok := bindBuiltinArgs("sample", args, builtinSignature{
		Name: "sample",
		Params: []builtinParam{
			{Name: "values", Required: true},
			{Name: "size"},
			{Name: "replace"},
		},
	}, at, diags)
	if !ok {
		return Null()
	}

	valuesArg := bound.ByName["values"]
	size, ok := sampleSize(bound, valuesArg.Value, at, diags)
	if !ok {
		return Null()
	}
	replace, ok := sampleReplace(bound, diags)
	if !ok {
		return Null()
	}
	return sampleValue(valuesArg.Value, size, replace, valuesArg.Span, at, diags, randomState(opts))
}

func sampleSize(bound builtinArgs, value Value, at diag.Span, diags *diag.Diagnostics) (int, bool) {
	arg, exists := bound.ByName["size"]
	if !exists || arg.Value.Kind == KindNull {
		length, ok := sampleInputLength(value)
		if !ok {
			return 0, true
		}
		return length, true
	}
	if arg.Value.Kind != KindInt {
		diags.AddError(diag.CodeE106, "sample() size argument must be an integer", arg.Span, "pass size as a non-negative integer")
		return 0, false
	}
	if arg.Value.I < 0 {
		diags.AddError(diag.CodeE106, "sample() size argument must be non-negative", arg.Span, "pass size as a non-negative integer")
		return 0, false
	}
	if arg.Value.I > maxHostInt64() {
		diags.AddError(diag.CodeE106, "sample() size argument is too large", arg.Span, "use a smaller size")
		return 0, false
	}
	size := int(arg.Value.I)
	if size > maxRepeatOutputUnits {
		diags.AddError(diag.CodeE106, "sample() result is too large", arg.Span, "use a smaller size")
		return 0, false
	}
	return size, true
}

func sampleReplace(bound builtinArgs, diags *diag.Diagnostics) (bool, bool) {
	arg, exists := bound.ByName["replace"]
	if !exists {
		return false, true
	}
	if arg.Value.Kind != KindBool {
		diags.AddError(diag.CodeE106, "sample() replace argument must be a boolean", arg.Span, "pass replace = true or replace = false")
		return false, false
	}
	return arg.Value.B, true
}

func sampleInputLength(value Value) (int, bool) {
	switch value.Kind {
	case KindList, KindTuple:
		return len(value.L), true
	case KindComb:
		if !IsComb(value) {
			return 0, false
		}
		return len(value.C.Rows), true
	default:
		return 0, false
	}
}

func sampleValue(value Value, size int, replace bool, valueSpan, at diag.Span, diags *diag.Diagnostics, rng *RandomState) Value {
	switch value.Kind {
	case KindList, KindTuple:
		return sampleSequence(value, size, replace, at, diags, rng)
	case KindComb:
		if !IsComb(value) {
			diags.AddError(diag.CodeE106, "sample() received a malformed table value", valueSpan, "use a table value")
			return Null()
		}
		return sampleTable(value, size, replace, at, diags, rng)
	default:
		diags.AddError(diag.CodeE106, "sample() expects list/tuple/table as first argument", valueSpan, "pass a list, tuple, or table value")
		return Null()
	}
}

func sampleSequence(value Value, size int, replace bool, at diag.Span, diags *diag.Diagnostics, rng *RandomState) Value {
	indices, ok := sampleIndices(len(value.L), size, replace, at, diags, rng)
	if !ok {
		return Null()
	}
	out := make([]Value, 0, len(indices))
	for _, index := range indices {
		out = append(out, CloneValue(value.L[index]))
	}
	if value.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

func sampleTable(value Value, size int, replace bool, at diag.Span, diags *diag.Diagnostics, rng *RandomState) Value {
	indices, ok := sampleIndices(len(value.C.Rows), size, replace, at, diags, rng)
	if !ok {
		return Null()
	}
	rows := make([]Row, 0, len(indices))
	for _, index := range indices {
		rows = append(rows, value.C.Rows[index])
	}
	if replace {
		return CombValue(&Comb{
			Order: append([]string(nil), value.C.Order...),
			Rows:  rebaseRowsByOutputRow(value.C.Order, rows),
		})
	}
	return tableValueFromOrderedRows(value.C.Order, rows)
}

func sampleIndices(length int, size int, replace bool, at diag.Span, diags *diag.Diagnostics, rng *RandomState) ([]int, bool) {
	if size == 0 {
		return nil, true
	}
	if length == 0 {
		diags.AddError(diag.CodeE106, "sample() cannot sample from an empty value", at, "use size = 0 or pass a non-empty value")
		return nil, false
	}
	if !replace && size > length {
		diags.AddError(diag.CodeE106, "sample() size exceeds input length when replace is false", at, "use a smaller size or replace = true")
		return nil, false
	}
	if rng == nil {
		diags.AddError(diag.CodeE199, "sample() random generator is not configured", at, "retry in a normal evaluation context")
		return nil, false
	}

	out := make([]int, size)
	if replace {
		for i := range out {
			out[i] = rng.Intn(length)
		}
		return out, true
	}

	perm := rng.Perm(length)
	copy(out, perm[:size])
	return out, true
}
