package cli

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestDiagnosticsFormattingIncludesSourceExcerptHintsAndRelatedSpans(t *testing.T) {
	if got := formatDiagnosticsWithSources(diag.Diagnostics{}, nil, ""); got != "" {
		t.Fatalf("empty diagnostics formatted as %q", got)
	}

	diags := diag.Diagnostics{Items: []diag.Diagnostic{{
		Severity: diag.SeverityWarning,
		Code:     "WTEST",
		Message:  "uses default file",
		Span: diag.NewSpan("",
			diag.NewPos(0, 2, 2),
			diag.NewPos(0, 2, 5)),
		Hint: "try again",
		Related: []diag.RelatedSpan{{
			Message: "related note",
			Span: diag.NewSpan("rel.jbs",
				diag.NewPos(0, 1, 1),
				diag.NewPos(0, 1, 2)),
		}},
	}, {
		Severity: diag.SeverityError,
		Code:     "ETEST",
		Message:  "falls back to default source",
		Span: diag.NewSpan("other.jbs",
			diag.NewPos(0, 1, 1),
			diag.NewPos(0, 1, 2)),
	}}}
	got := formatDiagnosticsWithSources(diags, map[string]string{
		"main.jbs": "alpha\nbeta\ngamma\n",
	}, "main.jbs")

	for _, want := range []string{
		"WARNING WTEST <input>:2:2",
		"uses default file",
		"   2 | beta",
		"Hint: try again",
		"Related: related note (rel.jbs:1:1)",
		"ERROR ETEST other.jbs:1:1",
		"   1 | alpha",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted diagnostics missing %q:\n%s", want, got)
		}
	}
}

func TestSourceExcerptHandlesInvalidAndLongSpans(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	if got := sourceExcerpt(lines, diag.Span{}); got != "" {
		t.Fatalf("zero span excerpt = %q", got)
	}
	if got := sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 0, 1), diag.NewPos(0, 0, 2))); got != "" {
		t.Fatalf("line zero excerpt = %q", got)
	}
	if got := sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 99, 1), diag.NewPos(0, 99, 2))); got != "" {
		t.Fatalf("out-of-range excerpt = %q", got)
	}

	got := sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 2, 2), diag.NewPos(0, 1, 1)))
	if !strings.Contains(got, "   2 | beta") || !strings.Contains(got, "       |  ^") {
		t.Fatalf("expected end-before-start excerpt, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 4, 1), diag.NewPos(0, 99, 2)))
	if !strings.Contains(got, "   4 | delta") || !strings.Contains(got, "   5 | epsilon") || strings.Contains(got, "   6 |") {
		t.Fatalf("expected excerpt capped at input length, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 5, 2)))
	if !strings.Contains(got, "   3 | gamma") || strings.Contains(got, "   4 | delta") {
		t.Fatalf("expected long excerpt capped to three lines, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 1, 0), diag.NewPos(0, 1, 0)))
	if !strings.Contains(got, "       | ^") {
		t.Fatalf("expected column floor caret, got %q", got)
	}

	got = sourceExcerpt(lines, diag.NewSpan("x.jbs", diag.NewPos(0, 1, 2), diag.NewPos(0, 1, 5)))
	if !strings.Contains(got, "       |  ^^^") {
		t.Fatalf("expected multi-column caret, got %q", got)
	}
}
