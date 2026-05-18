package eval

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"

func evalBuiltinValueCall(name string, args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	spec, ok := lookupBuiltinSpec(name)
	if !ok {
		diags.AddError(diag.CodeE199, "unknown function '"+name+"'", at, "use a supported builtin or define a function value before calling it")
		return Null()
	}
	if !checkBuiltinContext(spec, at, diags, opts) {
		return Null()
	}
	return callBuiltinSpec(spec, builtinCallContext{
		Name:  name,
		Args:  args,
		Env:   env,
		At:    at,
		Diags: diags,
		Opts:  opts,
		Ctx:   ctx,
	})
}

func bindRangeBuiltinValues(args []CallValueArg, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	hasNamed := false
	for _, arg := range args {
		if arg.Name != "" {
			hasNamed = true
			break
		}
	}
	if !hasNamed {
		values := callArgsToValues(args)
		if len(values) < 1 || len(values) > 3 {
			diags.AddError(diag.CodeE106, "range() expects 1, 2, or 3 arguments", at, "use range(stop), range(start, stop), or range(start, stop, step)")
			return nil, false
		}
		return values, true
	}
	bound, ok := bindBuiltinArgs("range", args, builtinSignature{
		Name: "range",
		Params: []builtinParam{
			{Name: "start"},
			{Name: "stop", Required: true},
			{Name: "step"},
		},
	}, at, diags)
	if !ok {
		return nil, false
	}

	stop := bound.ByName["stop"].Value
	startArg, hasStart := bound.ByName["start"]
	stepArg, hasStep := bound.ByName["step"]
	if !hasStart && !hasStep {
		return []Value{stop}, true
	}

	start := Int(0)
	if hasStart {
		start = startArg.Value
	}
	if !hasStep {
		return []Value{start, stop}, true
	}
	return []Value{start, stop, stepArg.Value}, true
}
