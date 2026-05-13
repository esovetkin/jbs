package eval

import (
	"fmt"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type builtinParam struct {
	Name     string
	Required bool
}

type builtinSignature struct {
	Name          string
	Params        []builtinParam
	Varargs       string
	NamedVarargs  bool
	Kwargs        string
	AllowNoArgs   bool
	MinPositional int
}

type builtinArgs struct {
	ByName  map[string]CallValueArg
	Ordered []CallValueArg
	Varargs []CallValueArg
	Kwargs  []CallValueArg
}

func bindBuiltinArgs(name string, args []CallValueArg, sig builtinSignature, at diag.Span, diags *diag.Diagnostics) (builtinArgs, bool) {
	if sig.Name == "" {
		sig.Name = name
	}
	bound := builtinArgs{
		ByName: make(map[string]CallValueArg, len(sig.Params)),
	}
	paramIndex := make(map[string]int, len(sig.Params))
	for i, param := range sig.Params {
		paramIndex[param.Name] = i
	}
	posIndex := 0
	namedSeen := false
	seenNamed := make(map[string]diag.Span)
	for _, arg := range args {
		if arg.Name == "" {
			if namedSeen {
				diags.AddError(diag.CodeE106, "positional arguments cannot follow named arguments", arg.Span, "pass positional arguments before any named arguments")
				return bound, false
			}
			if posIndex < len(sig.Params) {
				param := sig.Params[posIndex]
				bound.ByName[param.Name] = arg
				bound.Ordered = append(bound.Ordered, arg)
				posIndex++
				continue
			}
			if sig.Varargs != "" {
				bound.Varargs = append(bound.Varargs, arg)
				continue
			}
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() received too many positional arguments", name), arg.Span, "remove extra arguments")
			return bound, false
		}
		namedSeen = true
		if prev, exists := seenNamed[arg.Name]; exists {
			diags.AddError(
				diag.CodeE106,
				fmt.Sprintf("argument '%s' received multiple values", arg.Name),
				arg.Span,
				"pass each argument at most once",
				diag.RelatedSpan{Message: "previous value", Span: prev},
			)
			return bound, false
		}
		seenNamed[arg.Name] = arg.Span
		if sig.NamedVarargs && arg.Name == sig.Varargs {
			items, ok := callSpreadItems(arg.Value, arg.Span, diags)
			if !ok {
				return bound, false
			}
			for _, item := range items {
				bound.Varargs = append(bound.Varargs, CallValueArg{Value: item, Span: arg.Span})
			}
			continue
		}
		if _, ok := paramIndex[arg.Name]; ok {
			if _, exists := bound.ByName[arg.Name]; exists {
				diags.AddError(diag.CodeE106, fmt.Sprintf("argument '%s' received multiple values", arg.Name), arg.Span, "pass each argument at most once")
				return bound, false
			}
			bound.ByName[arg.Name] = arg
			continue
		}
		if sig.Kwargs != "" {
			bound.Kwargs = append(bound.Kwargs, arg)
			continue
		}
		diags.AddError(diag.CodeE106, fmt.Sprintf("unknown named argument '%s' for %s()", arg.Name, name), arg.Span, "use one of: "+builtinAcceptedNames(sig))
		return bound, false
	}
	if len(args) == 0 && !sig.AllowNoArgs {
		diags.AddError(diag.CodeE106, fmt.Sprintf("%s() expects arguments", name), at, "pass the required arguments")
		return bound, false
	}
	if len(bound.Ordered)+len(bound.Varargs) < sig.MinPositional {
		diags.AddError(diag.CodeE106, fmt.Sprintf("%s() expects at least %d positional arguments", name, sig.MinPositional), at, "pass the required positional arguments")
		return bound, false
	}
	for _, param := range sig.Params {
		if !param.Required {
			continue
		}
		if _, ok := bound.ByName[param.Name]; !ok {
			diags.AddError(diag.CodeE106, fmt.Sprintf("%s() missing required argument '%s'", name, param.Name), at, "pass all required arguments")
			return bound, false
		}
	}
	return bound, true
}

func builtinAcceptedNames(sig builtinSignature) string {
	names := make([]string, 0, len(sig.Params)+2)
	for _, param := range sig.Params {
		names = append(names, param.Name)
	}
	if sig.Varargs != "" && sig.NamedVarargs {
		names = append(names, sig.Varargs)
	}
	if sig.Kwargs != "" {
		names = append(names, "**"+sig.Kwargs)
	}
	return strings.Join(names, ", ")
}

func callArgsToValues(args []CallValueArg) []Value {
	values := make([]Value, 0, len(args))
	for _, arg := range args {
		values = append(values, arg.Value)
	}
	return values
}

func kwargsDict(args []CallValueArg) Value {
	entries := make([]DictEntry, 0, len(args))
	for _, arg := range args {
		entries = append(entries, DictEntry{
			Key:   DictKey{Kind: DictKeyString, S: arg.Name},
			Value: arg.Value,
		})
	}
	return DictValue(entries)
}
