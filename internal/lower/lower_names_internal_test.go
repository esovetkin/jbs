package lower

import (
	"reflect"
	"testing"
)

func TestUniqueName(t *testing.T) {
	ctx := &lowerContext{
		names: map[string]struct{}{
			"taken":   {},
			"taken_1": {},
		},
	}

	if got := ctx.uniqueName("fresh"); got != "fresh" {
		t.Fatalf("expected fresh name, got %q", got)
	}
	if got := ctx.uniqueName("taken"); got != "taken_2" {
		t.Fatalf("expected suffix increment to skip existing names, got %q", got)
	}
	if got := ctx.uniqueName("taken"); got != "taken_3" {
		t.Fatalf("expected next suffix on repeated request, got %q", got)
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "x"},
		{in: "abc_123", want: "abc_123"},
		{in: "a-b.c", want: "a_b_c"},
		{in: "!!", want: "__"},
		{in: "äöß", want: "äöß"},
	}
	for _, tt := range tests {
		if got := sanitize(tt.in); got != tt.want {
			t.Fatalf("sanitize(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSubsetEmittedName(t *testing.T) {
	if got := subsetEmittedName(subsetVarSpec{Visible: "a", Emitted: "b"}); got != "b" {
		t.Fatalf("expected emitted alias to win, got %q", got)
	}
	if got := subsetEmittedName(subsetVarSpec{Visible: "a", Emitted: ""}); got != "a" {
		t.Fatalf("expected visible name fallback, got %q", got)
	}
}

func TestApplyEmittedNames(t *testing.T) {
	base := []Parameter{
		{Name: "a", Value: "1"},
		{Name: "b", Value: "2"},
	}

	if got := applyEmittedNames(nil, map[string]string{"a": "_ja__a"}); got != nil {
		t.Fatalf("expected nil passthrough for nil params, got %#v", got)
	}

	got := applyEmittedNames(base, map[string]string{
		"a": "_ja__a",
		"c": "_ja__c",
	})
	want := []Parameter{
		{Name: "_ja__a", Value: "1"},
		{Name: "b", Value: "2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected emitted-name application: got=%#v want=%#v", got, want)
	}
	if base[0].Name != "a" {
		t.Fatalf("applyEmittedNames must not mutate input slice, got %#v", base)
	}

	got = applyEmittedNames(base, map[string]string{})
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("expected identity when alias map empty, got=%#v want=%#v", got, base)
	}
}

func TestShortNameBuilders(t *testing.T) {
	if got := indexVariableName("ctx-1"); got != "_ji_ctx_1" {
		t.Fatalf("unexpected indexVariableName: %q", got)
	}
	if got := shortParamIndexName(""); got != "_ji_x" {
		t.Fatalf("unexpected shortParamIndexName for empty context: %q", got)
	}
	if got := shortSubsetBaseName("step-1", "src.mod", []string{"a", "b"}); got != "_js__step_1__src_mod__a_b" {
		t.Fatalf("unexpected shortSubsetBaseName: %q", got)
	}
	if got := shortSubsetIndexName("step-1", "src.mod", []string{"a", "b"}); got != "_ji__step_1__src_mod__a_b" {
		t.Fatalf("unexpected shortSubsetIndexName: %q", got)
	}
	if got := shortSubsetRowsName("step-1", "src.mod", []string{"a", "b"}); got != "_jr__step_1__src_mod__a_b" {
		t.Fatalf("unexpected shortSubsetRowsName: %q", got)
	}
	if got := shortPatternAliasName("g-1", "pat.2", "step-3", "name.4"); got != "_jp__g_1_pat_2__step_3__name_4" {
		t.Fatalf("unexpected shortPatternAliasName: %q", got)
	}
}
