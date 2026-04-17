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

func TestEvaluateReplExpressionHandled(t *testing.T) {
	cwd := t.TempDir()
	result, diagText, handled, hasErrors, err := evaluateReplExpression(cwd, "a = range(5)", "len(a)")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if !handled {
		t.Fatalf("expected expression to be handled")
	}
	if hasErrors {
		t.Fatalf("expected no expression errors, diag=%q", diagText)
	}
	if strings.TrimSpace(diagText) != "" {
		t.Fatalf("expected empty diagnostics, got %q", diagText)
	}
	if result != "5" {
		t.Fatalf("unexpected expression result: got=%q want=%q", result, "5")
	}
}

func TestEvaluateReplExpressionFallbackForStatement(t *testing.T) {
	cwd := t.TempDir()
	result, diagText, handled, hasErrors, err := evaluateReplExpression(cwd, "", "x = 1")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if handled {
		t.Fatalf("expected statement-shaped input to fall back; result=%q diag=%q hasErrors=%v", result, diagText, hasErrors)
	}
}

func TestEvaluateReplExpressionReportsExpressionError(t *testing.T) {
	cwd := t.TempDir()
	result, diagText, handled, hasErrors, err := evaluateReplExpression(cwd, "", "range(,)")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if !handled {
		t.Fatalf("expected malformed expression to be handled in expression path")
	}
	if !hasErrors {
		t.Fatalf("expected expression errors, result=%q diag=%q", result, diagText)
	}
	if !strings.Contains(diagText, "E058") {
		t.Fatalf("expected E058 in diagnostics, got %q", diagText)
	}
}

func TestEvaluateReplExpressionReportsSourceErrors(t *testing.T) {
	cwd := t.TempDir()
	result, diagText, handled, hasErrors, err := evaluateReplExpression(cwd, "do s {", "range(2)")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if !handled {
		t.Fatalf("expected source parse failure to be handled")
	}
	if !hasErrors {
		t.Fatalf("expected source errors, result=%q diag=%q", result, diagText)
	}
	if strings.TrimSpace(diagText) == "" {
		t.Fatalf("expected non-empty diagnostics for source failure")
	}
}

func TestEvaluateReplExpressionWithUseImport(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("v = 41\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	source := "use v from \"./lib.jbs\"\n" +
		"x = v + 1\n"
	result, diagText, handled, hasErrors, err := evaluateReplExpression(cwd, source, "x")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if !handled {
		t.Fatalf("expected expression to be handled")
	}
	if hasErrors {
		t.Fatalf("expected no errors, diag=%q", diagText)
	}
	if strings.TrimSpace(diagText) != "" {
		t.Fatalf("expected empty diagnostics, got %q", diagText)
	}
	if result != "42" {
		t.Fatalf("unexpected expression result: got=%q want=%q", result, "42")
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

func TestEvaluateReplExpressionUseImportFailureDiagnostics(t *testing.T) {
	cwd := t.TempDir()
	source := "use v from \"./missing.jbs\"\n"
	result, diagText, handled, hasErrors, err := evaluateReplExpression(cwd, source, "1")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled expression path")
	}
	if !hasErrors {
		t.Fatalf("expected errors for missing import, result=%q diag=%q", result, diagText)
	}
	if strings.TrimSpace(diagText) == "" {
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
