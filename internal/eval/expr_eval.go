// evaluate assignment-level JBS expressions
//
// i.e. in
// ```jbs
// a = [1,2,3] * 2   # [2,4,6]
// b = ("a","b") * 2 # ("a","b","a","b")
// a */+ b
// ```
//
// implement the `a` and `b` assignments. It also handles operations
// with literals/unary/binary/compare/conditional/mode/calls
package eval

import (
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"
	"unicode/utf8"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

type EvalContext int

const (
	EvalCtxDefault EvalContext = iota
	EvalCtxBindingAssign
	EvalCtxScalarGlobalAssign
	EvalCtxSubmitField
	EvalCtxAnalyseAssign
)

type ExprOptions struct {
	GlobalAssignmentTupleArithmetic bool
	Context                         EvalContext
	Names                           *NameCatalog
	Files                           *FileAccess
	Frame                           *Frame
}

func EvalExpr(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics) Value {
	return EvalExprWithOptions(expr, env, diags, ExprOptions{})
}

func EvalExprWithOptions(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions) Value {
	frame := opts.Frame
	if frame == nil {
		frame = NewRootFrame(env)
	}
	return evalExprWithCtx(expr, env, diags, opts, &evalCtx{
		overflowWarned: make(map[string]struct{}),
		frame:          frame,
	})
}

type evalCtx struct {
	overflowWarned map[string]struct{}
	frame          *Frame
}

func evalExprWithCtx(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if expr == nil {
		return Null()
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if v, ok := lookupValue(e.Name, env, e.Span, diags, ctx); ok {
			return v
		}
		return Null()
	case ast.QualifiedIdentExpr:
		if ns, found, assigned := lookupLocalOrCapturedValue(e.Namespace, env, e.Span, diags, ctx); found {
			if !assigned {
				diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", e.Namespace), e.Span, "assign the local before reading it")
				return Null()
			}
			if IsComb(ns) {
				col, exists := CombColumn(ns, e.Name)
				if !exists {
					diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s.%s'", e.Namespace, e.Name), e.Span, "import or define the variable before use")
					return Null()
				}
				if len(col) == 1 {
					return col[0]
				}
				return List(col)
			}
			diags.AddError(diag.CodeE106, fmt.Sprintf("qualified access '%s.%s' requires a table namespace", e.Namespace, e.Name), e.Span, "use qualified access only on table values in expressions")
			return Null()
		}
		key := e.Namespace + "." + e.Name
		if v, found, assigned := lookupLocalOrCapturedValue(key, env, e.Span, diags, ctx); found {
			if !assigned {
				diags.AddError(diag.CodeE100, fmt.Sprintf("local variable '%s' is used before assignment", key), e.Span, "assign the local before reading it")
				return Null()
			}
			return v
		}
		diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", key), e.Span, "import or define the variable before use")
		return Null()
	case ast.MemberExpr:
		base := evalExprWithCtx(e.Base, env, diags, opts, ctx)
		if !IsComb(base) {
			diags.AddError(diag.CodeE106, fmt.Sprintf("member access '.%s' requires a table base", e.Name), e.Span, "use member access only on table values")
			return Null()
		}
		col, ok := CombColumn(base, e.Name)
		if !ok {
			diags.AddError(diag.CodeE100, fmt.Sprintf("unknown variable '%s'", e.Name), e.Span, "select an existing table column")
			return Null()
		}
		if len(col) == 1 {
			return col[0]
		}
		return List(col)
	case ast.StringExpr:
		return String(e.Value)
	case ast.NumberExpr:
		if e.Int {
			return Int(e.IntValue)
		}
		return Float(e.FloatValue)
	case ast.BoolExpr:
		return Bool(e.Value)
	case ast.ListExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, evalExprWithCtx(it, env, diags, opts, ctx))
		}
		return List(items)
	case ast.TupleExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, evalExprWithCtx(it, env, diags, opts, ctx))
		}
		return Tuple(items)
	case ast.FunctionExpr:
		return newFunctionValue(e, env, diags, opts, ctx)
	case ast.AliasExpr:
		diags.AddError(diag.CodeE106, "alias expression is only allowed in table-valued assignment operands", e.Span, "replace it with table(name = expr) or a named table operation")
		return Null()
	case ast.CallExpr:
		return evalCall(e.Callee, e.Args, env, e.Span, diags, opts, ctx)
	case ast.IndexExpr:
		base := evalExprWithCtx(e.Base, env, diags, opts, ctx)
		if !IsComb(base) {
			diags.AddError(diag.CodeE106, "index expression requires a table base", e.Span, "use syntax: table_value[col] or table_value[col0,col1]")
			return Null()
		}
		selectors := make([]string, 0, len(e.Items))
		for _, item := range e.Items {
			switch n := item.(type) {
			case ast.IdentExpr:
				selectors = append(selectors, n.Name)
			case ast.QualifiedIdentExpr:
				selectors = append(selectors, n.Namespace+"."+n.Name)
			default:
				diags.AddError(diag.CodeE106, "table index selectors must be identifiers", item.GetSpan(), "use syntax: table_value[col] or table_value[col0,col1]")
				return Null()
			}
		}
		if len(selectors) == 0 {
			diags.AddError(diag.CodeE106, "table index selectors cannot be empty", e.Span, "use at least one selector inside []")
			return Null()
		}
		projected, ok := CombProject(base, selectors)
		if !ok {
			diags.AddError(diag.CodeE106, "invalid table projection selector", e.Span, "select existing table columns only")
			return Null()
		}
		return projected
	case ast.UnaryExpr:
		v := evalExprWithCtx(e.Expr, env, diags, opts, ctx)
		return evalUnary(e.Op, v, e.Span, diags, ctx)
	case ast.BinaryExpr:
		if opts.Context == EvalCtxBindingAssign && (e.Op == "+" || e.Op == "*") && binaryNeedsRelaxedCombEval(e) {
			return evalRelaxedCombBinary(e, env, diags, opts, ctx)
		}
		l := evalExprWithCtx(e.Left, env, diags, opts, ctx)
		r := evalExprWithCtx(e.Right, env, diags, opts, ctx)
		if (e.Op == "+" || e.Op == "*") && (IsComb(l) || IsComb(r)) {
			opNode := ast.CombBinary{Op: e.Op, OpSpan: e.Span, Span: e.Span}
			leftRows := combRowsFromBinaryOperand(e.Left, l, env, diags, opts, ctx)
			rightRows := combRowsFromBinaryOperand(e.Right, r, env, diags, opts, ctx)
			if e.Op == "+" {
				return combValueFromRows(zipRows(leftRows, rightRows, opNode, diags))
			}
			return combValueFromRows(productRows(leftRows, rightRows, opNode, diags))
		}
		return evalBinary(e.Op, l, r, e.Span, diags, opts, ctx)
	case ast.CompareExpr:
		l := evalExprWithCtx(e.Left, env, diags, opts, ctx)
		r := evalExprWithCtx(e.Right, env, diags, opts, ctx)
		return evalCompare(e.Op, l, r, e.Span, diags)
	case ast.ConditionalExpr:
		c := evalExprWithCtx(e.Cond, env, diags, opts, ctx)
		if c.Kind != KindBool {
			diags.AddError(diag.CodeE102, "conditional requires boolean condition", e.Cond.GetSpan(), "ensure condition evaluates to true/false")
			return evalExprWithCtx(e.Then, env, diags, opts, ctx)
		}
		if c.B {
			return evalExprWithCtx(e.Then, env, diags, opts, ctx)
		}
		return evalExprWithCtx(e.Else, env, diags, opts, ctx)
	case ast.ModeExpr:
		return evalExprWithCtx(e.Expr, env, diags, opts, ctx)
	default:
		diags.AddError(diag.CodeE199, "unsupported expression node", expr.GetSpan(), "check expression syntax")
		return Null()
	}
}

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
	case "table":
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
	}
	switch name {
	case "read_csv":
		return evalReadCSVCall(args, at, diags, opts)
	case "int", "float", "str":
		return evalUnaryConvertCall(name, args, at, diags)
	case "len":
		return evalLenCall(args, at, diags)
	case "filter":
		return evalFilterCall(args, at, diags)
	case "all":
		return evalAllAnyCall("all", args, at, diags)
	case "any":
		return evalAllAnyCall("any", args, at, diags)
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
	if _, ok := kernelFuncs[ident.Name]; ok {
		return ident.Name, true
	}
	switch ident.Name {
	case "table", "zip", "product", "select", "names", "map", "reduce", "read_csv", "int", "float", "str", "len", "filter", "all", "any":
		return ident.Name, true
	default:
		return "", false
	}
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

func evalConvert(target string, value Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch target {
	case "tuple":
		return evalTupleCall([]Value{value}, at, diags)
	case "list":
		return evalListCall([]Value{value}, at, diags)
	case "int":
		return convertToInt(value, at, diags)
	case "float":
		return convertToFloat(value, at, diags)
	case "str":
		return convertToString(value)
	default:
		return value
	}
}

func evalUnaryConvertCall(name string, args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, name+"() expects exactly one argument", at, "use "+name+"(value)")
		return Null()
	}
	return evalConvert(name, args[0], at, diags)
}

func convertToInt(v Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch v.Kind {
	case KindInt:
		return v
	case KindFloat:
		if math.IsNaN(v.F) || math.IsInf(v.F, 0) || v.F < float64(math.MinInt64) || v.F > float64(math.MaxInt64) {
			diags.AddError(diag.CodeE106, "int() float must be finite and within 64-bit signed range", at, "use a finite float value within int64 range")
			return Null()
		}
		return Int(int64(v.F))
	case KindBool:
		if v.B {
			return Int(1)
		}
		return Int(0)
	case KindString:
		n, err := strconv.ParseInt(v.S, 10, 64)
		if err != nil {
			diags.AddError(diag.CodeE106, "int() string must be a base-10 integer", at, "use text such as '0', '-7', or '42'")
			return Null()
		}
		return Int(n)
	default:
		diags.AddError(diag.CodeE106, "int() expects int/float/bool/string value", at, "convert scalar values only")
		return Null()
	}
}

func convertToFloat(v Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch v.Kind {
	case KindFloat:
		return v
	case KindInt:
		return Float(float64(v.I))
	case KindBool:
		if v.B {
			return Float(1.0)
		}
		return Float(0.0)
	case KindString:
		f, err := strconv.ParseFloat(v.S, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			diags.AddError(diag.CodeE106, "float() string must be a finite decimal number", at, "use text such as '1', '1.5', or '1e3'")
			return Null()
		}
		return Float(f)
	default:
		diags.AddError(diag.CodeE106, "float() expects int/float/bool/string value", at, "convert scalar values only")
		return Null()
	}
}

func convertToString(v Value) Value {
	return String(v.String())
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
	if len(diags.Items) > before {
		return Null()
	}
	if !IsComb(value) {
		diags.AddError(diag.CodeE106, "names() expects a module namespace or table value", rawArgs[0].GetSpan(), "use names(), names(module), or names(table)")
		return Null()
	}
	return stringListValue(CombNames(value))
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
	case KindComb:
		return Int(int64(CombRowCount(v)))
	default:
		diags.AddError(diag.CodeE106, "len() expects list/tuple/string/table value", at, "use len() with supported value kinds")
		return Null()
	}
}

func evalFilterCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 2 {
		diags.AddError(diag.CodeE106, "filter() expects exactly two arguments", at, "use filter(values, mask)")
		return Null()
	}
	target := args[0]
	maskVals := ToSeries(args[1])
	if len(maskVals) == 0 {
		diags.AddError(diag.CodeE106, "filter() mask cannot be empty", at, "use a non-empty boolean mask")
		return Null()
	}
	switch target.Kind {
	case KindList, KindTuple:
		out := make([]Value, 0, len(target.L))
		mask := broadcastMask(maskVals, len(target.L), at, diags)
		for i, item := range target.L {
			if mask[i] {
				out = append(out, item)
			}
		}
		if target.Kind == KindTuple {
			return Tuple(out)
		}
		return List(out)
	case KindComb:
		if !IsComb(target) {
			return CombValue(&Comb{Order: nil, Rows: nil})
		}
		outRows := make([]Row, 0, len(target.C.Rows))
		mask := broadcastMask(maskVals, len(target.C.Rows), at, diags)
		for i, row := range target.C.Rows {
			if mask[i] {
				outRows = append(outRows, row.clone())
			}
		}
		return CombValue(&Comb{
			Order: append([]string(nil), target.C.Order...),
			Rows:  outRows,
		})
	default:
		diags.AddError(diag.CodeE106, "filter() expects list/tuple/table as first argument", at, "use filter() with list, tuple, or table values")
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
	case KindComb:
		return CombRowCount(v) > 0, true
	default:
		return true, true
	}
}

func broadcastMask(maskVals []Value, n int, at diag.Span, diags *diag.Diagnostics) []bool {
	if n <= 0 {
		return nil
	}
	m := len(maskVals)
	if m != n {
		shouldWarn := n%m != 0
		if shouldWarn {
			diags.AddWarning(
				diag.CodeW101,
				fmt.Sprintf("length mismatch in filter mask: values=%d mask=%d; cyclic broadcast to length %d", n, m, n),
				at,
				"align mask length with filtered value length",
			)
		}
	}
	out := make([]bool, n)
	castWarned := false
	for i := 0; i < n; i++ {
		b, casted := truthy(maskVals[i%m])
		if casted && !castWarned {
			castWarned = true
			diags.AddWarning(diag.CodeW101, "filter() cast non-boolean mask values via truthiness", at, "use explicit boolean mask values")
		}
		out[i] = b
	}
	return out
}

func binaryNeedsRelaxedCombEval(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case ast.AliasExpr:
		return true
	case ast.MemberExpr:
		return binaryNeedsRelaxedCombEval(e.Base)
	case ast.BinaryExpr:
		return binaryNeedsRelaxedCombEval(e.Left) || binaryNeedsRelaxedCombEval(e.Right)
	case ast.UnaryExpr:
		return binaryNeedsRelaxedCombEval(e.Expr)
	case ast.ModeExpr:
		return binaryNeedsRelaxedCombEval(e.Expr)
	case ast.CallExpr:
		if binaryNeedsRelaxedCombEval(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if binaryNeedsRelaxedCombEval(arg.Expr) {
				return true
			}
		}
		return false
	case ast.IndexExpr:
		if binaryNeedsRelaxedCombEval(e.Base) {
			return true
		}
		for _, item := range e.Items {
			if binaryNeedsRelaxedCombEval(item) {
				return true
			}
		}
		return false
	case ast.ListExpr:
		for _, item := range e.Items {
			if binaryNeedsRelaxedCombEval(item) {
				return true
			}
		}
		return false
	case ast.TupleExpr:
		for _, item := range e.Items {
			if binaryNeedsRelaxedCombEval(item) {
				return true
			}
		}
		return false
	case ast.CompareExpr:
		return binaryNeedsRelaxedCombEval(e.Left) || binaryNeedsRelaxedCombEval(e.Right)
	case ast.ConditionalExpr:
		return binaryNeedsRelaxedCombEval(e.Then) || binaryNeedsRelaxedCombEval(e.Cond) || binaryNeedsRelaxedCombEval(e.Else)
	default:
		return false
	}
}

func evalRelaxedCombBinary(expr ast.BinaryExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	left, okLeft := evalRelaxedCombOperand(expr.Left, env, diags, opts, ctx)
	right, okRight := evalRelaxedCombOperand(expr.Right, env, diags, opts, ctx)
	if !okLeft || !okRight {
		return Null()
	}
	opNode := ast.CombBinary{Op: expr.Op, OpSpan: expr.Span, Span: expr.Span}
	if expr.Op == "+" {
		return combValueFromRows(zipRows(left, right, opNode, diags))
	}
	return combValueFromRows(productRows(left, right, opNode, diags))
}

func evalRelaxedCombOperand(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) ([]Row, bool) {
	if expr == nil {
		return nil, false
	}
	if alias, ok := expr.(ast.AliasExpr); ok {
		if alias.Alias == "" {
			diags.AddError(diag.CodeE106, "table operand alias cannot be empty", alias.Span, "use syntax: expression as name")
			return nil, false
		}
		value := evalExprWithCtx(alias.Expr, env, diags, opts, ctx)
		if IsComb(value) {
			diags.AddError(diag.CodeE106, "alias cannot be applied to a table-valued expression", alias.Span, "apply alias only to non-table operands")
			return nil, false
		}
		return combRowsFromNamedValue(alias.Alias, value, alias.Span), true
	}
	value := evalExprWithCtx(expr, env, diags, opts, ctx)
	return combRowsFromBinaryOperand(expr, value, env, diags, opts, ctx), true
}

func combRowsFromNamedValue(name string, value Value, span diag.Span) []Row {
	if IsComb(value) {
		return cloneRows(value.C.Rows)
	}
	series := ToSeries(value)
	rows := make([]Row, 0, len(series))
	for _, v := range series {
		rows = append(rows, Row{
			Values: map[string]Cell{
				name: {
					Value:  v,
					Origin: span,
				},
			},
		})
	}
	return rows
}

func combRowsFromBinaryOperand(expr ast.Expr, value Value, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) []Row {
	if expr == nil {
		return combRowsFromValue(value, diag.Span{})
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		return combRowsFromNamedValue(e.Name, value, e.Span)
	case ast.QualifiedIdentExpr:
		return combRowsFromNamedValue(e.Namespace+"."+e.Name, value, e.Span)
	case ast.AliasExpr:
		rows, _ := evalRelaxedCombOperand(e, env, diags, opts, ctx)
		return rows
	default:
		return combRowsFromValue(value, expr.GetSpan())
	}
}

func firstDuplicatedColumnName(left, right []Row) (string, bool) {
	if len(left) == 0 || len(right) == 0 {
		return "", false
	}
	leftNames := RowVariableNames(left)
	if len(leftNames) == 0 {
		return "", false
	}
	leftSet := make(map[string]struct{}, len(leftNames))
	for _, name := range leftNames {
		leftSet[name] = struct{}{}
	}
	for _, name := range RowVariableNames(right) {
		if _, ok := leftSet[name]; ok {
			return name, true
		}
	}
	return "", false
}

func combRowsFromValue(value Value, _ diag.Span) []Row {
	if IsComb(value) {
		return cloneRows(value.C.Rows)
	}
	series := ToSeries(value)
	rows := make([]Row, 0, len(series))
	for range series {
		rows = append(rows, Row{Values: map[string]Cell{}})
	}
	return rows
}

func combValueFromRows(rows []Row) Value {
	if rows == nil {
		rows = make([]Row, 0)
	}
	return CombValue(&Comb{
		Order: RowVariableNames(rows),
		Rows:  cloneRows(rows),
	})
}

func evalRangeCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) < 1 || len(args) > 3 {
		diags.AddError(diag.CodeE106, "range() expects 1, 2, or 3 arguments", at, "use range(stop), range(start, stop), or range(start, stop, step)")
		return Null()
	}
	for _, arg := range args {
		if arg.Kind == KindNull {
			return Null()
		}
	}

	if len(args) < 3 {
		ints := make([]int64, len(args))
		for i, arg := range args {
			if arg.Kind != KindInt {
				diags.AddError(diag.CodeE106, "range() with 1 or 2 arguments expects integers", at, "use integer arguments only")
				return Null()
			}
			ints[i] = arg.I
		}
		start := int64(0)
		stop := int64(0)
		step := int64(1)
		switch len(ints) {
		case 1:
			stop = ints[0]
		case 2:
			start = ints[0]
			stop = ints[1]
		}
		return evalRangeInt(start, stop, step, at, diags)
	}

	allInt := true
	for _, arg := range args {
		if arg.Kind != KindInt {
			allInt = false
			break
		}
	}
	if allInt {
		return evalRangeInt(args[0].I, args[1].I, args[2].I, at, diags)
	}

	nums := make([]float64, 3)
	for i, arg := range args {
		switch arg.Kind {
		case KindInt:
			nums[i] = float64(arg.I)
		case KindFloat:
			nums[i] = arg.F
		default:
			diags.AddError(diag.CodeE106, "range() with 3 arguments expects numeric values", at, "use int or float arguments")
			return Null()
		}
	}
	return evalRangeFloat(nums[0], nums[1], nums[2], at, diags)
}

func evalRangeInt(start, stop, step int64, at diag.Span, diags *diag.Diagnostics) Value {
	if step <= 0 {
		diags.AddError(diag.CodeE106, "range() step must be a positive integer", at, "use step > 0")
		return Null()
	}
	if start >= stop {
		return List(nil)
	}
	items := make([]Value, 0)
	for current := start; current < stop; {
		items = append(items, Int(current))
		if current > math.MaxInt64-step {
			diags.AddError(diag.CodeE106, "range() overflow while generating values", at, "use smaller bounds or step")
			return Null()
		}
		current += step
	}
	return List(items)
}

func evalRangeFloat(start, stop, step float64, at diag.Span, diags *diag.Diagnostics) Value {
	if math.IsNaN(start) || math.IsNaN(stop) || math.IsNaN(step) || math.IsInf(start, 0) || math.IsInf(stop, 0) || math.IsInf(step, 0) {
		diags.AddError(diag.CodeE106, "range() with 3 arguments expects finite numeric values", at, "use finite int/float bounds and step")
		return Null()
	}
	if step <= 0 {
		diags.AddError(diag.CodeE106, "range() step must be positive", at, "use step > 0")
		return Null()
	}
	if start >= stop {
		return List(nil)
	}
	items := make([]Value, 0)
	for current := start; current < stop; {
		items = append(items, Float(current))
		next := current + step
		if !(next > current) {
			diags.AddError(diag.CodeE106, "range() step is too small to make progress", at, "use a larger step")
			return Null()
		}
		if math.IsNaN(next) || math.IsInf(next, 0) {
			diags.AddError(diag.CodeE106, "range() overflow while generating values", at, "use smaller bounds or step")
			return Null()
		}
		current = next
	}
	return List(items)
}

func evalRevCall(args []Value, at diag.Span, diags *diag.Diagnostics) Value {
	if len(args) != 1 {
		diags.AddError(diag.CodeE106, "rev() expects exactly one list/tuple argument", at, "use rev(list_or_tuple_expr)")
		return Null()
	}
	value := args[0]
	if value.Kind == KindNull {
		return Null()
	}
	if value.Kind != KindList && value.Kind != KindTuple {
		diags.AddError(diag.CodeE106, "rev() expects a list or tuple argument", at, "use rev(list_or_tuple_expr)")
		return Null()
	}
	out := slicesCloneValues(value.L)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	if value.Kind == KindTuple {
		return Tuple(out)
	}
	return List(out)
}

func evalUnary(op string, v Value, at diag.Span, diags *diag.Diagnostics, ctx *evalCtx) Value {
	if op == "!" {
		return evalLogicalNot(v, at, diags)
	}
	if isSequence(v) {
		out := make([]Value, len(v.L))
		for i, it := range v.L {
			out[i] = evalUnary(op, it, at, diags, ctx)
		}
		return List(out)
	}
	if !isNumeric(v) {
		diags.AddError(diag.CodeE103, fmt.Sprintf("unary '%s' requires numeric value", op), at, "use int/float values")
		return Null()
	}
	if op == "+" {
		return v
	}
	if v.Kind == KindFloat {
		return Float(-v.F)
	}
	result, overflow := negInt64Checked(v.I)
	if overflow {
		ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("-%d wraps to %d", v.I, result))
	}
	return Int(result)
}

func evalLogicalNot(v Value, at diag.Span, diags *diag.Diagnostics) Value {
	if isSequence(v) {
		out := make([]Value, 0, len(v.L))
		castWarned := false
		for _, item := range v.L {
			b, casted := truthy(item)
			if casted && !castWarned {
				castWarned = true
				diags.AddWarning(diag.CodeW101, "logical '!' cast non-boolean values via truthiness", at, "use explicit boolean expressions to avoid implicit casts")
			}
			out = append(out, Bool(!b))
		}
		return List(out)
	}
	b, casted := truthy(v)
	if casted {
		diags.AddWarning(diag.CodeW101, "logical '!' cast non-boolean value via truthiness", at, "use explicit boolean expressions to avoid implicit casts")
	}
	return Bool(!b)
}

func evalLogicalBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	if !isSequence(l) && !isSequence(r) {
		lb, lcast := truthy(l)
		rb, rcast := truthy(r)
		if lcast || rcast {
			diags.AddWarning(diag.CodeW101, fmt.Sprintf("logical '%s' cast non-boolean values via truthiness", op), at, "use explicit boolean expressions to avoid implicit casts")
		}
		if op == "&" {
			return Bool(lb && rb)
		}
		return Bool(lb || rb)
	}

	ls := ToSeries(l)
	rs := ToSeries(r)
	if len(ls) == 0 || len(rs) == 0 {
		return List(nil)
	}
	n := len(ls)
	if len(rs) > n {
		n = len(rs)
	}
	if len(ls) != len(rs) {
		diags.AddWarning(
			diag.CodeW101,
			fmt.Sprintf("length mismatch in logical '%s': left=%d right=%d; cyclic broadcast to length %d", op, len(ls), len(rs), n),
			at,
			"align lengths to avoid cyclic broadcast",
		)
	}
	out := make([]Value, 0, n)
	castWarned := false
	for i := 0; i < n; i++ {
		lb, lcast := truthy(ls[i%len(ls)])
		rb, rcast := truthy(rs[i%len(rs)])
		if (lcast || rcast) && !castWarned {
			castWarned = true
			diags.AddWarning(diag.CodeW101, fmt.Sprintf("logical '%s' cast non-boolean values via truthiness", op), at, "use explicit boolean expressions to avoid implicit casts")
		}
		if op == "&" {
			out = append(out, Bool(lb && rb))
		} else {
			out = append(out, Bool(lb || rb))
		}
	}
	return List(out)
}

func evalBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if op == "&" || op == "|" {
		return evalLogicalBinary(op, l, r, at, diags)
	}
	if l.Kind == KindFunction || r.Kind == KindFunction {
		diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' does not accept function values", op), at, "call the function first or remove it from the arithmetic expression")
		return Null()
	}

	if IsComb(l) || IsComb(r) {
		switch op {
		case "+", "*":
			leftRows := combRowsFromValue(l, at)
			rightRows := combRowsFromValue(r, at)
			opNode := ast.CombBinary{Op: op, OpSpan: at, Span: at}
			if op == "+" {
				return combValueFromRows(zipRows(leftRows, rightRows, opNode, diags))
			}
			return combValueFromRows(productRows(leftRows, rightRows, opNode, diags))
		default:
			diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' is not supported for table values", op), at, "use zip(), product(), select(), or filter() with table values")
			return Null()
		}
	}

	if opts.GlobalAssignmentTupleArithmetic && (IsTuple(l) || IsTuple(r)) {
		return evalParamTupleBinary(op, l, r, at, diags)
	}

	if isSequence(l) || isSequence(r) {
		return evalVectorBinary(op, l, r, at, diags, opts, ctx)
	}
	if l.Kind == KindString || r.Kind == KindString {
		switch op {
		case "+":
			return String(l.String() + r.String())
		case "*":
			if l.Kind == KindString {
				return evalStringRepeat(l, r, at, diags)
			}
			return evalStringRepeat(r, l, at, diags)
		default:
			diags.AddError(diag.CodeE105, fmt.Sprintf("operator '%s' is not supported for strings", op), at, "use '+' for concatenation or '*' for repetition")
			return Null()
		}
	}
	if !isNumeric(l) || !isNumeric(r) {
		diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' requires numeric or string operands", op), at, "check operand types")
		return Null()
	}

	lf := toFloat(l)
	rf := toFloat(r)
	switch op {
	case "+":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf + rf)
		}
		result, overflow := addInt64Checked(l.I, r.I)
		if overflow {
			ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("%d + %d wraps to %d", l.I, r.I, result))
		}
		return Int(result)
	case "-":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf - rf)
		}
		result, overflow := subInt64Checked(l.I, r.I)
		if overflow {
			ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("%d - %d wraps to %d", l.I, r.I, result))
		}
		return Int(result)
	case "*":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf * rf)
		}
		result, overflow := mulInt64Checked(l.I, r.I)
		if overflow {
			ctx.warnIntOverflow(diags, op, at, fmt.Sprintf("%d * %d wraps to %d", l.I, r.I, result))
		}
		return Int(result)
	case "/":
		if rf == 0 {
			diags.AddError(diag.CodeE107, "division by zero", at, "guard denominator")
			return Null()
		}
		return Float(lf / rf)
	case "%":
		if r.Kind == KindFloat || l.Kind == KindFloat {
			diags.AddError(diag.CodeE108, "modulo requires integer operands", at, "use int values with '%' operator")
			return Null()
		}
		if r.I == 0 {
			diags.AddError(diag.CodeE107, "modulo by zero", at, "guard denominator")
			return Null()
		}
		return Int(l.I % r.I)
	default:
		diags.AddError(diag.CodeE109, fmt.Sprintf("unknown operator '%s'", op), at, "use supported operators")
		return Null()
	}
}

func evalStringRepeat(str Value, count Value, at diag.Span, diags *diag.Diagnostics) Value {
	if count.Kind != KindInt {
		diags.AddError(diag.CodeE105, "string '*' requires integer repeat count", at, "use string * int or int * string")
		return Null()
	}
	if count.I < 0 {
		diags.AddError(diag.CodeE105, "string repetition count must be non-negative", at, "use an integer value >= 0")
		return Null()
	}
	maxInt := int64(^uint(0) >> 1)
	if count.I > maxInt {
		diags.AddError(diag.CodeE105, "string repetition count is too large", at, "use a smaller repeat count")
		return Null()
	}
	return String(strings.Repeat(str.S, int(count.I)))
}

func evalParamTupleBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch op {
	case "+":
		if !IsTuple(l) || !IsTuple(r) {
			diags.AddError(diag.CodeE106, "tuple '+' requires tuple operands on both sides", at, "use tuple + tuple")
			return Null()
		}
		items := make([]Value, 0, len(l.L)+len(r.L))
		items = append(items, l.L...)
		items = append(items, r.L...)
		return Tuple(items)
	case "*":
		if !IsTuple(l) || r.Kind != KindInt {
			diags.AddError(diag.CodeE106, "tuple '*' requires tuple * integer", at, "use tuple * non-negative integer")
			return Null()
		}
		if r.I < 0 {
			diags.AddError(diag.CodeE106, "tuple repetition count must be non-negative", at, "use an integer value >= 0")
			return Null()
		}
		if len(l.L) == 0 || r.I == 0 {
			return Tuple(nil)
		}
		items := make([]Value, 0, len(l.L)*int(r.I))
		for i := int64(0); i < r.I; i++ {
			items = append(items, l.L...)
		}
		return Tuple(items)
	default:
		diags.AddError(diag.CodeE106, fmt.Sprintf("operator '%s' is not supported for tuple arithmetic", op), at, "use '+' for concatenation or '*' for repetition")
		return Null()
	}
}

func evalVectorBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	ls := ToSeries(l)
	rs := ToSeries(r)
	if len(ls) == 0 || len(rs) == 0 {
		return List(nil)
	}
	n := len(ls)
	if len(rs) > n {
		n = len(rs)
	}
	out := make([]Value, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, evalBinary(op, ls[i%len(ls)], rs[i%len(rs)], at, diags, opts, ctx))
	}
	return List(out)
}

func isSequence(v Value) bool {
	return v.Kind == KindList || v.Kind == KindTuple
}

func slicesCloneValues(v []Value) []Value {
	if len(v) == 0 {
		return nil
	}
	out := make([]Value, len(v))
	copy(out, v)
	return out
}

func addInt64Checked(a, b int64) (int64, bool) {
	result := a + b
	overflow := (a > 0 && b > 0 && result < 0) || (a < 0 && b < 0 && result >= 0)
	return result, overflow
}

func subInt64Checked(a, b int64) (int64, bool) {
	result := a - b
	overflow := (a >= 0 && b < 0 && result < 0) || (a < 0 && b > 0 && result >= 0)
	return result, overflow
}

func mulInt64Checked(a, b int64) (int64, bool) {
	result := a * b
	if a == 0 || b == 0 {
		return result, false
	}
	absA := absInt64ToUint64(a)
	absB := absInt64ToUint64(b)
	hi, lo := bits.Mul64(absA, absB)
	if hi != 0 {
		return result, true
	}
	negative := (a < 0) != (b < 0)
	if negative {
		return result, lo > (uint64(1) << 63)
	}
	return result, lo > uint64(math.MaxInt64)
}

func negInt64Checked(v int64) (int64, bool) {
	result := -v
	return result, v == math.MinInt64
}

func absInt64ToUint64(v int64) uint64 {
	if v >= 0 {
		return uint64(v)
	}
	if v == math.MinInt64 {
		return uint64(1) << 63
	}
	return uint64(-v)
}

func (c *evalCtx) warnIntOverflow(diags *diag.Diagnostics, op string, at diag.Span, detail string) {
	key := fmt.Sprintf("%s|%s|%d|%d", op, at.File, at.Start.Offset, at.End.Offset)
	if _, exists := c.overflowWarned[key]; exists {
		return
	}
	c.overflowWarned[key] = struct{}{}
	diags.AddWarning(
		diag.CodeW102,
		fmt.Sprintf("integer overflow in '%s': %s", op, detail),
		at,
		"use smaller values or switch to floating-point arithmetic",
	)
}

func evalCompare(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	if isSequence(l) || isSequence(r) {
		ls := ToSeries(l)
		rs := ToSeries(r)
		if len(ls) == 0 || len(rs) == 0 {
			return List(nil)
		}
		n := len(ls)
		if len(rs) > n {
			n = len(rs)
		}
		out := make([]Value, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, evalCompare(op, ls[i%len(ls)], rs[i%len(rs)], at, diags))
		}
		return List(out)
	}
	if l.Kind == KindFunction || r.Kind == KindFunction {
		diags.AddError(diag.CodeE110, fmt.Sprintf("comparison '%s' does not accept function values", op), at, "call the function first or compare non-function values")
		return Bool(false)
	}

	switch op {
	case "==":
		return Bool(Equal(l, r))
	case "!=":
		return Bool(!Equal(l, r))
	}

	if l.Kind == KindString && r.Kind == KindString {
		switch op {
		case "<":
			return Bool(l.S < r.S)
		case "<=":
			return Bool(l.S <= r.S)
		case ">":
			return Bool(l.S > r.S)
		case ">=":
			return Bool(l.S >= r.S)
		}
	}
	if isNumeric(l) && isNumeric(r) {
		lf := toFloat(l)
		rf := toFloat(r)
		switch op {
		case "<":
			return Bool(lf < rf)
		case "<=":
			return Bool(lf <= rf)
		case ">":
			return Bool(lf > rf)
		case ">=":
			return Bool(lf >= rf)
		}
	}
	diags.AddError(diag.CodeE110, fmt.Sprintf("unsupported comparison '%s' for operand types", op), at, "compare compatible types")
	return Bool(false)
}
