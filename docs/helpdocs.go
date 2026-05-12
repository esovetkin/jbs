// embed existing markdown pages within compiled binary
//
// we use the `embed` package, and `//go:embed help_*.md` makes go
// embed all the matching markdown files
package helpdocs

import (
	"embed"
	"fmt"
	"maps"
	"slices"
	"strings"
)

//go:embed help_*.md function_help/*.md
var pageFS embed.FS

var topicToFile = map[string]string{
	"analyse":    "help_analyse.md",
	"archive":    "help_archive.md",
	"continue":   "help_continue.md",
	"do":         "help_do.md",
	"fwait":      "help_fwait.md",
	"globals":    "help_globals.md",
	"ls-analyse": "help_ls_analyse.md",
	"param":      "help_param.md",
	"repl":       "help_repl.md",
	"status":     "help_status.md",
	"tree":       "help_tree.md",
	"use":        "help_use.md",
}

type FunctionHelpEntry struct {
	Name    string
	File    string
	AliasOf string
}

var functionHelpEntries = []FunctionHelpEntry{
	{Name: "all", File: "function_help/all.md"},
	{Name: "any", File: "function_help/any.md"},
	{Name: "bool", File: "function_help/bool.md"},
	{Name: "delete", File: "function_help/delete.md"},
	{Name: "dict", File: "function_help/dict.md"},
	{Name: "env", File: "function_help/env.md"},
	{Name: "filter", File: "function_help/filter.md"},
	{Name: "float", File: "function_help/float.md"},
	{Name: "get", File: "function_help/get.md"},
	{Name: "int", File: "function_help/int.md"},
	{Name: "len", File: "function_help/len.md"},
	{Name: "list", File: "function_help/list.md"},
	{Name: "map", File: "function_help/map.md"},
	{Name: "names", File: "function_help/names.md"},
	{Name: "product", File: "function_help/product.md"},
	{Name: "print", File: "function_help/print.md"},
	{Name: "range", File: "function_help/range.md"},
	{Name: "read_csv", File: "function_help/read_csv.md"},
	{Name: "reduce", File: "function_help/reduce.md"},
	{Name: "rev", File: "function_help/rev.md"},
	{Name: "rows", File: "function_help/rows.md"},
	{Name: "select", File: "function_help/select.md"},
	{Name: "shell", File: "function_help/shell.md"},
	{Name: "str", File: "function_help/str.md"},
	{Name: "table", File: "function_help/table.md"},
	{Name: "t", File: "function_help/table.md", AliasOf: "table"},
	{Name: "tuple", File: "function_help/tuple.md"},
	{Name: "update", File: "function_help/update.md"},
	{Name: "zip", File: "function_help/zip.md"},
}

var functionHelpByName = buildFunctionHelpByName()

func Page(topic string) (string, error) {
	name, ok := topicToFile[topic]
	if !ok {
		return "", fmt.Errorf("unknown help topic %q", topic)
	}
	data, err := pageFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded help page %q: %w", topic, err)
	}
	return string(data), nil
}

func Topics() []string {
	return slices.Sorted(maps.Keys(topicToFile))
}

func FunctionPage(name string) (string, error) {
	entry, ok := functionHelpByName[name]
	if !ok {
		return "", fmt.Errorf("unknown internal function %q", name)
	}
	data, err := pageFS.ReadFile(entry.File)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded function help %q: %w", name, err)
	}
	page := string(data)
	if entry.AliasOf != "" {
		return fmt.Sprintf("# `%s(...)`\n\nAlias of `%s(...)`.\n\n%s", entry.Name, entry.AliasOf, page), nil
	}
	return page, nil
}

func FunctionNames() []string {
	return slices.Sorted(maps.Keys(functionHelpByName))
}

func FunctionNamesText() string {
	return strings.Join(FunctionNames(), ", ")
}

func buildFunctionHelpByName() map[string]FunctionHelpEntry {
	out := make(map[string]FunctionHelpEntry, len(functionHelpEntries))
	for _, entry := range functionHelpEntries {
		if entry.Name == "" {
			panic("empty function help name")
		}
		if _, exists := out[entry.Name]; exists {
			panic(fmt.Sprintf("duplicate function help name %q", entry.Name))
		}
		out[entry.Name] = entry
	}
	return out
}
