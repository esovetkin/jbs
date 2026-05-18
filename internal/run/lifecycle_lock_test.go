package run

import (
	"path/filepath"
	"testing"
)

func TestSuiteLifecycleLockRequests(t *testing.T) {
	t.Run("single suite deduplicates root", func(t *testing.T) {
		requests := suiteLifecycleLockRequests(runtimeSuitePlan{
			RootName: "bench",
			Plans:    []runtimePlan{{RootDir: "bench"}},
		}, false)
		if len(requests) != 1 {
			t.Fatalf("requests = %#v, want one request", requests)
		}
		if requests[0] != (rootLockRequest{Root: "bench"}) {
			t.Fatalf("request = %#v, want bench create request", requests[0])
		}
	})

	t.Run("configured suite orders top then components", func(t *testing.T) {
		requests := suiteLifecycleLockRequests(runtimeSuitePlan{
			RootName:   "bench",
			Configured: true,
			Plans: []runtimePlan{
				{RootDir: filepath.Join("bench", "z")},
				{RootDir: filepath.Join("bench", "a")},
			},
		}, true)
		want := []rootLockRequest{
			{Root: "bench", Existing: true},
			{Root: filepath.Join("bench", "a"), Existing: true},
			{Root: filepath.Join("bench", "z"), Existing: true},
		}
		if len(requests) != len(want) {
			t.Fatalf("requests = %#v, want %#v", requests, want)
		}
		for i := range want {
			if requests[i] != want[i] {
				t.Fatalf("request %d = %#v, want %#v", i, requests[i], want[i])
			}
		}
	})

	t.Run("normalizes and deduplicates paths", func(t *testing.T) {
		requests := suiteLifecycleLockRequests(runtimeSuitePlan{
			RootName: filepath.Join(".", "bench"),
			Plans: []runtimePlan{
				{RootDir: filepath.Join("bench", ".")},
				{RootDir: filepath.Join("bench", "small")},
				{RootDir: filepath.Join("bench", "small", "..", "small")},
			},
		}, false)
		want := []string{"bench", filepath.Join("bench", "small")}
		if len(requests) != len(want) {
			t.Fatalf("requests = %#v, want roots %#v", requests, want)
		}
		for i, root := range want {
			if requests[i].Root != root {
				t.Fatalf("request %d root = %q, want %q", i, requests[i].Root, root)
			}
			if requests[i].Existing {
				t.Fatalf("request %d Existing = true, want false", i)
			}
		}
	})

	t.Run("infers single manual suite root", func(t *testing.T) {
		requests := suiteLifecycleLockRequests(runtimeSuitePlan{
			Plans: []runtimePlan{{RootDir: "manual"}},
		}, true)
		if len(requests) != 1 || requests[0] != (rootLockRequest{Root: "manual", Existing: true}) {
			t.Fatalf("requests = %#v, want inferred manual root", requests)
		}
	})
}
