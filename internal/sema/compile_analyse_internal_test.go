package sema

import (
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
	"jbs/internal/eval"
)

func TestNormalizePatternRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRegex string
		wantType  string
		wantOK    bool
	}{
		{
			name:      "int placeholder",
			input:     "Count: %d",
			wantRegex: "Count: $jube_pat_int",
			wantType:  "int",
			wantOK:    true,
		},
		{
			name:      "float placeholder wins type",
			input:     "A=%d B=%f",
			wantRegex: "A=$jube_pat_int B=$jube_pat_fp",
			wantType:  "float",
			wantOK:    true,
		},
		{
			name:      "word placeholder keeps string type",
			input:     "Word: %w",
			wantRegex: "Word: $jube_pat_wrd",
			wantType:  "string",
			wantOK:    true,
		},
		{
			name:      "escaped percent",
			input:     "Rate %% done",
			wantRegex: "Rate % done",
			wantType:  "string",
			wantOK:    true,
		},
		{
			name:      "trailing percent is literal",
			input:     "tail %",
			wantRegex: "tail %",
			wantType:  "string",
			wantOK:    true,
		},
		{
			name:      "unsupported placeholder fails",
			input:     "Letter: %s",
			wantRegex: "",
			wantType:  "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRegex, gotType, gotOK := normalizePatternRegex(tt.input)
			if gotRegex != tt.wantRegex || gotType != tt.wantType || gotOK != tt.wantOK {
				t.Fatalf("normalizePatternRegex(%q)=(%q,%q,%v), want (%q,%q,%v)",
					tt.input, gotRegex, gotType, gotOK, tt.wantRegex, tt.wantType, tt.wantOK)
			}
		})
	}
}

func TestHasErrorCodeSince(t *testing.T) {
	if hasErrorCodeSince(nil, 0, diag.CodeE100) {
		t.Fatalf("expected false for nil diagnostics")
	}

	diags := &diag.Diagnostics{
		Items: []diag.Diagnostic{
			{Severity: diag.SeverityWarning, Code: string(diag.CodeE100)},
			{Severity: diag.SeverityError, Code: string(diag.CodeE102)},
			{Severity: diag.SeverityError, Code: string(diag.CodeE100)},
		},
	}

	if !hasErrorCodeSince(diags, -10, diag.CodeE100) {
		t.Fatalf("expected true when matching error exists after clamped start")
	}
	if hasErrorCodeSince(diags, len(diags.Items), diag.CodeE100) {
		t.Fatalf("expected false when start is at/after end")
	}
	if hasErrorCodeSince(diags, 2, diag.CodeE102) {
		t.Fatalf("expected false when error code does not occur in selected tail")
	}
}

func TestResolveAnalyseWithImports(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	res := &Result{
		ParamByName: map[string]*Paramset{},
		LetByName: map[string]*LetNamespace{
			"l1": {
				Name: "l1",
				Vars: map[string]eval.Value{
					"ok":  eval.String("a"),
					"num": eval.Int(7),
				},
			},
			"l2": {
				Name: "l2",
				Vars: map[string]eval.Value{
					"ok": eval.String("b"),
				},
			},
			"l3": {
				Name: "l3",
				Vars: map[string]eval.Value{
					"ok": eval.String("c"),
				},
			},
		},
		ImportSourceByName: map[string]*ImportSource{
			"l1": {
				Name:  "l1",
				Kind:  SourceKindLet,
				Order: []string{"ok", "num"},
				Vars: map[string][]eval.Value{
					"ok":  {eval.String("a")},
					"num": {eval.Int(7)},
				},
			},
			"l2": {
				Name:  "l2",
				Kind:  SourceKindLet,
				Order: []string{"ok"},
				Vars: map[string][]eval.Value{
					"ok": {eval.String("b")},
				},
			},
			"l3": {
				Name:  "l3",
				Kind:  SourceKindLet,
				Order: []string{"ok"},
				Vars: map[string][]eval.Value{
					"ok": {eval.String("c")},
				},
			},
			"ghost": {
				Name:  "ghost",
				Kind:  SourceKindLet,
				Order: []string{"missing"},
				Vars: map[string][]eval.Value{
					"missing": {eval.String("z")},
				},
			},
			"p0": {
				Name:  "p0",
				Kind:  SourceKindParam,
				Order: []string{"x"},
				Vars: map[string][]eval.Value{
					"x": {eval.Int(1)},
				},
			},
		},
	}
	items := []ast.WithItem{
		{Name: "ok", From: "l1", Span: sp},
		{Name: "ok", From: "l2", Span: sp},
		{Name: "ok", From: "l2", Span: sp},
		{Name: "ok", From: "l3", Span: sp},
		{Name: "num", From: "l1", Span: sp},
		{Name: "ghost", From: "l1", Span: sp},
		{Name: "x", From: "p0", Span: sp},
	}

	diags := &diag.Diagnostics{}
	got := resolveAnalyseWithImports(items, res, diags)

	entry, ok := got["ok"]
	if !ok {
		t.Fatalf("expected ok import to be present")
	}
	if entry.Source != "l1" || entry.SourceVar != "ok" {
		t.Fatalf("unexpected ok import entry: %#v", entry)
	}
	if _, exists := got["num"]; exists {
		t.Fatalf("did not expect non-string let import to be included")
	}
	if _, exists := got["missing"]; exists {
		t.Fatalf("did not expect ghost fallback source without let namespace to be included")
	}
	if countDiagCode(diags, "E214") != 2 {
		t.Fatalf("expected exactly two E214 conflict errors (l1/l2 and l1/l3), got %d: %s", countDiagCode(diags, "E214"), diags.String())
	}
	if countDiagCode(diags, "E422") != 1 {
		t.Fatalf("expected one E422 for non-string import, got %d: %s", countDiagCode(diags, "E422"), diags.String())
	}
	if countDiagCode(diags, "E420") != 1 {
		t.Fatalf("expected one E420 for disallowed param source, got %d: %s", countDiagCode(diags, "E420"), diags.String())
	}
}

func TestCompileAnalyseBlockSubmitAndSyntheticPattern(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{},
		},
		ImportSourceByName: map[string]*ImportSource{
			"p": {
				Name:  "p",
				Kind:  SourceKindParam,
				Order: []string{"a"},
				Origins: map[string]diag.Span{
					"a": sp,
				},
				Vars: map[string][]eval.Value{
					"a": {eval.Int(1)},
				},
			},
		},
		LetByName:        map[string]*LetNamespace{},
		StepImportByName: map[string]*StepImportPlan{},
		Submits: []ast.SubmitBlock{
			{
				Name:      "run",
				WithItems: []ast.WithItem{{Name: "p", Span: sp}},
			},
		},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{
				Name: "rx",
				Expr: ast.StringExpr{Value: "A: %d", Span: sp},
				File: "out.log",
				Span: sp,
			},
		},
		Columns: []ast.AnalyseColumn{
			{Name: "a", Span: sp},
			{Name: "rx", Title: "parsed", Span: sp},
		},
		Span: sp,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, diags)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	if spec.StepKind != "submit" {
		t.Fatalf("expected submit step kind, got %q", spec.StepKind)
	}
	if len(spec.Assignments) != 1 {
		t.Fatalf("expected one assignment, got %d", len(spec.Assignments))
	}
	asn := spec.Assignments[0]
	if asn.Group != "_ja_run_rx" || asn.Pattern != "rx" {
		t.Fatalf("unexpected synthetic pattern naming: %#v", asn)
	}
	if asn.Template.Regex != "A: $jube_pat_int" || asn.Template.Type != "int" {
		t.Fatalf("unexpected template normalization: %#v", asn.Template)
	}
	if len(spec.Columns) != 2 || spec.Columns[0].Source != "a" || spec.Columns[1].Source != "rx" || spec.Columns[1].Title != "parsed" {
		t.Fatalf("unexpected columns: %#v", spec.Columns)
	}
}

func TestCompileAnalyseBlockUnknownIdentAvoidsE412(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{},
		},
		ImportSourceByName: map[string]*ImportSource{},
		LetByName:          map[string]*LetNamespace{},
		StepImportByName:   map[string]*StepImportPlan{},
		DoBlocks:           []ast.DoBlock{{Name: "run"}},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{
				Name: "x",
				Expr: ast.IdentExpr{Name: "missing", Span: sp},
				File: "out.log",
				Span: sp,
			},
		},
		Span: sp,
	}

	diags := &diag.Diagnostics{}
	spec := compileAnalyseBlock(block, res, diags)
	if spec == nil {
		t.Fatalf("expected non-nil analyse spec")
	}
	if countDiagCode(diags, "E100") == 0 {
		t.Fatalf("expected E100 for unknown identifier, got: %s", diags.String())
	}
	if countDiagCode(diags, "E412") != 0 {
		t.Fatalf("did not expect E412 when E100 already occurred, got: %s", diags.String())
	}
}

func TestCompileAnalyseBlockNestedHelperValueError(t *testing.T) {
	sp := diag.NewSpan("in.jbs", diag.NewPos(1, 1, 1), diag.NewPos(2, 1, 2))
	res := &Result{
		Globals: GlobalState{
			Values: map[string]eval.Value{},
		},
		ImportSourceByName: map[string]*ImportSource{},
		LetByName:          map[string]*LetNamespace{},
		StepImportByName:   map[string]*StepImportPlan{},
		DoBlocks:           []ast.DoBlock{{Name: "run"}},
	}
	block := ast.AnalyseBlock{
		StepName: "run",
		Assignments: []ast.AnalyseAssign{
			{
				Name: "helper",
				Expr: ast.ListExpr{
					Items: []ast.Expr{
						ast.ListExpr{
							Items: []ast.Expr{
								ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: sp},
							},
							Span: sp,
						},
					},
					Span: sp,
				},
				Span: sp,
			},
		},
		Span: sp,
	}

	diags := &diag.Diagnostics{}
	_ = compileAnalyseBlock(block, res, diags)
	if countDiagCode(diags, "E305") == 0 {
		t.Fatalf("expected E305 for nested helper list/tuple, got: %s", diags.String())
	}
}

func countDiagCode(diags *diag.Diagnostics, code string) int {
	count := 0
	for _, item := range diags.Items {
		if item.Code == code {
			count++
		}
	}
	return count
}
