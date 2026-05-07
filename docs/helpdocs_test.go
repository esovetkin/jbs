package helpdocs

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func TestTopicsResolveToNonEmptyPages(t *testing.T) {
	for _, topic := range Topics() {
		text, err := Page(topic)
		if err != nil {
			t.Fatalf("topic %q did not resolve: %v", topic, err)
		}
		if text == "" {
			t.Fatalf("topic %q returned empty page", topic)
		}
	}
}

func TestUnknownTopicReturnsError(t *testing.T) {
	if _, err := Page("template"); err == nil {
		t.Fatalf("expected unknown topic to return error")
	}
}

func TestFunctionsHelpMentionsExplicitTableOperations(t *testing.T) {
	text, err := Page("functions")
	if err != nil {
		t.Fatalf("functions topic did not resolve: %v", err)
	}
	if !strings.Contains(text, "table(") || !strings.Contains(text, "product(") || !strings.Contains(text, "select(") {
		t.Fatalf("expected functions help to describe the explicit table API, got:\n%s", text)
	}
}

func TestFunctionHelpPagesResolve(t *testing.T) {
	for _, name := range FunctionNames() {
		text, err := FunctionPage(name)
		if err != nil {
			t.Fatalf("function help %q did not resolve: %v", name, err)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("function help %q is empty", name)
		}
	}
}

func TestFunctionHelpPagesHaveRequiredSections(t *testing.T) {
	for _, name := range FunctionNames() {
		text, err := FunctionPage(name)
		if err != nil {
			t.Fatalf("function help %q did not resolve: %v", name, err)
		}
		for _, section := range []string{"# `", "## Arguments", "## Returns", "## Example", "```jbs"} {
			if !strings.Contains(text, section) {
				t.Fatalf("function help %q missing %q:\n%s", name, section, text)
			}
		}
	}
}

func TestFunctionHelpCoversEvaluatorBuiltins(t *testing.T) {
	for _, name := range eval.BuiltinCallNames() {
		if _, err := FunctionPage(name); err != nil {
			t.Fatalf("missing help for builtin %q: %v", name, err)
		}
	}
}

func TestFunctionHelpAliasT(t *testing.T) {
	text, err := FunctionPage("t")
	if err != nil {
		t.Fatalf("expected t help to resolve: %v", err)
	}
	if !strings.Contains(text, "Alias of `table(...)`") {
		t.Fatalf("expected t help to mention table alias, got:\n%s", text)
	}
}

func TestFunctionNamesSortedAndUnique(t *testing.T) {
	names := FunctionNames()
	if !slices.IsSorted(names) {
		t.Fatalf("expected sorted function names, got %#v", names)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			t.Fatalf("duplicate function name %q in %#v", name, names)
		}
		seen[name] = struct{}{}
	}
}

func TestUnknownFunctionHelpReturnsError(t *testing.T) {
	if _, err := FunctionPage("missing"); err == nil {
		t.Fatalf("expected missing function help to fail")
	}
	if _, err := FunctionPage(""); err == nil {
		t.Fatalf("expected empty function help to fail")
	}
}

func TestHelpTopicsRemainStable(t *testing.T) {
	want := []string{"analyse", "do", "functions", "globals", "repl", "use"}
	if got := Topics(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected help topics: got=%#v want=%#v", got, want)
	}
	if _, err := Page("functions"); err != nil {
		t.Fatalf("functions topic did not resolve: %v", err)
	}
}
