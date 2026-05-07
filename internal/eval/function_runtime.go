package eval

import (
	"fmt"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type FunctionValue struct {
	Params   []ast.FuncParam
	Body     []ast.FuncBodyStmt
	Capture  *Frame
	Files    *FileAccess
	Names    *NameCatalog
	Span     diag.Span
	Defaults map[int]FunctionDefault
}

type FunctionDefault struct {
	Value        Value
	PreEvaluated bool
}

type CallValueArg struct {
	Name  string
	Value Value
	Span  diag.Span
}

type functionResult struct {
	Value    Value
	Returned bool
	Break    bool
	Continue bool
	Span     diag.Span
}

func newFunctionValue(expr ast.FunctionExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	capture := (*Frame)(nil)
	if ctx != nil {
		capture = ctx.frame
	}
	return Function(&FunctionValue{
		Params:   append([]ast.FuncParam(nil), expr.Params...),
		Body:     append([]ast.FuncBodyStmt(nil), expr.Body...),
		Capture:  capture,
		Files:    cloneFileAccess(opts.Files),
		Names:    cloneNameCatalog(opts.Names),
		Span:     expr.Span,
		Defaults: preEvaluateFunctionDefaults(expr, env, diags, opts, ctx),
	})
}

func preEvaluateFunctionDefaults(expr ast.FunctionExpr, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) map[int]FunctionDefault {
	defaults := make(map[int]FunctionDefault)
	earlier := make(map[string]struct{}, len(expr.Params))
	for i, param := range expr.Params {
		if param.Default != nil && !exprReferencesAnyName(param.Default, earlier) {
			defaults[i] = FunctionDefault{
				Value:        evalExprWithCtx(param.Default, env, diags, opts, ctx),
				PreEvaluated: true,
			}
		}
		if param.Name != "" {
			earlier[param.Name] = struct{}{}
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	return defaults
}

func exprReferencesAnyName(expr ast.Expr, names map[string]struct{}) bool {
	if expr == nil || len(names) == 0 {
		return false
	}
	var walk func(ast.Expr, map[string]struct{}) bool
	isBound := func(name string, bound map[string]struct{}) bool {
		_, ok := bound[name]
		return ok
	}
	walk = func(node ast.Expr, bound map[string]struct{}) bool {
		if node == nil {
			return false
		}
		switch n := node.(type) {
		case ast.IdentExpr:
			_, target := names[n.Name]
			return target && !isBound(n.Name, bound)
		case ast.QualifiedIdentExpr:
			_, target := names[n.Namespace]
			return target && !isBound(n.Namespace, bound)
		case ast.MemberExpr:
			return walk(n.Base, bound)
		case ast.ListExpr:
			for _, item := range n.Items {
				if walk(item, bound) {
					return true
				}
			}
		case ast.TupleExpr:
			for _, item := range n.Items {
				if walk(item, bound) {
					return true
				}
			}
		case ast.CallExpr:
			if walk(n.Callee, bound) {
				return true
			}
			for _, arg := range n.Args {
				if walk(arg.Expr, bound) {
					return true
				}
			}
		case ast.FunctionExpr:
			nextBound := cloneNameSet(bound)
			for _, param := range n.Params {
				if walk(param.Default, nextBound) {
					return true
				}
				if param.Name != "" {
					nextBound[param.Name] = struct{}{}
				}
			}
			collectFunctionLocalNames(n.Body, nextBound)
			if functionBodyReferencesAnyName(n.Body, nextBound, walk) {
				return true
			}
		case ast.AliasExpr:
			return walk(n.Expr, bound)
		case ast.IndexExpr:
			return walk(n.Base, bound)
		case ast.UnaryExpr:
			return walk(n.Expr, bound)
		case ast.BinaryExpr:
			return walk(n.Left, bound) || walk(n.Right, bound)
		case ast.CompareExpr:
			return walk(n.Left, bound) || walk(n.Right, bound)
		case ast.ConditionalExpr:
			return walk(n.Then, bound) || walk(n.Cond, bound) || walk(n.Else, bound)
		}
		return false
	}
	return walk(expr, nil)
}

func collectFunctionLocalNames(body []ast.FuncBodyStmt, out map[string]struct{}) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			if node.Name != "" {
				out[node.Name] = struct{}{}
			}
		case ast.FuncIfStmt:
			collectFunctionLocalNames(node.Then, out)
			collectFunctionLocalNames(node.Else, out)
		case ast.FuncForStmt:
			if node.Target != "" {
				out[node.Target] = struct{}{}
			}
			collectFunctionLocalNames(node.Body, out)
		case ast.FuncWhileStmt:
			collectFunctionLocalNames(node.Body, out)
		}
	}
}

func functionBodyReferencesAnyName(body []ast.FuncBodyStmt, bound map[string]struct{}, walk func(ast.Expr, map[string]struct{}) bool) bool {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			if walk(node.Expr, bound) {
				return true
			}
		case ast.ReturnStmt:
			if walk(node.Expr, bound) {
				return true
			}
		case ast.ExprStmt:
			if walk(node.Expr, bound) {
				return true
			}
		case ast.FuncIfStmt:
			if walk(node.Cond, bound) {
				return true
			}
			if functionBodyReferencesAnyName(node.Then, bound, walk) {
				return true
			}
			if functionBodyReferencesAnyName(node.Else, bound, walk) {
				return true
			}
		case ast.FuncForStmt:
			if walk(node.Iterable, bound) {
				return true
			}
			nextBound := cloneNameSet(bound)
			if node.Target != "" {
				nextBound[node.Target] = struct{}{}
			}
			if functionBodyReferencesAnyName(node.Body, nextBound, walk) {
				return true
			}
		case ast.FuncWhileStmt:
			if walk(node.Cond, bound) {
				return true
			}
			if functionBodyReferencesAnyName(node.Body, bound, walk) {
				return true
			}
		}
	}
	return false
}

func cloneNameSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for name := range in {
		out[name] = struct{}{}
	}
	return out
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
	args, _ := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
	return executeFunctionCallValues(fn, args, env, at, diags, opts, ctx)
}

func evalCallValueArgs(rawArgs []ast.CallArg, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) ([]CallValueArg, bool) {
	args := make([]CallValueArg, 0, len(rawArgs))
	for _, arg := range rawArgs {
		args = append(args, CallValueArg{
			Name:  arg.Name,
			Value: evalExprWithCtx(arg.Expr, env, diags, opts, ctx),
			Span:  arg.Span,
		})
	}
	return args, true
}

func executeFunctionCallValues(fn *FunctionValue, args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
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

	boundValues, ok := bindFunctionArguments(fn, args, at, diags)
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
		var value Value
		if defaultValue, ok := fn.Defaults[i]; ok && defaultValue.PreEvaluated {
			value = defaultValue.Value
		} else {
			callOpts.Names = callNameCatalog(fn.Names, callFrame)
			value = evalExprWithCtx(param.Default, env, diags, callOpts, ctx.withFrame(callFrame))
		}
		callFrame.AssignLocal(param.Name, value, param.Span)
	}
	predeclareFunctionLocals(fn.Body, callFrame)
	callOpts.Names = callNameCatalog(fn.Names, callFrame)
	result := executeFunctionBody(fn.Body, env, diags, callOpts, ctx.withFrame(callFrame))
	if result.Break || result.Continue {
		diags.AddError(diag.CodeE080, "'break' and 'continue' are only allowed inside loops", result.Span, "move the statement into a for/while body")
		return Null()
	}
	return result.Value
}

func bindFunctionArguments(fn *FunctionValue, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) (map[int]Value, bool) {
	bound := make(map[int]Value, len(args))
	paramIndex := make(map[string]int, len(fn.Params))
	for i, param := range fn.Params {
		if param.Name == "" {
			continue
		}
		paramIndex[param.Name] = i
	}
	namedSeen := false
	nextPositional := 0
	for _, arg := range args {
		if arg.Name == "" {
			if namedSeen {
				diags.AddError(diag.CodeE106, "positional arguments cannot follow named arguments", arg.Span, "pass positional arguments before any named arguments")
				return nil, false
			}
			if nextPositional >= len(fn.Params) {
				diags.AddError(diag.CodeE106, "too many positional arguments", arg.Span, "remove extra arguments or add parameters")
				return nil, false
			}
			bound[nextPositional] = arg.Value
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
		bound[idx] = arg.Value
	}
	return bound, true
}

func predeclareFunctionLocals(body []ast.FuncBodyStmt, frame *Frame) {
	for _, stmt := range body {
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			if node.Name != "" {
				frame.DeclareLocal(node.Name)
			}
		case ast.FuncIfStmt:
			predeclareFunctionLocals(node.Then, frame)
			predeclareFunctionLocals(node.Else, frame)
		case ast.FuncForStmt:
			if node.Target != "" {
				frame.DeclareLocal(node.Target)
			}
			predeclareFunctionLocals(node.Body, frame)
		case ast.FuncWhileStmt:
			predeclareFunctionLocals(node.Body, frame)
		}
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
		case ast.BreakStmt:
			return functionResult{Value: last, Break: true, Span: node.Span}
		case ast.ContinueStmt:
			return functionResult{Value: last, Continue: true, Span: node.Span}
		case ast.FuncIfStmt:
			cond, ok := evalBoolConditionWithCtx("if", node.Cond, env, diags, opts, ctx)
			if !ok {
				continue
			}
			if cond {
				result := executeFunctionBody(node.Then, env, diags, opts, ctx)
				if result.Returned || result.Break || result.Continue {
					return result
				}
				last = result.Value
				continue
			}
			if len(node.Else) == 0 {
				continue
			}
			result := executeFunctionBody(node.Else, env, diags, opts, ctx)
			if result.Returned || result.Break || result.Continue {
				return result
			}
			last = result.Value
		case ast.FuncForStmt:
			result := executeFuncForStmt(node, env, diags, opts, ctx)
			if result.Returned || result.Break || result.Continue {
				return result
			}
			last = result.Value
		case ast.FuncWhileStmt:
			result := executeFuncWhileStmt(node, env, diags, opts, ctx)
			if result.Returned || result.Break || result.Continue {
				return result
			}
			last = result.Value
		}
	}
	return functionResult{Value: last}
}

func executeFuncForStmt(stmt ast.FuncForStmt, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) functionResult {
	iterable := evalExprWithCtx(stmt.Iterable, env, diags, opts, ctx)
	items, ok := IterableElements(iterable, astExprSpan(stmt.Iterable), diags)
	if !ok {
		return functionResult{Value: Null()}
	}
	last := Null()
	for i, item := range items {
		if i >= MaxLoopIterations {
			diags.AddError(diag.CodeE106, "loop exceeded 100000 iterations", stmt.Span, "check the loop condition or iterable size")
			return functionResult{Value: last}
		}
		if ctx != nil && ctx.frame != nil && stmt.Target != "" {
			ctx.frame.AssignLocal(stmt.Target, item, stmt.Span)
		}
		result := executeFunctionBody(stmt.Body, env, diags, opts, ctx)
		if result.Returned {
			return result
		}
		if result.Break {
			return functionResult{Value: result.Value}
		}
		last = result.Value
		if result.Continue {
			continue
		}
	}
	return functionResult{Value: last}
}

func executeFuncWhileStmt(stmt ast.FuncWhileStmt, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) functionResult {
	last := Null()
	for i := 0; ; i++ {
		if i >= MaxLoopIterations {
			diags.AddError(diag.CodeE106, "loop exceeded 100000 iterations", stmt.Span, "check the while condition")
			return functionResult{Value: last}
		}
		cond, ok := evalBoolConditionWithCtx("while", stmt.Cond, env, diags, opts, ctx)
		if !ok || !cond {
			return functionResult{Value: last}
		}
		result := executeFunctionBody(stmt.Body, env, diags, opts, ctx)
		if result.Returned {
			return result
		}
		if result.Break {
			return functionResult{Value: result.Value}
		}
		last = result.Value
		if result.Continue {
			continue
		}
	}
}

func astExprSpan(expr ast.Expr) diag.Span {
	if expr == nil {
		return diag.Span{}
	}
	return expr.GetSpan()
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
