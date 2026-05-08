package eval

import (
	"fmt"
	"slices"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

var specialBuiltinCallNames = map[string]struct{}{
	"all":      {},
	"any":      {},
	"bool":     {},
	"filter":   {},
	"float":    {},
	"int":      {},
	"len":      {},
	"map":      {},
	"names":    {},
	"product":  {},
	"print":    {},
	"read_csv": {},
	"reduce":   {},
	"select":   {},
	"str":      {},
	"table":    {},
	"t":        {},
	"zip":      {},
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
		return evalTableCall(rawArgs, env, at, diags, opts, ctx)
	case "zip":
		return evalZipCall(rawArgs, env, at, diags, opts, ctx)
	case "product":
		return evalProductCall(rawArgs, env, at, diags, opts, ctx)
	case "select":
		return evalSelectCall(rawArgs, env, at, diags, opts, ctx)
	case "names":
		return evalNamesCall(callArgExprs(rawArgs), env, at, diags, opts, ctx)
	case "map":
		return evalMapCall(rawArgs, env, at, diags, opts, ctx)
	case "reduce":
		return evalReduceCall(rawArgs, env, at, diags, opts, ctx)
	}
	args := make([]Value, 0, len(rawArgs))
	for _, arg := range rawArgs {
		args = append(args, evalExprWithCtx(arg.Expr, env, diags, opts, ctx))
		if ctx.recursionLimitHit() {
			return Null()
		}
	}
	switch name {
	case "read_csv":
		return evalReadCSVCall(args, at, diags, opts)
	case "bool", "int", "float", "str":
		return evalUnaryConvertCall(name, args, at, diags)
	case "len":
		return evalLenCall(args, at, diags)
	case "filter":
		return evalFilterCall(args, at, diags)
	case "all":
		return evalAllAnyCall("all", args, at, diags)
	case "any":
		return evalAllAnyCall("any", args, at, diags)
	case "print":
		return evalPrintCall(args, at, opts)
	}
	return evalKernelCall(name, args, at, diags, opts)
}

func callArgExprs(args []ast.CallArg) []ast.Expr {
	if len(args) == 0 {
		return nil
	}
	out := make([]ast.Expr, 0, len(args))
	for _, arg := range args {
		out = append(out, arg.Expr)
	}
	return out
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
	diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", name), at, "import or define the variable before use")
	return Null(), false
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
