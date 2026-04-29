package helpdocs

import (
	"strings"
	"testing"
)

func TestTopicsResolveToNonEmptyPages(t *testing.T) {
	for _, topic := range Topics() {
		text, err := Page(topic)
		if err != nil {
			t.Fatalf("topic %q did not resolve: %v", topic, err)
		}
		if text == "" {
			t.Fatalf("topic %q returned empty page", topic)
		}
	}
}

func TestUnknownTopicReturnsError(t *testing.T) {
	if _, err := Page("template"); err == nil {
		t.Fatalf("expected unknown topic to return error")
	}
}

func TestGlobalsHelpMentionsTopLevelAssignments(t *testing.T) {
	text, err := Page("globals")
	if err != nil {
		t.Fatalf("globals topic did not resolve: %v", err)
	}
	if !strings.Contains(text, "top-level assignments") {
		t.Fatalf("expected globals help to describe top-level assignments, got:\n%s", text)
	}
	if !strings.Contains(text, "table(") {
		t.Fatalf("expected globals help to use table() examples, got:\n%s", text)
	}
}

func TestFunctionsHelpMentionsExplicitTableOperations(t *testing.T) {
	text, err := Page("functions")
	if err != nil {
		t.Fatalf("functions topic did not resolve: %v", err)
	}
	if !strings.Contains(text, "table(") || !strings.Contains(text, "product(") || !strings.Contains(text, "select(") {
		t.Fatalf("expected functions help to describe the explicit table API, got:\n%s", text)
	}
}
