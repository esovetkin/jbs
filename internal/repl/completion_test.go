package repl

import (
	"reflect"
	"testing"
)

func TestSymbolCompleterCompletesBuiltinPrefix(t *testing.T) {
	c := newSymbolCompleter([]string{"range", "read_csv"}, nil)
	got, off := c.Do([]rune("ra"), 2)
	if off != 2 || !reflect.DeepEqual(runeSlicesToStrings(got), []string{"nge"}) {
		t.Fatalf("completion = %#v off=%d", got, off)
	}
}

func TestSymbolCompleterUpdatesGlobals(t *testing.T) {
	c := newSymbolCompleter([]string{"range"}, []string{"alpha"})
	c.SetGlobals([]string{"beta"})

	got, off := c.Do([]rune("be"), 2)
	if off != 2 || !reflect.DeepEqual(runeSlicesToStrings(got), []string{"ta"}) {
		t.Fatalf("completion = %#v off=%d", got, off)
	}
}

func TestSymbolCompleterDeduplicatesShadowedBuiltin(t *testing.T) {
	c := newSymbolCompleter([]string{"sum"}, []string{"sum"})
	got, off := c.Do([]rune("su"), 2)
	if off != 2 || !reflect.DeepEqual(runeSlicesToStrings(got), []string{"m"}) {
		t.Fatalf("completion = %#v off=%d", got, off)
	}
}

func TestSymbolCompleterSkipsInvalidContexts(t *testing.T) {
	c := newSymbolCompleter([]string{"range"}, nil)
	cases := []string{`"ra`, `# ra`, `lib.ra`, `lib.`}
	for _, line := range cases {
		got, off := c.Do([]rune(line), len([]rune(line)))
		if len(got) != 0 || off != 0 {
			t.Fatalf("%q completion = %#v off=%d", line, got, off)
		}
	}
}

func TestSymbolCompleterCompletesExpressionFragment(t *testing.T) {
	c := newSymbolCompleter([]string{"range"}, nil)
	line := []rune("foo + ra")
	got, off := c.Do(line, len(line))
	if off != 2 || !reflect.DeepEqual(runeSlicesToStrings(got), []string{"nge"}) {
		t.Fatalf("completion = %#v off=%d", got, off)
	}
}

func TestSymbolCompleterRejectsDigitStartedFragment(t *testing.T) {
	c := newSymbolCompleter([]string{"range"}, []string{"r2"})
	got, off := c.Do([]rune("1r"), 2)
	if len(got) != 0 || off != 0 {
		t.Fatalf("completion = %#v off=%d", got, off)
	}
}

func runeSlicesToStrings(in [][]rune) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		out = append(out, string(item))
	}
	return out
}
