package valuefmt

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestReplValueListTupleFormatting(t *testing.T) {
	list := eval.List(intValues(10))
	if got := ReplValue(list); got != "[0, 1, 2, 3, 4, 5, 6, 7, 8, 9]" {
		t.Fatalf("unexpected list preview: %q", got)
	}

	tuple := eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")})
	if got := ReplValue(tuple); got != "(\"a\", \"b\")" {
		t.Fatalf("unexpected tuple preview: %q", got)
	}

	single := eval.Tuple([]eval.Value{eval.String("a")})
	if got := ReplValue(single); got != "(\"a\",)" {
		t.Fatalf("unexpected one-item tuple preview: %q", got)
	}
}

func TestReplValueSequenceNRowBudget(t *testing.T) {
	got := ReplValueWithOptions(eval.List(intValues(20)), Options{NRow: 2, Width: 10})
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two rows, got %d:\n%s", len(lines), got)
	}
	if !strings.HasSuffix(got, "...]") {
		t.Fatalf("expected truncated sequence, got:\n%s", got)
	}
	for _, line := range lines {
		if runeLen(line) > 10 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestReplValueSequenceNRowZeroPrintsAll(t *testing.T) {
	got := ReplValueWithOptions(eval.List(intValues(20)), Options{NRow: 0, Width: 10})
	if strings.Contains(got, "...") {
		t.Fatalf("did not expect truncation:\n%s", got)
	}
	if !strings.Contains(got, "19]") {
		t.Fatalf("expected final element in output:\n%s", got)
	}
}

func TestReplValueSequenceDoesNotSplitStrings(t *testing.T) {
	long := strings.Repeat("x", 90)
	got := ReplValueWithOptions(eval.List([]eval.Value{eval.String(long), eval.Int(1)}), Options{NRow: 2, Width: 20})
	if !strings.Contains(got, `"`+long+`"`) {
		t.Fatalf("long string was not preserved whole:\n%s", got)
	}
}

func TestReplValueDictionaryPretty(t *testing.T) {
	dict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a"}, Value: eval.Int(1)},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "b"}, Value: eval.List([]eval.Value{eval.Int(0), eval.Int(1), eval.Int(2), eval.Int(3), eval.Int(4)})},
	})
	want := "{\"a\": 1,\n \"b\": [0, 1, 2, 3, 4]}"
	if got := ReplValue(dict); got != want {
		t.Fatalf("unexpected dictionary preview:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplValueDictionaryNRowLimit(t *testing.T) {
	dict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a"}, Value: eval.Int(1)},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "b"}, Value: eval.Int(2)},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "c"}, Value: eval.Int(3)},
	})
	want := "{\"a\": 1,\n \"b\": 2,\n ...}"
	if got := ReplValueWithOptions(dict, Options{NRow: 2, Width: 80}); got != want {
		t.Fatalf("unexpected dictionary limit:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplValueNestedDictionaryAndTableAreCompact(t *testing.T) {
	table := eval.CombValue(&eval.Comb{
		Order: []string{"x"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(1)}}},
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(2)}}},
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(3)}}},
		},
	})
	dict := eval.DictValue([]eval.DictEntry{
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "config"}, Value: eval.DictValue([]eval.DictEntry{
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "a"}, Value: eval.Int(1)},
			{Key: eval.DictKey{Kind: eval.DictKeyString, S: "b"}, Value: eval.Int(2)},
		})},
		{Key: eval.DictKey{Kind: eval.DictKeyString, S: "cases"}, Value: table},
	})
	want := "{\"config\": {\"a\": 1, \"b\": 2},\n \"cases\": table(rows=3, cols=[x])}"
	if got := ReplValue(dict); got != want {
		t.Fatalf("unexpected nested dictionary:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplValueTablePretty(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Order: []string{"a", "b"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}, "b": {Value: eval.String("x")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(2)}, "b": {Value: eval.String("y")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(3)}, "b": {Value: eval.String("z")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(4)}, "b": {Value: eval.String("w")}}},
		},
	})
	want := "| a | b |\n|---|---|\n| 1 | x |\n| 2 | y |\n| 3 | z |\n| 4 | w |"
	if got := ReplValue(comb); got != want {
		t.Fatalf("unexpected table:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplValueTableNRowLimit(t *testing.T) {
	table := eval.CombValue(&eval.Comb{
		Order: []string{"id"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"id": {Value: eval.Int(1)}}},
			{Values: map[string]eval.Cell{"id": {Value: eval.Int(2)}}},
			{Values: map[string]eval.Cell{"id": {Value: eval.Int(3)}}},
		},
	})
	want := "| id |\n|----|\n| 1  |\n| 2  |\n... 1 more rows"
	if got := ReplValueWithOptions(table, Options{NRow: 3, Width: 80}); got != want {
		t.Fatalf("unexpected table limit:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplValueTableNRowOne(t *testing.T) {
	table := eval.CombValue(&eval.Comb{
		Order: []string{"id"},
		Rows:  []eval.Row{{Values: map[string]eval.Cell{"id": {Value: eval.Int(1)}}}},
	})
	want := "| id |\n|----|\n... 1 more rows"
	if got := ReplValueWithOptions(table, Options{NRow: 1, Width: 80}); got != want {
		t.Fatalf("unexpected table nrow=1:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplValueTableNRowZero(t *testing.T) {
	table := eval.CombValue(&eval.Comb{
		Order: []string{"id"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"id": {Value: eval.Int(1)}}},
			{Values: map[string]eval.Cell{"id": {Value: eval.Int(2)}}},
		},
	})
	got := ReplValueWithOptions(table, Options{NRow: 0, Width: 80})
	if strings.Contains(got, "more rows") || !strings.Contains(got, "| 2  |") {
		t.Fatalf("unexpected unlimited table output:\n%s", got)
	}
}

func TestReplValueTableFallbackColumnOrderAndMissingCells(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Rows: []eval.Row{{
			Values: map[string]eval.Cell{
				"z": {Value: eval.Int(1)},
				"a": {Value: eval.Int(2)},
			},
		}},
	})
	got := ReplValue(comb)
	if !strings.HasPrefix(got, "| a | z |") {
		t.Fatalf("expected sorted fallback columns, got:\n%s", got)
	}

	missing := eval.CombValue(&eval.Comb{
		Order: []string{"a", "b"},
		Rows:  []eval.Row{{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}}}},
	})
	if got := ReplValue(missing); !strings.Contains(got, "| 1 |   |") {
		t.Fatalf("expected blank missing cell, got:\n%s", got)
	}
}

func TestReplValueFunctionPlaceholder(t *testing.T) {
	got := ReplValue(eval.Function(&eval.FunctionValue{}))
	if got != "<function>" {
		t.Fatalf("unexpected function preview: %q", got)
	}
}

func TestPrintLine(t *testing.T) {
	values := []eval.Value{
		eval.String("x"),
		eval.List([]eval.Value{eval.Int(1), eval.Int(2), eval.Int(3), eval.Int(4)}),
		eval.Function(&eval.FunctionValue{}),
	}
	if got := PrintLine(values); got != "\"x\" [1, 2, 3, 4] <function>" {
		t.Fatalf("unexpected print line: %q", got)
	}
	if got := PrintLine(nil); got != "" {
		t.Fatalf("expected blank print line, got %q", got)
	}
}

func TestPrintLineQuotesTopLevelStrings(t *testing.T) {
	got := PrintLine([]eval.Value{
		eval.String("a\nb"),
		eval.String(`quote: "`),
	})
	want := `"a\nb" "quote: \""`
	if got != want {
		t.Fatalf("unexpected quoted strings: got %q want %q", got, want)
	}
}

func TestPrintLineWithMultilineValue(t *testing.T) {
	table := eval.CombValue(&eval.Comb{
		Order: []string{"id"},
		Rows:  []eval.Row{{Values: map[string]eval.Cell{"id": {Value: eval.Int(1)}}}},
	})
	want := "\"cases\"\n| id |\n|----|\n| 1  |"
	if got := PrintLine([]eval.Value{eval.String("cases"), table}); got != want {
		t.Fatalf("unexpected multiline print:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintLineQuotesTableStringCells(t *testing.T) {
	table := eval.CombValue(&eval.Comb{
		Order: []string{"label"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"label": {Value: eval.String("x")}}},
		},
	})
	got := PrintLine([]eval.Value{table})
	if !strings.Contains(got, `| "x"   |`) {
		t.Fatalf("expected quoted string cell:\n%s", got)
	}
}

func intValues(n int) []eval.Value {
	out := make([]eval.Value, n)
	for i := range out {
		out[i] = eval.Int(int64(i))
	}
	return out
}
