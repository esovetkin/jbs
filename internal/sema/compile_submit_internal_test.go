package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestCompileSubmitBlockNestedListReportsE305(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(10, 1, 11))
	block := ast.SubmitBlock{
		Name: "run",
		Span: span,
		Fields: []ast.SubmitField{
			{
				Name: "nodes",
				Op:   ast.AssignEq,
				Expr: ast.ListExpr{
					Items: []ast.Expr{
						ast.ListExpr{
							Items: []ast.Expr{
								ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span},
							},
							Span: span,
						},
					},
					Span: span,
				},
				Span: span,
			},
			{
				Name: "args_exec",
				Op:   ast.AssignEq,
				Expr: ast.StringExpr{Value: "-lc hostname", Span: span},
				Span: span,
			},
		},
	}
	diags := &diag.Diagnostics{}
	spec := compileSubmitBlock(block, map[string]*ImportSource{}, map[string]eval.Value{}, map[string]VarOrigin{}, diags)
	if spec == nil {
		t.Fatalf("expected compiled submit spec")
	}
	if countDiagCode(diags, "E305") == 0 {
		t.Fatalf("expected E305 for nested tuple/list submit value, got: %s", diags.String())
	}
}

func TestCompileSubmitBlockHelperAliasUniquenessAndOriginFallback(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(10, 1, 11))
	sources := map[string]*ImportSource{
		"defaults": {
			Name: "defaults",
			Kind: SourceKindLet,
			Order: []string{
				"queue", "x-y", "x y",
			},
			Vars: map[string][]eval.Value{
				"queue": {eval.String("batch")},
				"x-y":   {eval.String("a")},
				"x y":   {eval.String("b")},
			},
			Origins: map[string]diag.Span{
				// queue origin intentionally absent to exercise source-span fallback
				"x-y": span,
				"x y": span,
			},
			Modes: map[string]string{},
			Span:  span,
		},
	}
	block := ast.SubmitBlock{
		Name:     "run",
		UseNames: []string{"defaults"},
		Span:     span,
		Fields: []ast.SubmitField{
			{
				Name: "args_exec",
				Op:   ast.AssignEq,
				Expr: ast.StringExpr{Value: "-lc hostname", Span: span},
				Span: span,
			},
		},
	}
	diags := &diag.Diagnostics{}
	spec := compileSubmitBlock(block, sources, map[string]eval.Value{}, map[string]VarOrigin{}, diags)
	if spec == nil {
		t.Fatalf("expected compiled submit spec")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	queue, ok := submitValueByNameForInternal(spec, "queue")
	if !ok {
		t.Fatalf("expected queue from submit-use defaults: %#v", spec.Values)
	}
	if queue.Span != span {
		t.Fatalf("expected queue span fallback to source span when var origin is missing, got %#v", queue.Span)
	}
	helperA, okA := submitHelperByOriginalForInternal(spec, "x-y")
	helperB, okB := submitHelperByOriginalForInternal(spec, "x y")
	if !okA || !okB {
		t.Fatalf("expected both helpers from let defaults, got %#v", spec.Helpers)
	}
	if helperA.Aliased == helperB.Aliased {
		t.Fatalf("expected unique helper aliases for colliding sanitized names, got %q and %q", helperA.Aliased, helperB.Aliased)
	}
	if helperA.Aliased != "_jk__run_x_y" || helperB.Aliased != "_jk__run_x_y_1" {
		t.Fatalf("unexpected helper alias assignment: a=%q b=%q", helperA.Aliased, helperB.Aliased)
	}
}

func TestCompileSubmitHelpers(t *testing.T) {
	t.Run("sanitizeSubmitHelperPart", func(t *testing.T) {
		if got := sanitizeSubmitHelperPart(""); got != "x" {
			t.Fatalf("expected empty helper part fallback, got %q", got)
		}
		if got := sanitizeSubmitHelperPart("ab-1 x"); got != "ab_1_x" {
			t.Fatalf("unexpected sanitized helper part: %q", got)
		}
	})

	t.Run("submitValueHasEmptyString", func(t *testing.T) {
		if submitValueHasEmptyString(SubmitValue{IsRaw: true, Raw: "echo"}) {
			t.Fatalf("raw submit value should never be considered empty-string")
		}
		if !submitValueHasEmptyString(SubmitValue{Value: eval.String("")}) {
			t.Fatalf("empty string submit value should be considered empty")
		}
	})

	t.Run("evalValueHasEmptyString", func(t *testing.T) {
		if !evalValueHasEmptyString(eval.String("")) {
			t.Fatalf("expected empty string to be empty")
		}
		if evalValueHasEmptyString(eval.String("x")) {
			t.Fatalf("expected non-empty string to be non-empty")
		}
		if !evalValueHasEmptyString(eval.List(nil)) {
			t.Fatalf("expected empty list to be treated as empty-string value")
		}
		if !evalValueHasEmptyString(eval.Tuple([]eval.Value{eval.String(""), eval.String("")})) {
			t.Fatalf("expected tuple of empty strings to be treated as empty")
		}
		if evalValueHasEmptyString(eval.List([]eval.Value{eval.String(""), eval.Int(1)})) {
			t.Fatalf("expected mixed list to be non-empty")
		}
		if evalValueHasEmptyString(eval.Int(1)) {
			t.Fatalf("expected non-string scalar to be non-empty")
		}
	})

	t.Run("submitDirectIdentifier", func(t *testing.T) {
		if ident, ok := submitDirectIdentifier(ast.IdentExpr{Name: "nodes"}); !ok || ident != "nodes" {
			t.Fatalf("expected direct ident detection, got %q %v", ident, ok)
		}
		if ident, ok := submitDirectIdentifier(ast.ModeExpr{Mode: "python", Expr: ast.IdentExpr{Name: "nodes"}}); !ok || ident != "nodes" {
			t.Fatalf("expected mode-wrapped direct ident detection, got %q %v", ident, ok)
		}
		if _, ok := submitDirectIdentifier(ast.StringExpr{Value: "nodes"}); ok {
			t.Fatalf("did not expect non-ident expression to be treated as direct identifier")
		}
	})
}

func TestSubmitSeriesRowCount(t *testing.T) {
	if n, ok := submitSeriesRowCount(eval.List([]eval.Value{eval.Int(1)})); ok || n != 0 {
		t.Fatalf("expected single-item list to be non-series, got n=%d ok=%v", n, ok)
	}
	if n, ok := submitSeriesRowCount(eval.Tuple([]eval.Value{eval.Int(1), eval.Int(2)})); !ok || n != 2 {
		t.Fatalf("expected tuple series row count 2, got n=%d ok=%v", n, ok)
	}
	if n, ok := submitSeriesRowCount(eval.String("x")); ok || n != 0 {
		t.Fatalf("expected non-list/tuple to be non-series, got n=%d ok=%v", n, ok)
	}
}

func TestCompileSubmitBlockEffectiveEnvAndSeriesWarning(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(10, 1, 11))
	block := ast.SubmitBlock{
		Name: "run",
		Span: span,
		Fields: []ast.SubmitField{
			{Name: "account", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "a", Span: span}, Span: span},
			{Name: "queue", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "q", Span: span}, Span: span},
			{Name: "starter", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "srun", Span: span}, Span: span},
			{Name: "nodes", Op: ast.AssignEq, Expr: ast.IdentExpr{Name: "nodes", Span: span}, Span: span},
			{Name: "args_exec", Op: ast.AssignEq, Expr: ast.StringExpr{Value: "-lc hostname", Span: span}, Span: span},
		},
	}
	sources := map[string]*ImportSource{
		"p": {
			Name:  "p",
			Kind:  SourceKindParam,
			Order: []string{"nodes"},
			Vars: map[string][]eval.Value{
				"nodes": {eval.Int(1), eval.Int(2)},
			},
			Origins: map[string]diag.Span{
				"nodes": span,
			},
		},
	}
	effective := map[string]VarOrigin{
		// sourceVar empty branch uses visible name fallback
		"nodes": {Name: "nodes", SourceVar: "", Paramset: "p", Kind: SourceKindParam, Span: span},
		// missing source branch should be ignored safely
		"ghost": {Name: "ghost", SourceVar: "ghost", Paramset: "missing", Kind: SourceKindParam, Span: span},
	}
	diags := &diag.Diagnostics{}
	spec := compileSubmitBlock(block, sources, map[string]eval.Value{}, effective, diags)
	if spec == nil {
		t.Fatalf("expected compiled submit spec")
	}
	if countDiagCode(diags, "W075") == 0 {
		t.Fatalf("expected W075 for series direct-identifier submit assignment, got: %s", diags.String())
	}
	nodes, ok := submitValueByNameForInternal(spec, "nodes")
	if !ok {
		t.Fatalf("expected resolved nodes submit value")
	}
	if nodes.Value.Kind != eval.KindList || len(nodes.Value.L) != 2 {
		t.Fatalf("expected nodes to evaluate to series list with two rows, got %#v", nodes.Value)
	}
	tasks, ok := submitValueByNameForInternal(spec, "tasks")
	if !ok {
		t.Fatalf("expected tasks key to be auto-populated when missing")
	}
	if tasks.Value.Kind != eval.KindList || len(tasks.Value.L) != 2 {
		t.Fatalf("expected tasks to inherit nodes series, got %#v", tasks.Value)
	}
}

func submitValueByNameForInternal(spec *SubmitSpec, name string) (SubmitValue, bool) {
	for _, value := range spec.Values {
		if value.Name == name {
			return value, true
		}
	}
	return SubmitValue{}, false
}

func submitHelperByOriginalForInternal(spec *SubmitSpec, name string) (SubmitHelper, bool) {
	for _, helper := range spec.Helpers {
		if helper.Original == name {
			return helper, true
		}
	}
	return SubmitHelper{}, false
}
