package diag

import "testing"

func TestDiagnosticsHasErrors(t *testing.T) {
	var d Diagnostics
	d.AddWarning(CodeW320, "warn", Span{}, "")
	if d.HasErrors() {
		t.Fatalf("warnings only should not count as errors")
	}
	d.AddError(CodeE001, "err", Span{}, "")
	if !d.HasErrors() {
		t.Fatalf("expected HasErrors=true")
	}
}
