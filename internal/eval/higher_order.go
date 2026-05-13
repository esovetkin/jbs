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
	switch v.Kind {
	case KindList:
		return slicesCloneValues(v.L), false, true
	case KindTuple:
		return slicesCloneValues(v.L), true, true
	default:
		diags.AddError(diag.CodeE106, kind+"() expects list or tuple as second argument", at, "pass a list or tuple value")
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
