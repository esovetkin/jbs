package shared

import (
	"strings"
	"testing"
)

func TestListSortedAndContainsJSC(t *testing.T) {
	files, err := List()
	if err != nil {
		t.Fatalf("list embedded files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected at least one embedded file")
	}
	found := false
	for i := range files {
		if i > 0 && files[i-1] > files[i] {
			t.Fatalf("expected sorted embedded files, got %#v", files)
		}
		if files[i] == "jsc.jbs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected jsc.jbs in embedded files, got %#v", files)
	}
}

func TestReadSupportsNameWithAndWithoutExtension(t *testing.T) {
	a, err := Read("jsc")
	if err != nil {
		t.Fatalf("read jsc: %v", err)
	}
	b, err := Read("jsc.jbs")
	if err != nil {
		t.Fatalf("read jsc.jbs: %v", err)
	}
	if a != b {
		t.Fatalf("expected same content with/without extension")
	}
	if !strings.Contains(a, "systemname = shell(") {
		t.Fatalf("unexpected jsc content: %q", a)
	}
}

func TestReadUnknownReturnsError(t *testing.T) {
	if _, err := Read("does_not_exist"); err == nil {
		t.Fatalf("expected error for unknown embedded file")
	}
}
