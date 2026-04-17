package cli

import (
	"io"
	"strings"
	"testing"
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
