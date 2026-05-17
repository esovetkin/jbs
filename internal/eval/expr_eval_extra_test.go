package eval

import (
	"math"
	"slices"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestEvalLenCallBranches(t *testing.T) {
	t.Run("arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalLenCall([]Value{}, spanAt(300, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("list tuple string comb", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		if got := evalLenCall([]Value{List([]Value{Int(1), Int(2), Int(3)})}, spanAt(301, 1), diags); got.Kind != KindInt || got.I != 3 {
			t.Fatalf("unexpected len(list) result: %#v", got)
		}
		if got := evalLenCall([]Value{Tuple([]Value{Int(1), Int(2)})}, spanAt(302, 1), diags); got.Kind != KindInt || got.I != 2 {
			t.Fatalf("unexpected len(tuple) result: %#v", got)
		}
		if got := evalLenCall([]Value{String("aβ")}, spanAt(303, 1), diags); got.Kind != KindInt || got.I != 2 {
			t.Fatalf("unexpected len(string) result: %#v", got)
		}
		comb := CombValue(&Comb{
			Order: []string{"x"},
			Rows: []Row{
				{Values: map[string]Cell{"x": {Value: Int(1)}}},
				{Values: map[string]Cell{"x": {Value: Int(2)}}},
			},
		})
		if got := evalLenCall([]Value{comb}, spanAt(304, 1), diags); got.Kind != KindInt || got.I != 2 {
			t.Fatalf("unexpected len(comb) result: %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
	})

	t.Run("unsupported kind", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalLenCall([]Value{Bool(true)}, spanAt(305, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null for unsupported len target, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})
}

func hasDiagMessage(diags *diag.Diagnostics, needle string) bool {
	for _, item := range diags.Items {
		if strings.Contains(item.Message, needle) {
			return true
		}
	}
	return false
}

func TestEvalAllAnyCallBranches(t *testing.T) {
	t.Run("arity error", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("all", []Value{}, spanAt(320, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("comb rejected", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("any", []Value{CombValue(&Comb{})}, spanAt(321, 1), diags)
		if got.Kind != KindNull {
			t.Fatalf("expected null, got %#v", got)
		}
		if diagCount(diags, "E106") != 1 {
			t.Fatalf("expected E106, got: %s", diags.String())
		}
	})

	t.Run("empty list defaults", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		if got := evalAllAnyCall("all", []Value{List(nil)}, spanAt(322, 1), diags); got.Kind != KindBool || !got.B {
			t.Fatalf("expected all([])=true, got %#v", got)
		}
		if got := evalAllAnyCall("any", []Value{List(nil)}, spanAt(323, 1), diags); got.Kind != KindBool || got.B {
			t.Fatalf("expected any([])=false, got %#v", got)
		}
		if diags.HasErrors() {
			t.Fatalf("unexpected errors: %s", diags.String())
		}
	})

	t.Run("truthiness cast warning only once", func(t *testing.T) {
		diags := &diag.Diagnostics{}
		got := evalAllAnyCall("all", []Value{List([]Value{Int(1), String("")})}, spanAt(324, 1), diags)
		if got.Kind != KindBool || got.B {
			t.Fatalf("expected all([1,\"\"])=false, got %#v", got)
		}
		if diagCount(diags, "W101") != 1 {
			t.Fatalf("expected one cast warning, got: %s", diags.String())
		}
	})
}

func TestExprEvalHelpersTruthyAndSeries(t *testing.T) {
	comb := CombValue(&Comb{
		Order: []string{"x"},
		Rows:  []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}},
	})
	tests := []struct {
		name   string
		in     Value
		want   bool
		casted bool
	}{
		{name: "bool", in: Bool(true), want: true, casted: false},
		{name: "int", in: Int(0), want: false, casted: true},
		{name: "float", in: Float(2.0), want: true, casted: true},
		{name: "string", in: String(""), want: false, casted: true},
		{name: "null", in: Null(), want: false, casted: true},
		{name: "list", in: List([]Value{Int(1)}), want: true, casted: true},
		{name: "tuple", in: Tuple(nil), want: false, casted: true},
		{name: "comb", in: comb, want: true, casted: true},
		{name: "unknown", in: Value{Kind: Kind("mystery")}, want: true, casted: true},
	}
	for _, tc := range tests {
		got, casted := truthy(tc.in)
		if got != tc.want || casted != tc.casted {
			t.Fatalf("%s: expected (%v,%v), got (%v,%v)", tc.name, tc.want, tc.casted, got, casted)
		}
	}

	if got := toSeriesOrScalar(Int(7)); len(got) != 1 || got[0].I != 7 {
		t.Fatalf("unexpected scalar conversion: %#v", got)
	}
	seq := Tuple([]Value{Int(1), Int(2)})
	series := toSeriesOrScalar(seq)
	if len(series) != 2 {
		t.Fatalf("unexpected tuple conversion to series: %#v", series)
	}
	seq.L[0] = Int(99)
	if series[0].I != 1 {
		t.Fatalf("expected series clone independent from original")
	}

}

func TestBuiltinCallNames(t *testing.T) {
	names := BuiltinCallNames()
	for _, name := range []string{"bool", "env", "range", "rev", "table", "t", "map", "reduce", "rename", "rbind", "sum", "prod", "head", "tail", "order", "sort", "print", "read_csv", "shell"} {
		if !slices.Contains(names, name) {
			t.Fatalf("BuiltinCallNames missing %q: %#v", name, names)
		}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("BuiltinCallNames must be sorted, got %#v", names)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			t.Fatalf("BuiltinCallNames contains duplicate %q: %#v", name, names)
		}
		seen[name] = struct{}{}
	}
	for _, name := range []string{"range", "table", "t", "shell", "env", "rename", "rbind", "sum", "prod", "head", "tail", "order", "sort"} {
		if !IsBuiltinCallName(name) {
			t.Fatalf("expected %q to be a builtin call name", name)
		}
	}
	for _, name := range []string{"python", "missing", "product", "select", "zip"} {
		if IsBuiltinCallName(name) {
			t.Fatalf("did not expect %q to be a builtin call name", name)
		}
	}
}

func TestBuiltinSymbolNamesIncludesCallsAndConstants(t *testing.T) {
	names := BuiltinSymbolNames()
	for _, name := range []string{"range", "table", "read_csv", "order", "sort", "None"} {
		if !slices.Contains(names, name) {
			t.Fatalf("BuiltinSymbolNames missing %q: %#v", name, names)
		}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("BuiltinSymbolNames must be sorted, got %#v", names)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			t.Fatalf("BuiltinSymbolNames contains duplicate %q: %#v", name, names)
		}
		seen[name] = struct{}{}
	}
}

func TestEvalBoolBuiltinShadowing(t *testing.T) {
	span := spanAt(329, 1)
	call := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "bool", Span: span},
		Args:   ast.PosCallArgs(ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}),
		Span:   span,
	}

	diags := &diag.Diagnostics{}
	got := EvalExpr(call, map[string]Value{"bool": Int(1)}, diags)
	if got.Kind != KindNull {
		t.Fatalf("expected null for non-callable shadow, got %#v", got)
	}
	if diagCount(diags, "E199") != 1 {
		t.Fatalf("expected E199 for non-callable shadow, got: %s", diags.String())
	}

	diags = &diag.Diagnostics{}
	fn := Function(&FunctionValue{
		Params: []ast.FuncParam{{Name: "x", Span: span}},
		Body: []ast.FuncBodyStmt{
			ast.ReturnStmt{
				Expr: ast.StringExpr{Value: "shadowed", Span: span},
				Span: span,
			},
		},
		Span: span,
	})
	got = EvalExpr(call, map[string]Value{"bool": fn}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for callable shadow: %s", diags.String())
	}
	if got.Kind != KindString || got.S != "shadowed" {
		t.Fatalf("expected callable shadow result, got %#v", got)
	}
}

func TestEvalPrintCallCollectsEvents(t *testing.T) {
	span := spanAt(330, 1)
	tests := []struct {
		name string
		args []ast.CallArg
		want []Value
	}{
		{
			name: "zero args",
		},
		{
			name: "one arg",
			args: ast.PosCallArgs(ast.StringExpr{Value: "hello", Span: span}),
			want: []Value{String("hello")},
		},
		{
			name: "multiple args",
			args: ast.PosCallArgs(
				ast.StringExpr{Value: "x", Span: span},
				ast.NumberExpr{Int: true, IntValue: 7, Raw: "7", Span: span},
				ast.ListExpr{Items: []ast.Expr{ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}}, Span: span},
			),
			want: []Value{String("x"), Int(7), List([]Value{Int(1)})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			events := []PrintEvent{}
			seq := 40
			got := EvalExprWithOptions(ast.CallExpr{
				Callee: ast.IdentExpr{Name: "print", Span: span},
				Args:   tc.args,
				Span:   span,
			}, nil, diags, ExprOptions{
				Context:    EvalCtxBindingAssign,
				PrintIndex: 3,
				Print: func(event PrintEvent) {
					events = append(events, event)
				},
				NextPrintSeq: func() int {
					seq++
					return seq
				},
			})
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if got.Kind != KindNull {
				t.Fatalf("expected print to return null, got %#v", got)
			}
			if len(events) != 1 {
				t.Fatalf("expected one print event, got %#v", events)
			}
			if events[0].Index != 3 || events[0].Seq != 41 || events[0].Span != span {
				t.Fatalf("unexpected print event metadata: %#v", events[0])
			}
			if len(events[0].Values) != len(tc.want) {
				t.Fatalf("unexpected print values: got=%#v want=%#v", events[0].Values, tc.want)
			}
			for i := range tc.want {
				if !Equal(events[0].Values[i], tc.want[i]) {
					t.Fatalf("value %d: got=%#v want=%#v", i, events[0].Values[i], tc.want[i])
				}
			}
		})
	}
}

func TestEvalPrintCallOptions(t *testing.T) {
	tests := []struct {
		name       string
		expr       ast.Expr
		wantNRow   int
		wantValues []Value
		opts       ExprOptions
	}{
		{
			name:       "positional with nrow",
			expr:       callExpr(ident("print"), posArg(intExpr(1)), namedArg("nrow", intExpr(3))),
			wantNRow:   3,
			wantValues: []Value{Int(1)},
		},
		{
			name:       "values with unlimited nrow",
			expr:       callExpr(ident("print"), namedArg("values", listExpr(intExpr(1), intExpr(2))), namedArg("nrow", intExpr(0))),
			wantNRow:   0,
			wantValues: []Value{Int(1), Int(2)},
		},
		{
			name: "builtin function value",
			expr: callExpr(ident("p"), posArg(listExpr(intExpr(1), intExpr(2))), namedArg("nrow", intExpr(2))),
			opts: ExprOptions{Frame: func() *Frame {
				frame := NewRootFrame(nil)
				assignBuiltinFunction(t, frame, "p", "print")
				return frame
			}()},
			wantNRow:   2,
			wantValues: []Value{List([]Value{Int(1), Int(2)})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			events := []PrintEvent{}
			opts := tc.opts
			opts.Context = EvalCtxBindingAssign
			opts.Print = func(event PrintEvent) {
				events = append(events, event)
			}
			got := EvalExprWithOptions(tc.expr, nil, diags, opts)
			if diags.HasErrors() {
				t.Fatalf("unexpected diagnostics: %s", diags.String())
			}
			if got.Kind != KindNull {
				t.Fatalf("expected print to return null, got %#v", got)
			}
			if len(events) != 1 {
				t.Fatalf("expected one print event, got %#v", events)
			}
			if events[0].Options.NRow != tc.wantNRow {
				t.Fatalf("unexpected nrow: got %d want %d", events[0].Options.NRow, tc.wantNRow)
			}
			if len(events[0].Values) != len(tc.wantValues) {
				t.Fatalf("unexpected values: got=%#v want=%#v", events[0].Values, tc.wantValues)
			}
			for i := range tc.wantValues {
				if !Equal(events[0].Values[i], tc.wantValues[i]) {
					t.Fatalf("value %d: got=%#v want=%#v", i, events[0].Values[i], tc.wantValues[i])
				}
			}
		})
	}
}

func TestEvalPrintCallOptionDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			name: "nrow string",
			expr: callExpr(ident("print"), namedArg("nrow", stringExpr("10"))),
			want: "nrow argument must be an integer",
		},
		{
			name: "nrow negative",
			expr: callExpr(ident("print"), namedArg("nrow", intExpr(-1))),
			want: "nrow argument must be non-negative",
		},
		{
			name: "unknown option",
			expr: callExpr(ident("print"), namedArg("width", intExpr(80))),
			want: "unknown named argument 'width' for print()",
		},
		{
			name: "positional after named",
			expr: callExpr(ident("print"), namedArg("nrow", intExpr(1)), posArg(intExpr(2))),
			want: "positional arguments cannot follow named arguments",
		},
		{
			name: "values not sequence",
			expr: callExpr(ident("print"), namedArg("values", intExpr(1))),
			want: "* call expansion expects a list or tuple",
		},
		{
			name: "duplicate nrow",
			expr: callExpr(ident("print"), namedArg("nrow", intExpr(1)), namedArg("nrow", intExpr(2))),
			want: "argument 'nrow' received multiple values",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			events := []PrintEvent{}
			got := EvalExprWithOptions(tc.expr, nil, diags, ExprOptions{
				Context: EvalCtxBindingAssign,
				Print: func(event PrintEvent) {
					events = append(events, event)
				},
			})
			if got.Kind != KindNull {
				t.Fatalf("expected null on diagnostic, got %#v", got)
			}
			if diagCount(diags, "E106") != 1 || !strings.Contains(diags.String(), tc.want) {
				t.Fatalf("expected E106 containing %q, got: %s", tc.want, diags.String())
			}
			if len(events) != 0 {
				t.Fatalf("expected no print event on invalid call, got %#v", events)
			}
		})
	}
}

func TestEvalPrintCallNoSinkAndClone(t *testing.T) {
	span := spanAt(331, 1)
	env := map[string]Value{"x": List([]Value{Int(1)})}
	diags := &diag.Diagnostics{}
	events := []PrintEvent{}
	got := EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "print", Span: span},
		Args:   ast.PosCallArgs(ast.IdentExpr{Name: "x", Span: span}),
		Span:   span,
	}, env, diags, ExprOptions{
		Context: EvalCtxBindingAssign,
		Print: func(event PrintEvent) {
			events = append(events, event)
		},
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if got.Kind != KindNull {
		t.Fatalf("expected print to return null, got %#v", got)
	}
	mutated := env["x"]
	mutated.L[0] = Int(99)
	env["x"] = mutated
	if len(events) != 1 || len(events[0].Values) != 1 || events[0].Values[0].L[0].I != 1 {
		t.Fatalf("expected cloned print values, got %#v", events)
	}

	diags = &diag.Diagnostics{}
	got = EvalExprWithOptions(ast.CallExpr{
		Callee: ast.IdentExpr{Name: "print", Span: span},
		Args:   ast.PosCallArgs(ast.StringExpr{Value: "quiet", Span: span}),
		Span:   span,
	}, nil, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() || got.Kind != KindNull {
		t.Fatalf("expected no-sink print to be quiet null, got=%#v diags=%s", got, diags.String())
	}
}

func TestEvalPrintBuiltinShadowing(t *testing.T) {
	span := spanAt(332, 1)
	call := ast.CallExpr{
		Callee: ast.IdentExpr{Name: "print", Span: span},
		Span:   span,
	}

	diags := &diag.Diagnostics{}
	events := []PrintEvent{}
	got := EvalExprWithOptions(call, map[string]Value{"print": Int(1)}, diags, ExprOptions{
		Context: EvalCtxBindingAssign,
		Print: func(event PrintEvent) {
			events = append(events, event)
		},
	})
	if got.Kind != KindNull {
		t.Fatalf("expected null for non-callable shadow, got %#v", got)
	}
	if diagCount(diags, "E199") != 1 {
		t.Fatalf("expected E199 for non-callable shadow, got: %s", diags.String())
	}
	if len(events) != 0 {
		t.Fatalf("expected no builtin print event when shadowed, got %#v", events)
	}

	diags = &diag.Diagnostics{}
	fn := Function(&FunctionValue{
		Body: []ast.FuncBodyStmt{
			ast.ReturnStmt{
				Expr: ast.NumberExpr{Int: true, IntValue: 7, Raw: "7", Span: span},
				Span: span,
			},
		},
		Span: span,
	})
	got = EvalExprWithOptions(call, map[string]Value{"print": fn}, diags, ExprOptions{Context: EvalCtxBindingAssign})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for callable shadow: %s", diags.String())
	}
	if got.Kind != KindInt || got.I != 7 {
		t.Fatalf("expected callable shadow result 7, got %#v", got)
	}
}

func TestEvalRangeFloatBranches(t *testing.T) {
	at := spanAt(340, 1)
	tests := []struct {
		name      string
		start     float64
		stop      float64
		step      float64
		wantKind  Kind
		wantLen   int
		wantCode  string
		wantError bool
	}{
		{
			name:      "reject non-finite input",
			start:     math.NaN(),
			stop:      1.0,
			step:      0.1,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:      "reject non-positive step",
			start:     0.0,
			stop:      1.0,
			step:      0.0,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:     "start greater or equal stop yields empty list",
			start:    2.0,
			stop:     2.0,
			step:     0.5,
			wantKind: KindList,
			wantLen:  0,
		},
		{
			name:      "step too small to make progress",
			start:     1e308,
			stop:      math.MaxFloat64,
			step:      1.0,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:      "overflow while generating values",
			start:     math.MaxFloat64 * 0.75,
			stop:      math.MaxFloat64,
			step:      math.MaxFloat64 * 0.75,
			wantKind:  KindNull,
			wantCode:  "E106",
			wantError: true,
		},
		{
			name:     "valid float range",
			start:    0.0,
			stop:     1.5,
			step:     0.5,
			wantKind: KindList,
			wantLen:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := evalRangeFloat(tc.start, tc.stop, tc.step, at, diags)
			if got.Kind != tc.wantKind {
				t.Fatalf("unexpected kind: got=%s want=%s value=%#v", got.Kind, tc.wantKind, got)
			}
			if tc.wantKind == KindList && len(got.L) != tc.wantLen {
				t.Fatalf("unexpected list length: got=%d want=%d value=%#v", len(got.L), tc.wantLen, got)
			}
			if tc.wantError {
				if diagCount(diags, tc.wantCode) == 0 {
					t.Fatalf("expected %s, got: %s", tc.wantCode, diags.String())
				}
				return
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
		})
	}
}

func TestBinaryNeedsRelaxedCombEvalCoverage(t *testing.T) {
	cases := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{name: "nil", expr: nil, want: false},
		{name: "alias", expr: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "k"}, want: true},
		{name: "ident non alias", expr: ast.IdentExpr{Name: "n"}, want: false},
		{name: "qualified non alias", expr: ast.QualifiedIdentExpr{Namespace: "ns", Name: "col"}, want: false},
		{name: "binary recurse", expr: ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 2}, Alias: "c"}}, want: true},
		{name: "unary recurse", expr: ast.UnaryExpr{Op: "-", Expr: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}}, want: true},
		{name: "call recurse args", expr: ast.CallExpr{Callee: ast.IdentExpr{Name: "tuple"}, Args: ast.PosCallArgs(ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"})}, want: true},
		{name: "index recurse", expr: ast.IndexExpr{Base: ast.NumberExpr{Int: true, IntValue: 1}, Items: []ast.Expr{ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 2}, Alias: "x"}}}, want: true},
		{name: "member recurse", expr: ast.MemberExpr{Base: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}, Name: "x"}, want: true},
		{name: "list recurse", expr: ast.ListExpr{Items: []ast.Expr{ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}}}, want: true},
		{name: "tuple recurse", expr: ast.TupleExpr{Items: []ast.Expr{ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}}}, want: true},
		{name: "compare recurse", expr: ast.CompareExpr{Left: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}, Op: "==", Right: ast.NumberExpr{Int: true, IntValue: 1}}, want: true},
		{name: "conditional recurse", expr: ast.ConditionalExpr{Then: ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 1}, Alias: "c"}, Cond: ast.BoolExpr{Value: true}, Else: ast.NumberExpr{Int: true, IntValue: 0}}, want: true},
		{name: "default", expr: ast.NumberExpr{Int: true, IntValue: 1}, want: false},
	}
	for _, tc := range cases {
		if got := binaryNeedsRelaxedCombEval(tc.expr); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestCombRowsHelpersCoverage(t *testing.T) {
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
	env := map[string]Value{
		"a": Int(1),
	}

	if rows := combRowsFromBinaryOperand(nil, List([]Value{Int(1), Int(2)}), env, diags, ExprOptions{}, ctx); len(rows) != 2 {
		t.Fatalf("expected 2 rows for nil expr fallback, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.IdentExpr{Name: "a", Span: spanAt(330, 1)}, Int(3), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected one named row for ident, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.QualifiedIdentExpr{Namespace: "ns", Name: "x", Span: spanAt(331, 1)}, Int(4), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected one named row for qualified ident, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.AliasExpr{Expr: ast.NumberExpr{Int: true, IntValue: 5}, Alias: "z", Span: spanAt(332, 1)}, Int(0), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected one named row for alias helper, got %#v", rows)
	}
	if rows := combRowsFromBinaryOperand(ast.NumberExpr{Int: true, IntValue: 7, Span: spanAt(333, 1)}, Int(7), env, diags, ExprOptions{}, ctx); len(rows) != 1 {
		t.Fatalf("expected scalar fallback row, got %#v", rows)
	}

	base := []Row{{Values: map[string]Cell{"x": {Value: Int(1)}}}}
	combRows := combRowsFromValue(CombValue(&Comb{Order: []string{"x"}, Rows: base}), diag.Span{})
	if len(combRows) != 1 {
		t.Fatalf("expected cloned comb rows, got %#v", combRows)
	}
	combRows[0].Values["x"] = Cell{Value: Int(9)}
	if base[0].Values["x"].Value.I != 1 {
		t.Fatalf("expected combRowsFromValue to clone comb rows")
	}
}

func TestEvalExprWithCtxDefaultUnsupportedNode(t *testing.T) {
	diags := &diag.Diagnostics{}
	ctx := &evalCtx{overflowWarned: map[string]struct{}{}}
	expr := &ast.StringExpr{Value: "x", Span: spanAt(340, 1)}
	got := evalExprWithCtx(expr, map[string]Value{}, diags, ExprOptions{}, ctx)
	if got.Kind != KindNull {
		t.Fatalf("expected null for unsupported pointer node, got %#v", got)
	}
	if diagCount(diags, "E199") != 1 {
		t.Fatalf("expected one E199, got: %s", diags.String())
	}
}

func TestEvalBuiltinCallsIntegration(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want Value
	}{
		{
			name: "len call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "len"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 1},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
				),
			},
			want: Int(2),
		},
		{
			name: "filter call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "filter"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{
						ast.NumberExpr{Int: true, IntValue: 0},
						ast.NumberExpr{Int: true, IntValue: 2},
					}},
					ast.IdentExpr{Name: "bool"},
				),
			},
			want: List([]Value{Int(2)}),
		},
		{
			name: "all call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "all"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{ast.BoolExpr{Value: true}, ast.BoolExpr{Value: true}}},
				),
			},
			want: Bool(true),
		},
		{
			name: "any call",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "any"},
				Args: ast.PosCallArgs(
					ast.ListExpr{Items: []ast.Expr{ast.BoolExpr{Value: false}, ast.BoolExpr{Value: true}}},
				),
			},
			want: Bool(true),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := &diag.Diagnostics{}
			got := EvalExpr(tc.expr, map[string]Value{}, diags)
			if !Equal(got, tc.want) {
				t.Fatalf("unexpected builtin-call result: got=%#v want=%#v", got, tc.want)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected errors: %s", diags.String())
			}
		})
	}
}
