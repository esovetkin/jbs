package sema

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/eval"
)

func testScalarBinding(name, publicName string, value eval.Value, span diag.Span) *GlobalBinding {
	return &GlobalBinding{
		Name:       name,
		PublicName: publicName,
		Value:      value,
		Shape:      BindingScalar,
		Order:      []string{publicName},
		Vars:       map[string][]eval.Value{publicName: {value}},
		Origins:    map[string]diag.Span{publicName: span},
		Span:       span,
	}
}

func normalizedCaptureTypes(t *testing.T, regex string, byName map[string]string) []string {
	t.Helper()
	re := regexp.MustCompile(regex)
	names := re.SubexpNames()
	out := make([]string, 0, re.NumSubexp())
	for i := 1; i < len(names); i++ {
		typ := byName[names[i]]
		if typ == "" {
			typ = "string"
		}
		out = append(out, typ)
	}
	return out
}

func TestNormalizePatternRegexAndHasErrorCodeSince(t *testing.T) {
	regex, types, ok := normalizePatternRegex("value=%d%%-%f-%w")
	if !ok || strings.Join(normalizedCaptureTypes(t, regex, types), ",") != "int,float,string" {
		t.Fatalf("unexpected normalized regex: regex=%q types=%#v ok=%v", regex, types, ok)
	}
	if !regexp.MustCompile(regex).MatchString("value=-7%-1.5-word") {
		t.Fatalf("normalized regex did not match expected value: %q", regex)
	}
	regex, types, ok = normalizePatternRegex("count=%d")
	if !ok || strings.Join(normalizedCaptureTypes(t, regex, types), ",") != "int" {
		t.Fatalf("unexpected int normalization: regex=%q types=%#v ok=%v", regex, types, ok)
	}
	regex, types, ok = normalizePatternRegex("pair=([A-Z]+)-([0-9]+)")
	if !ok || regex != "pair=([A-Z]+)-([0-9]+)" || len(types) != 0 {
		t.Fatalf("unexpected manual group normalization: regex=%q types=%#v ok=%v", regex, types, ok)
	}
	regex, types, ok = normalizePatternRegex("literal%%")
	if !ok || regex != "literal%" || len(types) != 0 {
		t.Fatalf("unexpected literal-percent normalization: regex=%q types=%#v ok=%v", regex, types, ok)
	}
	if _, _, ok := normalizePatternRegex("value %"); ok {
		t.Fatalf("expected trailing percent normalization to fail")
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
		Globals: GlobalState{
			Values: map[string]eval.Value{
				"fn": eval.Function(&eval.FunctionValue{}),
			},
		},
		BindingsByName: map[string]*GlobalBinding{
			"pattern":     testScalarBinding("pattern", "pattern", eval.String("%d"), span),
			"pattern_dup": testScalarBinding("pattern_dup", "pattern", eval.String("%w"), span),
			"empty": {
				Name:    "empty",
				Shape:   BindingScalar,
				Order:   []string{"empty"},
				Vars:    map[string][]eval.Value{"empty": {}},
				Origins: map[string]diag.Span{"empty": span},
				Span:    span,
			},
			"other": testScalarBinding("other", "other", eval.String("%f"), span),
		},
	}
	items := []ast.WithItem{
		withIdentItem("pattern", span),
		withIdentItem("pattern_dup", span),
		withIdentItem("empty", span),
		withIdentItem("fn", span),
		withIdentItem("missing", span),
	}

	diags := &diag.Diagnostics{}
	got := resolveAnalyseImportsCanonical(items, res.BindingsByName, res.Globals.Values, res.Namespaces, diags, analyseImportOptions{EmitDiagnostics: true})
	if imported, ok := got["pattern"]; !ok || imported.Source != "pattern" || imported.SourceVar != "pattern" {
		t.Fatalf("expected pattern import, got %#v", got["pattern"])
	}
	if imported, ok := got["pattern_dup"]; !ok || imported.Source != "pattern_dup" || imported.SourceVar != "pattern" {
		t.Fatalf("expected pattern_dup import, got %#v", got["pattern_dup"])
	}
	if _, ok := got["empty"]; ok {
		t.Fatalf("did not expect empty non-string import, got %#v", got["empty"])
	}
	if countDiagCode(diags, "E214") != 0 {
		t.Fatalf("did not expect analyse import conflict diagnostic, got %d: %s", countDiagCode(diags, "E214"), diags.String())
	}
	if countDiagCode(diags, "E422") != 0 {
		t.Fatalf("did not expect analyse import string-type diagnostic, got %d: %s", countDiagCode(diags, "E422"), diags.String())
	}
	if countDiagCode(diags, "E020") != 1 {
		t.Fatalf("expected one unknown analyse import source diagnostic, got %d: %s", countDiagCode(diags, "E020"), diags.String())
	}
	if countDiagCode(diags, "E420") != 2 {
		t.Fatalf("expected two disallowed analyse import diagnostics, got %d: %s", countDiagCode(diags, "E420"), diags.String())
	}
}

func TestResolveAnalyseImportsCanonicalReportsPreciseSourceRejections(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	bindings := map[string]*GlobalBinding{
		"cases": {
			Name: "cases",
			Value: tableValueFromVars([]string{"x"}, map[string][]eval.Value{
				"x": {eval.String("a")},
			}),
			Shape: BindingTable,
			Order: []string{"x"},
			Vars: map[string][]eval.Value{
				"x": {eval.String("a")},
			},
			Span: span,
		},
		"pair": {
			Name:  "pair",
			Value: eval.Tuple([]eval.Value{eval.String("a"), eval.String("b")}),
			Shape: BindingScalar,
			Order: []string{"x", "y"},
			Vars: map[string][]eval.Value{
				"x": {eval.String("a")},
				"y": {eval.String("b")},
			},
			Span: span,
		},
		"num_pat": testScalarBinding("num_pat", "num_pat", eval.Int(1), span),
	}
	globals := map[string]eval.Value{
		"make_pat": eval.Function(&eval.FunctionValue{}),
	}
	items := []ast.WithItem{
		withIdentItem("cases", span),
		withIdentItem("pair", span),
		withIdentItem("num_pat", span),
		withIdentItem("make_pat", span),
	}

	diags := &diag.Diagnostics{}
	got := resolveAnalyseImportsCanonical(items, bindings, globals, nil, diags, analyseImportOptions{EmitDiagnostics: true})
	if len(got) != 0 {
		t.Fatalf("expected no imports, got %#v", got)
	}
	if countDiagCode(diags, "E420") != 4 {
		t.Fatalf("expected four source-level diagnostics, got %d: %s", countDiagCode(diags, "E420"), diags.String())
	}
	text := diags.String()
	for _, want := range []string{
		"'cases' is a table",
		"'pair' is not string-valued",
		"'num_pat' is not string-valued",
		"'make_pat' is not a data binding",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in diagnostics:\n%s", want, text)
		}
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
			"pattern": testScalarBinding("pattern", "pattern", eval.String("%d"), span),
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
			withIdentItem("pattern", span),
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
	spec := compileAnalyseBlock(block, res, AnalyzeOptions{}, diags)
	if spec.StepKind != "do" {
		t.Fatalf("expected analyse target kind do, got %q", spec.StepKind)
	}
	if len(spec.Assignments) != 2 {
		t.Fatalf("expected two valid analyse extraction assignments, got %#v", spec.Assignments)
	}
	if spec.Assignments[0].Name != "capture" || strings.Join(normalizedCaptureTypes(t, spec.Assignments[0].Template.Regex, spec.Assignments[0].Template.CaptureTypesByName), ",") != "int" {
		t.Fatalf("unexpected imported analyse assignment: %#v", spec.Assignments[0])
	}
	if spec.Assignments[1].Name != "localCap" || strings.Join(normalizedCaptureTypes(t, spec.Assignments[1].Template.Regex, spec.Assignments[1].Template.CaptureTypesByName), ",") != "float" {
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
	}, AnalyzeOptions{}, diags)
	if spec.StepKind != "" {
		t.Fatalf("expected unknown step kind, got %q", spec.StepKind)
	}
	if countDiagCode(diags, "E410") != 1 {
		t.Fatalf("expected one unknown-step diagnostic, got %d: %s", countDiagCode(diags, "E410"), diags.String())
	}
}

func TestCompileAnalyseBlockRejectsTrailingPercentPlaceholder(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		Globals:        GlobalState{Values: map[string]eval.Value{}},
		BindingsByName: map[string]*GlobalBinding{},
		DoBlocks:       []ast.DoBlock{{Name: "run", Span: span}},
		StepScopeByName: map[string]*StepScopePlan{
			"run": {Effective: map[string]VisibleBinding{}},
		},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{Name: "value", File: "out.txt", Expr: ast.StringExpr{Value: "value %", Span: span}, Span: span},
		},
		Columns: []ast.AnalyseColumn{{Name: "value", Span: span}},
		Span:    span,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, AnalyzeOptions{}, diags)
	if len(spec.Assignments) != 0 {
		t.Fatalf("expected invalid trailing-percent assignment to be skipped, got %#v", spec.Assignments)
	}
	if countDiagCode(diags, "E402") != 1 {
		t.Fatalf("expected one invalid-placeholder diagnostic, got %d: %s", countDiagCode(diags, "E402"), diags.String())
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
	spec := compileAnalyseBlock(block, res, AnalyzeOptions{}, diags)
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
	spec := compileAnalyseBlock(block, res, AnalyzeOptions{}, diags)
	if spec == nil {
		t.Fatalf("expected analyse spec")
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if len(spec.Assignments) != 1 {
		t.Fatalf("expected one extraction assignment, got %#v", spec.Assignments)
	}
	if spec.Assignments[0].Template.Regex != "2" || len(spec.Assignments[0].Template.CaptureTypesByName) != 0 {
		t.Fatalf("expected helper-backed extraction regex '2', got %#v", spec.Assignments[0])
	}
}

func TestCompileAnalyseBlockRejectsFunctionValuedHelpers(t *testing.T) {
	span := diag.NewSpan("analyse.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{
				"helperFn": eval.Function(&eval.FunctionValue{}),
			},
		},
		BindingsByName:  map[string]*GlobalBinding{},
		DoBlocks:        []ast.DoBlock{{Name: "run", Span: span}},
		StepScopeByName: map[string]*StepScopePlan{"run": {Effective: map[string]VisibleBinding{}}},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{Name: "helper", Expr: ast.IdentExpr{Name: "helperFn", Span: span}, Span: span},
		},
		Span: span,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, AnalyzeOptions{}, diags)
	if spec == nil {
		t.Fatalf("expected analyse spec")
	}
	if countDiagCode(diags, "E412") != 1 {
		t.Fatalf("expected one function-valued analyse-helper diagnostic, got %d: %s", countDiagCode(diags, "E412"), diags.String())
	}
}
