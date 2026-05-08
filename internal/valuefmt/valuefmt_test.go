package valuefmt

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestReplValueListTupleTruncation(t *testing.T) {
	list := eval.List([]eval.Value{eval.Int(0), eval.Int(1), eval.Int(2), eval.Int(3)})
	if got := ReplValue(list); got != "[0, 1, 2, ...]" {
		t.Fatalf("unexpected list preview: %q", got)
	}

	tuple := eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")})
	if got := ReplValue(tuple); got != "(\"a\", \"b\")" {
		t.Fatalf("unexpected tuple preview: %q", got)
	}
}

func TestReplValueTableSummary(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Order: []string{"a", "b"},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}, "b": {Value: eval.String("x")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(2)}, "b": {Value: eval.String("y")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(3)}, "b": {Value: eval.String("z")}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(4)}, "b": {Value: eval.String("w")}}},
		},
	})
	got := ReplValue(comb)
	if !strings.Contains(got, "table(rows=4") {
		t.Fatalf("expected rows summary, got: %q", got)
	}
	if !strings.Contains(got, "cols=[a, b]") {
		t.Fatalf("expected column summary, got: %q", got)
	}
	if !strings.Contains(got, "head=[{a:1, b:\"x\"}, {a:2, b:\"y\"}, {a:3, b:\"z\"}, ...]") {
		t.Fatalf("expected truncated head summary, got: %q", got)
	}
}

func TestReplValueTableFallbackColumnOrder(t *testing.T) {
	comb := eval.CombValue(&eval.Comb{
		Rows: []eval.Row{{
			Values: map[string]eval.Cell{
				"z": {Value: eval.Int(1)},
				"a": {Value: eval.Int(2)},
			},
		}},
	})
	got := ReplValue(comb)
	if !strings.Contains(got, "cols=[a, z]") {
		t.Fatalf("expected sorted fallback columns, got: %q", got)
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
	if got := PrintLine(values); got != "x [1, 2, 3, ...] <function>" {
		t.Fatalf("unexpected print line: %q", got)
	}
	if got := PrintLine(nil); got != "" {
		t.Fatalf("expected blank print line, got %q", got)
	}
}
