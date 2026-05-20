package eval

import (
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestSetSeedAndSampleReproducible(t *testing.T) {
	state := NewRandomStateWithSeed(0)
	span := spanAt(2000, 1)
	values := List([]Value{Int(0), Int(1), Int(2), Int(3), Int(4), Int(5), Int(6), Int(7), Int(8), Int(9)})

	diags := &diag.Diagnostics{}
	evalSetSeedValueCall([]CallValueArg{{Value: Int(11), Span: span}}, span, diags, ExprOptions{Random: state})
	first := evalSampleValueCall([]CallValueArg{
		{Value: values, Span: span},
		{Name: "size", Value: Int(5), Span: span},
	}, span, diags, ExprOptions{Random: state})

	evalSetSeedValueCall([]CallValueArg{{Value: Int(11), Span: span}}, span, diags, ExprOptions{Random: state})
	second := evalSampleValueCall([]CallValueArg{
		{Value: values, Span: span},
		{Name: "size", Value: Int(5), Span: span},
	}, span, diags, ExprOptions{Random: state})

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(first, second) {
		t.Fatalf("seeded samples differ: first=%#v second=%#v", first, second)
	}
	if first.Kind != KindList || len(first.L) != 5 || !uniqueInts(first.L) || !allIntsInRange(first.L, 0, 9) {
		t.Fatalf("unexpected sample result: %#v", first)
	}
}

func TestSampleSequenceShapesAndDefaults(t *testing.T) {
	span := spanAt(2001, 1)
	list := List([]Value{Int(1), Int(2), Int(3)})
	diags := &diag.Diagnostics{}
	got := evalSampleValueCall([]CallValueArg{{Value: list, Span: span}}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(1)})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindList || len(got.L) != 3 || !sameIntSet(got.L, []int64{1, 2, 3}) {
		t.Fatalf("unexpected full-list sample: %#v", got)
	}
	if !Equal(list, List([]Value{Int(1), Int(2), Int(3)})) {
		t.Fatalf("sample() mutated input list: %#v", list)
	}

	tuple := Tuple([]Value{String("a"), String("b"), String("c")})
	got = evalSampleValueCall([]CallValueArg{
		{Value: tuple, Span: span},
		{Name: "size", Value: Int(2), Span: span},
	}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(2)})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindTuple || len(got.L) != 2 {
		t.Fatalf("expected two tuple items, got %#v", got)
	}
	for _, item := range got.L {
		if item.Kind != KindString || (item.S != "a" && item.S != "b" && item.S != "c") {
			t.Fatalf("unexpected tuple sample item: %#v", item)
		}
	}
}

func TestSampleTableRows(t *testing.T) {
	span := spanAt(2002, 1)
	cases := CombValue(&Comb{
		Order: []string{"id", "label"},
		Rows: []Row{
			{Values: map[string]Cell{"id": {Value: Int(1)}, "label": {Value: String("a")}}},
			{Values: map[string]Cell{"id": {Value: Int(2)}, "label": {Value: String("b")}}},
			{Values: map[string]Cell{"id": {Value: Int(3)}, "label": {Value: String("c")}}},
		},
	})
	diags := &diag.Diagnostics{}
	got := evalSampleValueCall([]CallValueArg{
		{Value: cases, Span: span},
		{Name: "size", Value: Int(2), Span: span},
	}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(3)})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !IsComb(got) || len(got.C.Rows) != 2 || len(got.C.Order) != 2 || got.C.Order[0] != "id" || got.C.Order[1] != "label" {
		t.Fatalf("unexpected sampled table: %#v", got)
	}
	ids := sampledTableIDs(t, got)
	if !uniqueInt64s(ids) || !int64sInRange(ids, 1, 3) {
		t.Fatalf("unexpected sampled ids: %#v", ids)
	}
	got.C.Rows[0].Values["id"] = Cell{Value: Int(99)}
	if cases.C.Rows[0].Values["id"].Value.I == 99 || cases.C.Rows[1].Values["id"].Value.I == 99 || cases.C.Rows[2].Values["id"].Value.I == 99 {
		t.Fatalf("sample() mutated input table: %#v", cases)
	}
}

func TestSampleWithReplacement(t *testing.T) {
	span := spanAt(2003, 1)
	diags := &diag.Diagnostics{}
	got := evalSampleValueCall([]CallValueArg{
		{Value: List([]Value{Int(1)}), Span: span},
		{Name: "size", Value: Int(3), Span: span},
		{Name: "replace", Value: Bool(true), Span: span},
	}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(4)})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, List([]Value{Int(1), Int(1), Int(1)})) {
		t.Fatalf("unexpected replacement sample: %#v", got)
	}
}

func TestSampleEmptyInputs(t *testing.T) {
	span := spanAt(2004, 1)
	diags := &diag.Diagnostics{}
	got := evalSampleValueCall([]CallValueArg{
		{Value: List(nil), Span: span},
		{Name: "size", Value: Int(0), Span: span},
	}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(5)})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, List(nil)) {
		t.Fatalf("unexpected empty sample: %#v", got)
	}

	diags = &diag.Diagnostics{}
	got = evalSampleValueCall([]CallValueArg{
		{Value: List(nil), Span: span},
		{Name: "size", Value: Int(1), Span: span},
		{Name: "replace", Value: Bool(true), Span: span},
	}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(5)})
	if got.Kind != KindNull || diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), "sample() cannot sample from an empty value") {
		t.Fatalf("expected empty-input error, got value=%#v diags=%s", got, diags.String())
	}
}

func TestSampleDiagnostics(t *testing.T) {
	span := spanAt(2005, 1)
	tests := []struct {
		name     string
		args     []CallValueArg
		wantText string
	}{
		{
			name:     "non sequence value",
			args:     []CallValueArg{{Value: Int(1), Span: span}},
			wantText: "sample() expects list/tuple/table as first argument",
		},
		{
			name: "size too large without replacement",
			args: []CallValueArg{
				{Value: List([]Value{Int(1)}), Span: span},
				{Name: "size", Value: Int(2), Span: span},
			},
			wantText: "sample() size exceeds input length when replace is false",
		},
		{
			name: "non integer size",
			args: []CallValueArg{
				{Value: List([]Value{Int(1)}), Span: span},
				{Name: "size", Value: Float(1.5), Span: span},
			},
			wantText: "sample() size argument must be an integer",
		},
		{
			name: "negative size",
			args: []CallValueArg{
				{Value: List([]Value{Int(1)}), Span: span},
				{Name: "size", Value: Int(-1), Span: span},
			},
			wantText: "sample() size argument must be non-negative",
		},
		{
			name: "non boolean replace",
			args: []CallValueArg{
				{Value: List([]Value{Int(1)}), Span: span},
				{Name: "replace", Value: String("yes"), Span: span},
			},
			wantText: "sample() replace argument must be a boolean",
		},
		{
			name:     "malformed table",
			args:     []CallValueArg{{Value: CombValue(nil), Span: span}},
			wantText: "sample() received a malformed table value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalSampleValueCall(tc.args, span, diags, ExprOptions{Random: NewRandomStateWithSeed(6)})
			if got.Kind != KindNull {
				t.Fatalf("expected null result on error, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), tc.wantText) {
				t.Fatalf("expected E106 containing %q, got: %s", tc.wantText, diags.String())
			}
		})
	}

	diags := &diag.Diagnostics{}
	got := evalSetSeedValueCall([]CallValueArg{{Value: String("seed"), Span: span}}, span, diags, ExprOptions{Random: NewRandomStateWithSeed(7)})
	if got.Kind != KindNull || diagCount(diags, "E106") == 0 || !strings.Contains(diags.String(), "setseed() seed argument must be an integer") {
		t.Fatalf("expected setseed type error, got value=%#v diags=%s", got, diags.String())
	}
}

func TestSampleAndSetSeedBuiltinFunctionValues(t *testing.T) {
	for _, name := range []string{"sample", "setseed"} {
		value, ok := BuiltinFunctionValue(name)
		if !ok || value.Kind != KindFunction || value.Fn == nil || value.Fn.BuiltinName != name {
			t.Fatalf("expected builtin function value for %q, got ok=%v value=%#v", name, ok, value)
		}
	}

	frame := NewRootFrame(nil)
	assignBuiltinFunction(t, frame, "draw", "sample")
	assignBuiltinFunction(t, frame, "seed", "setseed")
	state := NewRandomStateWithSeed(0)
	diags := &diag.Diagnostics{}
	seeded := EvalExprWithOptions(callExpr(ident("seed"), posArg(intExpr(1))), nil, diags, ExprOptions{Frame: frame, Random: state})
	if seeded.Kind != KindNull {
		t.Fatalf("expected setseed function value to return null, got %#v", seeded)
	}
	got := EvalExprWithOptions(callExpr(ident("draw"),
		posArg(listExpr(intExpr(1))),
		namedArg("size", intExpr(3)),
		namedArg("replace", boolExpr(true)),
	), nil, diags, ExprOptions{Frame: frame, Random: state})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, List([]Value{Int(1), Int(1), Int(1)})) {
		t.Fatalf("unexpected sample function value result: %#v", got)
	}
}

func sameIntSet(values []Value, want []int64) bool {
	if len(values) != len(want) {
		return false
	}
	seen := make(map[int64]int, len(values))
	for _, value := range values {
		if value.Kind != KindInt {
			return false
		}
		seen[value.I]++
	}
	for _, value := range want {
		if seen[value] != 1 {
			return false
		}
	}
	return true
}

func uniqueInts(values []Value) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value.Kind != KindInt {
			return false
		}
		if _, exists := seen[value.I]; exists {
			return false
		}
		seen[value.I] = struct{}{}
	}
	return true
}

func allIntsInRange(values []Value, min, max int64) bool {
	for _, value := range values {
		if value.Kind != KindInt || value.I < min || value.I > max {
			return false
		}
	}
	return true
}

func sampledTableIDs(t *testing.T, value Value) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(value.C.Rows))
	for _, row := range value.C.Rows {
		cell, ok := row.Values["id"]
		if !ok || cell.Value.Kind != KindInt {
			t.Fatalf("unexpected id cell: %#v", cell)
		}
		ids = append(ids, cell.Value.I)
	}
	return ids
}

func uniqueInt64s(values []int64) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func int64sInRange(values []int64, min, max int64) bool {
	for _, value := range values {
		if value < min || value > max {
			return false
		}
	}
	return true
}
