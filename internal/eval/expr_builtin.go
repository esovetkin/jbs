package eval

import (
	"fmt"
	"unicode/utf8"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type kernelFunc struct {
	allowed map[EvalContext]struct{}
	eval    func(args []Value, at diag.Span, diags *diag.Diagnostics) Value
}

var kernelFuncs = map[string]kernelFunc{
	"range": {
		allowed: map[EvalContext]struct{}{
			EvalCtxBindingAssign: {},
		},
		eval: evalRangeCall,
	},
	"rev": {
		allowed: map[EvalContext]struct{}{
			EvalCtxBindingAssign: {},
		},
		eval: evalRevCall,
	},
	"tuple": {
		eval: evalTupleCall,
	},
	"list": {
		eval: evalListCall,
	},
}

func evalKernelCall(name string, args []Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	fn, ok := kernelFuncs[name]
	if !ok {
		diags.AddError(diag.CodeE199, fmt.Sprintf("unknown function '%s'", name), at, "use a supported kernel function")
		return Null()
	}
	if len(fn.allowed) > 0 {
		if _, ok := fn.allowed[opts.Context]; !ok {
			diags.AddError(
				diag.CodeE199,
				fmt.Sprintf("function '%s' is only allowed in top-level global assignments", name),
				at,
				"use this function only in top-level global assignment expressions",
			)
			return Null()
		}
	}
	return fn.eval(args, at, diags)
}

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
	if !IsComb(value) {
		diags.AddError(diag.CodeE106, "names() expects a module namespace or table value", rawArgs[0].GetSpan(), "use names(), names(module), or names(table)")
		return Null()
	}
	return stringListValue(CombNames(value))
}

func evalNamesValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	if len(args) > 1 {
		diags.AddError(diag.CodeE106, "names() expects zero or one argument", at, "use names() or names(table)")
		return Null()
	}
	if opts.Names == nil {
		diags.AddError(diag.CodeE106, "names() requires scope metadata in this evaluation context", at, "call names() only in normal compiled expression contexts")
		return Null()
	}
	if len(args) == 0 {
		return stringListValue(opts.Names.Visible)
	}
	if args[0].Name != "" {
		diags.AddError(diag.CodeE106, "names() does not accept named arguments", args[0].Span, "pass the table as a positional argument")
		return Null()
	}
	if !IsComb(args[0].Value) {
		diags.AddError(diag.CodeE106, "names() function value expects a table value", args[0].Span, "use names() or names(table)")
		return Null()
	}
	return stringListValue(CombNames(args[0].Value))
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
