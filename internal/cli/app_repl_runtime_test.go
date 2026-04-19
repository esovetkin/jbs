package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/emit"
	"jbs/internal/eval"
	"jbs/internal/lower"
)

func TestFilterDiagnosticsBySeverity(t *testing.T) {
	diags := &diag.Diagnostics{}
	diags.AddWarning(diag.CodeW310, "warn", diag.Span{}, "")
	diags.AddError(diag.CodeE100, "err", diag.Span{}, "")

	errs := filterDiagnosticsBySeverity(diags, diag.SeverityError)
	if len(errs.Items) != 1 {
		t.Fatalf("expected one error diagnostic, got %d", len(errs.Items))
	}
	if errs.Items[0].Severity != diag.SeverityError {
		t.Fatalf("expected error severity, got %q", errs.Items[0].Severity)
	}
}

func TestFormatReplValueListTupleTruncation(t *testing.T) {
	list := eval.List([]eval.Value{eval.Int(0), eval.Int(1), eval.Int(2), eval.Int(3)})
	if got := formatReplValue(list); got != "[0, 1, 2, ...]" {
		t.Fatalf("unexpected list preview: %q", got)
	}

	tuple := eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")})
	if got := formatReplValue(tuple); got != "(\"a\", \"b\")" {
		t.Fatalf("unexpected tuple preview: %q", got)
	}
}

func TestFormatReplValueCombSummary(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Order: []string{"a", "b"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}, "b": {Value: eval.String("x")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(2)}, "b": {Value: eval.String("y")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(3)}, "b": {Value: eval.String("z")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(4)}, "b": {Value: eval.String("w")}}},
		},
	})
	got := formatReplValue(comb)
	if !strings.Contains(got, "comb(rows=4") {
		t.Fatalf("expected rows summary, got: %q", got)
	}
	if !strings.Contains(got, "cols=[a, b]") {
		t.Fatalf("expected column summary, got: %q", got)
	}
	if !strings.Contains(got, "head=[{a:1, b:\"x\"}, {a:2, b:\"y\"}, {a:3, b:\"z\"}, ...]") {
		t.Fatalf("expected truncated head summary, got: %q", got)
	}
}

func TestFormatReplValueCombFallbackColumnOrder(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Rows: []eval.Row{{
			Values: map[string]eval.Cell{
				"z": {Value: eval.Int(1)},
				"a": {Value: eval.Int(2)},
			},
		}},
	})
	got := formatReplValue(comb)
	if !strings.Contains(got, "cols=[a, z]") {
		t.Fatalf("expected sorted fallback columns, got: %q", got)
	}
}

func TestCommitReplChunkAssignmentProducesNoOutput(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "a = range(5)")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected no errors, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 0 {
		t.Fatalf("expected no expr output for assignment, got %#v", commit.ExprOutput)
	}
	if commit.Source != "a = range(5)" {
		t.Fatalf("unexpected committed source: %q", commit.Source)
	}
}

func TestCommitReplChunkEmitsTopLevelExprOutput(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "a = range(5)")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "len(a)")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected no expression errors, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "5" {
		t.Fatalf("unexpected expr output: %#v", second.ExprOutput)
	}
	if second.Source != "a = range(5)\nlen(a)" {
		t.Fatalf("unexpected committed source: %q", second.Source)
	}
}

func TestCommitReplChunkReportsExpressionError(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "range(,)")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if !commit.HasErrors {
		t.Fatalf("expected expression errors, commit=%#v", commit)
	}
	if !strings.Contains(commit.DiagText, "E058") {
		t.Fatalf("expected E058 in diagnostics, got %q", commit.DiagText)
	}
	if commit.Source != "" {
		t.Fatalf("expected failed commit to preserve prior source, got %q", commit.Source)
	}
}

func TestCommitReplChunkWithNamespaceImport(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("z = \"ok\"\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	first, err := commitReplChunk(cwd, "", "use \"./lib.jbs\" as lib")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors || len(first.ExprOutput) != 0 {
		t.Fatalf("expected import commit without output, got %#v", first)
	}
	second, err := commitReplChunk(cwd, first.Source, "lib.z")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected namespace expr to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "ok" {
		t.Fatalf("unexpected namespace expr output: %#v", second.ExprOutput)
	}
}

func TestCommitReplChunkWithEmbeddedJSCNamespace(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "use jsc")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors || len(first.ExprOutput) != 0 {
		t.Fatalf("expected namespace import commit without output, got %#v", first)
	}
	second, err := commitReplChunk(cwd, first.Source, "jsc.systemname")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected embedded namespace expr to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || strings.TrimSpace(second.ExprOutput[0]) == "" {
		t.Fatalf("expected one non-empty expr output, got %#v", second.ExprOutput)
	}
}

func TestCommitReplChunkMixedMultiLineCommit(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("z = 7\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	commit, err := commitReplChunk(cwd, "", "use \"./lib.jbs\" as lib\nlib.z")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected mixed commit to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != "7" {
		t.Fatalf("unexpected mixed commit expr output: %#v", commit.ExprOutput)
	}
	if commit.Source != "use \"./lib.jbs\" as lib\nlib.z" {
		t.Fatalf("unexpected mixed commit source: %q", commit.Source)
	}
}

func TestAnalyzeSourceAllowsUseInReplPath(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("z = \"ok\"\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	diags := &diag.Diagnostics{}
	bundle, err := analyzeSource("<repl>", "use z from \"./lib.jbs\"\na = z\n", cwd, diags)
	if err != nil {
		t.Fatalf("analyzeSource failed: %v", err)
	}
	if bundle == nil {
		t.Fatalf("expected analysis bundle")
	}
	if hasDiag(diags, "E430") {
		t.Fatalf("did not expect E430 in repl analysis: %s", diags.String())
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("did not expect error diagnostics: %s", diags.String())
	}
	if _, ok := bundle.Sources["<repl>"]; !ok {
		t.Fatalf("expected <repl> source in bundle")
	}
	if _, ok := bundle.Sources[libPath]; !ok {
		t.Fatalf("expected imported file source in bundle")
	}
}

func TestCommitReplChunkUseImportFailureDiagnostics(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "use v from \"./missing.jbs\"")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if !commit.HasErrors {
		t.Fatalf("expected errors for missing import, commit=%#v", commit)
	}
	if strings.TrimSpace(commit.DiagText) == "" {
		t.Fatalf("expected non-empty diagnostics")
	}
}

func TestAnalyzeSourceYAMLWithUseImport(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("value = 3\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	src := "use value from \"./lib.jbs\"\n" +
		"a = value + 1\n"
	diags := &diag.Diagnostics{}
	bundle, err := analyzeSource("<repl>", src, cwd, diags)
	if err != nil {
		t.Fatalf("analyzeSource failed: %v", err)
	}
	if bundle == nil {
		t.Fatalf("expected analysis bundle")
	}
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("expected no error diagnostics: %s", diags.String())
	}
	doc := lower.ToJUBEYAML(bundle.Result, diags)
	if len(filterDiagnosticsBySeverity(diags, diag.SeverityError).Items) > 0 {
		t.Fatalf("unexpected lowering errors: %s", diags.String())
	}
	out, err := emit.YAML(doc)
	if err != nil {
		t.Fatalf("emit YAML failed: %v", err)
	}
	if !strings.Contains(string(out), "name:") {
		t.Fatalf("expected benchmark YAML output, got: %s", string(out))
	}
}

func hasDiag(diags *diag.Diagnostics, code string) bool {
	for _, item := range diags.Items {
		if item.Code == code {
			return true
		}
	}
	return false
}
