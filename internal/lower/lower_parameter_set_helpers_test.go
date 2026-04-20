package lower

import (
	"reflect"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
	"jbs/internal/sema"
)

func TestJoinIntIndicesAndPickValuesAtIndices(t *testing.T) {
	if got := joinIntIndices(nil); got != "" {
		t.Fatalf("expected empty join for nil indices, got %q", got)
	}
	if got := joinIntIndices([]int{1, 2, 3}); got != "1,2,3" {
		t.Fatalf("unexpected joined indices: %q", got)
	}

	values := []eval.Value{eval.String("a"), eval.String("b")}
	picked := pickValuesAtIndices(values, []int{1, 3, -1, 0})
	want := []eval.Value{eval.String("b"), eval.Null(), eval.Null(), eval.String("a")}
	if !reflect.DeepEqual(picked, want) {
		t.Fatalf("unexpected pickValuesAtIndices result: got=%#v want=%#v", picked, want)
	}
	if got := pickValuesAtIndices(values, nil); got != nil {
		t.Fatalf("expected nil pick for nil indices, got %#v", got)
	}
}

func TestOriginForAndValuesFor(t *testing.T) {
	bindingSpan := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(2, 1, 3))
	nameSpan := diag.NewSpan("in.jbs", diag.NewPos(5, 2, 1), diag.NewPos(6, 2, 2))
	binding := &sema.GlobalBinding{
		Span: bindingSpan,
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(10)}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(20)}}},
		},
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1), eval.Int(2), eval.Int(3)},
			"b": {eval.String("x"), eval.String("y")},
			"c": {},
		},
		Origins: map[string]diag.Span{"a": nameSpan},
	}

	gotRows := valuesFor(binding, "a", 2)
	wantRows := []eval.Value{eval.Int(10), eval.Int(20)}
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Fatalf("expected full row coverage to use row values, got=%#v want=%#v", gotRows, wantRows)
	}

	gotFallback := valuesFor(binding, "b", 3)
	wantFallback := []eval.Value{eval.String("x"), eval.String("y"), eval.String("x")}
	if !reflect.DeepEqual(gotFallback, wantFallback) {
		t.Fatalf("expected fallback cyclic base values, got=%#v want=%#v", gotFallback, wantFallback)
	}

	gotNulls := valuesFor(binding, "c", 3)
	wantNulls := []eval.Value{eval.Null(), eval.Null(), eval.Null()}
	if !reflect.DeepEqual(gotNulls, wantNulls) {
		t.Fatalf("expected null fill for empty base values, got=%#v want=%#v", gotNulls, wantNulls)
	}

	if got := originFor(binding, "a"); got != nameSpan {
		t.Fatalf("expected explicit origin span, got=%+v want=%+v", got, nameSpan)
	}
	if got := originFor(binding, "missing"); got != bindingSpan {
		t.Fatalf("expected fallback binding span, got=%+v want=%+v", got, bindingSpan)
	}
}

func TestAllEqualAsStringAndTemplateValue(t *testing.T) {
	if !allEqualValues(nil) || !allEqualValues([]eval.Value{eval.Int(1)}) {
		t.Fatalf("allEqualValues should be true for len<=1")
	}
	if !allEqualValues([]eval.Value{eval.Int(1), eval.Float(1.0)}) {
		t.Fatalf("allEqualValues should use eval equality semantics")
	}
	if allEqualValues([]eval.Value{eval.Int(1), eval.Int(2)}) {
		t.Fatalf("allEqualValues should detect mismatches")
	}

	if got := asString(eval.String("x")); got != "x" {
		t.Fatalf("asString should preserve raw strings, got %q", got)
	}
	if got := asString(eval.Int(7)); got != "7" {
		t.Fatalf("asString should stringify non-string values, got %q", got)
	}

	tests := []struct {
		in   eval.Value
		want string
	}{
		{in: eval.Int(7), want: "7"},
		{in: eval.Float(1.5), want: "1.5"},
		{in: eval.String("x"), want: "x"},
		{in: eval.Bool(true), want: "true"},
		{in: eval.Bool(false), want: "false"},
		{in: eval.List([]eval.Value{eval.Int(1)}), want: "[1]"},
	}
	for _, tc := range tests {
		if got := templateValue(tc.in); got != tc.want {
			t.Fatalf("templateValue(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPythonExpressionsAndLiteral(t *testing.T) {
	if got := pythonIndexExpr([]eval.Value{eval.Int(1), eval.String("a")}, "$idx"); got != `[1,"a"][$idx]` {
		t.Fatalf("unexpected pythonIndexExpr: %q", got)
	}
	if got := pythonStringMapLookupExpr([]int{0, 2}, []string{"alpha"}, "slot"); got != `{"0":"alpha","2":""}["${slot}"]` {
		t.Fatalf("unexpected pythonStringMapLookupExpr: %q", got)
	}
	if got := pythonStringLookupExpr([]string{"0,1", "2,3"}, []string{"0", "2"}, "rows_prev"); got != `{"0,1":"0","2,3":"2"}["${rows_prev}"]` {
		t.Fatalf("unexpected pythonStringLookupExpr: %q", got)
	}
	if got := parseIntIndices("0, 2, bad, 4"); !reflect.DeepEqual(got, []int{0, 2, 4}) {
		t.Fatalf("unexpected parseIntIndices: %#v", got)
	}

	tests := []struct {
		in   eval.Value
		want string
	}{
		{in: eval.Null(), want: "None"},
		{in: eval.Int(1), want: "1"},
		{in: eval.Float(2.5), want: "2.5"},
		{in: eval.String("x"), want: `"x"`},
		{in: eval.Bool(true), want: "True"},
		{in: eval.Bool(false), want: "False"},
		{in: eval.List([]eval.Value{eval.Int(1), eval.String("a")}), want: `[1,"a"]`},
		{in: eval.Tuple([]eval.Value{eval.Int(1)}), want: "(1,)"},
		{in: eval.Tuple([]eval.Value{eval.Int(1), eval.Int(2)}), want: "(1,2)"},
		{in: eval.Value{Kind: "other", S: "q"}, want: `""`},
	}
	for _, tc := range tests {
		if got := pythonLiteral(tc.in); got != tc.want {
			t.Fatalf("pythonLiteral(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
