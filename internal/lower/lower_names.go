// define synthetic name generation used during lowering
//
// defines stable prefixes/contracts for generated parameter/pattern
// helper names, sanitizes source identifiers for YAML-safe/internal
// use, and provides short-name collision-safe builders
package lower

import (
	"fmt"
	"strings"
	"unicode"
)

func (ctx *lowerContext) uniqueName(base string) string {
	if _, exists := ctx.names[base]; !exists {
		ctx.names[base] = struct{}{}
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if _, exists := ctx.names[candidate]; !exists {
			ctx.names[candidate] = struct{}{}
			return candidate
		}
	}
}

func sanitize(name string) string {
	if name == "" {
		return "x"
	}
	b := strings.Builder{}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func subsetEmittedName(v subsetVarSpec) string {
	if v.Emitted != "" {
		return v.Emitted
	}
	return v.Visible
}

func applyEmittedNames(params []Parameter, emittedByVisible map[string]string) []Parameter {
	if len(params) == 0 || len(emittedByVisible) == 0 {
		return params
	}
	out := make([]Parameter, len(params))
	copy(out, params)
	for i := range out {
		if alias, ok := emittedByVisible[out[i].Name]; ok && alias != "" {
			out[i].Name = alias
		}
	}
	return out
}

func indexVariableName(context string) string {
	return shortParamIndexName(context)
}

// Synthetic naming contract: _ji_<ctx>, _js__<step>__<source>__<vars>, _ji__<step>__<source>__<vars>, _jr__<step>__<source>__<vars>, _jp__<group>_<pattern>__<step>__<alias>.
func shortParamIndexName(context string) string {
	name := sanitize(context)
	if name == "" {
		name = "set"
	}
	return "_ji_" + name
}

func shortSubsetBaseName(step, source string, vars []string) string {
	return "_js__" + sanitize(step) + "__" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_"))
}

func shortSubsetIndexName(step, source string, vars []string) string {
	return "_ji__" + sanitize(step) + "__" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_"))
}

func shortSubsetRowsName(step, source string, vars []string) string {
	return "_jr__" + sanitize(step) + "__" + sanitize(source) + "__" + sanitize(strings.Join(vars, "_"))
}

func shortPatternAliasName(group, pattern, step, alias string) string {
	return "_jp__" + sanitize(group) + "_" + sanitize(pattern) +
		"__" + sanitize(step) + "__" + sanitize(alias)
}
