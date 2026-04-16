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
)

//go:embed help_*.md
var pageFS embed.FS

var topicToFile = map[string]string{
	"analyse":   "help_analyse.md",
	"do":        "help_do.md",
	"functions": "help_functions.md",
	"globals":   "help_globals.md",
	"submit":    "help_submit.md",
	"use":       "help_use.md",
}

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
