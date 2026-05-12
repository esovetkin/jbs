package run

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProgressSnapshotDone(t *testing.T) {
	s := ProgressSnapshot{
		Total:       10,
		Finished:    4,
		Error:       2,
		Blocked:     1,
		Interrupted: 1,
		Running:     2,
	}
	if got, want := s.Done(), 8; got != want {
		t.Fatalf("Done() = %d, want %d", got, want)
	}
}

func TestProgressSuffix(t *testing.T) {
	got := progressSuffix(ProgressSnapshot{Running: 3, Error: 1})
	if got != "3R|1E" {
		t.Fatalf("suffix = %q", got)
	}
	got = progressSuffix(ProgressSnapshot{Running: 0, Error: 1, Interrupted: 2})
	if got != "0R|1E|2I" {
		t.Fatalf("suffix with interrupted = %q", got)
	}
	got = progressSuffix(ProgressSnapshot{Running: 1, Error: 2, Blocked: 3})
	if got != "1R|2E|3B" {
		t.Fatalf("suffix with blocked = %q", got)
	}
}

func TestProgressLineMode(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressWithOptions(&buf, ProgressOptions{Mode: ProgressLines})
	p.Update(ProgressSnapshot{Total: 100, Finished: 40, Error: 2, Blocked: 3, Running: 3})

	got := buf.String()
	if !strings.Contains(got, "45% (45/100)") {
		t.Fatalf("line output missing count: %q", got)
	}
	if !strings.Contains(got, "3R|2E|3B") {
		t.Fatalf("line output missing status suffix: %q", got)
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("line output should not contain carriage returns: %q", got)
	}
}

func TestProgressBarModeSmoke(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressWithOptions(&buf, ProgressOptions{
		Mode:  ProgressBar,
		Width: 8,
	})
	p.Update(ProgressSnapshot{Total: 10})
	p.Update(ProgressSnapshot{Total: 10, Finished: 4, Error: 1, Blocked: 1, Running: 2})
	p.Close(StatusError)

	got := buf.String()
	for _, want := range []string{"60%", "6/10", "2R|1E|1B"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bar output missing %q in %q", want, got)
		}
	}
}

func TestProgressBarRendersDoneChangeDespiteThrottle(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressWithOptions(&buf, ProgressOptions{
		Mode:     ProgressBar,
		Width:    8,
		Throttle: time.Hour,
	})

	p.Update(ProgressSnapshot{Total: 4})
	p.Update(ProgressSnapshot{Total: 4, Finished: 2, Running: 2})

	got := buf.String()
	if !strings.Contains(got, "50%") || !strings.Contains(got, "2/4") {
		t.Fatalf("progress did not render completed jobs: %q", got)
	}
}

func TestProgressFlushRendersPendingRunningState(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressWithOptions(&buf, ProgressOptions{
		Mode:     ProgressBar,
		Width:    8,
		Throttle: time.Hour,
	})

	p.Update(ProgressSnapshot{Total: 4})
	p.Update(ProgressSnapshot{Total: 4, Running: 2})
	p.Flush()

	got := buf.String()
	if !strings.Contains(got, "2R|0E") {
		t.Fatalf("flush did not render latest running state: %q", got)
	}
}

func TestInterruptedProgressBarDoesNotForceComplete(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressWithOptions(&buf, ProgressOptions{
		Mode:  ProgressBar,
		Width: 8,
	})
	p.Update(ProgressSnapshot{Total: 10})
	p.Update(ProgressSnapshot{Total: 10, Finished: 1, Interrupted: 1})
	p.Close(StatusInterrupted)

	got := buf.String()
	if strings.Contains(got, "100%") {
		t.Fatalf("interrupted progress should not force completion: %q", got)
	}
	if !strings.Contains(got, "2/10") {
		t.Fatalf("interrupted progress should show terminal count: %q", got)
	}
}
