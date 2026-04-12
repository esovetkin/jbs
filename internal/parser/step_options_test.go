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

func TestAllowedStepOptionKeysHintBranchCases(t *testing.T) {
	orig := append([]string(nil), stepOptionKeys...)
	t.Cleanup(func() {
		stepOptionKeys = orig
	})

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "empty", keys: []string{}, want: ""},
		{name: "single", keys: []string{"max_async"}, want: "max_async"},
		{name: "double", keys: []string{"max_async", "procs"}, want: "max_async and procs"},
		{name: "many", keys: []string{"a", "b", "c"}, want: "a, b and c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stepOptionKeys = append([]string(nil), tt.keys...)
			if got := allowedStepOptionKeysHint(); got != tt.want {
				t.Fatalf("allowedStepOptionKeysHint()=%q, want %q", got, tt.want)
			}
		})
	}
}
