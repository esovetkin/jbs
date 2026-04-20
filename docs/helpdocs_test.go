package helpdocs

import (
	"regexp"
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

func TestCanonicalHelpPagesDoNotMentionLegacyBlocks(t *testing.T) {
	legacyRE := regexp.MustCompile(`\b(let|param)\b`)
	for _, topic := range []string{"globals", "do", "submit", "analyse", "use"} {
		text, err := Page(topic)
		if err != nil {
			t.Fatalf("topic %q did not resolve: %v", topic, err)
		}
		if legacyRE.MatchString(text) {
			t.Fatalf("topic %q still mentions legacy block syntax:\n%s", topic, text)
		}
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
}
