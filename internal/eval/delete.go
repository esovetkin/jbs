package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func evalDeleteCall(rawArgs []ast.CallArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if len(rawArgs) == 0 {
		diags.AddError(diag.CodeE106, "delete() expects at least one variable", at, "use delete(name)")
		return Null()
	}
	seen := make(map[string]diag.Span, len(rawArgs))
	for _, arg := range rawArgs {
		if arg.Name != "" {
			diags.AddError(diag.CodeE106, "delete() does not accept named arguments", arg.Span, "pass bare variable names")
			continue
		}
		ident, ok := arg.Expr.(ast.IdentExpr)
		if !ok || ident.Name == "" {
			diags.AddError(diag.CodeE106, "delete() targets must be bare identifiers", arg.Span, "use delete(name)")
			continue
		}
		if prev, duplicate := seen[ident.Name]; duplicate {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("delete() target '%s' is listed more than once", ident.Name),
				ident.Span,
				"delete each variable at most once",
				diag.RelatedSpan{Message: "previous target", Span: prev},
			)
			continue
		}
		seen[ident.Name] = ident.Span
		deleteOneName(ident.Name, ident.Span, diags, opts, ctx)
	}
	return Null()
}

func deleteOneName(name string, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) {
	if canDeleteTopLevel(opts, ctx) {
		opts.DeleteName(name, at, diags)
		return
	}
	if ctx != nil && ctx.frame != nil && ctx.frame.DeleteLocal(name) {
		return
	}
	if IsBuiltinCallName(name) {
		diags.AddError(diag.CodeE106, fmt.Sprintf("cannot delete built-in function '%s'", name), at, "built-in functions are always available")
		return
	}
	diags.AddError(diag.CodeE100, fmt.Sprintf("unknown local variable '%s'", name), at, "delete only variables declared in the current scope")
}

func canDeleteTopLevel(opts ExprOptions, ctx *evalCtx) bool {
	return opts.DeleteName != nil && opts.Frame != nil && ctx != nil && ctx.frame == opts.Frame
}
