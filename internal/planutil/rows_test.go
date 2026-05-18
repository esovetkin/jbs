package planutil

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestBuildProjectedRowGroupsRestrictsAndRegroups(t *testing.T) {
	values := map[string][]eval.Value{
		"b": {
			eval.String("a"), eval.String("a"),
			eval.String("b"), eval.String("b"),
			eval.String("c"), eval.String("c"),
			eval.String("a"), eval.String("a"),
			eval.String("b"), eval.String("b"),
			eval.String("c"), eval.String("c"),
			eval.String("a"), eval.String("a"),
		},
		"c": {
			eval.String("x"), eval.String("x"),
			eval.String("x"), eval.String("x"),
			eval.String("x"), eval.String("x"),
			eval.String("x"), eval.String("x"),
			eval.String("x"), eval.String("x"),
			eval.String("x"), eval.String("x"),
			eval.String("z"), eval.String("z"),
		},
	}

	got := BuildProjectedRowGroups([]int{0, 1, 12, 13}, []string{"b", "c"}, values, false)
	want := []RowGroup{
		{Rep: 0, Rows: []int{0, 1}},
		{Rep: 12, Rows: []int{12, 13}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildProjectedRowGroups() = %#v, want %#v", got, want)
	}
}

func TestBuildRowGroupsEdgeCases(t *testing.T) {
	if got := BuildRowGroups([]string{"x"}, nil, 0); got != nil {
		t.Fatalf("BuildRowGroups(rowCount=0) = %#v, want nil", got)
	}

	got := BuildRowGroups(nil, nil, 3)
	want := []RowGroup{{Rep: 0, Rows: []int{0, 1, 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRowGroups(no vars) = %#v, want %#v", got, want)
	}
}

func TestBuildRowGroupsRegroupsByStableTupleKey(t *testing.T) {
	values := map[string][]eval.Value{
		"a": {eval.Int(1), eval.Int(1), eval.Int(2)},
		"b": {eval.String("x")},
	}

	got := BuildRowGroups([]string{"a", "b"}, values, 5)
	want := []RowGroup{
		{Rep: 0, Rows: []int{0}},
		{Rep: 1, Rows: []int{1}},
		{Rep: 2, Rows: []int{2}},
		{Rep: 3, Rows: []int{3, 4}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRowGroups() = %#v, want %#v", got, want)
	}
}

func TestBuildProjectedRowGroupsEmptyAndNoVars(t *testing.T) {
	if got := BuildProjectedRowGroups(nil, []string{"x"}, nil, false); got != nil {
		t.Fatalf("BuildProjectedRowGroups(empty) = %#v, want nil", got)
	}

	allowed := []int{2, 4, 7}
	got := BuildProjectedRowGroups(allowed, nil, nil, false)
	allowed[0] = 99
	want := []RowGroup{{Rep: 2, Rows: []int{2, 4, 7}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildProjectedRowGroups(no vars) = %#v, want %#v", got, want)
	}
}

func TestBuildProjectedRowGroupsPreservesRowsForFullImports(t *testing.T) {
	got := BuildProjectedRowGroups([]int{0, 1, 12, 13}, []string{"b", "c"}, nil, true)
	want := []RowGroup{
		{Rep: 0, Rows: []int{0}},
		{Rep: 1, Rows: []int{1}},
		{Rep: 12, Rows: []int{12}},
		{Rep: 13, Rows: []int{13}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildProjectedRowGroups(full) = %#v, want %#v", got, want)
	}
}

func TestBuildProjectedRowGroupsUsesStableValueKeys(t *testing.T) {
	values := map[string][]eval.Value{
		"a": {eval.String("x|1:y"), eval.String("x"), eval.String("x|1:y")},
		"b": {eval.String("z"), eval.String("1:y|z"), eval.String("z")},
	}

	got := BuildProjectedRowGroups([]int{0, 1, 2}, []string{"a", "b"}, values, false)
	want := []RowGroup{
		{Rep: 0, Rows: []int{0, 2}},
		{Rep: 1, Rows: []int{1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildProjectedRowGroups() = %#v, want %#v", got, want)
	}
}

func TestSequentialIndices(t *testing.T) {
	for _, n := range []int{-1, 0} {
		if got := SequentialIndices(n); got != nil {
			t.Fatalf("SequentialIndices(%d) = %#v, want nil", n, got)
		}
	}

	got := SequentialIndices(4)
	want := []int{0, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SequentialIndices(4) = %#v, want %#v", got, want)
	}
}
