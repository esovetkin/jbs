package sema

import (
	"testing"

	"jbs/internal/diag"
)

func TestImportConflictTrackerAdd(t *testing.T) {
	spanA := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	spanB := diag.NewSpan("in.jbs", diag.NewPos(0, 2, 1), diag.NewPos(1, 2, 2))
	tracker := newImportConflictTracker()

	if prev, conflict, first := tracker.Add("x", "p0", spanA); conflict || first || prev.Source != "" {
		t.Fatalf("first add should only register source, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}

	if prev, conflict, first := tracker.Add("x", "p0", spanB); conflict || first || prev.Source != "p0" || prev.Span != spanA {
		t.Fatalf("same source should not conflict and should return original origin, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}

	if prev, conflict, first := tracker.Add("x", "p1", spanB); !conflict || !first || prev.Source != "p0" || prev.Span != spanA {
		t.Fatalf("first cross-source collision should conflict and be first report, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}

	if prev, conflict, first := tracker.Add("x", "p1", spanB); !conflict || first || prev.Source != "p0" {
		t.Fatalf("repeated collision for same pair should conflict but not first, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}

	if prev, conflict, first := tracker.Add("x", "p0", spanA); conflict || first || prev.Source != "p0" {
		t.Fatalf("switching back to original source should not conflict, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}

	if prev, conflict, first := tracker.Add("y", "q0", spanA); conflict || first || prev.Source != "" {
		t.Fatalf("different variable name should start cleanly, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}

	if _, conflict, first := tracker.Add("z", "z1", spanA); conflict || first {
		t.Fatalf("initial z add should not conflict")
	}
	if prev, conflict, first := tracker.Add("z", "z0", spanB); !conflict || !first || prev.Source != "z1" {
		t.Fatalf("expected first z conflict even when source order reverses, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}
	if prev, conflict, first := tracker.Add("z", "z0", spanB); !conflict || first || prev.Source != "z1" {
		t.Fatalf("expected repeated z conflict to be deduplicated, got prev=%+v conflict=%v first=%v", prev, conflict, first)
	}
}
