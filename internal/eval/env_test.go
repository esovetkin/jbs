package eval

import (
	"slices"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestEvalEnvLookupAndFallback(t *testing.T) {
	diags := &diag.Diagnostics{}
	opts := ExprOptions{Environ: fixedEnviron("A=one", "EMPTY=")}

	got := EvalExprWithOptions(envCall(stringExpr("A")), nil, diags, opts)
	if !Equal(got, String("one")) {
		t.Fatalf("unexpected env lookup: %#v", got)
	}
	if got := EvalExprWithOptions(envCall(stringExpr("MISSING")), nil, diags, opts); !Equal(got, Null()) {
		t.Fatalf("expected missing env without fallback to be None, got %#v", got)
	}
	if got := EvalExprWithOptions(envCall(stringExpr("MISSING"), stringExpr("")), nil, diags, opts); !Equal(got, String("")) {
		t.Fatalf("expected explicit empty-string fallback, got %#v", got)
	}
	if got := EvalExprWithOptions(envCall(stringExpr("MISSING"), stringExpr("fallback")), nil, diags, opts); !Equal(got, String("fallback")) {
		t.Fatalf("unexpected env string fallback: %#v", got)
	}
	if got := EvalExprWithOptions(callExpr(ident("env"), namedArg("name", stringExpr("A"))), nil, diags, opts); !Equal(got, String("one")) {
		t.Fatalf("unexpected named env lookup: %#v", got)
	}
	if got := EvalExprWithOptions(callExpr(ident("env"), namedArg("name", stringExpr("MISSING")), namedArg("default", stringExpr("named-fallback"))), nil, diags, opts); !Equal(got, String("named-fallback")) {
		t.Fatalf("unexpected named env fallback: %#v", got)
	}
	if got := EvalExprWithOptions(envCall(stringExpr("MISSING"), intExpr(7)), nil, diags, opts); !Equal(got, Int(7)) {
		t.Fatalf("unexpected env non-string fallback: %#v", got)
	}
	if got := EvalExprWithOptions(envCall(stringExpr("EMPTY"), stringExpr("fallback")), nil, diags, opts); !Equal(got, String("")) {
		t.Fatalf("expected present empty env value to win, got %#v", got)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestEvalEnvDictionary(t *testing.T) {
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(envCall(), nil, diags, ExprOptions{
		Environ: fixedEnviron("B=two", "BROKEN", "A=one", "A=last"),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindDict || got.D == nil {
		t.Fatalf("expected dictionary, got %#v", got)
	}
	if len(got.D.Order) != 2 || got.D.Order[0].S != "A" || got.D.Order[1].S != "B" {
		t.Fatalf("unexpected env dictionary order: %#v", got.D.Order)
	}
	if !Equal(got.D.Entries[DictKey{Kind: DictKeyString, S: "A"}], String("last")) {
		t.Fatalf("expected duplicate environment variable to keep last value, got %#v", got.D)
	}
	if !Equal(got.D.Entries[DictKey{Kind: DictKeyString, S: "B"}], String("two")) {
		t.Fatalf("unexpected B value: %#v", got.D)
	}
}

func TestEvalEnvDiagnostics(t *testing.T) {
	cases := []ast.Expr{
		envCall(intExpr(1)),
		envCall(stringExpr("HOME"), stringExpr("fallback"), stringExpr("extra")),
		callExpr(ident("env"), namedArg("unknown", stringExpr("HOME"))),
	}
	for _, expr := range cases {
		diags := &diag.Diagnostics{}
		got := EvalExprWithOptions(expr, nil, diags, ExprOptions{Environ: fixedEnviron("HOME=/tmp")})
		if got.Kind != KindNull {
			t.Fatalf("expected null for invalid env call, got %#v", got)
		}
		if diagCount(diags, "E106") == 0 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	}
}

func TestEvalEnvBuiltinShadowing(t *testing.T) {
	span := spanAt(731, 1)
	fn := Function(&FunctionValue{
		Params: []ast.FuncParam{{Name: "name", Span: span}},
		Body: []ast.FuncBodyStmt{
			ast.ReturnStmt{
				Expr: ast.StringExpr{Value: "shadowed", Span: span},
				Span: span,
			},
		},
		Span: span,
	})
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(envCall(stringExpr("A")), map[string]Value{"env": fn}, diags, ExprOptions{
		Environ: fixedEnviron("A=one"),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if !Equal(got, String("shadowed")) {
		t.Fatalf("expected shadowed env function, got %#v", got)
	}
}

func TestShellUsesInjectedEnvironmentProvider(t *testing.T) {
	var call ShellCommand
	diags := &diag.Diagnostics{}
	got := EvalExprWithOptions(shellCall(ast.StringExpr{Value: "echo $BASE"}), nil, diags, ExprOptions{
		Environ: fixedEnviron("BASE=from-provider"),
		ShellRunner: func(spec ShellCommand) ([]byte, error) {
			call = spec
			return []byte(envMap(spec.Env)["BASE"] + "\n"), nil
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindString || got.S != "from-provider" {
		t.Fatalf("unexpected shell result: %#v", got)
	}
	if env := envMap(call.Env); env["BASE"] != "from-provider" {
		t.Fatalf("expected injected env provider, got %#v", env)
	}
}

func TestBuiltinCallNamesIncludesEnv(t *testing.T) {
	if !slices.Contains(BuiltinCallNames(), "env") {
		t.Fatalf("BuiltinCallNames missing env: %#v", BuiltinCallNames())
	}
	if !IsBuiltinCallName("env") {
		t.Fatalf("expected env to be a builtin call name")
	}
}

func envCall(args ...ast.Expr) ast.CallExpr {
	return callExpr(ident("env"), ast.PosCallArgs(args...)...)
}

func fixedEnviron(items ...string) func() []string {
	return func() []string {
		return append([]string(nil), items...)
	}
}
