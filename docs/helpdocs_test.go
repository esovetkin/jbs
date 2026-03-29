package helpdocs

import "testing"

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
