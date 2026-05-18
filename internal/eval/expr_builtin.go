package eval

import (
	"unicode/utf8"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalNamesCall(rawArgs []ast.Expr, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) > 1 {
		diags.AddError(diag.CodeE106, "names() expects zero or one argument", at, "use names() or names(expr)")
		return Null()
	}
	if opts.Names == nil {
		diags.AddError(diag.CodeE106, "names() requires scope metadata in this evaluation context", at, "call names() only in normal compiled expression contexts")
		return Null()
	}
	if len(rawArgs) == 0 {
		return stringListValue(opts.Names.Visible)
	}
	if nsName, ok := namespaceArgName(rawArgs[0], opts.Names); ok {
		return stringListValue(opts.Names.Namespaces[nsName].Members)
	}
	before := len(diags.Items)
	value := evalExprWithCtx(rawArgs[0], env, diags, opts, ctx)
	if ctx.recursionLimitHit() {
		return Null()
	}
	if len(diags.Items) > before {
		return Null()
	}
	if IsComb(value) {
		return stringListValue(CombNames(value))
	}
	if value.Kind == KindDict {
		return dictKeyListValue(value.D)
	}
	diags.AddError(diag.CodeE106, "names() expects a module namespace, table, or dictionary value", rawArgs[0].GetSpan(), "use names(), names(module), names(table), or names(dictionary)")
	return Null()
}

func evalNamesDirectCall(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	direct := true
	exprs := make([]ast.Expr, 0, len(rawArgs))
	for _, arg := range rawArgs {
		if arg.EffectiveKind() != ast.CallArgPositional {
			direct = false
			break
		}
		exprs = append(exprs, arg.Expr)
	}
	if direct {
		return evalNamesCall(exprs, env, at, diags, opts, ctx)
	}
	args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
	if !ok {
		return Null()
	}
	return evalNamesValueCall(args, at, diags, opts)
}

func evalNamesValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	if opts.Names == nil {
		diags.AddError(diag.CodeE106, "names() requires scope metadata in this evaluation context", at, "call names() only in normal compiled expression contexts")
		return Null()
	}
	bound, ok := bindBuiltinArgs("names", args, builtinSignature{Name: "names", Varargs: "values", NamedVarargs: true, AllowNoArgs: true}, at, diags)
	if !ok {
		return Null()
	}
	if len(bound.Varargs) > 1 {
		diags.AddError(diag.CodeE106, "names() expects zero or one argument", at, "use names(), names(table), or names(dictionary)")
		return Null()
	}
	if len(bound.Varargs) == 0 {
		return stringListValue(opts.Names.Visible)
	}
	arg := bound.Varargs[0]
	if IsComb(arg.Value) {
		return stringListValue(CombNames(arg.Value))
	}
	if arg.Value.Kind == KindDict {
		return dictKeyListValue(arg.Value.D)
	}
	diags.AddError(diag.CodeE106, "names() function value expects a table or dictionary value", arg.Span, "use names(), names(table), or names(dictionary)")
	return Null()
}

func namespaceArgName(expr ast.Expr, catalog *NameCatalog) (string, bool) {
	if catalog == nil {
		return "", false
	}
	switch n := expr.(type) {
	case ast.IdentExpr:
		_, ok := catalog.Namespaces[n.Name]
		return n.Name, ok
	case ast.QualifiedIdentExpr:
		if n.Namespace == "" || n.Name == "" {
			return "", false
		}
		key := n.Namespace + "." + n.Name
		_, ok := catalog.Namespaces[key]
		return key, ok
	default:
		return "", false
	}
}

func stringListValue(names []string) Value {
	items := make([]Value, 0, len(names))
	for _, name := range names {
		items = append(items, String(name))
	}
	return List(items)
}

func dictKeyListValue(dict *Dict) Value {
	if dict == nil || len(dict.Order) == 0 {
		return List(nil)
	}
	items := make([]Value, 0, len(dict.Order))
	for _, key := range dict.Order {
		if _, ok := dict.Entries[key]; !ok {
			continue
		}
		items = append(items, ValueFromDictKey(key))
	}
	return List(items)
}

func evalTupleCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, "tuple() expects exactly one argument", at, "use tuple(expr)")
		return Null()
	}
	value := args[0]
	switch value.Kind {
	case KindComb:
		diags.AddError(diag.CodeE106, "tuple() does not accept table values", at, "project table columns before tuple() conversion")
		return Null()
	case KindList, KindTuple:
		return Tuple(slicesCloneValues(value.L))
	default:
		return Tuple([]Value{value})
	}
}

func evalListCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, "list() expects exactly one argument", at, "use list(expr)")
		return Null()
	}
	value := args[0]
	switch value.Kind {
	case KindComb:
		diags.AddError(diag.CodeE106, "list() does not accept table values", at, "project table columns before list() conversion")
		return Null()
	case KindList, KindTuple:
		return List(slicesCloneValues(value.L))
	default:
		return List([]Value{value})
	}
}

func evalLenCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, "len() expects exactly one argument", at, "use len(value)")
		return Null()
	}
	v := args[0]
	switch v.Kind {
	case KindList, KindTuple:
		return Int(int64(len(v.L)))
	case KindString:
		return Int(int64(utf8.RuneCountInString(v.S)))
	case KindDict:
		return Int(int64(dictLen(v.D)))
	case KindComb:
		return Int(int64(CombRowCount(v)))
	default:
		diags.AddError(diag.CodeE106, "len() expects list/tuple/string/dictionary/table value", at, "use len() with supported value kinds")
		return Null()
	}
}

func evalAllAnyCall(kind string, args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, kind+"() expects exactly one argument", at, "use "+kind+"(values)")
		return Null()
	}
	v := args[0]
	if v.Kind == KindComb {
		diags.AddError(diag.CodeE106, kind+"() does not accept table values", at, "project table columns before using "+kind+"()")
		return Null()
	}
	values := toSeriesOrScalar(v)
	if len(values) == 0 {
		if kind == "all" {
			return Bool(true)
		}
		return Bool(false)
	}
	castWarned := false
	test := func(item Value) bool {
		b, casted := truthy(item)
		if casted && !castWarned {
			castWarned = true
			diags.AddWarning(diag.CodeW101, kind+"() cast non-boolean values via truthiness", at, "use explicit boolean expressions to avoid implicit casts")
		}
		return b
	}
	if kind == "all" {
		for _, item := range values {
			if !test(item) {
				return Bool(false)
			}
		}
		return Bool(true)
	}
	for _, item := range values {
		if test(item) {
			return Bool(true)
		}
	}
	return Bool(false)
}

func toSeriesOrScalar(v Value) []Value {
	if v.Kind == KindList || v.Kind == KindTuple {
		return slicesCloneValues(v.L)
	}
	return []Value{v}
}

func truthy(v Value) (bool, bool) {
	switch v.Kind {
	case KindBool:
		return v.B, false
	case KindInt:
		return v.I != 0, true
	case KindFloat:
		return v.F != 0.0, true
	case KindString:
		return v.S != "", true
	case KindNull:
		return false, true
	case KindList, KindTuple:
		return len(v.L) > 0, true
	case KindDict:
		return dictLen(v.D) > 0, true
	case KindComb:
		return CombRowCount(v) > 0, true
	default:
		return true, true
	}
}
