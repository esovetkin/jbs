package sema

import (
	"os"
	"path/filepath"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestNormalizePatternRegexAndHasErrorCodeSince(t *testing.T) {
	regex, typ, ok := normalizePatternRegex("value=%d%%-%f-%w")
	if !ok || regex != "value=$jube_pat_int%-$jube_pat_fp-$jube_pat_wrd" || typ != "float" {
		t.Fatalf("unexpected normalized regex: regex=%q type=%q ok=%v", regex, typ, ok)
	}
	regex, typ, ok = normalizePatternRegex("count=%d")
	if !ok || regex != "count=$jube_pat_int" || typ != "int" {
		t.Fatalf("unexpected int normalization: regex=%q type=%q ok=%v", regex, typ, ok)
	}
	regex, typ, ok = normalizePatternRegex("literal%")
	if !ok || regex != "literal%" || typ != "string" {
		t.Fatalf("unexpected trailing-percent normalization: regex=%q type=%q ok=%v", regex, typ, ok)
	}
	if _, _, ok := normalizePatternRegex("%x"); ok {
		t.Fatalf("expected invalid placeholder normalization to fail")
	}

	diags := &diag.Diagnostics{}
	diags.AddWarning(diag.CodeW071, "warn", diag.Span{}, "")
	diags.AddError(diag.CodeE402, "bad", diag.Span{}, "")
	diags.AddError(diag.CodeE410, "worse", diag.Span{}, "")
	if hasErrorCodeSince(nil, 0, diag.CodeE402) {
		t.Fatalf("nil diagnostics should never report errors")
	}
	if !hasErrorCodeSince(diags, 0, diag.CodeE402) {
		t.Fatalf("expected E402 to be found")
	}
	if hasErrorCodeSince(diags, 2, diag.CodeE402) {
		t.Fatalf("did not expect E402 after index 2")
	}
	if !hasErrorCodeSince(diags, -1, diag.CodeE410) {
		t.Fatalf("expected negative start to clamp to zero")
	}
	if hasErrorCodeSince(diags, 99, diag.CodeE410) {
		t.Fatalf("expected out-of-range start to return false")
	}
}

func TestResolveAnalyseImportsCanonical(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		BindingsByName: map[string]*GlobalBinding{
			"pattern": scalarBinding("pattern", "pattern", eval.String("%d"), span),
			"empty": {
				Name:    "empty",
				Shape:   BindingScalar,
				Order:   []string{"empty"},
				Vars:    map[string][]eval.Value{"empty": {}},
				Origins: map[string]diag.Span{"empty": span},
				Span:    span,
			},
			"other": scalarBinding("other", "other", eval.String("%f"), span),
		},
	}
	items := []ast.WithItem{
		{Name: "pattern", Span: span},
		{Name: "pattern", From: "pattern", Alias: "dup", Span: span},
		{Name: "other", From: "other", Alias: "dup", Span: span},
		{Name: "empty", Span: span},
		{Name: "missing", Span: span},
	}

	diags := &diag.Diagnostics{}
	got := resolveAnalyseImportsCanonical(items, res, diags, analyseImportOptions{EmitDiagnostics: true})
	if imported, ok := got["pattern"]; !ok || imported.Source != "pattern" || imported.SourceVar != "pattern" {
		t.Fatalf("expected pattern import, got %#v", got["pattern"])
	}
	if imported, ok := got["dup"]; !ok || imported.Source != "pattern" || imported.SourceVar != "pattern" {
		t.Fatalf("expected first conflicting import to win, got %#v", got["dup"])
	}
	if _, ok := got["empty"]; ok {
		t.Fatalf("did not expect empty non-string analyse import to be retained, got %#v", got["empty"])
	}
	if countDiagCode(diags, "E214") != 1 {
		t.Fatalf("expected one analyse import conflict diagnostic, got %d: %s", countDiagCode(diags, "E214"), diags.String())
	}
	if countDiagCode(diags, "E422") != 1 {
		t.Fatalf("expected one analyse import string-type diagnostic, got %d: %s", countDiagCode(diags, "E422"), diags.String())
	}
	if countDiagCode(diags, "E020") != 1 {
		t.Fatalf("expected one unknown analyse import source diagnostic, got %d: %s", countDiagCode(diags, "E020"), diags.String())
	}
}

func TestCompileAnalyseBlock(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{},
		},
		BindingsByName: map[string]*GlobalBinding{
			"stepSource": {
				Name:    "stepSource",
				Shape:   BindingScalar,
				Order:   []string{"stepVar", "otherStepVar"},
				Vars:    map[string][]eval.Value{"stepVar": {eval.String("step")}, "otherStepVar": {eval.String("other")}},
				Origins: map[string]diag.Span{"stepVar": span, "otherStepVar": span},
				Span:    span,
			},
			"pattern": scalarBinding("pattern", "pattern", eval.String("%d"), span),
		},
		DoBlocks: []ast.DoBlock{{Name: "run", Span: span}},
		StepScopeByName: map[string]*StepScopePlan{
			"run": {
				Effective: map[string]VisibleBinding{
					"stepVar":      {Name: "stepVar", Source: "stepSource", Span: span},
					"otherStepVar": {Name: "otherStepVar", Source: "stepSource", Span: span},
				},
			},
		},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		WithItems: []ast.WithItem{
			{Name: "pattern", Span: span},
		},
		Assignments: []ast.AnalyseAssign{
			{Name: "stepVar", Expr: ast.StringExpr{Value: "helper", Span: span}, Span: span},
			{Name: "helperList", Expr: ast.ListExpr{Items: []ast.Expr{ast.ListExpr{Items: []ast.Expr{ast.StringExpr{Value: "x", Span: span}}, Span: span}}, Span: span}, Span: span},
			{Name: "capture", File: "out.txt", Expr: ast.IdentExpr{Name: "pattern", Span: span}, Span: span},
			{Name: "localCap", File: "out.txt", Expr: ast.StringExpr{Value: "%f", Span: span}, Span: span},
			{Name: "otherStepVar", File: "out.txt", Expr: ast.StringExpr{Value: "%d", Span: span}, Span: span},
			{Name: "bad", File: "out.txt", Expr: ast.StringExpr{Value: "%x", Span: span}, Span: span},
			{Name: "notString", File: "out.txt", Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}, Span: span},
			{Name: "unknownExpr", File: "out.txt", Expr: ast.IdentExpr{Name: "missingName", Span: span}, Span: span},
		},
		Columns: []ast.AnalyseColumn{
			{Name: "stepVar", Title: "step", Span: span},
			{Name: "capture", Title: "capture", Span: span},
			{Name: "missing", Title: "missing", Span: span},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, diags)
	if spec.StepKind != "do" {
		t.Fatalf("expected analyse target kind do, got %q", spec.StepKind)
	}
	if len(spec.Assignments) != 2 {
		t.Fatalf("expected two valid analyse extraction assignments, got %#v", spec.Assignments)
	}
	if spec.Assignments[0].Group != "pattern" || spec.Assignments[0].Pattern != "pattern" || spec.Assignments[0].Template.Regex != "$jube_pat_int" || spec.Assignments[0].Template.Type != "int" {
		t.Fatalf("unexpected imported analyse assignment: %#v", spec.Assignments[0])
	}
	if spec.Assignments[1].Group != "_ja_run_localCap" || spec.Assignments[1].Pattern != "localCap" || spec.Assignments[1].Template.Regex != "$jube_pat_fp" || spec.Assignments[1].Template.Type != "float" {
		t.Fatalf("unexpected synthetic analyse assignment: %#v", spec.Assignments[1])
	}
	if len(spec.Columns) != 2 || spec.Columns[0].Source != "stepVar" || spec.Columns[1].Source != "capture" {
		t.Fatalf("unexpected analyse columns: %#v", spec.Columns)
	}
	if countDiagCode(diags, "W320") != 1 {
		t.Fatalf("expected one helper-shadow warning, got %d: %s", countDiagCode(diags, "W320"), diags.String())
	}
	if countDiagCode(diags, "E305") != 1 {
		t.Fatalf("expected one nested helper list diagnostic, got %d: %s", countDiagCode(diags, "E305"), diags.String())
	}
	if countDiagCode(diags, "E413") != 1 {
		t.Fatalf("expected one extraction collision diagnostic, got %d: %s", countDiagCode(diags, "E413"), diags.String())
	}
	if countDiagCode(diags, "E402") != 1 {
		t.Fatalf("expected one invalid placeholder diagnostic, got %d: %s", countDiagCode(diags, "E402"), diags.String())
	}
	if countDiagCode(diags, "E412") != 1 {
		t.Fatalf("expected one non-string extraction diagnostic, got %d: %s", countDiagCode(diags, "E412"), diags.String())
	}
	if countDiagCode(diags, "E415") != 1 {
		t.Fatalf("expected one unknown analyse column diagnostic, got %d: %s", countDiagCode(diags, "E415"), diags.String())
	}
}

func TestCompileAnalyseBlockUnknownStep(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(ast.AnalyseBlock{StepName: "missing", Span: span}, &Result{
		Globals:         GlobalState{Values: map[string]eval.Value{}},
		BindingsByName:  map[string]*GlobalBinding{},
		StepScopeByName: map[string]*StepScopePlan{},
	}, diags)
	if spec.StepKind != "" {
		t.Fatalf("expected unknown step kind, got %q", spec.StepKind)
	}
	if countDiagCode(diags, "E410") != 1 {
		t.Fatalf("expected one unknown-step diagnostic, got %d: %s", countDiagCode(diags, "E410"), diags.String())
	}
}

func TestCompileAnalyseBlockSupportsNamesBuiltin(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{
				"globalv": eval.Int(1),
			},
		},
		BindingsByName: map[string]*GlobalBinding{
			"stepSource": {
				Name:    "stepSource",
				Shape:   BindingScalar,
				Order:   []string{"stepVar"},
				Vars:    map[string][]eval.Value{"stepVar": {eval.String("step")}},
				Origins: map[string]diag.Span{"stepVar": span},
				Span:    span,
			},
		},
		DoBlocks: []ast.DoBlock{{Name: "run", Span: span}},
		StepScopeByName: map[string]*StepScopePlan{
			"run": {
				Effective: map[string]VisibleBinding{
					"stepVar": {Name: "stepVar", Source: "stepSource", Span: span},
				},
			},
		},
		Namespaces: map[string]*Namespace{
			"mod": {
				Name:     "mod",
				Bindings: []string{"mod.alpha", "mod.beta", "mod.child.gamma"},
			},
		},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{
				Name: "helperCount",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args: ast.PosCallArgs(
						ast.CallExpr{Callee: ast.IdentExpr{Name: "names", Span: span}, Span: span},
					),
					Span: span,
				},
				Span: span,
			},
			{
				Name: "namespaceCount",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args: ast.PosCallArgs(
						ast.CallExpr{
							Callee: ast.IdentExpr{Name: "names", Span: span},
							Args:   ast.PosCallArgs(ast.IdentExpr{Name: "mod", Span: span}),
							Span:   span,
						},
					),
					Span: span,
				},
				Span: span,
			},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, diags)
	if spec == nil {
		t.Fatalf("expected analyse spec")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
}

func TestCompileAnalyseBlockSupportsReadCSVBuiltin(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "cases.csv"), []byte("x,y\n1,2\n3,4\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	analyseFile := filepath.Join(cwd, "analyse.jbs")
	span := diag.NewSpan(analyseFile, diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		Globals:         GlobalState{Values: map[string]eval.Value{}},
		BindingsByName:  map[string]*GlobalBinding{},
		DoBlocks:        []ast.DoBlock{{Name: "run", Span: span}},
		StepScopeByName: map[string]*StepScopePlan{"run": {Effective: map[string]VisibleBinding{}}},
		BaseDirByFile:   map[string]string{analyseFile: cwd},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{
				Name: "rowCount",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "len", Span: span},
					Args: ast.PosCallArgs(
						ast.CallExpr{
							Callee: ast.IdentExpr{Name: "read_csv", Span: span},
							Args:   ast.PosCallArgs(ast.StringExpr{Value: "./cases.csv", Span: span}),
							Span:   span,
						},
					),
					Span: span,
				},
				Span: span,
			},
			{
				Name: "capture",
				File: "out.txt",
				Expr: ast.CallExpr{
					Callee: ast.IdentExpr{Name: "str", Span: span},
					Args:   ast.PosCallArgs(ast.IdentExpr{Name: "rowCount", Span: span}),
					Span:   span,
				},
				Span: span,
			},
		},
		Columns: []ast.AnalyseColumn{{Name: "capture", Title: "capture", Span: span}},
		Span:    span,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, diags)
	if spec == nil {
		t.Fatalf("expected analyse spec")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(spec.Assignments) != 1 {
		t.Fatalf("expected one extraction assignment, got %#v", spec.Assignments)
	}
	if spec.Assignments[0].Template.Regex != "2" || spec.Assignments[0].Template.Type != "string" {
		t.Fatalf("expected helper-backed extraction regex '2', got %#v", spec.Assignments[0])
	}
}
