package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalBuiltinValueCall(name string, args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	switch name {
	case "table", "t":
		return evalTableValueCall(args, at, diags)
	case "zip":
		return evalZipValueCall(args, at, diags)
	case "product":
		return evalProductValueCall(args, at, diags)
	case "select":
		return evalSelectValueCall(args, at, diags)
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
	case "filter":
		return evalFilterValueCall(args, env, at, diags, opts, ctx)
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
	}

	values, ok := positionalBuiltinValues(name, args, at, diags)
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
	case "print":
		return evalPrintCall(values, at, opts)
	}
	return evalKernelCall(name, values, at, diags, opts)
}

func positionalBuiltinValues(name string, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	values := make([]Value, 0, len(args))
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, name+"() does not accept named arguments", arg.Span, "pass positional arguments only")
			return nil, false
		}
		values = append(values, arg.Value)
	}
	if !IsBuiltinCallName(name) {
		diags.AddError(diag.CodeE199, fmt.Sprintf("unknown function '%s'", name), at, "use a supported builtin or define a function value before calling it")
		return nil, false
	}
	return values, true
}
