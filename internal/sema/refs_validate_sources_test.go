package sema

import (
	"reflect"
	"testing"

	"jbs/internal/diag"
	"jbs/internal/eval"
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

func TestBuildGlobalSourceDepsDedupesSkipsAndSorts(t *testing.T) {
	res := &Result{
		GlobalVarOrder: []string{"a", "b", "params", "derived", "self", "hidden", "empty", "missing"},
		GlobalVarByName: map[string]*GlobalVar{
			"a":       {Name: "a"},
			"b":       {Name: "b"},
			"params":  {Name: "params", DependsOn: []string{"b", "a", "b", "", "params", "outside"}},
			"derived": {Name: "derived", DependsOn: []string{"params", "a"}},
			"self":    {Name: "self", DependsOn: []string{"self"}},
			"hidden":  {Name: "hidden", DependsOn: []string{"a"}},
			"empty":   {Name: ""},
		},
	}
	exposed := map[string]map[string]diag.Span{
		"a":       {"a": {}},
		"b":       {"b": {}},
		"params":  {"a": {}, "b": {}},
		"derived": {"v": {}},
		"self":    {"v": {}},
	}

	got := buildGlobalSourceDeps(res, exposed)
	want := map[string][]string{
		"derived": {"a", "params"},
		"params":  {"a", "b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deps: got=%#v want=%#v", got, want)
	}
}

func TestCloneUsedBySourceDeepCopiesMaps(t *testing.T) {
	used := map[string]map[string]bool{
		"alpha": {"x": true},
		"beta":  {},
	}

	clone := cloneUsedBySource(used)
	if len(clone) != 2 || clone["beta"] == nil {
		t.Fatalf("expected clone to preserve keys and create empty map, got %#v", clone)
	}

	clone["alpha"]["x"] = false
	clone["alpha"]["y"] = true
	clone["beta"]["z"] = true
	if used["alpha"]["x"] != true || used["alpha"]["y"] || len(used["beta"]) != 0 {
		t.Fatalf("mutating clone should not affect source: source=%#v clone=%#v", used, clone)
	}
}

func TestPropagateUsedByGlobalDepsMarksDependenciesAndStaysCycleSafe(t *testing.T) {
	used := map[string]map[string]bool{
		"params": {"row": true},
	}
	exposed := map[string]map[string]diag.Span{
		"params": {"row": {}},
		"mid":    {"m1": {}, "m2": {}},
		"leaf":   {"x": {}},
		"empty":  {},
	}
	deps := map[string][]string{
		"params": {"mid", "leaf"},
		"mid":    {"leaf", "params"},
		"leaf":   {"empty"},
	}

	propagateUsedByGlobalDeps(used, exposed, deps)

	want := map[string]map[string]bool{
		"params": {"row": true},
		"mid":    {"m1": true, "m2": true},
		"leaf":   {"x": true},
	}
	if !reflect.DeepEqual(used, want) {
		t.Fatalf("unexpected propagated usage: got=%#v want=%#v", used, want)
	}
}

func TestMarkSubmitUseBindingRefsForDirectBindingsAndNamespaces(t *testing.T) {
	res := &Result{
		BindingsByName: map[string]*GlobalBinding{
			"direct": {
				Name:  "direct",
				Shape: BindingScalar,
				Order: []string{"b", "a"},
				Vars: map[string][]eval.Value{
					"a": {eval.Int(1)},
					"b": {eval.Int(2)},
				},
			},
			"ns.valid": {
				Name:  "ns.valid",
				Shape: BindingScalar,
				Order: []string{"x"},
				Vars: map[string][]eval.Value{
					"x": {eval.String("v")},
				},
			},
			"ns.table": {
				Name:  "ns.table",
				Shape: BindingTable,
				Order: []string{"y"},
				Vars: map[string][]eval.Value{
					"y": {eval.String("v")},
				},
			},
			"ns.deep.child": {
				Name:  "ns.deep.child",
				Shape: BindingScalar,
				Order: []string{"z"},
				Vars: map[string][]eval.Value{
					"z": {eval.String("v")},
				},
			},
		},
		Namespaces: map[string]*Namespace{
			"ns": {
				Name:     "ns",
				Bindings: []string{"ns.valid", "ns.table", "ns.deep.child"},
			},
		},
	}

	calls := make([]string, 0)
	mark := func(source, name string) {
		calls = append(calls, source+":"+name)
	}

	markSubmitUseBindingRefs(res, "direct", mark)
	if !reflect.DeepEqual(calls, []string{"direct:b", "direct:a"}) {
		t.Fatalf("unexpected direct binding marks: %#v", calls)
	}

	calls = calls[:0]
	markSubmitUseBindingRefs(res, "ns", mark)
	if !reflect.DeepEqual(calls, []string{"ns.valid:x"}) {
		t.Fatalf("unexpected namespace marks: %#v", calls)
	}

	calls = calls[:0]
	markSubmitUseBindingRefs(res, "missing", mark)
	if len(calls) != 0 {
		t.Fatalf("expected missing source to produce no marks, got %#v", calls)
	}
}
