package parser

import "testing"

func TestIsAllowedStepOptionKey(t *testing.T) {
	if !isAllowedStepOptionKey("max_async") {
		t.Fatalf("expected max_async to be allowed")
	}
	if !isAllowedStepOptionKey("procs") {
		t.Fatalf("expected procs to be allowed")
	}
	if !isAllowedStepOptionKey("iterations") {
		t.Fatalf("expected iterations to be allowed")
	}
	if isAllowedStepOptionKey("iterattions") {
		t.Fatalf("expected iterattions to be rejected")
	}
}

func TestAllowedStepOptionKeysHint(t *testing.T) {
	got := allowedStepOptionKeysHint()
	want := "max_async, procs and iterations"
	if got != want {
		t.Fatalf("unexpected hint text: got=%q want=%q", got, want)
	}
}
