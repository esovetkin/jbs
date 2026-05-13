package eval

import (
	"os"
	"slices"
	"strings"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func currentEnviron(opts ExprOptions) []string {
	if opts.Environ != nil {
		return opts.Environ()
	}
	return os.Environ()
}

func environmentMap(opts ExprOptions) map[string]string {
	out := make(map[string]string)
	for _, item := range currentEnviron(opts) {
		name, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}

func environmentDict(opts ExprOptions) Value {
	values := environmentMap(opts)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)

	entries := make([]DictEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, DictEntry{
			Key:   DictKey{Kind: DictKeyString, S: name},
			Value: String(values[name]),
		})
	}
	return DictValue(entries)
}

func evalEnvValueCall(args []CallValueArg, at diag.Span, diags *diag.Diagnostics, opts ExprOptions) Value {
	if len(args) == 0 {
		return environmentDict(opts)
	}
	bound, ok := bindBuiltinArgs("env", args, builtinSignature{
		Name: "env",
		Params: []builtinParam{
			{Name: "name", Required: true},
			{Name: "default"},
		},
	}, at, diags)
	if !ok {
		return Null()
	}
	nameArg := bound.ByName["name"]
	if nameArg.Value.Kind != KindString {
		diags.AddError(diag.CodeE106, "env() variable name must be a string", nameArg.Span, `use env("NAME")`)
		return Null()
	}

	fallback := Null()
	if defaultArg, ok := bound.ByName["default"]; ok {
		fallback = defaultArg.Value
	}
	values := environmentMap(opts)
	if value, ok := values[nameArg.Value.S]; ok {
		return String(value)
	}
	return CloneValue(fallback)
}
