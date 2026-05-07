package sema

import (
	"reflect"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestScopeNameCatalogHelpers(t *testing.T) {
	ns := &Namespace{
		Name:     "mod",
		Members:  []string{"mod.value", "mod.child.value", "mod.value"},
		Bindings: []string{"mod.value", "mod.child.value", "mod.value"},
		Steps:    []string{"mod.run"},
	}
	if got := directNamespaceMembers("mod", ns); !reflect.DeepEqual(got, []string{"value"}) {
		t.Fatalf("unexpected direct namespace members: %#v", got)
	}

	envNames := visibleNamesFromEnv(map[string]eval.Value{
		"a":          eval.Int(1),
		"mod.value":  eval.Int(2),
		"  ":         eval.Int(3),
		"local_name": eval.Int(4),
	})
	if !reflect.DeepEqual(eval.NewNameCatalog(envNames, nil).Visible, []string{"a", "local_name"}) {
		t.Fatalf("unexpected visible env names: %#v", envNames)
	}

	catalog := scopeNameCatalog(
		[]string{"z", "a", "a"},
		map[string]*Namespace{
			"mod":       ns,
			"mod.child": {Name: "mod.child", Members: []string{"mod.child.value", "mod.child.other"}, Bindings: []string{"mod.child.value", "mod.child.other"}},
		},
	)
	if !reflect.DeepEqual(catalog.Visible, []string{"a", "z"}) {
		t.Fatalf("unexpected scope catalog visible names: %#v", catalog.Visible)
	}
	if !reflect.DeepEqual(catalog.Namespaces["mod"].Members, []string{"value"}) {
		t.Fatalf("unexpected mod namespace catalog: %#v", catalog.Namespaces["mod"])
	}
	if !reflect.DeepEqual(catalog.Namespaces["mod.child"].Members, []string{"other", "value"}) {
		t.Fatalf("unexpected nested namespace catalog: %#v", catalog.Namespaces["mod.child"])
	}
}

func TestCloneAndMergeVisibleNamespaces(t *testing.T) {
	span := diag.NewSpan("catalog.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	original := map[string]*Namespace{
		"mod": {Name: "mod", Members: []string{"mod.value"}, Bindings: []string{"mod.value"}, Steps: []string{"mod.run"}},
	}
	cloned := cloneVisibleNamespaces(original)
	cloned["mod"].Members[0] = "mod.changed"
	if original["mod"].Members[0] != "mod.value" {
		t.Fatalf("expected cloneVisibleNamespaces to deep-copy members")
	}

	merged := mergeVisibleNamespaces(
		map[string]*Namespace{
			"mod": {Name: "mod", Members: []string{"mod.value"}, Bindings: []string{"mod.value"}, Steps: []string{"mod.run"}},
		},
		map[string]*Namespace{
			"mod":       {Name: "mod", Members: []string{"mod.other"}, Bindings: []string{"mod.other"}, Steps: []string{"mod.extra"}},
			"mod.child": {Name: "mod.child", Members: []string{"mod.child.value"}, Bindings: []string{"mod.child.value"}, Steps: []string{span.File}},
		},
	)
	if !reflect.DeepEqual(merged["mod"].Members, []string{"mod.value", "mod.other"}) {
		t.Fatalf("unexpected merged members: %#v", merged["mod"])
	}
	if !reflect.DeepEqual(merged["mod"].Bindings, []string{"mod.value", "mod.other"}) {
		t.Fatalf("unexpected merged bindings: %#v", merged["mod"])
	}
	if !reflect.DeepEqual(merged["mod"].Steps, []string{"mod.run", "mod.extra"}) {
		t.Fatalf("unexpected merged steps: %#v", merged["mod"])
	}
	if merged["mod.child"] == nil {
		t.Fatalf("expected merged child namespace")
	}
}
