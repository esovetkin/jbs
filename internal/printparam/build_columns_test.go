package printparam

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestFilterColumnsByUsage(t *testing.T) {
	candidates := []string{"p.a", "p.b", "p.a"}
	used := map[string]struct{}{"p.b": {}, "q.c": {}}
	got := filterColumnsByUsage(candidates, used)
	want := []string{"p.b", "q.c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterColumnsByUsage mismatch: got=%#v want=%#v", got, want)
	}
}

func TestPruneHeaderOnlyColumns(t *testing.T) {
	cols := []string{"p.a", "p.b", "p.c"}
	rows := []Row{{Values: map[string]string{"p.a": "1"}}, {Values: map[string]string{"p.b": ""}}}
	got := pruneHeaderOnlyColumns(cols, rows)
	want := []string{"p.a", "p.b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pruneHeaderOnlyColumns mismatch: got=%#v want=%#v", got, want)
	}
}

func TestCollectQualifiedColumns(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"a": {Name: "mod", Order: []string{"x", "y"}, Vars: map[string][]eval.Value{"x": {eval.Int(1)}, "y": {eval.Int(2)}}},
		"b": {Name: "mod", Order: []string{"x"}, Vars: map[string][]eval.Value{"x": {eval.Int(3)}}},
		"c": nil,
		"d": {Name: "other", Vars: map[string][]eval.Value{"z": {eval.Int(4)}}},
	}
	got := collectQualifiedColumns(bindings)
	want := []string{"mod.x", "mod.y", "other.z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectQualifiedColumns mismatch: got=%#v want=%#v", got, want)
	}
}

func TestDisplayColumnKeyScalarIdentity(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"x": {
			Name:       "x",
			PublicName: "x",
			Shape:      sema.BindingScalar,
			Order:      []string{"x"},
			Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
		},
	}
	if got := displayColumnKey(bindings, "x", "x"); got != "x" {
		t.Fatalf("expected scalar identity column x, got %q", got)
	}
}

func TestDisplayColumnKeyNamespacedScalarIdentity(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"_js__1__x": {
			Name:       "_js__1__x",
			PublicName: "mod.x",
			Shape:      sema.BindingScalar,
			Order:      []string{"x"},
			Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
		},
	}
	if got := displayColumnKey(bindings, "_js__1__x", "x"); got != "mod.x" {
		t.Fatalf("expected namespaced scalar identity column mod.x, got %q", got)
	}
}

func TestDisplayColumnKeySingleColumnTableStaysQualified(t *testing.T) {
	bindings := map[string]*sema.GlobalBinding{
		"cases": {
			Name:       "cases",
			PublicName: "cases",
			Shape:      sema.BindingTable,
			Order:      []string{"x"},
			Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
		},
	}
	if got := displayColumnKey(bindings, "cases", "x"); got != "cases.x" {
		t.Fatalf("expected qualified table column cases.x, got %q", got)
	}
}
