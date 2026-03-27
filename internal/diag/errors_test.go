package diag

import "testing"

func TestDiagnosticsHasErrors(t *testing.T) {
	var d Diagnostics
	d.AddWarning("W1", "warn", Span{}, "")
	if d.HasErrors() {
		t.Fatalf("warnings only should not count as errors")
	}
	d.AddError("E1", "err", Span{}, "")
	if !d.HasErrors() {
		t.Fatalf("expected HasErrors=true")
	}
}
