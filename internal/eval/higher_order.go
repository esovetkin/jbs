package eval

import (
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type builtinArgValues struct {
	Values []Value
	Ok     bool
}

func evalStrictPositionalBuiltinArgs(name string, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx, want int) builtinArgValues {
	if len(rawArgs) != want {
		diags.AddError(diag.CodeE106, name+"() expects exactly two arguments", at, "use "+name+"(function, values)")
		return builtinArgValues{}
	}
	values := make([]Value, 0, len(rawArgs))
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, name+"() does not accept named arguments", arg.Span, "pass positional arguments only")
			return builtinArgValues{}
		}
		values = append(values, evalExprWithCtx(arg.Expr, env, diags, opts, ctx))
		if ctx.recursionLimitHit() {
			return builtinArgValues{}
		}
	}
	return builtinArgValues{Values: values, Ok: true}
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
	args := evalStrictPositionalBuiltinArgs("map", rawArgs, env, at, diags, opts, ctx, 2)
	if !args.Ok {
		return Null()
	}
	fnValue := args.Values[0]
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "map() expects function value as first argument", rawArgs[0].Span, "pass a function literal or function-valued variable")
		return Null()
	}
	items, isTuple, ok := sequenceItems("map", args.Values[1], rawArgs[1].Span, diags)
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
	args := evalStrictPositionalBuiltinArgs("reduce", rawArgs, env, at, diags, opts, ctx, 2)
	if !args.Ok {
		return Null()
	}
	fnValue := args.Values[0]
	if fnValue.Kind != KindFunction || fnValue.Fn == nil {
		diags.AddError(diag.CodeE106, "reduce() expects function value as first argument", rawArgs[0].Span, "pass a function literal or function-valued variable")
		return Null()
	}
	items, _, ok := sequenceItems("reduce", args.Values[1], rawArgs[1].Span, diags)
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
