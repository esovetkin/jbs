package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/valuefmt"
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
	if got := valuefmt.ReplValue(list); got != "[0, 1, 2, 3]" {
		t.Fatalf("unexpected list preview: %q", got)
	}

	tuple := eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")})
	if got := valuefmt.ReplValue(tuple); got != "(\"a\", \"b\")" {
		t.Fatalf("unexpected tuple preview: %q", got)
	}
}

func TestFormatReplValueDictionaryPreview(t *testing.T) {
	dict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "name"}, Value: eval.String("case")},
		{Key: eval.DictKey{Kind: eval.DictKeyInt, I: 2}, Value: eval.List([]eval.Value{eval.Int(1), eval.Int(2), eval.Int(3), eval.Int(4)})},
		{Key: eval.DictKey{Kind: eval.DictKeyBool, B: true}, Value: eval.Bool(false)},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "extra"}, Value: eval.Int(9)},
	})
	want := "{\"name\": \"case\",\n 2: [1, 2, 3, 4],\n true: false,\n \"extra\": 9}"
	if got := valuefmt.ReplValue(dict); got != want {
		t.Fatalf("unexpected dictionary preview: %q", got)
	}
}

func TestFormatReplValueTableSummary(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Order: []string{"a", "b"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}, "b": {Value: eval.String("x")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(2)}, "b": {Value: eval.String("y")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(3)}, "b": {Value: eval.String("z")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(4)}, "b": {Value: eval.String("w")}}},
		},
	})
	got := valuefmt.ReplValue(comb)
	if !strings.Contains(got, "| a | b |") || !strings.Contains(got, "| 4 | w |") {
		t.Fatalf("expected pretty table, got: %q", got)
	}
}

func TestFormatReplValueTableFallbackColumnOrder(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Rows: []eval.Row{{
			Values: map[string]eval.Cell{
				"z": {Value: eval.Int(1)},
				"a": {Value: eval.Int(2)},
			},
		}},
	})
	got := valuefmt.ReplValue(comb)
	if !strings.HasPrefix(got, "| a | z |") {
		t.Fatalf("expected sorted fallback columns, got: %q", got)
	}
}

func TestFormatReplValueFunctionPlaceholder(t *testing.T) {
	got := valuefmt.ReplValue(eval.Function(&eval.FunctionValue{}))
	if got != "<function>" {
		t.Fatalf("unexpected function preview: %q", got)
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

func TestCommitReplChunkEmitsExpandedListOutput(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "range(10)")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected no expression errors, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != "[0, 1, 2, 3, 4, 5, 6, 7, 8, 9]" {
		t.Fatalf("unexpected list output: %#v", commit.ExprOutput)
	}
}

func TestCommitReplChunkEmitsPrettyTableOutput(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", `table(id = [1, 2], label = ["a", "bbb"])`)
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected no expression errors, diag=%q", commit.DiagText)
	}
	want := "| id | label |\n|----|-------|\n| 1  | a     |\n| 2  | bbb   |"
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != want {
		t.Fatalf("unexpected table output:\n%#v", commit.ExprOutput)
	}
}

func TestCommitReplChunkPrintHonorsNRow(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", `print(range(100), nrow = 1)`)
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected no print errors, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || !strings.HasPrefix(commit.ExprOutput[0], "[0, 1, 2") || !strings.HasSuffix(commit.ExprOutput[0], "...]") {
		t.Fatalf("unexpected nrow print output: %#v", commit.ExprOutput)
	}
}

func TestCommitReplChunkEmitsPrintOutput(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "print(\"x\")")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected no print errors, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != "x" {
		t.Fatalf("unexpected print output: %#v", commit.ExprOutput)
	}

	blank, err := commitReplChunk(cwd, commit.Source, "print()")
	if err != nil {
		t.Fatalf("unexpected blank print error: %v", err)
	}
	if blank.HasErrors {
		t.Fatalf("expected blank print to succeed, diag=%q", blank.DiagText)
	}
	if len(blank.ExprOutput) != 1 || blank.ExprOutput[0] != "" {
		t.Fatalf("expected one blank print line, got %#v", blank.ExprOutput)
	}
}

func TestCommitReplChunkShellExpressionOutput(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", `shell("printf hi")`)
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected shell expression to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != "hi" {
		t.Fatalf("unexpected shell expression output: %#v", commit.ExprOutput)
	}
}

func TestCommitReplChunkEnvExpressionOutput(t *testing.T) {
	cwd := t.TempDir()
	t.Setenv("JBS_ENV_REPL_TEST", "from-repl")
	commit, err := commitReplChunk(cwd, "", `env("JBS_ENV_REPL_TEST")`)
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected env expression to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != "from-repl" {
		t.Fatalf("unexpected env expression output: %#v", commit.ExprOutput)
	}

	if err := os.Unsetenv("JBS_ENV_REPL_MISSING"); err != nil {
		t.Fatal(err)
	}
	commit, err = commitReplChunk(cwd, commit.Source, `env("JBS_ENV_REPL_MISSING", "fallback")`)
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected env fallback expression to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 1 || commit.ExprOutput[0] != "fallback" {
		t.Fatalf("unexpected env fallback output: %#v", commit.ExprOutput)
	}
}

func TestCommitReplChunkMergesPrintAndExpressionOutput(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", strings.Join([]string{
		"print(\"a\")",
		"1",
		"print(\"b\")",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected no errors, diag=%q", commit.DiagText)
	}
	want := []string{"a", "1", "b"}
	if strings.Join(commit.ExprOutput, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected merged output: got=%#v want=%#v", commit.ExprOutput, want)
	}
}

func TestCommitReplChunkPrintInsideFunctionUsesCallChunk(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "f = function() { print(\"x\"); 7 }")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors || len(first.ExprOutput) != 0 {
		t.Fatalf("expected function definition to be quiet, errors=%v output=%#v diag=%q", first.HasErrors, first.ExprOutput, first.DiagText)
	}
	second, err := commitReplChunk(cwd, first.Source, "f()")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected function call to succeed, diag=%q", second.DiagText)
	}
	want := []string{"x", "7"}
	if strings.Join(second.ExprOutput, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected function print output: got=%#v want=%#v", second.ExprOutput, want)
	}
}

func TestCommitReplChunkDoesNotEmitPrintOutputOnErrors(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "print(\"x\")\nmissing")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if !commit.HasErrors {
		t.Fatalf("expected failing chunk")
	}
	if len(commit.ExprOutput) != 0 {
		t.Fatalf("expected no output from failing chunk, got %#v", commit.ExprOutput)
	}
	if commit.Source != "" {
		t.Fatalf("expected source to remain uncommitted, got %q", commit.Source)
	}
}

func TestCommitReplChunkIfOutputAndErrors(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", strings.Join([]string{
		"if true {",
		"  x = 1",
		"  x",
		"} else {",
		"  2",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors {
		t.Fatalf("expected if chunk to succeed, diag=%q", first.DiagText)
	}
	if len(first.ExprOutput) != 1 || first.ExprOutput[0] != "1" {
		t.Fatalf("unexpected selected branch output: %#v", first.ExprOutput)
	}

	second, err := commitReplChunk(cwd, first.Source, "if 1 { x = 2 }")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if !second.HasErrors || !strings.Contains(second.DiagText, "ERROR E102") {
		t.Fatalf("expected E102, got errors=%v diag=%q", second.HasErrors, second.DiagText)
	}

	third, err := commitReplChunk(cwd, first.Source, "if true { do run { echo bad } }")
	if err != nil {
		t.Fatalf("unexpected third commit error: %v", err)
	}
	if !third.HasErrors || !strings.Contains(third.DiagText, "ERROR E080") {
		t.Fatalf("expected E080, got errors=%v diag=%q", third.HasErrors, third.DiagText)
	}
}

func TestCommitReplChunkLoopOutputBreakAndContinue(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", strings.Join([]string{
		"for x in range(5) {",
		"  if x == 1 {",
		"    continue",
		"  }",
		"  if x == 3 {",
		"    break",
		"  }",
		"  x",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected loop chunk to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 2 || commit.ExprOutput[0] != "0" || commit.ExprOutput[1] != "2" {
		t.Fatalf("unexpected loop expression output: %#v", commit.ExprOutput)
	}

	invalid, err := commitReplChunk(cwd, commit.Source, "while 1 { break }")
	if err != nil {
		t.Fatalf("unexpected invalid commit error: %v", err)
	}
	if !invalid.HasErrors || !strings.Contains(invalid.DiagText, "ERROR E102") {
		t.Fatalf("expected E102, got errors=%v diag=%q", invalid.HasErrors, invalid.DiagText)
	}
}

func TestCommitReplChunkAllowsReassignmentAndCompoundAssignment(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "a = 1")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "a = 2")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected reassignment to succeed, diag=%q", second.DiagText)
	}
	third, err := commitReplChunk(cwd, second.Source, "a += 3\na")
	if err != nil {
		t.Fatalf("unexpected third commit error: %v", err)
	}
	if third.HasErrors {
		t.Fatalf("expected compound assignment to succeed, diag=%q", third.DiagText)
	}
	if len(third.ExprOutput) != 1 || third.ExprOutput[0] != "5" {
		t.Fatalf("unexpected expr output after compound assignment: %#v", third.ExprOutput)
	}
}

func TestCommitReplChunkEmitsNamesOutput(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "a = 1")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "names()")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected names() to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "[\"a\", \"jbs_benchmarks\", \"jbs_database\", \"jbs_name\", \"jbs_nproc\"]" {
		t.Fatalf("unexpected names() expr output: %#v", second.ExprOutput)
	}
}

func TestCommitReplChunkDeleteIsQuietAndRemovesName(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "a = 1")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "delete(a)")
	if err != nil {
		t.Fatalf("unexpected delete commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected delete(a) to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 0 {
		t.Fatalf("delete(a) should not echo output, got %#v", second.ExprOutput)
	}
	third, err := commitReplChunk(cwd, second.Source, "names()")
	if err != nil {
		t.Fatalf("unexpected names commit error: %v", err)
	}
	if third.HasErrors {
		t.Fatalf("expected names() to succeed after delete, diag=%q", third.DiagText)
	}
	if len(third.ExprOutput) != 1 {
		t.Fatalf("expected one names() output, got %#v", third.ExprOutput)
	}
	if strings.Contains(third.ExprOutput[0], "\"a\"") {
		t.Fatalf("deleted name is still visible: %#v", third.ExprOutput)
	}
}

func TestCommitReplChunkDeleteProtectedGlobalKeepsSource(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", "a = 1")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "delete(jbs_name)")
	if err != nil {
		t.Fatalf("unexpected delete commit error: %v", err)
	}
	if !second.HasErrors {
		t.Fatalf("expected delete(jbs_name) to fail")
	}
	if second.Source != first.Source {
		t.Fatalf("failed delete should not update accepted source: %q", second.Source)
	}
	if !strings.Contains(second.DiagText, "cannot delete global variable 'jbs_name'") {
		t.Fatalf("missing protected global diagnostic: %q", second.DiagText)
	}
}

func TestCommitReplChunkFunctionDefinitionAndCall(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", strings.Join([]string{
		"add = function(a, b = 1) {",
		"  a + b",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors {
		t.Fatalf("expected function definition to succeed, diag=%q", first.DiagText)
	}
	if len(first.ExprOutput) != 0 {
		t.Fatalf("expected no expr output for function definition, got %#v", first.ExprOutput)
	}

	second, err := commitReplChunk(cwd, first.Source, "add")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected function lookup to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "<function>" {
		t.Fatalf("unexpected function lookup output: %#v", second.ExprOutput)
	}

	third, err := commitReplChunk(cwd, second.Source, "add(1, b = 2)")
	if err != nil {
		t.Fatalf("unexpected third commit error: %v", err)
	}
	if third.HasErrors {
		t.Fatalf("expected function call to succeed, diag=%q", third.DiagText)
	}
	if len(third.ExprOutput) != 1 || third.ExprOutput[0] != "3" {
		t.Fatalf("unexpected function call output: %#v", third.ExprOutput)
	}
}

func TestCommitReplChunkClosurePersistsAcrossInputs(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", strings.Join([]string{
		"make_adder = function(delta) {",
		"  function(x) {",
		"    x + delta",
		"  }",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors {
		t.Fatalf("expected closure factory to succeed, diag=%q", first.DiagText)
	}

	second, err := commitReplChunk(cwd, first.Source, "add2 = make_adder(2)")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected closure assignment to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 0 {
		t.Fatalf("expected no expr output for closure assignment, got %#v", second.ExprOutput)
	}

	third, err := commitReplChunk(cwd, second.Source, "add2")
	if err != nil {
		t.Fatalf("unexpected third commit error: %v", err)
	}
	if third.HasErrors {
		t.Fatalf("expected closure lookup to succeed, diag=%q", third.DiagText)
	}
	if len(third.ExprOutput) != 1 || third.ExprOutput[0] != "<function>" {
		t.Fatalf("unexpected closure lookup output: %#v", third.ExprOutput)
	}

	fourth, err := commitReplChunk(cwd, third.Source, "add2(3)")
	if err != nil {
		t.Fatalf("unexpected fourth commit error: %v", err)
	}
	if fourth.HasErrors {
		t.Fatalf("expected closure call to succeed, diag=%q", fourth.DiagText)
	}
	if len(fourth.ExprOutput) != 1 || fourth.ExprOutput[0] != "5" {
		t.Fatalf("unexpected closure call output: %#v", fourth.ExprOutput)
	}
}

func TestCommitReplChunkNamesIncludeFunctionValuedGlobals(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", strings.Join([]string{
		"add = function(x) {",
		"  x + 1",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "names()")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected names() to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "[\"add\", \"jbs_benchmarks\", \"jbs_database\", \"jbs_name\", \"jbs_nproc\"]" {
		t.Fatalf("unexpected names() output for function globals: %#v", second.ExprOutput)
	}
}

func TestCommitReplChunkMapReduceAcrossInputs(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", strings.Join([]string{
		"inc = function(x) {",
		"  x + 1",
		"}",
		"sum2 = function(acc, x) {",
		"  acc + x",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors {
		t.Fatalf("expected function definitions to succeed, diag=%q", first.DiagText)
	}

	second, err := commitReplChunk(cwd, first.Source, "map(inc, [1,2,3])\nreduce(sum2, [1,2,3])")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected map/reduce to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 2 || second.ExprOutput[0] != "[2, 3, 4]" || second.ExprOutput[1] != "6" {
		t.Fatalf("unexpected map/reduce expr output: %#v", second.ExprOutput)
	}
}

func TestCommitReplChunkMapWithPersistedClosure(t *testing.T) {
	cwd := t.TempDir()
	first, err := commitReplChunk(cwd, "", strings.Join([]string{
		"make_adder = function(delta) {",
		"  function(x) {",
		"    x + delta",
		"  }",
		"}",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	if first.HasErrors {
		t.Fatalf("expected closure factory to succeed, diag=%q", first.DiagText)
	}

	second, err := commitReplChunk(cwd, first.Source, "add2 = make_adder(2)")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected closure assignment to succeed, diag=%q", second.DiagText)
	}

	third, err := commitReplChunk(cwd, second.Source, "map(add2, [1,2])")
	if err != nil {
		t.Fatalf("unexpected third commit error: %v", err)
	}
	if third.HasErrors {
		t.Fatalf("expected map with persisted closure to succeed, diag=%q", third.DiagText)
	}
	if len(third.ExprOutput) != 1 || third.ExprOutput[0] != "[3, 4]" {
		t.Fatalf("unexpected closure map output: %#v", third.ExprOutput)
	}
}

func TestCommitReplChunkReduceEmptyInputReportsError(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", strings.Join([]string{
		"sum2 = function(acc, x) {",
		"  acc + x",
		"}",
		"reduce(sum2, [])",
	}, "\n"))
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if !commit.HasErrors {
		t.Fatalf("expected reduce([]) error, diag=%q", commit.DiagText)
	}
	if !strings.Contains(commit.DiagText, "reduce() cannot operate on an empty list/tuple") {
		t.Fatalf("unexpected reduce([]) diagnostics: %q", commit.DiagText)
	}
}

func TestCommitReplChunkEmitsConversionOutputs(t *testing.T) {
	cwd := t.TempDir()
	commit, err := commitReplChunk(cwd, "", "int(\"42\")\nfloat(true)\nstr([1,2])\nbool(\"\")\nbool(\"x\")")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected conversion expressions to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 5 {
		t.Fatalf("expected 5 expr outputs, got %#v", commit.ExprOutput)
	}
	want := []string{"42", "1.0", "[1,2]", "false", "true"}
	if !slices.Equal(commit.ExprOutput, want) {
		t.Fatalf("unexpected conversion expr output: %#v", commit.ExprOutput)
	}
}

func TestCommitReplChunkEmitsReadCSVOutputs(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "cases.csv")
	if err := os.WriteFile(path, []byte("x,y\n1,2\n3,4\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	commit, err := commitReplChunk(cwd, "", "names(read_csv(\"./cases.csv\"))\nlen(read_csv(\"./cases.csv\"))")
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if commit.HasErrors {
		t.Fatalf("expected read_csv expressions to succeed, diag=%q", commit.DiagText)
	}
	if len(commit.ExprOutput) != 2 {
		t.Fatalf("expected 2 expr outputs, got %#v", commit.ExprOutput)
	}
	if commit.ExprOutput[0] != "[\"x\", \"y\"]" || commit.ExprOutput[1] != "2" {
		t.Fatalf("unexpected read_csv expr output: %#v", commit.ExprOutput)
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

func TestCommitReplChunkWithNamespaceNames(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte("z = \"ok\"\na = 1\n"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	first, err := commitReplChunk(cwd, "", "use \"./lib.jbs\" as lib")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "names(lib)")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected names(lib) to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "[\"a\", \"z\"]" {
		t.Fatalf("unexpected names(lib) output: %#v", second.ExprOutput)
	}
}

func TestCommitReplChunkWithNamespacedImportedFunction(t *testing.T) {
	cwd := t.TempDir()
	libPath := filepath.Join(cwd, "lib.jbs")
	if err := os.WriteFile(libPath, []byte(strings.Join([]string{
		"base = 40",
		"add = function(a, b) {",
		"  a + b + base",
		"}",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	first, err := commitReplChunk(cwd, "", "use \"./lib.jbs\" as lib")
	if err != nil {
		t.Fatalf("unexpected first commit error: %v", err)
	}
	second, err := commitReplChunk(cwd, first.Source, "lib.add(1, 2)")
	if err != nil {
		t.Fatalf("unexpected second commit error: %v", err)
	}
	if second.HasErrors {
		t.Fatalf("expected namespaced imported function call to succeed, diag=%q", second.DiagText)
	}
	if len(second.ExprOutput) != 1 || second.ExprOutput[0] != "43" {
		t.Fatalf("unexpected namespaced function output: %#v", second.ExprOutput)
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

func hasDiag(diags *diag.Diagnostics, code string) bool {
	for _, item := range diags.Items {
		if item.Code == code {
			return true
		}
	}
	return false
}
