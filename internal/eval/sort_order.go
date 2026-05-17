package eval

import (
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalOrderValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	bound, ok := bindBuiltinArgs("order", args, builtinSignature{
		Name: "order",
		Params: []builtinParam{
			{Name: "values", Required: true},
			{Name: "by"},
		},
	}, at, diags)
	if !ok {
		return Null()
	}

	valuesArg := bound.ByName["values"]
	byArg, hasBy := bound.ByName["by"]
	byFn, ok := optionalSortComparator("order", byArg, hasBy, diags)
	if !ok {
		return Null()
	}
	items, ok := sortOrderItems("order", valuesArg.Value, valuesArg.Span, diags)
	if !ok {
		return Null()
	}
	perm, ok := sortPermutation("order", items, byFn, env, at, diags, opts, ctx)
	if !ok {
		return Null()
	}
	return permutationValue(perm)
}

func evalSortValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	bound, ok := bindBuiltinArgs("sort", args, builtinSignature{
		Name: "sort",
		Params: []builtinParam{
			{Name: "values", Required: true},
			{Name: "by"},
			{Name: "inplace"},
		},
	}, at, diags)
	if !ok {
		return Null()
	}

	valuesArg := bound.ByName["values"]
	byArg, hasBy := bound.ByName["by"]
	byFn, ok := optionalSortComparator("sort", byArg, hasBy, diags)
	if !ok {
		return Null()
	}
	inplace := false
	if inplaceArg, exists := bound.ByName["inplace"]; exists {
		if inplaceArg.Value.Kind != KindBool {
			diags.AddError(diag.CodeE106, "sort() inplace argument must be a boolean", inplaceArg.Span, "pass inplace = True or inplace = False")
			return Null()
		}
		inplace = inplaceArg.Value.B
	}

	return sortSequence(valuesArg.Value, byFn, inplace, env, valuesArg.Span, at, diags, opts, ctx)
}

func optionalSortComparator(caller string, arg CallValueArg, hasArg bool, diags *diag.Diagnostics) (*FunctionValue, bool) {
	if !hasArg || arg.Value.Kind == KindNull {
		return nil, true
	}
	if arg.Value.Kind != KindFunction || arg.Value.Fn == nil {
		diags.AddError(diag.CodeE106, caller+"() by argument must be a function", arg.Span, "pass a comparator function such as function(a, b) { a < b }")
		return nil, false
	}
	return arg.Value.Fn, true
}

func sortOrderItems(caller string, value Value, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	switch value.Kind {
	case KindList, KindTuple:
		return CloneValues(value.L), true
	default:
		diags.AddError(diag.CodeE106, caller+"() expects list or tuple as first argument", at, "pass a list or tuple value")
		return nil, false
	}
}

func sortSequence(value Value, byFn *FunctionValue, inplace bool, env map[string]Value, valueSpan, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if value.Kind != KindList && value.Kind != KindTuple {
		diags.AddError(diag.CodeE106, "sort() expects list or tuple as first argument", valueSpan, "pass a list or tuple value")
		return Null()
	}
	if inplace && value.Kind != KindList {
		diags.AddError(diag.CodeE106, "sort() inplace argument requires a list input", valueSpan, "use inplace = True only with lists")
		return Null()
	}

	items := CloneValues(value.L)
	perm, ok := sortPermutation("sort", items, byFn, env, at, diags, opts, ctx)
	if !ok {
		return Null()
	}
	sorted := applyPermutation(items, perm)
	if inplace {
		copy(value.L, sorted)
		return Null()
	}
	if value.Kind == KindTuple {
		return Tuple(sorted)
	}
	return List(sorted)
}

func sortPermutation(caller string, items []Value, byFn *FunctionValue, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) ([]int, bool) {
	perm := make([]int, len(items))
	for i := range perm {
		perm[i] = i
	}
	if len(perm) < 2 {
		return perm, true
	}

	failed := false
	slices.SortStableFunc(perm, func(i, j int) int {
		if failed || ctx.recursionLimitHit() {
			return 0
		}
		less, ok := compareForSort(caller, items[i], items[j], byFn, env, at, diags, opts, ctx)
		if !ok {
			failed = true
			return 0
		}
		if less {
			return -1
		}
		reverseLess, ok := compareForSort(caller, items[j], items[i], byFn, env, at, diags, opts, ctx)
		if !ok {
			failed = true
			return 0
		}
		if reverseLess {
			return 1
		}
		return 0
	})
	if failed || ctx.recursionLimitHit() {
		return nil, false
	}
	return perm, true
}

func compareForSort(caller string, left, right Value, byFn *FunctionValue, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) (bool, bool) {
	if byFn == nil {
		beforeErrors := diagErrorCount(diags)
		got := evalCompare("<", left, right, at, diags)
		if diagErrorCount(diags) > beforeErrors {
			return false, false
		}
		if got.Kind != KindBool {
			diags.AddError(diag.CodeE106, caller+"() default comparison did not produce a boolean", at, "compare values accepted by '<'")
			return false, false
		}
		return got.B, true
	}

	beforeErrors := diagErrorCount(diags)
	got := executeFunctionCallValues(byFn, []CallValueArg{
		{Value: CloneValue(left), Span: at},
		{Value: CloneValue(right), Span: at},
	}, env, at, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return false, false
	}
	if diagErrorCount(diags) > beforeErrors {
		return false, false
	}
	if got.Kind != KindBool {
		diags.AddError(diag.CodeE106, caller+"() comparator must return a boolean value", at, "return true when the first argument should sort before the second")
		return false, false
	}
	return got.B, true
}

func applyPermutation(items []Value, perm []int) []Value {
	out := make([]Value, len(perm))
	for i, sourceIndex := range perm {
		out[i] = CloneValue(items[sourceIndex])
	}
	return out
}

func permutationValue(perm []int) Value {
	out := make([]Value, len(perm))
	for i, sourceIndex := range perm {
		out[i] = Int(int64(sourceIndex))
	}
	return List(out)
}
