package eval

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalStrictPositionalCallValueArgs(name string, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx, want int) ([]CallValueArg, bool) {
	if len(rawArgs) != want {
		diags.AddError(diag.CodeE106, name+"() expects exactly two arguments", at, "use "+name+"(function, values)")
		return nil, false
	}
	args := make([]CallValueArg, 0, len(rawArgs))
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, name+"() does not accept named arguments", arg.Span, "pass positional arguments only")
			return nil, false
		}
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return nil, false
		}
		args = append(args, CallValueArg{Value: value, Span: arg.Span})
	}
	return args, true
}

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

func evalMapCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	args, ok := evalStrictPositionalCallValueArgs("map", rawArgs, env, at, diags, opts, ctx, 2)
	if !ok {
		return Null()
	}
	return evalMapValueCall(args, env, at, diags, opts, ctx)
}

func evalMapValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(args) != 2 {
		diags.AddError(diag.CodeE106, "map() expects exactly two arguments", at, "use map(function, values)")
		return Null()
	}
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "map() does not accept named arguments", arg.Span, "pass positional arguments only")
			return Null()
		}
	}
	fnValue := args[0].Value
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "map() expects function value as first argument", args[0].Span, "pass a function literal, built-in function, or function-valued variable")
		return Null()
	}
	items, isTuple, ok := sequenceItems("map", args[1].Value, args[1].Span, diags)
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

func evalReduceCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	args, ok := evalStrictPositionalCallValueArgs("reduce", rawArgs, env, at, diags, opts, ctx, 2)
	if !ok {
		return Null()
	}
	return evalReduceValueCall(args, env, at, diags, opts, ctx)
}

func evalReduceValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(args) != 2 {
		diags.AddError(diag.CodeE106, "reduce() expects exactly two arguments", at, "use reduce(function, values)")
		return Null()
	}
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "reduce() does not accept named arguments", arg.Span, "pass positional arguments only")
			return Null()
		}
	}
	fnValue := args[0].Value
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "reduce() expects function value as first argument", args[0].Span, "pass a function literal, built-in function, or function-valued variable")
		return Null()
	}
	items, _, ok := sequenceItems("reduce", args[1].Value, args[1].Span, diags)
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

func evalFilterCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) != 2 {
		diags.AddError(diag.CodeE106, "filter() expects exactly two arguments", at, "use filter(values, function)")
		return Null()
	}
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "filter() does not accept named arguments", arg.Span, "pass positional arguments only")
			return Null()
		}
	}
	values := evalExprWithCtx(rawArgs[0].Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	predicate := evalExprWithCtx(rawArgs[1].Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	return evalFilterValueCall([]CallValueArg{
		{Value: values, Span: rawArgs[0].Span},
		{Value: predicate, Span: rawArgs[1].Span},
	}, env, at, diags, opts, ctx)
}

func evalFilterValueCall(args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(args) != 2 {
		diags.AddError(diag.CodeE106, "filter() expects exactly two arguments", at, "use filter(values, function)")
		return Null()
	}
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "filter() does not accept named arguments", arg.Span, "pass positional arguments only")
			return Null()
		}
	}
	target := args[0].Value
	fnValue := args[1].Value
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "filter() expects function value as second argument", args[1].Span, "pass a function literal, built-in function, or function-valued variable")
		return Null()
	}
	switch target.Kind {
	case KindList, KindTuple:
		return evalFilterSequence(target, fnValue.Fn, env, at, diags, opts, ctx)
	case KindComb:
		return evalFilterTable(target, fnValue.Fn, env, args[0].Span, at, diags, opts, ctx)
	default:
		diags.AddError(diag.CodeE106, "filter() expects list/tuple/table as first argument", args[0].Span, "pass a list, tuple, or table value")
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

func evalFoldOperatorCall(name, op string, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) != 1 {
		diags.AddError(diag.CodeE106, name+"() expects exactly one argument", at, "use "+name+"(values)")
		return Null()
	}
	arg := rawArgs[0]
	if arg.Name != "" {
		diags.AddError(diag.CodeE106, name+"() does not accept named arguments", arg.Span, "pass positional arguments only")
		return Null()
	}
	value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	return evalFoldOperator(name, op, value, arg.Span, at, diags, opts, ctx)
}

func evalFoldOperatorValueCall(name, op string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, name+"() expects exactly one argument", at, "use "+name+"(values)")
		return Null()
	}
	if args[0].Name != "" {
		diags.AddError(diag.CodeE106, name+"() does not accept named arguments", args[0].Span, "pass positional arguments only")
		return Null()
	}
	return evalFoldOperator(name, op, args[0].Value, args[0].Span, at, diags, opts, ctx)
}

func evalFoldOperator(name, op string, value Value, valueSpan diag.Span, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	items, _, ok := sequenceItemsForArg(name, "first", value, valueSpan, diags)
	if !ok {
		return Null()
	}
	if len(items) == 0 {
		diags.AddError(diag.CodeE106, name+"() cannot operate on an empty list/tuple", at, "use a non-empty list/tuple")
		return Null()
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
