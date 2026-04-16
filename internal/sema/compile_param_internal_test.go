package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestCompileParamBlockFinalNilReturnsEmptyParamset(t *testing.T) {
	block := ast.ParamBlock{
		Name: "p",
		Assignments: []ast.Assignment{
			{
				Name: "x",
				Op:   ast.AssignEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 1},
			},
		},
		Final: nil,
	}
	diags := &diag.Diagnostics{}
	got := compileParamBlock(block, map[string]*Paramset{}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if got == nil {
		t.Fatalf("expected paramset, got nil")
	}
	if got.Name != "p" {
		t.Fatalf("unexpected name: %q", got.Name)
	}
	if got.Rows != nil {
		t.Fatalf("expected nil rows for nil final expression")
	}
	if len(got.Vars) != 0 || len(got.BaseVars) != 0 || len(got.Origins) != 0 || len(got.Modes) != 0 {
		t.Fatalf("expected empty maps for nil final expression, got vars=%d base=%d origins=%d modes=%d", len(got.Vars), len(got.BaseVars), len(got.Origins), len(got.Modes))
	}
}

func TestCompileParamBlockSkipsNilKnownAndLetSources(t *testing.T) {
	sp := diag.Span{}
	block := ast.ParamBlock{
		Name: "p",
		WithItems: []ast.WithItem{
			{Name: "base", Span: sp},
			{Name: "defaults", Span: sp},
		},
		Assignments: []ast.Assignment{
			{
				Name: "x",
				Op:   ast.AssignEq,
				Expr: ast.NumberExpr{Int: true, IntValue: 1, Span: sp},
				Span: sp,
			},
		},
		Final: ast.CombIdent{Name: "x", Span: sp},
	}
	diags := &diag.Diagnostics{}
	got := compileParamBlock(
		block,
		map[string]*Paramset{"base": nil},
		map[string]eval.Value{},
		map[string]*LetNamespace{"defaults": nil},
		diags,
	)
	if got == nil {
		t.Fatalf("expected paramset, got nil")
	}
	if got.Name != "p" {
		t.Fatalf("unexpected paramset name %q", got.Name)
	}
	if len(got.Vars["x"]) != 1 || got.Vars["x"][0].Kind != eval.KindInt || got.Vars["x"][0].I != 1 {
		t.Fatalf("expected x to be compiled despite nil known/let sources, got %#v", got.Vars["x"])
	}
	if countDiagCode(diags, "E020") == 0 {
		t.Fatalf("expected E020 for unknown with sources after nil source skip, got: %s", diags.String())
	}
}

func TestCompileParamBlockImportParamFallsBackToVarsAndCarriesMode(t *testing.T) {
	sp := diag.Span{}
	base := &Paramset{
		Name: "p0",
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(1), Origin: sp}}},
			{Values: map[string]eval.Cell{"x": {Value: eval.Int(2), Origin: sp}}},
		},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1), eval.Int(2)},
		},
		BaseVars: map[string][]eval.Value{},
		Origins: map[string]diag.Span{
			"x": sp,
		},
		Modes: map[string]string{
			"x": "shell",
		},
		Order: []string{"x"},
	}
	block := ast.ParamBlock{
		Name: "p1",
		WithItems: []ast.WithItem{
			{Name: "x", From: "p0", Span: sp},
		},
		Final: ast.CombIdent{Name: "x", Span: sp},
	}
	diags := &diag.Diagnostics{}
	got := compileParamBlock(block, map[string]*Paramset{"p0": base}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got == nil {
		t.Fatalf("expected paramset, got nil")
	}
	if got.Modes["x"] != "shell" {
		t.Fatalf("expected imported mode 'shell', got %q", got.Modes["x"])
	}
	if len(got.Vars["x"]) != 2 || got.Vars["x"][0].I != 1 || got.Vars["x"][1].I != 2 {
		t.Fatalf("unexpected imported values for x: %#v", got.Vars["x"])
	}
}

func TestCompileParamBlockImportLetConflictKeepsFirstBinding(t *testing.T) {
	sp := diag.Span{}
	lets := map[string]*LetNamespace{
		"l0": {
			Name: "l0",
			Vars: map[string]eval.Value{
				"v": eval.String("left"),
			},
			Origins: map[string]diag.Span{
				"v": sp,
			},
			Modes: map[string]string{
				"v": "python",
			},
		},
		"l1": {
			Name: "l1",
			Vars: map[string]eval.Value{
				"v": eval.String("right"),
			},
		},
	}
	block := ast.ParamBlock{
		Name: "p",
		WithItems: []ast.WithItem{
			{Name: "v", From: "l0", Span: sp},
			{Name: "v", From: "l1", Span: sp},
		},
		Final: ast.CombIdent{Name: "v", Span: sp},
	}
	diags := &diag.Diagnostics{}
	got := compileParamBlock(block, map[string]*Paramset{}, map[string]eval.Value{}, lets, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if got == nil {
		t.Fatalf("expected paramset, got nil")
	}
	if len(got.Vars["v"]) != 1 || got.Vars["v"][0].String() != "left" {
		t.Fatalf("expected first let binding to win, got %#v", got.Vars["v"])
	}
	if got.Modes["v"] != "python" {
		t.Fatalf("expected imported let mode 'python', got %q", got.Modes["v"])
	}
}

func TestCompileParamBlockAmbiguousSourceSymbolAliasReportsE221(t *testing.T) {
	sp := diag.Span{}
	p0 := &Paramset{
		Name: "p0",
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1), Origin: sp}}},
		},
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1)},
		},
		BaseVars: map[string][]eval.Value{
			"a": {eval.Int(1)},
		},
		Origins: map[string]diag.Span{
			"a": sp,
		},
		Order: []string{"a"},
	}
	p1 := &Paramset{
		Name: "p1",
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"b": {Value: eval.Int(2), Origin: sp}}},
		},
		Vars: map[string][]eval.Value{
			"b": {eval.Int(2)},
		},
		BaseVars: map[string][]eval.Value{
			"b": {eval.Int(2)},
		},
		Origins: map[string]diag.Span{
			"b": sp,
		},
		Order: []string{"b"},
	}
	block := ast.ParamBlock{
		Name: "p",
		WithItems: []ast.WithItem{
			{Name: "p0", Alias: "src", Span: sp},
			{Name: "p1", Alias: "src", Span: sp},
		},
		Final: ast.CombIdent{Name: "src", Span: sp},
	}
	diags := &diag.Diagnostics{}
	_ = compileParamBlock(block, map[string]*Paramset{"p0": p0, "p1": p1}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if countDiagCode(diags, "E221") == 0 {
		t.Fatalf("expected E221 for ambiguous aliased source symbol, got: %s", diags.String())
	}
}

func TestCompileParamBlockMixedSourceDuplicateVarReportsSingleE220(t *testing.T) {
	sp := diag.Span{}
	p0 := &Paramset{
		Name: "p0",
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(1), Origin: sp}}},
			{Values: map[string]eval.Cell{"a": {Value: eval.Int(2), Origin: sp}}},
		},
		Vars: map[string][]eval.Value{
			"a": {eval.Int(1), eval.Int(2)},
		},
		BaseVars: map[string][]eval.Value{
			"a": {eval.Int(1), eval.Int(2)},
		},
		Origins: map[string]diag.Span{
			"a": sp,
		},
		Order: []string{"a"},
	}
	block := ast.ParamBlock{
		Name: "p",
		WithItems: []ast.WithItem{
			{Name: "p0", Span: sp},
		},
		Final: ast.CombBinary{
			Left: ast.CombBinary{
				Left:   ast.CombIdent{Name: "p0", Span: sp},
				Op:     "+",
				Right:  ast.CombIdent{Name: "a", Span: sp},
				OpSpan: sp,
				Span:   sp,
			},
			Op:     "+",
			Right:  ast.CombIdent{Name: "a", Span: sp},
			OpSpan: sp,
			Span:   sp,
		},
	}
	diags := &diag.Diagnostics{}
	_ = compileParamBlock(block, map[string]*Paramset{"p0": p0}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if got := countDiagCode(diags, "E220"); got != 1 {
		t.Fatalf("expected exactly one E220 for duplicate mixed refs, got %d: %s", got, diags.String())
	}
}

func TestCompileParamBlockNestedTupleAssignmentReportsE305(t *testing.T) {
	sp := diag.Span{}
	block := ast.ParamBlock{
		Name: "p",
		Assignments: []ast.Assignment{
			{
				Name: "x",
				Op:   ast.AssignEq,
				Expr: ast.TupleExpr{
					Items: []ast.Expr{
						ast.TupleExpr{
							Items: []ast.Expr{
								ast.NumberExpr{Int: true, IntValue: 1, Span: sp},
							},
							Span: sp,
						},
						ast.TupleExpr{
							Items: []ast.Expr{
								ast.NumberExpr{Int: true, IntValue: 2, Span: sp},
							},
							Span: sp,
						},
					},
					Span: sp,
				},
				Span: sp,
			},
		},
		Final: ast.CombIdent{Name: "x", Span: sp},
	}
	diags := &diag.Diagnostics{}
	_ = compileParamBlock(block, map[string]*Paramset{}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if countDiagCode(diags, "E305") == 0 {
		t.Fatalf("expected E305 for nested tuple/list assignment, got: %s", diags.String())
	}
}

func TestCompileParamBlockModeExprCoercionReportsE215(t *testing.T) {
	sp := diag.Span{}
	block := ast.ParamBlock{
		Name: "p",
		Assignments: []ast.Assignment{
			{
				Name: "tuple_mode",
				Op:   ast.AssignEq,
				Expr: ast.ModeExpr{
					Mode: "shell",
					Expr: ast.TupleExpr{
						Items: []ast.Expr{
							ast.StringExpr{Value: "ok", Span: sp},
							ast.NumberExpr{Int: true, IntValue: 5, Span: sp},
						},
						Span: sp,
					},
					Span: sp,
				},
				Span: sp,
			},
			{
				Name: "scalar_mode",
				Op:   ast.AssignEq,
				Expr: ast.ModeExpr{
					Mode: "python",
					Expr: ast.NumberExpr{Int: true, IntValue: 7, Span: sp},
					Span: sp,
				},
				Span: sp,
			},
		},
		Final: ast.CombBinary{
			Left:   ast.CombIdent{Name: "tuple_mode", Span: sp},
			Op:     "+",
			Right:  ast.CombIdent{Name: "scalar_mode", Span: sp},
			OpSpan: sp,
			Span:   sp,
		},
	}
	diags := &diag.Diagnostics{}
	got := compileParamBlock(block, map[string]*Paramset{}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if got == nil {
		t.Fatalf("expected paramset, got nil")
	}
	if got.Modes["tuple_mode"] != "shell" || got.Modes["scalar_mode"] != "python" {
		t.Fatalf("unexpected modes: %#v", got.Modes)
	}
	if len(got.Vars["tuple_mode"]) != 2 || got.Vars["tuple_mode"][0].String() != "ok" || got.Vars["tuple_mode"][1].String() != "5" {
		t.Fatalf("expected coerced tuple_mode values [ok,5], got %#v", got.Vars["tuple_mode"])
	}
	if len(got.Vars["scalar_mode"]) == 0 || got.Vars["scalar_mode"][0].Kind != eval.KindString || got.Vars["scalar_mode"][0].String() != "7" {
		t.Fatalf("expected coerced string for scalar_mode, got %#v", got.Vars["scalar_mode"])
	}
	if gotCount := countDiagCode(diags, "E215"); gotCount < 2 {
		t.Fatalf("expected E215 from tuple and scalar non-string mode values, got %d: %s", gotCount, diags.String())
	}
}

func TestCompileParamBlockFullSourceMissingCellsFallbackToSeries(t *testing.T) {
	sp := diag.Span{}
	p0 := &Paramset{
		Name: "p0",
		Rows: []eval.Row{
			{Values: map[string]eval.Cell{
				"x": {Value: eval.Int(1), Origin: sp},
			}},
		},
		Vars: map[string][]eval.Value{
			"x": {eval.Int(1)},
			"y": {eval.Int(2)},
		},
		BaseVars: map[string][]eval.Value{
			"x": {eval.Int(1)},
			"y": {eval.Int(2)},
		},
		Origins: map[string]diag.Span{
			"x": sp,
			"y": sp,
		},
		Order: []string{"x", "y"},
	}
	block := ast.ParamBlock{
		Name: "p1",
		WithItems: []ast.WithItem{
			{Name: "p0", Span: sp},
		},
		Final: ast.CombIdent{Name: "p0", Span: sp},
	}
	diags := &diag.Diagnostics{}
	got := compileParamBlock(block, map[string]*Paramset{"p0": p0}, map[string]eval.Value{}, map[string]*LetNamespace{}, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if len(got.Vars["y"]) != 1 || got.Vars["y"][0].I != 2 {
		t.Fatalf("expected y fallback from series, got %#v", got.Vars["y"])
	}
}
