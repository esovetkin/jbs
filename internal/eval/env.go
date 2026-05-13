package eval

import (
	"os"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func currentEnviron(opts ExprOptions) []string {
	if opts.Environ != nil {
		return opts.Environ()
	}
	return os.Environ()
}

func environmentMap(opts ExprOptions) map[string]string {
	out := make(map[string]string)
	for _, item := range currentEnviron(opts) {
		name, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}

func environmentDict(opts ExprOptions) Value {
	values := environmentMap(opts)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)

	entries := make([]DictEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, DictEntry{
			Key:   DictKey{Kind: DictKeyString, S: name},
			Value: String(values[name]),
		})
	}
	return DictValue(entries)
}

func evalEnvCall(rawArgs []ast.CallArg, scopeEnv map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) > 2 {
		diags.AddError(diag.CodeE106, "env() expects zero, one, or two positional arguments", at, `use env(), env("NAME"), or env("NAME", default_value)`)
		return Null()
	}
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "env() does not accept named arguments", arg.Span, `use env("NAME", default_value)`)
			return Null()
		}
	}
	if len(rawArgs) == 0 {
		return environmentDict(opts)
	}

	nameValue := evalExprWithCtx(rawArgs[0].Expr, scopeEnv, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	if nameValue.Kind != KindString {
		diags.AddError(diag.CodeE106, "env() variable name must be a string", rawArgs[0].Span, `use env("NAME")`)
		return Null()
	}

	fallback := String("")
	if len(rawArgs) == 2 {
		fallback = evalExprWithCtx(rawArgs[1].Expr, scopeEnv, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
	}

	values := environmentMap(opts)
	if value, ok := values[nameValue.S]; ok {
		return String(value)
	}
	return CloneValue(fallback)
}

func evalEnvValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	if len(args) > 2 {
		diags.AddError(diag.CodeE106, "env() expects zero, one, or two positional arguments", at, `use env(), env("NAME"), or env("NAME", default_value)`)
		return Null()
	}
	for _, arg := range args {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "env() does not accept named arguments", arg.Span, `use env("NAME", default_value)`)
			return Null()
		}
	}
	if len(args) == 0 {
		return environmentDict(opts)
	}
	if args[0].Value.Kind != KindString {
		diags.AddError(diag.CodeE106, "env() variable name must be a string", args[0].Span, `use env("NAME")`)
		return Null()
	}

	fallback := String("")
	if len(args) == 2 {
		fallback = args[1].Value
	}
	values := environmentMap(opts)
	if value, ok := values[args[0].Value.S]; ok {
		return String(value)
	}
	return CloneValue(fallback)
}
