package eval

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestEvalShellCallSuccessAndStrip(t *testing.T) {
	calls := make([]ShellCommand, 0)
	runner := func(spec ShellCommand) ([]byte, error) {
		calls = append(calls, spec)
		return []byte("value\n"), nil
	}
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "printf value"}), nil, diags, ExprOptions{
		Files:       &FileAccess{BaseDir: "/tmp/jbs-shell-test"},
		ShellRunner: runner,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindString || got.S != "value" {
		t.Fatalf("unexpected shell result: %#v", got)
	}
	if len(calls) != 1 || calls[0].Command != "printf value" || calls[0].Dir != "/tmp/jbs-shell-test" {
		t.Fatalf("unexpected shell call: %#v", calls)
	}
}

func TestEvalShellCallStripFalse(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "shell", Span: spanAt(701, 1)},
		Args: []ast.CallArg{
			ast.PosCallArg(ast.StringExpr{Value: "printf value"}),
			{Name: "strip", Expr: ast.BoolExpr{Value: false, Span: spanAt(701, 20)}, Span: spanAt(701, 20)},
		},
		Span: spanAt(701, 1),
	}, nil, diags, ExprOptions{
		ShellRunner: func(ShellCommand) ([]byte, error) {
			return []byte("value\r\n"), nil
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindString || got.S != "value\r\n" {
		t.Fatalf("unexpected raw shell result: %#v", got)
	}
}

func TestEvalShellCallNamedCommand(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "shell", Span: spanAt(701, 40)},
		Args: []ast.CallArg{
			namedArg("command", ast.StringExpr{Value: "printf value"}),
			namedArg("strip", ast.BoolExpr{Value: true}),
		},
		Span: spanAt(701, 40),
	}, nil, diags, ExprOptions{
		ShellRunner: func(ShellCommand) ([]byte, error) {
			return []byte("value\n"), nil
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindString || got.S != "value" {
		t.Fatalf("unexpected named shell result: %#v", got)
	}
}

func TestEvalShellCallArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{
			name: "missing command",
			expr: shellCall(),
		},
		{
			name: "non string command",
			expr: shellCall(ast.NumberExpr{Int: true, IntValue: 1, Span: spanAt(702, 1)}),
		},
		{
			name: "unknown named",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "shell", Span: spanAt(703, 1)},
				Args: []ast.CallArg{
					ast.PosCallArg(ast.StringExpr{Value: "true", Span: spanAt(703, 7)}),
					{Name: "raw", Expr: ast.BoolExpr{Value: true, Span: spanAt(703, 20)}, Span: spanAt(703, 20)},
				},
				Span: spanAt(703, 1),
			},
		},
		{
			name: "non bool strip",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "shell", Span: spanAt(704, 1)},
				Args: []ast.CallArg{
					ast.PosCallArg(ast.StringExpr{Value: "true", Span: spanAt(704, 7)}),
					{Name: "strip", Expr: ast.StringExpr{Value: "no", Span: spanAt(704, 20)}, Span: spanAt(704, 20)},
				},
				Span: spanAt(704, 1),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{
				ShellRunner: func(ShellCommand) ([]byte, error) {
					t.Fatal("shell runner should not be called")
					return nil, nil
				},
			})
			if got.Kind != KindNull {
				t.Fatalf("expected null, got %#v", got)
			}
			if diagCount(diags, "E106") == 0 {
				t.Fatalf("expected E106, got: %s", diags.String())
			}
		})
	}
}

func TestEvalShellCallEnvironmentCapture(t *testing.T) {
	t.Setenv("x", "from-os")
	t.Setenv("JBS_SHELL_OS_ONLY", "keep")
	var call ShellCommand
	uses := make([]ShellUseEvent, 0)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "echo $x $JBS_SHELL_OS_ONLY"}), map[string]Value{
		"x": Int(42),
	}, diags, ExprOptions{
		ShellRunner: func(spec ShellCommand) ([]byte, error) {
			call = spec
			return []byte("ok\n"), nil
		},
		ShellUse: func(event ShellUseEvent) {
			uses = append(uses, event)
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindString || got.S != "ok" {
		t.Fatalf("unexpected shell result: %#v", got)
	}
	env := envMap(call.Env)
	if env["x"] != "42" {
		t.Fatalf("expected JBS x override, got env x=%q", env["x"])
	}
	if env["JBS_SHELL_OS_ONLY"] != "keep" {
		t.Fatalf("expected OS env passthrough, got %#v", env)
	}
	if len(uses) != 1 || uses[0].Name != "x" || !uses[0].Scalar {
		t.Fatalf("unexpected shell use events: %#v", uses)
	}
}

func TestEvalShellCallUnassignedNamePassesThrough(t *testing.T) {
	t.Setenv("x", "from-os")
	frame := NewRootFrame(nil)
	frame.DeclareLocal("x")
	var call ShellCommand
	uses := make([]ShellUseEvent, 0)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "echo $x"}), nil, diags, ExprOptions{
		Frame: frame,
		ShellRunner: func(spec ShellCommand) ([]byte, error) {
			call = spec
			return []byte("from-os\n"), nil
		},
		ShellUse: func(event ShellUseEvent) {
			uses = append(uses, event)
		},
	})
	if got.Kind != KindString || got.S != "from-os" {
		t.Fatalf("unexpected shell result: %#v", got)
	}
	if diagCount(diags, "E100") != 0 || diagCount(diags, "W103") != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if env := envMap(call.Env); env["x"] != "from-os" {
		t.Fatalf("expected OS env to remain visible, got x=%q", env["x"])
	}
	if len(uses) != 0 {
		t.Fatalf("unassigned shell name should not emit use event, got %#v", uses)
	}
}

func TestEvalShellCallNonScalarWarningAndEmptyOverride(t *testing.T) {
	t.Setenv("x", "from-os")
	var call ShellCommand
	uses := make([]ShellUseEvent, 0)
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "echo $x"}), map[string]Value{
		"x": List([]Value{Int(1)}),
	}, diags, ExprOptions{
		ShellRunner: func(spec ShellCommand) ([]byte, error) {
			call = spec
			return []byte("\n"), nil
		},
		ShellUse: func(event ShellUseEvent) {
			uses = append(uses, event)
		},
	})
	if got.Kind != KindString || got.S != "" {
		t.Fatalf("unexpected shell result: %#v", got)
	}
	if diagCount(diags, "W103") != 1 || diags.HasErrors() {
		t.Fatalf("expected one W103 warning only, got: %s", diags.String())
	}
	if env := envMap(call.Env); env["x"] != "" {
		t.Fatalf("expected non-scalar empty override, got x=%q", env["x"])
	}
	if len(uses) != 1 || uses[0].Name != "x" || uses[0].Scalar {
		t.Fatalf("unexpected shell use events: %#v", uses)
	}
}

func TestEvalShellCallRunnerErrors(t *testing.T) {
	t.Run("start failure", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "bad"}), nil, diags, ExprOptions{
			ShellRunner: func(ShellCommand) ([]byte, error) {
				return nil, ShellError{Err: errors.New("start failed")}
			},
		})
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 || !strings.Contains(diags.String(), "failed to start") {
			t.Fatalf("expected start diagnostic, got: %s", diags.String())
		}
	})

	t.Run("exit failure", func(t *testing.T) {
		code := 7
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "bad"}), nil, diags, ExprOptions{
			ShellRunner: func(ShellCommand) ([]byte, error) {
				return nil, ShellError{Err: errors.New("exit status 7"), ExitCode: &code, Stderr: "bad stderr\n"}
			},
		})
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		text := diags.String()
		if diagCount(diags, "E106") != 1 || !strings.Contains(text, "exit code 7") || !strings.Contains(text, "bad stderr") {
			t.Fatalf("expected exit diagnostic with stderr, got: %s", text)
		}
	})
}

func TestStripOneTrailingNewline(t *testing.T) {
	tests := map[string]string{
		"":      "",
		"a":     "a",
		"a\n":   "a",
		"a\r\n": "a",
		"a\n\n": "a\n",
		"a  \n": "a  ",
		"a  ":   "a  ",
	}
	for in, want := range tests {
		if got := stripOneTrailingNewline(in); got != want {
			t.Fatalf("stripOneTrailingNewline(%q) = %q, want %q", in, got, want)
		}
	}
}

func shellCall(args ...ast.Expr) ast.CallExpr {
	return ast.CallExpr{
		Callee: ast.IdentExpr{Name: "shell", Span: spanAt(700, 1)},
		Args:   ast.PosCallArgs(args...),
		Span:   spanAt(700, 1),
	}
}

func envMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}

func TestMergeEnvOverrideOrder(t *testing.T) {
	got := mergeEnv([]string{"b=old", "a=keep", "b=older"}, map[string]string{"b": "new", "c": "next"})
	if !slices.Equal(got, []string{"a=keep", "b=new", "c=next"}) {
		t.Fatalf("unexpected merged env: %#v", got)
	}
}
