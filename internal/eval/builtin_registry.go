package eval

import (
	"fmt"
	"sync"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type builtinCallContext struct {
	Name  string
	Args  []CallValueArg
	Env   map[string]Value
	At    diag.Span
	Diags *diag.Diagnostics
	Opts  ExprOptions
	Ctx   *evalCtx
}

type builtinDirectArgMode uint8

const (
	builtinEvalArgs builtinDirectArgMode = iota
	builtinRawArgs
)

type builtinSpec struct {
	Name            string
	DirectMode      builtinDirectArgMode
	AllowedContexts map[EvalContext]struct{}
	Value           func(builtinCallContext) Value
	Direct          func(rawArgs []ast.CallArg, env map[string]Value, at diag.Span, diags *diag.Diagnostics, opts ExprOptions, ctx *evalCtx) Value
}

var builtinBindingAssignOnly = map[EvalContext]struct{}{
	EvalCtxBindingAssign: {},
}

var builtinRegistry struct {
	once  sync.Once
	specs map[string]builtinSpec
}

func builtinSpecs() map[string]builtinSpec {
	builtinRegistry.once.Do(initBuiltinRegistry)
	return builtinRegistry.specs
}

func initBuiltinRegistry() {
	builtinRegistry.specs = map[string]builtinSpec{
		"all": {
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "values", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalAllAnyCall(c.Name, values, c.At, c.Diags)
			},
		},
		"any": {
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "values", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalAllAnyCall(c.Name, values, c.At, c.Diags)
			},
		},
		"bool": {
			Value: evalConversionBuiltin,
		},
		"delete": {
			DirectMode: builtinRawArgs,
			Direct:     evalDeleteCall,
			Value: func(c builtinCallContext) Value {
				return evalDeleteValueCall(c.Args, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"dict": {
			Value: func(c builtinCallContext) Value {
				return evalDictValueCall(c.Args, c.At, c.Diags)
			},
		},
		"duplicated": {
			Value: func(c builtinCallContext) Value {
				return evalUniqueDuplicatedValueCall(c.Name, c.Args, c.At, c.Diags)
			},
		},
		"env": {
			Value: func(c builtinCallContext) Value {
				return evalEnvValueCall(c.Args, c.At, c.Diags, c.Opts)
			},
		},
		"filter": {
			Value: func(c builtinCallContext) Value {
				return evalFilterValueCall(c.Args, c.Env, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"float": {
			Value: evalConversionBuiltin,
		},
		"get": {
			Value: func(c builtinCallContext) Value {
				return evalDictGetValueCall(c.Args, c.At, c.Diags)
			},
		},
		"head": {
			Value: func(c builtinCallContext) Value {
				return evalHeadTailValueCall(c.Name, c.Args, c.At, c.Diags)
			},
		},
		"int": {
			Value: evalConversionBuiltin,
		},
		"len": {
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "value", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalLenCall(values, c.At, c.Diags)
			},
		},
		"list": {
			AllowedContexts: nil,
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "value", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalListCall(values, c.At, c.Diags)
			},
		},
		"map": {
			Value: func(c builtinCallContext) Value {
				return evalMapValueCall(c.Args, c.Env, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"names": {
			DirectMode: builtinRawArgs,
			Direct:     evalNamesDirectCall,
			Value: func(c builtinCallContext) Value {
				return evalNamesValueCall(c.Args, c.At, c.Diags, c.Opts)
			},
		},
		"order": {
			Value: func(c builtinCallContext) Value {
				return evalOrderValueCall(c.Args, c.Env, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"print": {
			Value: func(c builtinCallContext) Value {
				bound, ok := bindPrintArgs(c.Args, c.Diags)
				if !ok {
					return Null()
				}
				return evalPrintCall(bound.Values, bound.Options, c.At, c.Opts)
			},
		},
		"prod": {
			Value: func(c builtinCallContext) Value {
				return evalFoldOperatorValueCall(c.Name, "*", c.Args, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"range": {
			AllowedContexts: builtinBindingAssignOnly,
			Value: func(c builtinCallContext) Value {
				values, ok := bindRangeBuiltinValues(c.Args, c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalRangeCall(values, c.At, c.Diags)
			},
		},
		"rbind": {
			Value: func(c builtinCallContext) Value {
				return evalRbindValueCall(c.Args, c.At, c.Diags)
			},
		},
		"read_csv": {
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "path", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalReadCSVCall(values, c.At, c.Diags, c.Opts)
			},
		},
		"reduce": {
			Value: func(c builtinCallContext) Value {
				return evalReduceValueCall(c.Args, c.Env, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"rename": {
			Value: func(c builtinCallContext) Value {
				return evalRenameValueCall(c.Args, c.At, c.Diags)
			},
		},
		"rev": {
			AllowedContexts: builtinBindingAssignOnly,
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "values", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalRevCall(values, c.At, c.Diags)
			},
		},
		"rows": {
			Value: func(c builtinCallContext) Value {
				return evalRowsValueCall(c.Args, c.At, c.Diags)
			},
		},
		"sample": {
			Value: func(c builtinCallContext) Value {
				return evalSampleValueCall(c.Args, c.At, c.Diags, c.Opts)
			},
		},
		"setseed": {
			Value: func(c builtinCallContext) Value {
				return evalSetSeedValueCall(c.Args, c.At, c.Diags, c.Opts)
			},
		},
		"shell": {
			Value: func(c builtinCallContext) Value {
				return evalShellValueCall(c.Args, c.Env, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"sort": {
			Value: func(c builtinCallContext) Value {
				return evalSortValueCall(c.Args, c.Env, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"str": {
			Value: evalConversionBuiltin,
		},
		"sum": {
			Value: func(c builtinCallContext) Value {
				return evalFoldOperatorValueCall(c.Name, "+", c.Args, c.At, c.Diags, c.Opts, c.Ctx)
			},
		},
		"table": {
			Value: func(c builtinCallContext) Value {
				return evalTableValueCall(c.Args, c.At, c.Diags)
			},
		},
		"t": {
			Value: func(c builtinCallContext) Value {
				return evalTableValueCall(c.Args, c.At, c.Diags)
			},
		},
		"tail": {
			Value: func(c builtinCallContext) Value {
				return evalHeadTailValueCall(c.Name, c.Args, c.At, c.Diags)
			},
		},
		"tuple": {
			Value: func(c builtinCallContext) Value {
				values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "value", c.At, c.Diags)
				if !ok {
					return Null()
				}
				return evalTupleCall(values, c.At, c.Diags)
			},
		},
		"unique": {
			Value: func(c builtinCallContext) Value {
				return evalUniqueDuplicatedValueCall(c.Name, c.Args, c.At, c.Diags)
			},
		},
		"update": {
			Value: func(c builtinCallContext) Value {
				return evalUpdateValueCall(c.Args, c.At, c.Diags)
			},
		},
	}
}

func lookupBuiltinSpec(name string) (builtinSpec, bool) {
	spec, ok := builtinSpecs()[name]
	if !ok {
		return builtinSpec{}, false
	}
	if spec.Name == "" {
		spec.Name = name
	}
	return spec, true
}

func callBuiltinSpec(spec builtinSpec, c builtinCallContext) Value {
	if spec.Name == "" {
		spec.Name = c.Name
	}
	if c.Name == "" {
		c.Name = spec.Name
	}
	if spec.Value == nil {
		c.Diags.AddError(diag.CodeE199, fmt.Sprintf("unknown function '%s'", c.Name), c.At, "use a supported builtin or define a function value before calling it")
		return Null()
	}
	return spec.Value(c)
}

func checkBuiltinContext(spec builtinSpec, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) bool {
	if len(spec.AllowedContexts) == 0 {
		return true
	}
	if _, ok := spec.AllowedContexts[opts.Context]; ok {
		return true
	}
	diags.AddError(
		diag.CodeE199,
		fmt.Sprintf("function '%s' is only allowed in top-level global assignments", spec.Name),
		at,
		"use this function only in top-level global assignment expressions",
	)
	return false
}

func evalConversionBuiltin(c builtinCallContext) Value {
	values, ok := bindUnaryBuiltinValues(c.Name, c.Args, "value", c.At, c.Diags)
	if !ok {
		return Null()
	}
	return evalUnaryConvertCall(c.Name, values, c.At, c.Diags)
}

func bindUnaryBuiltinValues(name string, args []CallValueArg, param string, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	bound, ok := bindBuiltinArgs(name, args, builtinSignature{Name: name, Params: []builtinParam{{Name: param, Required: true}}}, at, diags)
	if !ok {
		return nil, false
	}
	return []Value{bound.ByName[param].Value}, true
}
