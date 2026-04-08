package planutil

import (
	"reflect"
	"testing"

	"jbs/internal/eval"
)

func TestBuildRowGroupsGroupsEquivalentRows(t *testing.T) {
	valuesByName := map[string][]eval.Value{
		"a": {eval.Int(1), eval.Int(1), eval.Int(2), eval.Int(2)},
		"b": {eval.String("x"), eval.String("x"), eval.String("y"), eval.String("y")},
	}
	got := BuildRowGroups([]string{"a", "b"}, valuesByName, 4, func(v eval.Value) string {
		return v.String()
	})
	want := []RowGroup{
		{Rep: 0, Rows: []int{0, 1}},
		{Rep: 2, Rows: []int{2, 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRowGroups() = %#v, want %#v", got, want)
	}
}

func TestBuildRowGroupsNoVarsReturnsSingleRangeGroup(t *testing.T) {
	got := BuildRowGroups(nil, nil, 3, nil)
	want := []RowGroup{{Rep: 0, Rows: []int{0, 1, 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildRowGroups() = %#v, want %#v", got, want)
	}
}

func TestBuildRowGroupsZeroRows(t *testing.T) {
	got := BuildRowGroups([]string{"a"}, map[string][]eval.Value{"a": {eval.Int(1)}}, 0, nil)
	if got != nil {
		t.Fatalf("BuildRowGroups() = %#v, want nil", got)
	}
}

func TestSequentialIndices(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want []int
	}{
		{name: "negative", n: -1, want: nil},
		{name: "zero", n: 0, want: nil},
		{name: "one", n: 1, want: []int{0}},
		{name: "many", n: 4, want: []int{0, 1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SequentialIndices(tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SequentialIndices(%d) = %#v, want %#v", tt.n, got, tt.want)
			}
		})
	}
}
