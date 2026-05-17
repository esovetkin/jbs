package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalBuiltinValueCall(name string, args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	switch name {
	case "table", "t":
		return evalTableValueCall(args, at, diags)
	case "dict":
		return evalDictValueCall(args, at, diags)
	case "delete":
		return evalDeleteValueCall(args, at, diags, opts, ctx)
	case "env":
		return evalEnvValueCall(args, at, diags, opts)
	case "get":
		return evalDictGetValueCall(args, at, diags)
	case "names":
		return evalNamesValueCall(args, at, diags, opts)
	case "map":
		return evalMapValueCall(args, env, at, diags, opts, ctx)
	case "reduce":
		return evalReduceValueCall(args, env, at, diags, opts, ctx)
	case "rename":
		return evalRenameValueCall(args, at, diags)
	case "filter":
		return evalFilterValueCall(args, env, at, diags, opts, ctx)
	case "head", "tail":
		return evalHeadTailValueCall(name, args, at, diags)
	case "sum":
		return evalFoldOperatorValueCall("sum", "+", args, at, diags, opts, ctx)
	case "prod":
		return evalFoldOperatorValueCall("prod", "*", args, at, diags, opts, ctx)
	case "rows":
		return evalRowsValueCall(args, at, diags)
	case "shell":
		return evalShellValueCall(args, env, at, diags, opts, ctx)
	case "update":
		return evalUpdateValueCall(args, at, diags)
	case "print":
		bound, ok := bindPrintArgs(args, diags)
		if !ok {
			return Null()
		}
		return evalPrintCall(bound.Values, bound.Options, at, opts)
	}

	values, ok := simpleBuiltinValues(name, args, at, diags)
	if !ok {
		return Null()
	}
	switch name {
	case "read_csv":
		return evalReadCSVCall(values, at, diags, opts)
	case "bool", "int", "float", "str":
		return evalUnaryConvertCall(name, values, at, diags)
	case "len":
		return evalLenCall(values, at, diags)
	case "all":
		return evalAllAnyCall("all", values, at, diags)
	case "any":
		return evalAllAnyCall("any", values, at, diags)
	}
	return evalKernelCall(name, values, at, diags, opts)
}

func simpleBuiltinValues(name string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	if !IsBuiltinCallName(name) {
		diags.AddError(diag.CodeE199, fmt.Sprintf("unknown function '%s'", name), at, "use a supported builtin or define a function value before calling it")
		return nil, false
	}
	switch name {
	case "read_csv":
		bound, ok := bindBuiltinArgs(name, args, builtinSignature{Name: name, Params: []builtinParam{{Name: "path", Required: true}}}, at, diags)
		if !ok {
			return nil, false
		}
		return []Value{bound.ByName["path"].Value}, true
	case "bool", "int", "float", "str", "len", "all", "any", "rev", "tuple", "list":
		param := "value"
		if name == "all" || name == "any" || name == "rev" {
			param = "values"
		}
		bound, ok := bindBuiltinArgs(name, args, builtinSignature{Name: name, Params: []builtinParam{{Name: param, Required: true}}}, at, diags)
		if !ok {
			return nil, false
		}
		return []Value{bound.ByName[param].Value}, true
	case "range":
		return bindRangeBuiltinValues(args, at, diags)
	}
	values := make([]Value, 0, len(args))
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, name+"() does not accept named arguments", arg.Span, "pass positional arguments only")
			return nil, false
		}
		values = append(values, arg.Value)
	}
	return values, true
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
	if _, ok := bound.ByName["start"]; !ok {
		bound.ByName["start"] = CallValueArg{Value: Int(0), Span: at}
	}
	if _, ok := bound.ByName["step"]; !ok {
		bound.ByName["step"] = CallValueArg{Value: Int(1), Span: at}
	}
	return []Value{
		bound.ByName["start"].Value,
		bound.ByName["stop"].Value,
		bound.ByName["step"].Value,
	}, true
}
