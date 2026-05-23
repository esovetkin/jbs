package workplan

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestValueAtAndSortedSeriesNamesBranches(t *testing.T) {
	series := []eval.Value{eval.Int(1), eval.Int(2)}
	if got := valueAt(series, -1); got.Kind != eval.KindNull {
		t.Fatalf("negative index should return null, got %#v", got)
	}
	if got := valueAt(series, len(series)); got.Kind != eval.KindNull {
		t.Fatalf("out-of-range index should return null, got %#v", got)
	}
	if got := valueAt(series, 1); got.Kind != eval.KindInt || got.I != 2 {
		t.Fatalf("expected in-range value, got %#v", got)
	}
	if got := valueAt(nil, 0); got.Kind != eval.KindNull {
		t.Fatalf("empty series should return null, got %#v", got)
	}

	names := sortedSeriesNames(map[string][]eval.Value{
		"z": {eval.Int(1)},
		"a": {eval.Int(2)},
		"m": nil,
	})
	if !reflect.DeepEqual(names, []string{"a", "m", "z"}) {
		t.Fatalf("expected sorted series names, got %#v", names)
	}
}

func TestBindingForGroupLookupBranches(t *testing.T) {
	key := sema.BindingVersionKey{Public: "cases", Version: "v1"}
	byKey := &sema.GlobalBinding{Name: "cases_key"}
	bySource := &sema.GlobalBinding{Name: "cases_source"}

	if got := bindingForGroup(
		sourceGroup{Source: "cases", SourceKey: key},
		map[string]*sema.GlobalBinding{"cases": bySource},
		map[sema.BindingVersionKey]*sema.GlobalBinding{key: byKey},
	); got != byKey {
		t.Fatalf("expected source key lookup to win, got %#v", got)
	}
	if got := bindingForGroup(
		sourceGroup{Source: "cases", SourceKey: key},
		map[string]*sema.GlobalBinding{"cases": bySource},
		map[sema.BindingVersionKey]*sema.GlobalBinding{},
	); got != bySource {
		t.Fatalf("expected fallback source lookup, got %#v", got)
	}
	if got := bindingForGroup(sourceGroup{Source: "cases"}, nil, nil); got != nil {
		t.Fatalf("nil source map should return nil, got %#v", got)
	}
	if got := bindingForGroup(sourceGroup{Source: "missing"}, map[string]*sema.GlobalBinding{"cases": bySource}, nil); got != nil {
		t.Fatalf("missing source should return nil, got %#v", got)
	}
}
