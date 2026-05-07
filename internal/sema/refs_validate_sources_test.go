package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestBuildWarningSourcesUsesOrderAndOriginFallback(t *testing.T) {
	bindingSpan := diag.NewSpan("bindings.jbs", diag.NewPos(10, 2, 5), diag.NewPos(20, 2, 15))
	explicitOrigin := diag.NewSpan("bindings.jbs", diag.NewPos(30, 4, 2), diag.NewPos(35, 4, 7))
	res := &Result{
		Bindings: []*GlobalBinding{
			nil,
			{Name: "skip", Span: bindingSpan},
			{
				Name:  "table",
				Span:  bindingSpan,
				Order: []string{"b", "a"},
				Vars: map[string][]eval.Value{
					"a": {eval.String("x")},
					"b": {eval.String("y")},
				},
				Origins: map[string]diag.Span{
					"a": explicitOrigin,
				},
			},
			{
				Name: "sorted",
				Span: bindingSpan,
				Vars: map[string][]eval.Value{
					"z": {eval.Int(1)},
					"m": {eval.Int(2)},
				},
			},
		},
	}

	got := buildWarningSources(res)
	if len(got) != 2 {
		t.Fatalf("expected 2 warning sources, got %#v", got)
	}
	if got[0].Name != "table" || !reflect.DeepEqual(got[0].Order, []string{"b", "a"}) {
		t.Fatalf("unexpected first warning source: %#v", got[0])
	}
	if got[0].VarOrigins["b"] != bindingSpan {
		t.Fatalf("expected missing origin to fall back to binding span, got %+v", got[0].VarOrigins["b"])
	}
	if got[0].VarOrigins["a"] != explicitOrigin {
		t.Fatalf("expected explicit origin to be preserved, got %+v", got[0].VarOrigins["a"])
	}
	if got[1].Name != "sorted" || !reflect.DeepEqual(got[1].Order, []string{"m", "z"}) {
		t.Fatalf("expected sorted fallback order, got %#v", got[1])
	}
}

func TestWarningCatalogDedupesSameVersionSnapshots(t *testing.T) {
	base := &GlobalBinding{
		Name:       "cases",
		PublicName: "cases",
		VersionID:  "v1",
		Order:      []string{"x"},
		Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
	}
	snap := &GlobalBinding{
		Name:       "_js__1__cases",
		PublicName: "cases",
		VersionID:  "v1",
		Order:      []string{"x"},
		Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
	}
	catalog := buildWarningCatalog(&Result{
		Bindings: []*GlobalBinding{base},
		ScopeSnapshotsByIndex: map[int]*ScopeSnapshot{
			1: {Bindings: []*GlobalBinding{snap}, BindingsByName: map[string]*GlobalBinding{"cases": snap, snap.Name: snap}},
		},
	})

	if got := catalog.sources(); len(got) != 1 {
		t.Fatalf("expected same version to be deduped, got %#v", got)
	}
}

func TestWarningCatalogKeepsReboundPublicNameVersions(t *testing.T) {
	first := &GlobalBinding{
		Name:       "_js__1__cases",
		PublicName: "cases",
		VersionID:  "v1",
		Order:      []string{"x"},
		Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
	}
	second := &GlobalBinding{
		Name:       "cases",
		PublicName: "cases",
		VersionID:  "v2",
		Order:      []string{"a"},
		Vars:       map[string][]eval.Value{"a": {eval.Int(2)}},
	}
	catalog := buildWarningCatalog(&Result{Bindings: []*GlobalBinding{first, second}})

	if got := catalog.sources(); len(got) != 2 {
		t.Fatalf("expected rebound public name to produce two source keys, got %#v", got)
	}
	if catalog.byKey[BindingVersionKey{Public: "cases", Version: "v1"}] == nil {
		t.Fatalf("expected first cases version in catalog")
	}
	if catalog.byKey[BindingVersionKey{Public: "cases", Version: "v2"}] == nil {
		t.Fatalf("expected second cases version in catalog")
	}
}

func TestWarningCatalogResolvesPublicNamesThroughSnapshotBindings(t *testing.T) {
	first := &GlobalBinding{
		Name:       "_js__1__cases",
		PublicName: "cases",
		VersionID:  "v1",
		Order:      []string{"x"},
		Vars:       map[string][]eval.Value{"x": {eval.Int(1)}},
	}
	second := &GlobalBinding{
		Name:       "cases",
		PublicName: "cases",
		VersionID:  "v2",
		Order:      []string{"a"},
		Vars:       map[string][]eval.Value{"a": {eval.Int(2)}},
	}
	catalog := buildWarningCatalog(&Result{
		Bindings: []*GlobalBinding{first, second},
		BindingsByName: map[string]*GlobalBinding{
			"cases": second,
		},
	})
	snapshotBindings := map[string]*GlobalBinding{
		"cases": first,
	}

	if got, want := catalog.keyForSource(snapshotBindings, "cases"), (BindingVersionKey{Public: "cases", Version: "v1"}); got != want {
		t.Fatalf("expected snapshot public name to resolve to old version, got %#v want %#v", got, want)
	}
	if got, want := catalog.keyForSource(nil, "_js__1__cases"), (BindingVersionKey{Public: "cases", Version: "v1"}); got != want {
		t.Fatalf("expected exact synthetic name to resolve through catalog, got %#v want %#v", got, want)
	}
}

func TestBuildGlobalSourceDepsDedupesSkipsAndSorts(t *testing.T) {
	key := func(name string) BindingVersionKey {
		return BindingVersionKey{Public: name, Version: name}
	}
	res := &Result{
		Bindings: []*GlobalBinding{
			{Name: "a", Order: []string{"a"}, Vars: map[string][]eval.Value{"a": {eval.Int(1)}}},
			{Name: "b", Order: []string{"b"}, Vars: map[string][]eval.Value{"b": {eval.Int(1)}}},
			{Name: "params", Order: []string{"a", "b"}, Vars: map[string][]eval.Value{"a": {eval.Int(1)}, "b": {eval.Int(2)}}, DependsOn: []string{"b", "a", "b", "", "params", "outside"}},
			{Name: "derived", Order: []string{"v"}, Vars: map[string][]eval.Value{"v": {eval.Int(1)}}, DependsOn: []string{"params", "a"}},
			{Name: "self", Order: []string{"v"}, Vars: map[string][]eval.Value{"v": {eval.Int(1)}}, DependsOn: []string{"self"}},
			{Name: "hidden", DependsOn: []string{"a"}},
			{Name: ""},
		},
	}

	got := buildGlobalSourceDeps(buildWarningCatalog(res))
	want := map[BindingVersionKey][]BindingVersionKey{
		key("derived"): {key("a"), key("params")},
		key("params"):  {key("a"), key("b")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deps: got=%#v want=%#v", got, want)
	}
}

func TestCloneUsedBySourceDeepCopiesMaps(t *testing.T) {
	alpha := BindingVersionKey{Public: "alpha", Version: "alpha"}
	beta := BindingVersionKey{Public: "beta", Version: "beta"}
	used := usedBySource{
		alpha: {"x": true},
		beta:  {},
	}

	clone := cloneUsedBySource(used)
	if len(clone) != 2 || clone[beta] == nil {
		t.Fatalf("expected clone to preserve keys and create empty map, got %#v", clone)
	}

	clone[alpha]["x"] = false
	clone[alpha]["y"] = true
	clone[beta]["z"] = true
	if used[alpha]["x"] != true || used[alpha]["y"] || len(used[beta]) != 0 {
		t.Fatalf("mutating clone should not affect source: source=%#v clone=%#v", used, clone)
	}
}

func TestPropagateUsedByGlobalDepsMarksDependenciesAndStaysCycleSafe(t *testing.T) {
	key := func(name string) BindingVersionKey {
		return BindingVersionKey{Public: name, Version: name}
	}
	used := usedBySource{
		key("params"): {"row": true},
	}
	catalog := buildWarningCatalog(&Result{Bindings: []*GlobalBinding{
		{Name: "params", Order: []string{"row"}, Vars: map[string][]eval.Value{"row": {eval.Int(1)}}},
		{Name: "mid", Order: []string{"m1", "m2"}, Vars: map[string][]eval.Value{"m1": {eval.Int(1)}, "m2": {eval.Int(2)}}},
		{Name: "leaf", Order: []string{"x"}, Vars: map[string][]eval.Value{"x": {eval.Int(1)}}},
		{Name: "empty"},
	}})
	deps := map[BindingVersionKey][]BindingVersionKey{
		key("params"): {key("mid"), key("leaf")},
		key("mid"):    {key("leaf"), key("params")},
		key("leaf"):   {key("empty")},
	}

	propagateUsedByGlobalDeps(used, catalog, deps)

	want := usedBySource{
		key("params"): {"row": true},
		key("mid"):    {"m1": true, "m2": true},
		key("leaf"):   {"x": true},
	}
	if !reflect.DeepEqual(used, want) {
		t.Fatalf("unexpected propagated usage: got=%#v want=%#v", used, want)
	}
}
