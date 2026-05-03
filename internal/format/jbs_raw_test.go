package format

import (
	"reflect"
	"strings"
	"testing"

	"jbs/internal/diag"
)

func TestJBSFmtPreservesDoRawBlockFormatting(t *testing.T) {
	src := "do s with cases {\n    cat > run.sbatch <<EOF  \n#!/bin/bash\n\n  echo ${id}  \nEOF\n}\n"
	var diags diag.Diagnostics
	got, err := JBS("raw_do.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := "do s\n        with cases\n{\n    cat > run.sbatch <<EOF  \n#!/bin/bash\n\n  echo ${id}  \nEOF\n}\n"
	if got != want {
		t.Fatalf("unexpected formatted raw block\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestJBSFmtPreservesDoRawTabsAndBlankLines(t *testing.T) {
	src := "do s {\n\n\tprintf 'x'\n\n}\n"
	var diags diag.Diagnostics
	got, err := JBS("raw_tabs.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := "do s\n{\n\n\tprintf 'x'\n\n}\n"
	if got != want {
		t.Fatalf("unexpected formatted raw block\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestJBSFmtDoesNotTrimRawTrailingSpaces(t *testing.T) {
	src := "do s {\nprintf 'x'   \n}\n"
	var diags diag.Diagnostics
	got, err := JBS("raw_spaces.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "printf 'x'   \n") {
		t.Fatalf("raw trailing spaces were not preserved: %q", got)
	}
}

func TestJBSFmtStillTrimsNonRawTrailingSpaces(t *testing.T) {
	src := "x = 1   \n"
	var diags diag.Diagnostics
	got, err := JBS("plain_spaces.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "x = 1\n" {
		t.Fatalf("non-raw trailing spaces should still be trimmed: %q", got)
	}
}

func TestJBSFmtInlineDoRawBlockUsesCanonicalBraces(t *testing.T) {
	src := "do s {echo hi}\n"
	var diags diag.Diagnostics
	got, err := JBS("inline_raw.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := "do s\n{\necho hi\n}\n"
	if got != want {
		t.Fatalf("unexpected inline raw formatting\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestJBSFmtPreservesSubmitRawFieldFormatting(t *testing.T) {
	src := "submit run {\nqueue=\"batch\"\n# before raw\npreprocess = {\n\tprintf 'x'  \nEOF\n}\n# after raw\nargs_exec=\"-lc hostname\"\n}\n"
	var diags diag.Diagnostics
	got, err := JBS("raw_submit.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	checks := []string{
		`        queue = "batch"`,
		`        # before raw`,
		"\tprintf 'x'  \nEOF\n",
		`        # after raw`,
		`        args_exec = "-lc hostname"`,
	}
	for _, needle := range checks {
		if !strings.Contains(got, needle) {
			t.Fatalf("formatted submit missing %q\n--- output ---\n%s", needle, got)
		}
	}
}

func TestJBSFmtPreservesPostprocessRawFieldFormatting(t *testing.T) {
	src := "submit run {\npostprocess = {\ncat <<EOF\n  keep\nEOF\n}\n}\n"
	var diags diag.Diagnostics
	got, err := JBS("postprocess_raw.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !strings.Contains(got, "cat <<EOF\n  keep\nEOF\n") {
		t.Fatalf("postprocess raw payload was changed:\n%s", got)
	}
}

func TestJBSFmtInlineSubmitRawFieldDoesNotCreateWhitespaceBlankLine(t *testing.T) {
	src := "submit run { preprocess = {echo hi} }\n"
	var diags diag.Diagnostics
	got, err := JBS("inline_submit_raw.jbs", src, &diags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	want := "submit run\n{\n        preprocess = {\necho hi\n        }\n}\n"
	if got != want {
		t.Fatalf("unexpected inline submit raw formatting\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestPreserveRawBodyLinesBoundaryBlankLines(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty newline", raw: "\n", want: []string{}},
		{name: "normal multiline", raw: "\necho\n", want: []string{"echo"}},
		{name: "extra boundary blanks", raw: "\n\necho\n\n", want: []string{"", "echo", ""}},
		{name: "inline", raw: "echo", want: []string{"echo"}},
		{name: "crlf", raw: "\r\necho\r\n", want: []string{"echo"}},
		{name: "trailing spaces", raw: "\necho   \n", want: []string{"echo   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formattedLineTexts(preserveRawBodyLines(tc.raw))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("preserveRawBodyLines mismatch: got=%q want=%q", got, tc.want)
			}
		})
	}
}
