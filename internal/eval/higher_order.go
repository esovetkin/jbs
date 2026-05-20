package eval

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"

func sequenceItems(kind string, v Value, at diag.Span, diags *diag.Diagnostics) ([]Value, bool, bool) {
	return sequenceItemsForArg(kind, "second", v, at, diags)
}

func sequenceItemsForArg(kind, ordinal string, v Value, at diag.Span, diags *diag.Diagnostics) ([]Value, bool, bool) {
	switch v.Kind {
	case KindList:
		return slicesCloneValues(v.L), false, true
	case KindTuple:
		return slicesCloneValues(v.L), true, true
	default:
		diags.AddError(diag.CodeE106, kind+"() expects list or tuple as "+ordinal+" argument", at, "pass a list or tuple value")
		return nil, false, false
	}
}

func evalMapValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	bound, ok := bindBuiltinArgs("map", args, builtinSignature{
		Name: "map",
		Params: []builtinParam{
			{Name: "fn", Required: true},
			{Name: "values", Required: true},
		},
	}, at, diags)
	if !ok {
		return Null()
	}
	fnArg := bound.ByName["fn"]
	valuesArg := bound.ByName["values"]
	fnValue := fnArg.Value
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "map() expects function value as first argument", fnArg.Span, "pass a function literal, built-in function, or function-valued variable")
		return Null()
	}
	items, isTuple, ok := sequenceItems("map", valuesArg.Value, valuesArg.Span, diags)
	if !ok {
		return Null()
	}
	out := make([]Value, 0, len(items))
	for _, item := range items {
		if ctx.recursionLimitHit() {
			return Null()
		}
		beforeErrors := diagErrorCount(diags)
		got := executeFunctionCallValues(fnValue.Fn, []CallValueArg{{Value: item, Span: at}}, env, at, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		if diagErrorCount(diags) > beforeErrors {
			return Null()
		}
		out = append(out, got)
	}
	if isTuple {
		return Tuple(out)
	}
	return List(out)
}

func evalReduceValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	bound, ok := bindBuiltinArgs("reduce", args, builtinSignature{
		Name: "reduce",
		Params: []builtinParam{
			{Name: "fn", Required: true},
			{Name: "values", Required: true},
		},
	}, at, diags)
	if !ok {
		return Null()
	}
	fnArg := bound.ByName["fn"]
	valuesArg := bound.ByName["values"]
	fnValue := fnArg.Value
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "reduce() expects function value as first argument", fnArg.Span, "pass a function literal, built-in function, or function-valued variable")
		return Null()
	}
	items, _, ok := sequenceItems("reduce", valuesArg.Value, valuesArg.Span, diags)
	if !ok {
		return Null()
	}
	if len(items) == 0 {
		diags.AddError(diag.CodeE106, "reduce() cannot operate on an empty list/tuple", at, "use a non-empty list/tuple")
		return Null()
	}
	if len(items) == 1 {
		return items[0]
	}
	acc := items[0]
	for _, item := range items[1:] {
		if ctx.recursionLimitHit() {
			return Null()
		}
		beforeErrors := diagErrorCount(diags)
		acc = executeFunctionCallValues(fnValue.Fn, []CallValueArg{
			{Value: acc, Span: at},
			{Value: item, Span: at},
		}, env, at, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		if diagErrorCount(diags) > beforeErrors {
			return Null()
		}
	}
	return acc
}

func evalFilterValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	bound, ok := bindBuiltinArgs("filter", args, builtinSignature{
		Name: "filter",
		Params: []builtinParam{
			{Name: "values", Required: true},
			{Name: "fn", Required: true},
		},
	}, at, diags)
	if !ok {
		return Null()
	}
	valuesArg := bound.ByName["values"]
	fnArg := bound.ByName["fn"]
	target := valuesArg.Value
	fnValue := fnArg.Value
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "filter() expects function value as second argument", fnArg.Span, "pass a function literal, built-in function, or function-valued variable")
		return Null()
	}
	switch target.Kind {
	case KindList, KindTuple:
		return evalFilterSequence(target, fnValue.Fn, env, at, diags, opts, ctx)
	case KindComb:
		return evalFilterTable(target, fnValue.Fn, env, valuesArg.Span, at, diags, opts, ctx)
	default:
		diags.AddError(diag.CodeE106, "filter() expects list/tuple/table as first argument", valuesArg.Span, "pass a list, tuple, or table value")
		return Null()
	}
}

func evalFilterSequence(target Value, fn *FunctionValue, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	out := make([]Value, 0, len(target.L))
	castWarned := false
	for _, item := range target.L {
		keep, ok := evalFilterPredicate(fn, item, at, env, at, diags, opts, ctx, &castWarned)
		if !ok {
			return Null()
		}
		if keep {
			out = append(out, item)
		}
	}
	if target.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

func evalFilterTable(target Value, fn *FunctionValue, env map[string]Value, valueSpan diag.Span, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if !IsComb(target) {
		return CombValue(&Comb{Order: nil, Rows: nil})
	}
	names := CombNames(target)
	outRows := make([]Row, 0, len(target.C.Rows))
	castWarned := false
	for _, row := range target.C.Rows {
		rowDict, ok := dictFromTableRow("filter", names, row, valueSpan, diags)
		if !ok {
			return Null()
		}
		keep, ok := evalFilterPredicate(fn, rowDict, valueSpan, env, at, diags, opts, ctx, &castWarned)
		if !ok {
			return Null()
		}
		if keep {
			outRows = append(outRows, row.clone())
		}
	}
	return CombValue(&Comb{
		Order: append([]string(nil), target.C.Order...),
		Rows:  outRows,
	})
}

func evalFilterPredicate(fn *FunctionValue, arg Value, argSpan diag.Span, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx, castWarned *bool) (bool, bool) {
	if ctx.recursionLimitHit() {
		return false, false
	}
	beforeErrors := diagErrorCount(diags)
	result := executeFunctionCallValues(fn, []CallValueArg{{Value: arg, Span: argSpan}}, env, at, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return false, false
	}
	if diagErrorCount(diags) > beforeErrors {
		return false, false
	}
	keep, casted := truthy(result)
	if casted && !*castWarned {
		*castWarned = true
		diags.AddWarning(diag.CodeW101, "filter() cast non-boolean predicate result via truthiness", at, "return explicit boolean values from the predicate")
	}
	return keep, true
}

func evalFoldOperatorValueCall(name, op string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	boundArgs, ok := bindFoldOperatorArgs(name, args, at, diags)
	if !ok {
		return Null()
	}
	return evalFoldOperatorArgs(name, op, boundArgs, at, diags, opts, ctx)
}

func bindFoldOperatorArgs(name string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) ([]CallValueArg, bool) {
	bound, ok := bindBuiltinArgs(name, args, builtinSignature{
		Name:        name,
		Params:      []builtinParam{{Name: "values"}},
		Varargs:     "items",
		AllowNoArgs: true,
	}, at, diags)
	if !ok {
		return nil, false
	}
	inputs := make([]CallValueArg, 0, 1+len(bound.Varargs))
	if valueArg, exists := bound.ByName["values"]; exists {
		inputs = append(inputs, valueArg)
	}
	inputs = append(inputs, bound.Varargs...)
	return inputs, true
}

func evalFoldOperatorArgs(name, op string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	items := foldOperatorInputs(args)
	if len(items) == 0 {
		return foldOperatorIdentity(name)
	}
	if len(items) == 1 {
		return items[0]
	}
	acc := items[0]
	for _, item := range items[1:] {
		beforeErrors := diagErrorCount(diags)
		acc = evalBinary(op, acc, item, at, diags, opts, ctx)
		if diagErrorCount(diags) > beforeErrors {
			return Null()
		}
	}
	return acc
}

func foldOperatorInputs(args []CallValueArg) []Value {
	if len(args) == 1 {
		switch args[0].Value.Kind {
		case KindList, KindTuple:
			return CloneValues(args[0].Value.L)
		}
	}
	values := make([]Value, 0, len(args))
	for _, arg := range args {
		values = append(values, CloneValue(arg.Value))
	}
	return values
}

func foldOperatorIdentity(name string) Value {
	if name == "prod" {
		return Int(1)
	}
	return Int(0)
}

func diagErrorCount(diags *diag.Diagnostics) int {
	if diags == nil {
		return 0
	}
	count := 0
	for _, item := range diags.Items {
		if item.Severity == diag.SeverityError {
			count++
		}
	}
	return count
}
