package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestCloneSeriesMap(t *testing.T) {
	if got := cloneSeriesMap(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil input should return a non-nil empty map, got %#v", got)
	}

	empty := map[string][]eval.Value{}
	if got := cloneSeriesMap(empty); got == nil || len(got) != 0 {
		t.Fatalf("empty input should return a non-nil empty map, got %#v", got)
	}

	src := map[string][]eval.Value{
		"a": {eval.Int(1), eval.Int(2)},
		"b": {eval.String("x")},
	}
	got := cloneSeriesMap(src)
	if len(got) != len(src) {
		t.Fatalf("unexpected cloned map size: got %d want %d", len(got), len(src))
	}
	if &got["a"][0] == &src["a"][0] {
		t.Fatalf("expected cloned slice backing storage for key a")
	}
	if !reflect.DeepEqual(got["a"], src["a"]) || !reflect.DeepEqual(got["b"], src["b"]) {
		t.Fatalf("clone contents do not match source: got=%#v src=%#v", got, src)
	}

	got["a"][0] = eval.Int(99)
	got["b"] = append(got["b"], eval.String("y"))
	if !reflect.DeepEqual(src["a"], []eval.Value{eval.Int(1), eval.Int(2)}) {
		t.Fatalf("mutating clone should not affect source values: src=%#v", src)
	}
	if !reflect.DeepEqual(src["b"], []eval.Value{eval.String("x")}) {
		t.Fatalf("mutating clone slice should not affect source: src=%#v", src)
	}
}
