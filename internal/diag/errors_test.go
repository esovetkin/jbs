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

func TestDiagnosticsStringEmpty(t *testing.T) {
	var d Diagnostics
	if got := d.String(); got != "" {
		t.Fatalf("expected empty diagnostics string, got %q", got)
	}
}

func TestDiagnosticsStringFormatsEntriesAndSpans(t *testing.T) {
	errorSpan := NewSpan("main.jbs", NewPos(10, 2, 3), NewPos(11, 2, 4))

	// Exercise Merge with non-zero spans and empty file on left.
	merged := Merge(
		NewSpan("", NewPos(20, 5, 8), NewPos(25, 5, 13)),
		NewSpan("dep.jbs", NewPos(18, 4, 7), NewPos(30, 6, 2)),
	)
	if merged.IsZero() {
		t.Fatalf("expected merged span to be non-zero")
	}

	// Exercise Span.String branch for zero span with file only.
	fileOnly := Span{File: "context_only.jbs"}
	if got := fileOnly.String(); got != "context_only.jbs" {
		t.Fatalf("expected file-only span string, got %q", got)
	}

	var d Diagnostics
	d.AddError(
		CodeE001,
		"first error",
		errorSpan,
		"fix this first",
		RelatedSpan{Message: "unknown source", Span: Span{}},
		RelatedSpan{Message: "merged source", Span: merged},
		RelatedSpan{Message: "file only", Span: fileOnly},
	)
	d.AddWarning(
		CodeW320,
		"second warning",
		NewSpan("", NewPos(40, 7, 9), NewPos(41, 7, 10)),
		"",
	)

	want := "" +
		"ERROR E001 main.jbs:2:3\n" +
		"first error\n" +
		"Hint: fix this first\n" +
		"Related: unknown source (<unknown>)\n" +
		"Related: merged source (dep.jbs:4:7)\n" +
		"Related: file only (context_only.jbs)\n" +
		"WARNING W320 <input>:7:9\n" +
		"second warning"

	if got := d.String(); got != want {
		t.Fatalf("unexpected diagnostics string:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
