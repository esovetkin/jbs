package helpdocs

import (
	"embed"
	"fmt"
	"sort"
)

//go:embed help_*.md
var pageFS embed.FS

var topicToFile = map[string]string{
	"analyse":  "help_analyse.md",
	"do":       "help_do.md",
	"globals":  "help_globals.md",
	"let":      "help_let.md",
	"param":    "help_param.md",
	"submit":   "help_submit.md",
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
	topics := make([]string, 0, len(topicToFile))
	for topic := range topicToFile {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return topics
}
