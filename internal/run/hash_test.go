package run

import "testing"

func TestSourceBundleHashStableAndIncludesImports(t *testing.T) {
	a := map[string]string{
		"entry": "x = 1",
		"lib":   "y = 2",
	}
	b := map[string]string{
		"lib":   "y = 2",
		"entry": "x = 1",
	}
	if SourceBundleHash(a) != SourceBundleHash(b) {
		t.Fatalf("expected hash to be stable across map order")
	}
	b["lib"] = "y = 3"
	if SourceBundleHash(a) == SourceBundleHash(b) {
		t.Fatalf("expected imported source change to affect hash")
	}
}
