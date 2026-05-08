package run

import "testing"

func TestSourceBundleHashStableAcrossMapOrderAndContentSensitive(t *testing.T) {
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

func TestSourceBundleHashIncludesLabels(t *testing.T) {
	a := map[string]string{
		"/real/input.jbs": "x = 1\n",
	}
	b := map[string]string{
		"/link/input.jbs": "x = 1\n",
	}
	if SourceBundleHash(a) == SourceBundleHash(b) {
		t.Fatalf("expected source label change to affect hash")
	}
}
