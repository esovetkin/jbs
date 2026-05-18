package cli

import (
	"io"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/sema"
)

func TestRunNoArgsDispatchesToRepl(t *testing.T) {
	orig := runReplFn
	t.Cleanup(func() {
		runReplFn = orig
	})

	called := false
	runReplFn = func(stdout, stderr io.Writer) int {
		called = true
		return 17
	}

	var out, err strings.Builder
	code := Run(nil, &out, &err)
	if code != 17 {
		t.Fatalf("unexpected exit code: got=%d want=%d", code, 17)
	}
	if !called {
		t.Fatalf("expected repl dispatcher to be called")
	}
}

func TestRunReplCommandDispatchesToRepl(t *testing.T) {
	orig := runReplFn
	t.Cleanup(func() {
		runReplFn = orig
	})

	called := false
	runReplFn = func(stdout, stderr io.Writer) int {
		called = true
		return 5
	}

	var out, err strings.Builder
	code := Run([]string{"repl"}, &out, &err)
	if code != 5 {
		t.Fatalf("unexpected exit code: got=%d want=%d", code, 5)
	}
	if !called {
		t.Fatalf("expected repl dispatcher to be called")
	}
}

func TestReplCompletionNamesIncludeGlobals(t *testing.T) {
	if got := replCompletionNames(nil); got != nil {
		t.Fatalf("nil result completions = %#v", got)
	}

	names := replCompletionNames(&sema.Result{
		Globals: sema.GlobalState{Values: map[string]eval.Value{
			"":         eval.Int(0),
			"mod.name": eval.Int(1),
			"b":        eval.Int(2),
			"a":        eval.Int(3),
		}},
	})
	if !slices.Equal(names, []string{"a", "b"}) {
		t.Fatalf("completion names = %#v", names)
	}
}
