package sema

import (
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestGlobalBindingSupports(t *testing.T) {
	var nilBinding *GlobalBinding
	if nilBinding.Supports(ImportIntoStep) {
		t.Fatalf("nil binding should not support step imports")
	}
	if nilBinding.Supports(ImportIntoAnalyse) {
		t.Fatalf("nil binding should not support analyse imports")
	}
	if got := nilBinding.SupportIssue(ImportIntoAnalyse); got != DisallowedBindingNotData {
		t.Fatalf("nil binding should report not-data, got %v", got)
	}

	scalarString := &GlobalBinding{
		Value: eval.String("ok"),
		Shape: BindingScalar,
		Order: []string{"value"},
		Vars: map[string][]eval.Value{
			"value": {eval.String("ok")},
		},
	}
	scalarEmpty := &GlobalBinding{
		Shape: BindingScalar,
		Order: []string{"value"},
		Vars: map[string][]eval.Value{
			"value": {},
		},
	}
	scalarNumber := &GlobalBinding{
		Value: eval.Int(1),
		Shape: BindingScalar,
		Order: []string{"value"},
		Vars: map[string][]eval.Value{
			"value": {eval.Int(1)},
		},
	}
	scalarMultiColumn := &GlobalBinding{
		Value: eval.Tuple([]eval.Value{eval.String("x"), eval.String("y")}),
		Shape: BindingScalar,
		Order: []string{"a", "b"},
		Vars: map[string][]eval.Value{
			"a": {eval.String("x")},
			"b": {eval.String("y")},
		},
	}
	tableBinding := &GlobalBinding{
		Value: tableValueFromVars([]string{"value"}, map[string][]eval.Value{
			"value": {eval.String("ok")},
		}),
		Shape: BindingTable,
		Order: []string{"value"},
		Vars: map[string][]eval.Value{
			"value": {eval.String("ok")},
		},
	}

	if !scalarString.Supports(ImportIntoStep) {
		t.Fatalf("scalar binding should support step imports")
	}
	if !tableBinding.Supports(ImportIntoStep) {
		t.Fatalf("table binding should support step imports")
	}
	if !scalarString.Supports(ImportIntoAnalyse) {
		t.Fatalf("single-column string scalar should support analyse imports")
	}
	if scalarEmpty.Supports(ImportIntoAnalyse) {
		t.Fatalf("empty binding without a raw string value should not support analyse imports")
	}
	if tableBinding.Supports(ImportIntoAnalyse) {
		t.Fatalf("table binding should not support analyse imports")
	}
	if got := tableBinding.SupportIssue(ImportIntoAnalyse); got != DisallowedBindingAnalyseTable {
		t.Fatalf("table binding should report table reason, got %v", got)
	}
	if scalarMultiColumn.Supports(ImportIntoAnalyse) {
		t.Fatalf("multi-column scalar should not support analyse imports")
	}
	if got := scalarMultiColumn.SupportIssue(ImportIntoAnalyse); got != DisallowedBindingAnalyseNonString {
		t.Fatalf("tuple-valued scalar should report non-string reason, got %v", got)
	}
	if scalarNumber.Supports(ImportIntoAnalyse) {
		t.Fatalf("non-string scalar should not support analyse imports")
	}
	if got := scalarNumber.SupportIssue(ImportIntoAnalyse); got != DisallowedBindingAnalyseNonString {
		t.Fatalf("non-string scalar should report non-string reason, got %v", got)
	}
	if scalarString.Supports(ImportContext("unknown")) {
		t.Fatalf("unknown import context should not be supported")
	}
	if got := scalarString.SupportIssue(ImportContext("unknown")); got != DisallowedBindingNotData {
		t.Fatalf("unknown import context should report not-data, got %v", got)
	}
}

func TestBindingVersionKeyForBinding(t *testing.T) {
	binding := &GlobalBinding{
		Name:       "cases",
		PublicName: "cases",
		VersionID:  "v1",
	}
	got := BindingVersionKeyForBinding(binding, "fallback")
	want := BindingVersionKey{Public: "cases", Version: "v1"}
	if got != want {
		t.Fatalf("unexpected binding key: got=%#v want=%#v", got, want)
	}
	if got.Display() != "cases" {
		t.Fatalf("unexpected display name: %q", got.Display())
	}
}

func TestBindingVersionKeyFallsBackToSpan(t *testing.T) {
	span := diag.NewSpan("input.jbs", diag.NewPos(10, 2, 3), diag.NewPos(20, 2, 13))
	got := BindingVersionKeyForBinding(&GlobalBinding{
		Name: "cases",
		Span: span,
	}, "fallback")
	want := BindingVersionKey{Public: "cases", Version: "input.jbs:10:20"}
	if got != want {
		t.Fatalf("unexpected span fallback key: got=%#v want=%#v", got, want)
	}
}

func TestBindingVersionKeyForMissingSource(t *testing.T) {
	got := BindingVersionKeyForSource(map[string]*GlobalBinding{}, "missing")
	want := BindingVersionKey{Public: "missing", Version: "missing"}
	if got != want {
		t.Fatalf("unexpected missing source key: got=%#v want=%#v", got, want)
	}

	got = BindingVersionKeyForBinding(nil, "fallback")
	want = BindingVersionKey{Public: "fallback", Version: "fallback"}
	if got != want {
		t.Fatalf("unexpected nil binding key: got=%#v want=%#v", got, want)
	}
}
