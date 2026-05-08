package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func dictKeyFromValue(v Value, at diag.Span, diags *diag.Diagnostics) (DictKey, bool) {
	switch v.Kind {
	case KindString:
		return DictKey{Kind: DictKeyString, S: v.S}, true
	case KindInt:
		return DictKey{Kind: DictKeyInt, I: v.I}, true
	case KindBool:
		return DictKey{Kind: DictKeyBool, B: v.B}, true
	default:
		diags.AddError(diag.CodeE106, "dictionary key must be string, int, or bool", at, "use a hashable scalar key")
		return DictKey{}, false
	}
}

func evalDictExpr(expr ast.DictExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	entries := make([]DictEntry, 0, len(expr.Entries))
	for _, item := range expr.Entries {
		keyValue := evalExprWithCtx(item.Key, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		key, ok := dictKeyFromValue(keyValue, item.Key.GetSpan(), diags)
		if !ok {
			return Null()
		}
		value := evalExprWithCtx(item.Value, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		entries = append(entries, DictEntry{Key: key, Value: value})
	}
	return DictValue(entries)
}

func evalDictCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	entries := make([]DictEntry, 0, len(rawArgs))
	for _, arg := range rawArgs {
		if arg.Name == "" {
			diags.AddError(diag.CodeE106, "dict() expects named arguments only", arg.Span, "use dict(name = value)")
			return Null()
		}
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		entries = append(entries, DictEntry{Key: DictKey{Kind: DictKeyString, S: arg.Name}, Value: value})
	}
	return DictValue(entries)
}

func evalDictGetCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) != 3 {
		diags.AddError(diag.CodeE106, "get() expects exactly three arguments", at, "use get(dict_value, key, default_value)")
		return Null()
	}
	args := make([]Value, 0, len(rawArgs))
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "get() does not accept named arguments", arg.Span, "use get(dict_value, key, default_value)")
			return Null()
		}
		args = append(args, evalExprWithCtx(arg.Expr, env, diags, opts, ctx))
		if ctx.recursionLimitHit() {
			return Null()
		}
	}
	base := args[0]
	if base.Kind != KindDict || base.D == nil {
		diags.AddError(diag.CodeE106, "get() first argument must be a dictionary", rawArgs[0].Span, "use get(dict_value, key, default_value)")
		return Null()
	}
	key, ok := dictKeyFromValue(args[1], rawArgs[1].Span, diags)
	if !ok {
		return Null()
	}
	value, exists := base.D.Entries[key]
	if !exists {
		return CloneValue(args[2])
	}
	return CloneValue(value)
}

func evalUpdateCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) == 0 || rawArgs[0].Name != "" {
		diags.AddError(diag.CodeE106, "update() expects dictionary first argument", at, "use update(dict_value, key = value)")
		return Null()
	}
	base := evalExprWithCtx(rawArgs[0].Expr, env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	if base.Kind != KindDict || base.D == nil {
		diags.AddError(diag.CodeE106, "update() first argument must be a dictionary", rawArgs[0].Span, "use update(dict_value, key = value)")
		return Null()
	}
	out := CloneValue(base)
	for _, arg := range rawArgs[1:] {
		if arg.Name == "" {
			diags.AddError(diag.CodeE106, "update() updates must be named arguments", arg.Span, "use update(dict_value, key = value)")
			return Null()
		}
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return Null()
		}
		out.D.Set(DictKey{Kind: DictKeyString, S: arg.Name}, value)
	}
	return out
}

func mergeDicts(left, right Value) Value {
	out := CloneValue(left)
	if out.D == nil {
		out.D = &Dict{Entries: make(map[DictKey]Value)}
	}
	if right.D == nil {
		return out
	}
	for _, key := range right.D.Order {
		value, ok := right.D.Entries[key]
		if !ok {
			continue
		}
		out.D.Set(key, value)
	}
	return out
}

func evalDictIndex(base Value, items []ast.Expr, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(items) != 1 {
		diags.AddError(diag.CodeE106, "dictionary index expects exactly one key", at, "use syntax: dict_value[key]")
		return Null()
	}
	keyValue := evalExprWithCtx(items[0], env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	key, ok := dictKeyFromValue(keyValue, items[0].GetSpan(), diags)
	if !ok {
		return Null()
	}
	if base.D == nil {
		diags.AddError(diag.CodeE106, fmt.Sprintf("dictionary key %s not found", key.StableString()), items[0].GetSpan(), "use get(dict_value, key, default_value) for optional keys")
		return Null()
	}
	value, exists := base.D.Entries[key]
	if !exists {
		diags.AddError(diag.CodeE106, fmt.Sprintf("dictionary key %s not found", key.StableString()), items[0].GetSpan(), "use get(dict_value, key, default_value) for optional keys")
		return Null()
	}
	return CloneValue(value)
}
