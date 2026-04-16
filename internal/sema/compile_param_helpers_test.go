package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestHasErrorSince(t *testing.T) {
	if hasErrorSince(nil, 0) {
		t.Fatalf("expected false for nil diagnostics")
	}

	diags := &diag.Diagnostics{}
	diags.AddWarning(diag.CodeW101, "warn", diag.Span{}, "")
	if hasErrorSince(diags, 0) {
		t.Fatalf("expected false when only warnings are present")
	}

	diags.AddError(diag.CodeE100, "err", diag.Span{}, "")
	if !hasErrorSince(diags, 0) {
		t.Fatalf("expected true when an error is present from start")
	}
	if hasErrorSince(diags, 2) {
		t.Fatalf("expected false when start is past all errors")
	}
	if !hasErrorSince(diags, -10) {
		t.Fatalf("expected true for negative start index with existing errors")
	}
}

func TestExprHasBinaryOpBranches(t *testing.T) {
	if exprHasBinaryOp(nil, "+") {
		t.Fatalf("expected false for nil expression")
	}
	if exprHasBinaryOp(ast.NumberExpr{Int: true, IntValue: 1}, "") {
		t.Fatalf("expected false for empty operator")
	}

	tests := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{
			name: "binary direct",
			expr: ast.BinaryExpr{
				Left:  ast.NumberExpr{Int: true, IntValue: 1},
				Op:    "+",
				Right: ast.NumberExpr{Int: true, IntValue: 2},
			},
			want: true,
		},
		{
			name: "list nested",
			expr: ast.ListExpr{
				Items: []ast.Expr{
					ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				},
			},
			want: true,
		},
		{
			name: "tuple nested",
			expr: ast.TupleExpr{
				Items: []ast.Expr{
					ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				},
			},
			want: true,
		},
		{
			name: "convert nested",
			expr: ast.ConvertExpr{
				Target: "list",
				Expr:   ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
			},
			want: true,
		},
		{
			name: "call callee nested",
			expr: ast.CallExpr{
				Callee: ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				Args:   []ast.Expr{ast.NumberExpr{Int: true, IntValue: 3}},
			},
			want: true,
		},
		{
			name: "call arg nested",
			expr: ast.CallExpr{
				Callee: ast.IdentExpr{Name: "f"},
				Args: []ast.Expr{
					ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				},
			},
			want: true,
		},
		{
			name: "index base nested",
			expr: ast.IndexExpr{
				Base:  ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				Items: []ast.Expr{ast.IdentExpr{Name: "x"}},
			},
			want: true,
		},
		{
			name: "index item nested",
			expr: ast.IndexExpr{
				Base: ast.IdentExpr{Name: "c"},
				Items: []ast.Expr{
					ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				},
			},
			want: true,
		},
		{
			name: "unary nested",
			expr: ast.UnaryExpr{
				Op:   "-",
				Expr: ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
			},
			want: true,
		},
		{
			name: "compare nested",
			expr: ast.CompareExpr{
				Left:  ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				Op:    "==",
				Right: ast.NumberExpr{Int: true, IntValue: 3},
			},
			want: true,
		},
		{
			name: "conditional nested",
			expr: ast.ConditionalExpr{
				Then: ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				Cond: ast.BoolExpr{Value: true},
				Else: ast.NumberExpr{Int: true, IntValue: 0},
			},
			want: true,
		},
		{
			name: "mode nested",
			expr: ast.ModeExpr{
				Mode: "python",
				Expr: ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
			},
			want: true,
		},
		{
			name: "alias nested",
			expr: ast.AliasExpr{
				Expr:  ast.BinaryExpr{Left: ast.NumberExpr{Int: true, IntValue: 1}, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 2}},
				Alias: "a",
			},
			want: true,
		},
		{
			name: "default false",
			expr: ast.NumberExpr{Int: true, IntValue: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		if got := exprHasBinaryOp(tt.expr, "+"); got != tt.want {
			t.Fatalf("%s: expected %v, got %v", tt.name, tt.want, got)
		}
	}
}

func TestExprIdentRefsAndFinalParamRefs(t *testing.T) {
	span := diag.NewSpan("x.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))

	refs := exprIdentRefs(ast.ConditionalExpr{
		Then: ast.CompareExpr{
			Left:  ast.UnaryExpr{Op: "-", Expr: ast.IndexExpr{Base: ast.IdentExpr{Name: "base", Span: span}, Items: []ast.Expr{ast.IdentExpr{Name: "sel", Span: span}}}},
			Op:    "==",
			Right: ast.ModeExpr{Mode: "python", Expr: ast.AliasExpr{Expr: ast.IdentExpr{Name: "rhs", Span: span}, Alias: "alias", Span: span}},
			Span:  span,
		},
		Cond: ast.CallExpr{
			Callee: ast.QualifiedIdentExpr{Namespace: "ns", Name: "fn", Span: span},
			Args: []ast.Expr{
				ast.ConvertExpr{Target: "list", Expr: ast.ListExpr{Items: []ast.Expr{
					ast.TupleExpr{Items: []ast.Expr{ast.IdentExpr{Name: "x", Span: span}}},
					ast.NumberExpr{Int: true, IntValue: 1, Span: span},
				}}},
			},
			Span: span,
		},
		Else: ast.BinaryExpr{
			Left:  ast.IdentExpr{Name: "l", Span: span},
			Op:    "+",
			Right: ast.QualifiedIdentExpr{Namespace: "qns", Name: "q", Span: span},
			Span:  span,
		},
		Span: span,
	})
	if len(refs) == 0 {
		t.Fatalf("expected identifier refs for nested expression")
	}
	wantNames := map[string]bool{
		"base": true,
		"rhs":  true,
		"qns":  true,
		"x":    true,
		"l":    true,
	}
	for _, ref := range refs {
		delete(wantNames, ref.Name)
	}
	if len(wantNames) != 0 {
		t.Fatalf("missing expected ref names: %#v", wantNames)
	}

	combRefs := finalParamRefs(ast.ParamBlock{
		Final: ast.CombBinary{
			Left:  ast.CombIdent{Name: "a", Span: span},
			Op:    "+",
			Right: ast.CombIdent{Name: "b", Span: span},
			Span:  span,
		},
	})
	if len(combRefs) != 2 || combRefs[0].Name != "a" || combRefs[1].Name != "b" {
		t.Fatalf("unexpected comb refs: %#v", combRefs)
	}

	exprRefs := finalParamRefs(ast.ParamBlock{
		FinalExpr: ast.BinaryExpr{
			Left:  ast.IdentExpr{Name: "x", Span: span},
			Op:    "+",
			Right: ast.IdentExpr{Name: "y", Span: span},
			Span:  span,
		},
	})
	if len(exprRefs) != 2 || exprRefs[0].Name != "x" || exprRefs[1].Name != "y" {
		t.Fatalf("unexpected expr refs: %#v", exprRefs)
	}

	if got := finalParamRefs(ast.ParamBlock{}); got != nil {
		t.Fatalf("expected nil refs when both finals are missing, got %#v", got)
	}
}

func TestResolveFinalExposeOrderAmbiguousAndComb(t *testing.T) {
	refs := []combIdentRef{
		{Name: "src"},
		{Name: "combv"},
		{Name: "plain"},
		{Name: "src"},
	}
	sourceSymbols := map[string]sourceSymbolInfo{
		"src": {VarOrder: []string{"a", "b"}},
	}
	combOrders := map[string][]string{
		"combv": {"x", "y"},
	}
	ambiguous := map[string]bool{
		"src": true,
	}
	got := resolveFinalExposeOrder(refs, sourceSymbols, combOrders, ambiguous)
	want := []string{"src", "x", "y", "plain"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestWarnModeExprInCollectionsBranches(t *testing.T) {
	span := diag.NewSpan("x.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	modeExpr := ast.ModeExpr{Mode: "shell", Expr: ast.StringExpr{Value: "x", Span: span}, Span: span}

	tests := []struct {
		name string
		expr ast.Expr
		want int
	}{
		{name: "top-level mode no warning", expr: modeExpr, want: 0},
		{name: "list", expr: ast.ListExpr{Items: []ast.Expr{modeExpr}}, want: 1},
		{name: "tuple", expr: ast.TupleExpr{Items: []ast.Expr{modeExpr}}, want: 1},
		{name: "convert", expr: ast.ListExpr{Items: []ast.Expr{ast.ConvertExpr{Target: "list", Expr: modeExpr, Span: span}}}, want: 1},
		{name: "call", expr: ast.ListExpr{Items: []ast.Expr{ast.CallExpr{Callee: ast.IdentExpr{Name: "f", Span: span}, Args: []ast.Expr{modeExpr}, Span: span}}}, want: 1},
		{name: "alias", expr: ast.ListExpr{Items: []ast.Expr{ast.AliasExpr{Expr: modeExpr, Alias: "a", Span: span}}}, want: 1},
		{name: "unary", expr: ast.ListExpr{Items: []ast.Expr{ast.UnaryExpr{Op: "-", Expr: modeExpr, Span: span}}}, want: 1},
		{name: "binary", expr: ast.ListExpr{Items: []ast.Expr{ast.BinaryExpr{Left: modeExpr, Op: "+", Right: ast.NumberExpr{Int: true, IntValue: 1, Span: span}, Span: span}}}, want: 1},
		{name: "compare", expr: ast.ListExpr{Items: []ast.Expr{ast.CompareExpr{Left: modeExpr, Op: "==", Right: ast.StringExpr{Value: "x", Span: span}, Span: span}}}, want: 1},
		{name: "conditional", expr: ast.ListExpr{Items: []ast.Expr{ast.ConditionalExpr{Then: modeExpr, Cond: ast.BoolExpr{Value: true, Span: span}, Else: ast.StringExpr{Value: "x", Span: span}, Span: span}}}, want: 1},
	}

	for _, tt := range tests {
		diags := &diag.Diagnostics{}
		warnModeExprInCollections(tt.expr, diags)
		if got := countDiagCode(diags, "W301"); got != tt.want {
			t.Fatalf("%s: expected %d W301, got %d (%s)", tt.name, tt.want, got, diags.String())
		}
	}
}

func TestSeriesAsValueAndEvalValueKeyForComb(t *testing.T) {
	if got := seriesAsValue(nil); got.Kind != eval.KindNull {
		t.Fatalf("expected null for empty series, got %#v", got)
	}
	if got := seriesAsValue([]eval.Value{eval.Int(7)}); got.Kind != eval.KindInt || got.I != 7 {
		t.Fatalf("expected scalar passthrough for len1 series, got %#v", got)
	}
	list := seriesAsValue([]eval.Value{eval.Int(1), eval.Int(2)})
	if list.Kind != eval.KindList || len(list.L) != 2 {
		t.Fatalf("expected list for multi-value series, got %#v", list)
	}

	tests := []struct {
		name string
		v    eval.Value
		want string
	}{
		{name: "null", v: eval.Null(), want: "n:"},
		{name: "int", v: eval.Int(1), want: "i:1"},
		{name: "float", v: eval.Float(1.5), want: "f:1.5"},
		{name: "string", v: eval.String("x"), want: "s:x"},
		{name: "bool true", v: eval.Bool(true), want: "b:1"},
		{name: "bool false", v: eval.Bool(false), want: "b:0"},
		{name: "list", v: eval.List([]eval.Value{eval.Int(1), eval.String("x")}), want: "l:i:1,s:x"},
		{name: "tuple", v: eval.Tuple([]eval.Value{eval.Int(1), eval.Bool(false)}), want: "t:i:1,b:0"},
		{name: "comb nil", v: eval.CombValue(nil), want: "c:nil"},
		{name: "comb value", v: eval.CombValue(&eval.Comb{Order: []string{"a"}, Rows: []eval.Row{{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}}}}}), want: "c:1:1"},
		{name: "unknown", v: eval.Value{Kind: eval.Kind("custom")}, want: "u:"},
	}
	for _, tt := range tests {
		if got := evalValueKeyForComb(tt.v); got != tt.want {
			t.Fatalf("%s: expected %q, got %q", tt.name, tt.want, got)
		}
	}
}

func TestCombValueFromParamsetSlice(t *testing.T) {
	span := diag.NewSpan("src.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))

	if v, ok := combValueFromParamsetSlice(nil, []string{"a"}, span); ok || v.Kind != eval.KindNull {
		t.Fatalf("expected null,false for nil source")
	}
	if v, ok := combValueFromParamsetSlice(&Paramset{}, nil, span); ok || v.Kind != eval.KindNull {
		t.Fatalf("expected null,false for empty selection")
	}

	src := &Paramset{
		Name: "p0",
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1), eval.Int(2)},
			"b": {eval.String("x"), eval.String("y")},
		},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{
				"a": {Value: eval.Int(1)},
				"b": {Value: eval.String("x")},
			}},
			{Values: map[string]eval.Cell{
				"a": {Value: eval.Int(1)},
				"b": {Value: eval.String("x")},
			}},
			{Values: map[string]eval.Cell{
				"a": {Value: eval.Int(2), Origin: span},
				"b": {Value: eval.String("y"), Origin: span},
			}},
		},
	}

	if v, ok := combValueFromParamsetSlice(src, []string{""}, span); ok || v.Kind != eval.KindNull {
		t.Fatalf("expected null,false for empty selected name")
	}
	if v, ok := combValueFromParamsetSlice(src, []string{"missing"}, span); ok || v.Kind != eval.KindNull {
		t.Fatalf("expected null,false for missing selected var")
	}

	missingCell := &Paramset{
		Name: "p1",
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1)},
			"b": {eval.String("x")},
		},
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1)}}},
		},
	}
	if v, ok := combValueFromParamsetSlice(missingCell, []string{"a", "b"}, span); ok || v.Kind != eval.KindNull {
		t.Fatalf("expected null,false for rows missing projected cells")
	}

	v, ok := combValueFromParamsetSlice(src, []string{"b", "a", "b"}, span)
	if !ok {
		t.Fatalf("expected successful comb projection from paramset slice")
	}
	if !eval.IsComb(v) || v.C == nil {
		t.Fatalf("expected comb value, got %#v", v)
	}
	if len(v.C.Order) != 2 || v.C.Order[0] != "b" || v.C.Order[1] != "a" {
		t.Fatalf("unexpected projected order: %#v", v.C.Order)
	}
	if len(v.C.Rows) != 2 {
		t.Fatalf("expected deduplicated rows, got %d", len(v.C.Rows))
	}
	if v.C.Rows[0].Values["a"].Origin != span || v.C.Rows[0].Values["b"].Origin != span {
		t.Fatalf("expected zero origins to be filled with provided span, got %#v", v.C.Rows[0].Values)
	}
}
