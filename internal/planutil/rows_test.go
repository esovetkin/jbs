package planutil

import (
	"reflect"
	"testing"

	"jbs/internal/eval"
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

	got := BuildProjectedRowGroups([]int{0, 1, 12, 13}, []string{"b", "c"}, values, false, func(v eval.Value) string {
		return v.String()
	})
	want := []RowGroup{
		{Rep: 0, Rows: []int{0, 1}},
		{Rep: 12, Rows: []int{12, 13}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildProjectedRowGroups() = %#v, want %#v", got, want)
	}
}

func TestBuildProjectedRowGroupsPreservesRowsForFullImports(t *testing.T) {
	got := BuildProjectedRowGroups([]int{0, 1, 12, 13}, []string{"b", "c"}, nil, true, nil)
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
