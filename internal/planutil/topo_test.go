package planutil

import (
	"reflect"
	"testing"
)

func TestTopoStepOrderPrefersGivenOrderWithDependencies(t *testing.T) {
	deps := map[string][]string{
		"step0": nil,
		"step1": {"step0"},
		"step2": {"step0"},
	}
	got := TopoStepOrder(deps, []string{"step2", "step1"})
	want := []string{"step0", "step2", "step1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoStepOrder() = %v, want %v", got, want)
	}
}

func TestTopoStepOrderIncludesExtraNodesSorted(t *testing.T) {
	deps := map[string][]string{
		"b": nil,
		"a": nil,
		"c": nil,
	}
	got := TopoStepOrder(deps, nil)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoStepOrder() = %v, want %v", got, want)
	}
}

func TestTopoStepOrderIgnoresMissingDependencyReference(t *testing.T) {
	deps := map[string][]string{
		"step1": {"missing"},
	}
	got := TopoStepOrder(deps, []string{"step1"})
	want := []string{"step1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopoStepOrder() = %v, want %v", got, want)
	}
}
