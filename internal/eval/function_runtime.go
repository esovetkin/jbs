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

	BuiltinName string
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

func (fn *FunctionValue) isBuiltin() bool {
	return fn != nil && fn.BuiltinName != ""
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
			if ctx.recursionLimitHit() {
				return defaults
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
	isBound := func(name string, bound map[string]struct{}) bool {
		_, ok := bound[name]
		return ok
	}

	var walkExprBound func(ast.Expr, map[string]struct{}) bool
	var walkBodyBound func([]ast.FuncBodyStmt, map[string]struct{}) bool

	callbacksForBound := func(bound map[string]struct{}, found *bool) ast.WalkCallbacks {
		var callbacks ast.WalkCallbacks
		callbacks.Expr = func(node ast.Expr) ast.WalkAction {
			switch n := node.(type) {
			case ast.IdentExpr:
				if _, target := names[n.Name]; target && !isBound(n.Name, bound) {
					*found = true
					return ast.WalkStop
				}
			case ast.QualifiedIdentExpr:
				if _, target := names[n.Namespace]; target && !isBound(n.Namespace, bound) {
					*found = true
					return ast.WalkStop
				}
			case ast.FunctionExpr:
				nextBound := cloneNameSet(bound)
				for _, param := range n.Params {
					if walkExprBound(param.Default, nextBound) {
						*found = true
						return ast.WalkStop
					}
					if param.Name != "" {
						nextBound[param.Name] = struct{}{}
					}
				}
				collectFunctionLocalNames(n.Body, nextBound)
				if walkBodyBound(n.Body, nextBound) {
					*found = true
					return ast.WalkStop
				}
				return ast.WalkSkipChildren
			}
			return ast.WalkContinue
		}
		callbacks.FuncBodyStmt = func(stmt ast.FuncBodyStmt) ast.WalkAction {
			node, ok := stmt.(ast.FuncForStmt)
			if !ok {
				return ast.WalkContinue
			}
			if walkExprBound(node.Iterable, bound) {
				*found = true
				return ast.WalkStop
			}
			nextBound := cloneNameSet(bound)
			if node.Target != "" {
				nextBound[node.Target] = struct{}{}
			}
			if walkBodyBound(node.Body, nextBound) {
				*found = true
				return ast.WalkStop
			}
			return ast.WalkSkipChildren
		}
		return callbacks
	}

	walkExprBound = func(expr ast.Expr, bound map[string]struct{}) bool {
		found := false
		ast.WalkExpr(expr, callbacksForBound(bound, &found))
		return found
	}
	walkBodyBound = func(body []ast.FuncBodyStmt, bound map[string]struct{}) bool {
		found := false
		ast.WalkFuncBody(body, callbacksForBound(bound, &found))
		return found
	}
	return walkExprBound(expr, nil)
}

func collectFunctionLocalNames(body []ast.FuncBodyStmt, out map[string]struct{}) {
	ast.WalkFuncBody(body, ast.WalkCallbacks{
		Expr: func(ast.Expr) ast.WalkAction {
			return ast.WalkSkipChildren
		},
		FuncBodyStmt: func(stmt ast.FuncBodyStmt) ast.WalkAction {
			switch node := stmt.(type) {
			case ast.LocalAssignStmt:
				if node.Name != "" {
					out[node.Name] = struct{}{}
				}
			case ast.FuncForStmt:
				if node.Target != "" {
					out[node.Target] = struct{}{}
				}
			}
			return ast.WalkContinue
		},
	})
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
		return newEvalCtx(frame)
	}
	next := *ctx
	next.frame = frame
	if next.overflowWarned == nil {
		next.overflowWarned = make(map[string]struct{})
	}
	if next.abort == nil {
		next.abort = &evalAbortState{}
	}
	return &next
}

func checkFunctionCallDepth(ctx *evalCtx, opts ExprOptions, at diag.Span, diags *diag.Diagnostics) bool {
	if ctx == nil {
		ctx = newEvalCtx(nil)
	}
	if ctx.recursionLimitHit() {
		return false
	}
	limit := functionCallDepthLimit(opts)
	if ctx.callDepth < limit {
		return true
	}
	diags.AddError(
		diag.CodeE106,
		fmt.Sprintf("maximum function recursion depth of %d reached", limit),
		at,
		"check for a missing base case or rewrite the function iteratively",
	)
	ctx.markRecursionLimitHit()
	return false
}

func executeFunctionCall(fn *FunctionValue, rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	args, ok := evalCallValueArgs(rawArgs, env, diags, opts, ctx)
	if !ok {
		return Null()
	}
	return executeFunctionCallValues(fn, args, env, at, diags, opts, ctx)
}

func evalCallValueArgs(rawArgs []ast.CallArg, env map[string]Value, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) ([]CallValueArg, bool) {
	args := make([]CallValueArg, 0, len(rawArgs))
	for _, arg := range rawArgs {
		value := evalExprWithCtx(arg.Expr, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return nil, false
		}
		switch arg.EffectiveKind() {
		case ast.CallArgPositionalSpread:
			items, ok := callSpreadItems(value, arg.Span, diags)
			if !ok {
				return nil, false
			}
			for _, item := range items {
				args = append(args, CallValueArg{Value: item, Span: arg.Span})
			}
		case ast.CallArgKeywordSpread:
			entries, ok := callKeywordEntries(value, arg.Span, diags)
			if !ok {
				return nil, false
			}
			args = append(args, entries...)
		case ast.CallArgNamed:
			args = append(args, CallValueArg{Name: arg.Name, Value: value, Span: arg.Span})
		default:
			args = append(args, CallValueArg{Value: value, Span: arg.Span})
		}
	}
	return args, true
}

func callSpreadItems(value Value, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	switch value.Kind {
	case KindList, KindTuple:
		return CloneValues(value.L), true
	default:
		diags.AddError(diag.CodeE106, "* call expansion expects a list or tuple", at, "use *list_value or *tuple_value")
		return nil, false
	}
}

func callKeywordEntries(value Value, at diag.Span, diags *diag.Diagnostics) ([]CallValueArg, bool) {
	if value.Kind != KindDict || value.D == nil {
		diags.AddError(diag.CodeE106, "** call expansion expects a dictionary", at, "use **dict_value")
		return nil, false
	}
	out := make([]CallValueArg, 0, len(value.D.Order))
	for _, key := range value.D.Order {
		if key.Kind != DictKeyString || key.S == "" {
			diags.AddError(diag.CodeE106, "** call expansion keys must be non-empty strings", at, "use string keys such as name or mode")
			return nil, false
		}
		value, ok := value.D.Entries[key]
		if !ok {
			continue
		}
		out = append(out, CallValueArg{Name: key.S, Value: CloneValue(value), Span: at})
	}
	return out, true
}

func executeFunctionCallValues(fn *FunctionValue, args []CallValueArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value {
	if fn == nil {
		diags.AddError(diag.CodeE199, "expression is not callable", at, "call a function value or supported builtin")
		return Null()
	}
	if ctx == nil {
		ctx = newEvalCtx(nil)
	}
	if fn.isBuiltin() {
		return evalBuiltinValueCall(fn.BuiltinName, args, env, at, diags, opts, ctx)
	}
	if !checkFunctionCallDepth(ctx, opts, at, diags) {
		return Null()
	}
	callFrame := NewChildFrame(fn.Capture)
	callCtx := ctx.enterFunctionCall(callFrame)
	callOpts := opts
	if fn.Files != nil {
		callOpts.Files = cloneFileAccess(fn.Files)
	}
	callOpts.Names = callNameCatalog(fn.Names, callFrame)

	binding, ok := bindFunctionArguments(fn, args, at, diags)
	if !ok {
		return Null()
	}
	for i, param := range fn.Params {
		switch param.Kind {
		case ast.FuncParamArgs:
			callFrame.AssignLocal(param.Name, List(CloneValues(binding.Args)), param.Span)
			continue
		case ast.FuncParamKwargs:
			callFrame.AssignLocal(param.Name, DictValue(binding.Kwargs), param.Span)
			continue
		}
		if bound, exists := binding.Fixed[i]; exists {
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
			value = evalExprWithCtx(param.Default, env, diags, callOpts, callCtx)
			if callCtx.recursionLimitHit() {
				return Null()
			}
		}
		callFrame.AssignLocal(param.Name, value, param.Span)
	}
	predeclareFunctionLocals(fn.Body, callFrame)
	callOpts.Names = callNameCatalog(fn.Names, callFrame)
	result := executeFunctionBody(fn.Body, env, diags, callOpts, callCtx)
	if callCtx.recursionLimitHit() {
		return Null()
	}
	if result.Break || result.Continue {
		diags.AddError(diag.CodeE080, "'break' and 'continue' are only allowed inside loops", result.Span, "move the statement into a for/while body")
		return Null()
	}
	return result.Value
}

type functionArgBinding struct {
	Fixed  map[int]Value
	Args   []Value
	Kwargs []DictEntry
}

func bindFunctionArguments(fn *FunctionValue, args []CallValueArg, at diag.Span, diags *diag.Diagnostics) (functionArgBinding, bool) {
	binding := functionArgBinding{Fixed: make(map[int]Value, len(args))}
	paramIndex := make(map[string]int, len(fn.Params))
	normalOrder := make([]int, 0, len(fn.Params))
	hasArgs := false
	hasKwargs := false
	for i, param := range fn.Params {
		if param.Name == "" {
			continue
		}
		switch param.Kind {
		case ast.FuncParamArgs:
			hasArgs = true
		case ast.FuncParamKwargs:
			hasKwargs = true
		default:
			paramIndex[param.Name] = i
			normalOrder = append(normalOrder, i)
		}
	}
	namedSeen := false
	nextPositional := 0
	seenNamedArgs := make(map[string]diag.Span)
	for _, arg := range args {
		if arg.Name == "" {
			if namedSeen {
				diags.AddError(diag.CodeE106, "positional arguments cannot follow named arguments", arg.Span, "pass positional arguments before any named arguments")
				return binding, false
			}
			if nextPositional < len(normalOrder) {
				binding.Fixed[normalOrder[nextPositional]] = arg.Value
				nextPositional++
				continue
			}
			if hasArgs {
				binding.Args = append(binding.Args, CloneValue(arg.Value))
				continue
			}
			if nextPositional >= len(normalOrder) {
				diags.AddError(diag.CodeE106, "too many positional arguments", arg.Span, "remove extra arguments or add parameters")
				return binding, false
			}
			continue
		}
		namedSeen = true
		if prev, exists := seenNamedArgs[arg.Name]; exists {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("argument '%s' received multiple values", arg.Name),
				arg.Span,
				"pass each argument at most once",
				diag.RelatedSpan{Message: "previous value", Span: prev},
			)
			return binding, false
		}
		seenNamedArgs[arg.Name] = arg.Span
		idx, ok := paramIndex[arg.Name]
		if !ok {
			if hasKwargs {
				binding.Kwargs = append(binding.Kwargs, DictEntry{
					Key:   DictKey{Kind: DictKeyString, S: arg.Name},
					Value: CloneValue(arg.Value),
				})
				continue
			}
			diags.AddError(diag.CodeE106, fmt.Sprintf("unknown named argument '%s'", arg.Name), arg.Span, "use one of the declared parameter names")
			return binding, false
		}
		if _, exists := binding.Fixed[idx]; exists {
			diags.AddError(diag.CodeE106, fmt.Sprintf("parameter '%s' received multiple values", arg.Name), arg.Span, "pass each parameter at most once")
			return binding, false
		}
		binding.Fixed[idx] = arg.Value
	}
	return binding, true
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
			for _, branch := range node.Elifs {
				predeclareFunctionLocals(branch.Body, frame)
			}
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
		if ctx.recursionLimitHit() {
			return functionResult{Value: last}
		}
		switch node := stmt.(type) {
		case ast.LocalAssignStmt:
			executeLocalAssign(node, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return functionResult{Value: last}
			}
		case ast.ReturnStmt:
			if node.Expr == nil {
				return functionResult{Value: Null(), Returned: true}
			}
			value := evalExprWithCtx(node.Expr, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return functionResult{Value: Null(), Returned: true}
			}
			return functionResult{
				Value:    value,
				Returned: true,
			}
		case ast.ExprStmt:
			last = evalExprWithCtx(node.Expr, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return functionResult{Value: last}
			}
		case ast.BreakStmt:
			return functionResult{Value: last, Break: true, Span: node.Span}
		case ast.ContinueStmt:
			return functionResult{Value: last, Continue: true, Span: node.Span}
		case ast.FuncIfStmt:
			cond, ok := evalBoolConditionWithCtx("if", node.Cond, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return functionResult{Value: last}
			}
			if !ok {
				continue
			}
			if cond {
				result := executeFunctionBody(node.Then, env, diags, opts, ctx)
				if ctx.recursionLimitHit() {
					return result
				}
				if result.Returned || result.Break || result.Continue {
					return result
				}
				last = result.Value
				continue
			}
			selected := false
			for _, branch := range node.Elifs {
				branchCond, ok := evalBoolConditionWithCtx("elif", branch.Cond, env, diags, opts, ctx)
				if ctx.recursionLimitHit() {
					return functionResult{Value: last}
				}
				if !ok {
					selected = true
					break
				}
				if !branchCond {
					continue
				}
				result := executeFunctionBody(branch.Body, env, diags, opts, ctx)
				if ctx.recursionLimitHit() {
					return result
				}
				if result.Returned || result.Break || result.Continue {
					return result
				}
				last = result.Value
				selected = true
				break
			}
			if selected {
				continue
			}
			if len(node.Else) == 0 {
				continue
			}
			result := executeFunctionBody(node.Else, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return result
			}
			if result.Returned || result.Break || result.Continue {
				return result
			}
			last = result.Value
		case ast.FuncForStmt:
			result := executeFuncForStmt(node, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return result
			}
			if result.Returned || result.Break || result.Continue {
				return result
			}
			last = result.Value
		case ast.FuncWhileStmt:
			result := executeFuncWhileStmt(node, env, diags, opts, ctx)
			if ctx.recursionLimitHit() {
				return result
			}
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
	if ctx.recursionLimitHit() {
		return functionResult{Value: Null()}
	}
	items, ok := IterableElements(iterable, astExprSpan(stmt.Iterable), diags)
	if !ok {
		return functionResult{Value: Null()}
	}
	last := Null()
	for i, item := range items {
		if ctx.recursionLimitHit() {
			return functionResult{Value: last}
		}
		if i >= MaxLoopIterations {
			diags.AddError(diag.CodeE106, LoopLimitExceededMessage(), stmt.Span, "check the loop condition or iterable size")
			return functionResult{Value: last}
		}
		if ctx != nil && ctx.frame != nil && stmt.Target != "" {
			ctx.frame.AssignLocal(stmt.Target, item, stmt.Span)
		}
		result := executeFunctionBody(stmt.Body, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return result
		}
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
		if ctx.recursionLimitHit() {
			return functionResult{Value: last}
		}
		if i >= MaxLoopIterations {
			diags.AddError(diag.CodeE106, LoopLimitExceededMessage(), stmt.Span, "check the while condition")
			return functionResult{Value: last}
		}
		cond, ok := evalBoolConditionWithCtx("while", stmt.Cond, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return functionResult{Value: last}
		}
		if !ok || !cond {
			return functionResult{Value: last}
		}
		result := executeFunctionBody(stmt.Body, env, diags, opts, ctx)
		if ctx.recursionLimitHit() {
			return result
		}
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
	if ctx.recursionLimitHit() {
		return
	}
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
