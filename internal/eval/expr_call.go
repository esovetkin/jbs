package eval

import (
	"fmt"
	"slices"
	"sync"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

var specialBuiltinCallNames = map[string]struct{}{
	"all":      {},
	"any":      {},
	"bool":     {},
	"delete":   {},
	"dict":     {},
	"env":      {},
	"filter":   {},
	"float":    {},
	"get":      {},
	"head":     {},
	"int":      {},
	"len":      {},
	"map":      {},
	"names":    {},
	"prod":     {},
	"print":    {},
	"read_csv": {},
	"rbind":    {},
	"reduce":   {},
	"rename":   {},
	"rows":     {},
	"shell":    {},
	"str":      {},
	"sum":      {},
	"table":    {},
	"tail":     {},
	"t":        {},
	"update":   {},
}

var builtinFunctionValues struct {
	once   sync.Once
	values map[string]Value
}

func initBuiltinFunctionValues() {
	builtinFunctionValues.values = make(map[string]Value)
	for _, name := range BuiltinCallNames() {
		builtinFunctionValues.values[name] = Function(&FunctionValue{BuiltinName: name})
	}
}

func BuiltinFunctionValue(name string) (Value, bool) {
	builtinFunctionValues.once.Do(initBuiltinFunctionValues)
	value, ok := builtinFunctionValues.values[name]
	return value, ok
}

func evalCall(callee ast.Expr, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if fn, ok, fallback := resolveCallable(callee, env, diags, opts, ctx); ok {
		return executeFunctionCall(fn, rawArgs, env, at, diags, opts, ctx)
	} else if !fallback {
		return Null()
	}
	name, ok := builtinCallName(callee)
	if !ok {
		diags.AddError(diag.CodeE199, "expression is not callable", callee.GetSpan(), "call a function value or supported builtin")
		return Null()
	}
	switch name {
	case "table", "t":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalTableValueCall(args, at, diags)
	case "dict":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalDictValueCall(args, at, diags)
	case "delete":
		return evalDeleteCall(rawArgs, env, at, diags, opts, ctx)
	case "env":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalEnvValueCall(args, at, diags, opts)
	case "get":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalDictGetValueCall(args, at, diags)
	case "names":
		return evalNamesDirectCall(rawArgs, env, at, diags, opts, ctx)
	case "map":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalMapValueCall(args, env, at, diags, opts, ctx)
	case "reduce":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalReduceValueCall(args, env, at, diags, opts, ctx)
	case "rename":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalRenameValueCall(args, at, diags)
	case "rbind":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalRbindValueCall(args, at, diags)
	case "filter":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalFilterValueCall(args, env, at, diags, opts, ctx)
	case "head", "tail":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalHeadTailValueCall(name, args, at, diags)
	case "sum":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalFoldOperatorValueCall("sum", "+", args, at, diags, opts, ctx)
	case "prod":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalFoldOperatorValueCall("prod", "*", args, at, diags, opts, ctx)
	case "rows":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalRowsValueCall(args, at, diags)
	case "shell":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalShellValueCall(args, env, at, diags, opts, ctx)
	case "update":
		args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
		if !ok {
			return Null()
		}
		return evalUpdateValueCall(args, at, diags)
	}
	args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
	if !ok {
		return Null()
	}
	return evalBuiltinValueCall(name, args, env, at, diags, opts, ctx)
}

func lookupLocalOrCapturedValue(name string, env map[string]Value, at diag.Span, diags *diag.Diagnostics, ctx *evalCtx) (Value, bool, bool) {
	if name == "" {
		return Null(), false, false
	}
	if ctx != nil && ctx.frame != nil {
		if value, found, assigned := ctx.frame.ResolveValue(name, at, diags); found {
			return value, true, assigned
		}
	}
	v, ok := env[name]
	return v, ok, ok
}

func lookupValue(name string, env map[string]Value, at diag.Span, diags *diag.Diagnostics, ctx *evalCtx) (Value, bool) {
	if value, found, assigned := lookupLocalOrCapturedValue(name, env, at, diags, ctx); found {
		if assigned {
			return value, true
		}
		diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", name), at, "assign the local before reading it")
		return Null(), false
	}
	if value, ok := BuiltinFunctionValue(name); ok {
		return value, true
	}
	if value, ok := BuiltinConstantValue(name); ok {
		return value, true
	}
	diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", name), at, "import or define the variable before use")
	return Null(), false
}

func BuiltinConstantValue(name string) (Value, bool) {
	if name == "None" {
		return Null(), true
	}
	return Null(), false
}

func IsBuiltinConstantName(name string) bool {
	_, ok := BuiltinConstantValue(name)
	return ok
}

func resolveCallable(callee ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) (*FunctionValue, bool, bool) {
	if name, ok := builtinCallName(callee); ok {
		if value, found, assigned := lookupLocalOrCapturedValue(name, env, callee.GetSpan(), diags, ctx); found {
			if !assigned {
				diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", name), callee.GetSpan(), "assign the local before reading it")
				return nil, false, false
			}
			if value.Kind == KindFunction && value.Fn != nil {
				return value.Fn, true, false
			}
			diags.AddError(diag.CodeE199, "expression is not callable", callee.GetSpan(), "call a function value or supported builtin")
			return nil, false, false
		}
		return nil, false, true
	}
	if ident, ok := callee.(ast.IdentExpr); ok {
		if value, found, assigned := lookupLocalOrCapturedValue(ident.Name, env, callee.GetSpan(), diags, ctx); found {
			if !assigned {
				diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", ident.Name), callee.GetSpan(), "assign the local before reading it")
				return nil, false, false
			}
			if value.Kind == KindFunction && value.Fn != nil {
				return value.Fn, true, false
			}
			diags.AddError(diag.CodeE199, "expression is not callable", callee.GetSpan(), "call a function value or supported builtin")
			return nil, false, false
		}
		diags.AddError(diag.CodeE199, fmt.Sprintf("unknown function '%s'", ident.Name), callee.GetSpan(), "use a supported builtin or define a function value before calling it")
		return nil, false, false
	}
	before := len(diags.Items)
	value := evalExprWithCtx(callee, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return nil, false, false
	}
	if len(diags.Items) > before {
		return nil, false, false
	}
	if value.Kind != KindFunction || value.Fn == nil {
		diags.AddError(diag.CodeE199, "expression is not callable", callee.GetSpan(), "call a function value or supported builtin")
		return nil, false, false
	}
	return value.Fn, true, false
}

func builtinCallName(callee ast.Expr) (string, bool) {
	ident, ok := callee.(ast.IdentExpr)
	if !ok || ident.Name == "" {
		return "", false
	}
	if IsBuiltinCallName(ident.Name) {
		return ident.Name, true
	}
	return "", false
}

func IsBuiltinCallName(name string) bool {
	if _, ok := kernelFuncs[name]; ok {
		return true
	}
	_, ok := specialBuiltinCallNames[name]
	return ok
}

func BuiltinCallNames() []string {
	seen := make(map[string]struct{}, len(kernelFuncs)+len(specialBuiltinCallNames))
	for name := range kernelFuncs {
		seen[name] = struct{}{}
	}
	for name := range specialBuiltinCallNames {
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
