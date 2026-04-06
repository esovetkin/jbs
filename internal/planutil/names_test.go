package planutil

import (
	"reflect"
	"testing"

	"jbs/internal/eval"
)

func TestSourceVarNamesUsesOrder(t *testing.T) {
	order := []string{"b", "a"}
	vars := map[string][]eval.Value{
		"a": {eval.Int(1)},
		"b": {eval.Int(2)},
	}
	got := SourceVarNames(order, vars)
	if !reflect.DeepEqual(got, order) {
		t.Fatalf("SourceVarNames() = %v, want %v", got, order)
	}
}

func TestSourceVarNamesFallsBackToSortedKeys(t *testing.T) {
	vars := map[string][]eval.Value{
		"z": {eval.Int(1)},
		"a": {eval.Int(2)},
		"m": {eval.Int(3)},
	}
	want := []string{"a", "m", "z"}
	got := SourceVarNames(nil, vars)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SourceVarNames() = %v, want %v", got, want)
	}
}

func TestSourceRowCount(t *testing.T) {
	vars := map[string][]eval.Value{
		"a": {eval.Int(1), eval.Int(2)},
		"b": {eval.Int(3), eval.Int(4), eval.Int(5)},
	}
	if got, want := SourceRowCount(nil, vars), 3; got != want {
		t.Fatalf("SourceRowCount() = %d, want %d", got, want)
	}
}

func TestSourceRowCountEmpty(t *testing.T) {
	if got := SourceRowCount(nil, nil); got != 0 {
		t.Fatalf("SourceRowCount() = %d, want 0", got)
	}
}

func TestExpandValuesCycles(t *testing.T) {
	base := []eval.Value{eval.Int(10), eval.Int(20)}
	got := ExpandValues(base, 5)
	want := []eval.Value{eval.Int(10), eval.Int(20), eval.Int(10), eval.Int(20), eval.Int(10)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandValues() = %v, want %v", got, want)
	}
}

func TestExpandValuesEmptyBaseReturnsNulls(t *testing.T) {
	got := ExpandValues(nil, 3)
	want := []eval.Value{eval.Null(), eval.Null(), eval.Null()}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandValues() = %v, want %v", got, want)
	}
}
