package eval

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

type FunctionValue struct {
	Params  []ast.FuncParam
	Body    []ast.FuncBodyStmt
	Capture *Frame
	Files   *FileAccess
	Names   *NameCatalog
	Span    diag.Span
}

type functionResult struct {
	Value    Value
	Returned bool
}

func newFunctionValue(expr ast.FunctionExpr, capture *Frame, opts ExprOptions) Value {
	return Function(&FunctionValue{
		Params:  append([]ast.FuncParam(nil), expr.Params...),
		Body:    append([]ast.FuncBodyStmt(nil), expr.Body...),
		Capture: capture,
		Files:   cloneFileAccess(opts.Files),
		Names:   cloneNameCatalog(opts.Names),
		Span:    expr.Span,
	})
}

func cloneFileAccess(files *FileAccess) *FileAccess {
	if files == nil {
		return nil
	}
	return &FileAccess{
		BaseDir:  files.BaseDir,
		ReadFile: files.ReadFile,
	}
}

func cloneNameCatalog(catalog *NameCatalog) *NameCatalog {
	if catalog == nil {
		return nil
	}
	namespaces := make(map[string][]string, len(catalog.Namespaces))
	for name, ns := range catalog.Namespaces {
		namespaces[name] = append([]string(nil), ns.Members...)
	}
	return NewNameCatalog(append([]string(nil), catalog.Visible...), namespaces)
}

func callNameCatalog(catalog *NameCatalog, frame *Frame) *NameCatalog {
	if catalog == nil && frame == nil {
		return nil
	}
	visible := make([]string, 0)
	if catalog != nil {
		visible = append(visible, catalog.Visible...)
	}
	if frame != nil {
		visible = append(visible, frame.VisibleNames()...)
	}
	namespaces := make(map[string][]string)
	if catalog != nil {
		for name, ns := range catalog.Namespaces {
			namespaces[name] = append([]string(nil), ns.Members...)
		}
	}
	return NewNameCatalog(visible, namespaces)
}

func (ctx *evalCtx) withFrame(frame *Frame) *evalCtx {
	if ctx == nil {
		return &evalCtx{
			overflowWarned: make(map[string]struct{}),
			frame:          frame,
		}
	}
	next := *ctx
	next.frame = frame
	return &next
}

func executeFunctionCall(fn *FunctionValue, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if fn == nil {
		diags.AddError(diag.CodeE199, "expression is not callable", at, "call a function value or supported builtin")
		return Null()
	}
	callFrame := NewChildFrame(fn.Capture)
	callOpts := opts
	if fn.Files != nil {
		callOpts.Files = cloneFileAccess(fn.Files)
	}
	callOpts.Names = callNameCatalog(fn.Names, callFrame)

	boundValues, ok := bindFunctionArguments(fn, rawArgs, env, at, diags, opts, ctx)
	if !ok {
		return Null()
	}
	for i, param := range fn.Params {
		if bound, exists := boundValues[i]; exists {
			callFrame.AssignLocal(param.Name, bound, param.Span)
			continue
		}
		if param.Default == nil {
			diags.AddError(diag.CodeE106, fmt.Sprintf("missing required argument '%s'", param.Name), at, "pass a value for every required parameter")
			return Null()
		}
		callOpts.Names = callNameCatalog(fn.Names, callFrame)
		value := evalExprWithCtx(param.Default, env, diags, callOpts, ctx.withFrame(callFrame))
		callFrame.AssignLocal(param.Name, value, param.Span)
	}
	predeclareFunctionLocals(fn.Body, callFrame)
	callOpts.Names = callNameCatalog(fn.Names, callFrame)
	result := executeFunctionBody(fn.Body, env, diags, callOpts, ctx.withFrame(callFrame))
	return result.Value
}

func bindFunctionArguments(fn *FunctionValue, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) (map[int]Value, bool) {
	bound := make(map[int]Value, len(rawArgs))
	paramIndex := make(map[string]int, len(fn.Params))
	for i, param := range fn.Params {
		if param.Name == "" {
			continue
		}
		paramIndex[param.Name] = i
	}
	namedSeen := false
	nextPositional := 0
	for _, arg := range rawArgs {
		if arg.Name == "" {
			if namedSeen {
				diags.AddError(diag.CodeE106, "positional arguments cannot follow named arguments", arg.Span, "pass positional arguments before any named arguments")
				return nil, false
			}
			if nextPositional >= len(fn.Params) {
				diags.AddError(diag.CodeE106, "too many positional arguments", arg.Span, "remove extra arguments or add parameters")
				return nil, false
			}
			value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
			bound[nextPositional] = value
			nextPositional++
			continue
		}
		namedSeen = true
		idx, ok := paramIndex[arg.Name]
		if !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("unknown named argument '%s'", arg.Name), arg.Span, "use one of the declared parameter names")
			return nil, false
		}
		if _, exists := bound[idx]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("parameter '%s' received multiple values", arg.Name), arg.Span, "pass each parameter at most once")
			return nil, false
		}
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		bound[idx] = value
	}
	return bound, true
}

func predeclareFunctionLocals(body []ast.FuncBodyStmt, frame *Frame) {
	for _, stmt := range body {
		assign, ok := stmt.(ast.LocalAssignStmt)
		if !ok || assign.Name == "" {
			continue
		}
		frame.DeclareLocal(assign.Name)
	}
}

func executeFunctionBody(body []ast.FuncBodyStmt, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) functionResult {
	last := Null()
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			executeLocalAssign(node, env, diags, opts, ctx)
		case ast.ReturnStmt:
			if node.Expr == nil {
				return functionResult{Value: Null(), Returned: true}
			}
			return functionResult{
				Value:    evalExprWithCtx(node.Expr, env, diags, opts, ctx),
				Returned: true,
			}
		case ast.ExprStmt:
			last = evalExprWithCtx(node.Expr, env, diags, opts, ctx)
		}
	}
	return functionResult{Value: last}
}

func executeLocalAssign(stmt ast.LocalAssignStmt, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) {
	if ctx == nil || ctx.frame == nil || stmt.Name == "" {
		return
	}
	value := evalExprWithCtx(stmt.Expr, env, diags, opts, ctx)
	if stmt.Op == ast.AssignEq {
		ctx.frame.AssignLocal(stmt.Name, value, stmt.Span)
		return
	}
	current, ok := ctx.frame.Read(stmt.Name, stmt.Span, diags)
	if !ok {
		return
	}
	next := evalBinary(assignBinaryOp(stmt.Op), current, value, stmt.Span, diags, opts, ctx)
	ctx.frame.AssignLocal(stmt.Name, next, stmt.Span)
}

func assignBinaryOp(op ast.AssignOp) string {
	switch op {
	case ast.AssignPlusEq:
		return "+"
	case ast.AssignMinusEq:
		return "-"
	case ast.AssignStarEq:
		return "*"
	case ast.AssignSlashEq:
		return "/"
	case ast.AssignPctEq:
		return "%"
	default:
		return ""
	}
}
