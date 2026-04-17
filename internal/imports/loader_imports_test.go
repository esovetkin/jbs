package imports

import (
	"path/filepath"
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestProcessUseStmtDefaultAliasAndErrorBranches(t *testing.T) {
	diags := &diag.Diagnostics{}
	r := &resolver{
		cwd:       t.TempDir(),
		diags:     diags,
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
		sources:   map[string]string{},
	}
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))

	mod := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}
	r.processUseStmt(mod, ast.UseStmt{
		Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "jsc", Span: span},
		Span:   span,
	}, map[string]struct{}{})
	if _, ok := mod.Aliases["jsc"]; !ok {
		t.Fatalf("expected default alias 'jsc' to be created")
	}
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics after default alias import: %s", diags.String())
	}

	mod = &expandedModule{
		Ref: ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{
			"taken": {ID: "mod:a", Label: "a"},
		},
		Exports: map[string]ModuleExport{
			"jsc": {Name: "jsc", Span: span, ModuleID: "entry"},
		},
		Stmts: []ast.Stmt{},
	}
	r.processUseStmt(mod, ast.UseStmt{
		Alias:  "taken",
		Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "jsc", Span: span},
		Span:   span,
	}, map[string]struct{}{})
	if !hasDiagCode(diags, "E536") {
		t.Fatalf("expected E536 for alias collision, got: %s", diags.String())
	}

	before := len(diags.Items)
	r.processUseStmt(mod, ast.UseStmt{
		Alias:  "jsc",
		Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "jsc", Span: span},
		Span:   span,
	}, map[string]struct{}{})
	if !hasDiagCode(diags, "E534") || len(diags.Items) == before {
		t.Fatalf("expected E534 for alias-symbol collision, got: %s", diags.String())
	}

	before = len(diags.Items)
	r.processUseStmt(mod, ast.UseStmt{
		Names:  []string{"x"},
		Source: ast.UseSource{Kind: ast.UseSourceBare, Value: "missing_module_for_use_stmt_test", Span: span},
		Span:   span,
	}, map[string]struct{}{})
	if !hasDiagCode(diags, "E531") || len(diags.Items) == before {
		t.Fatalf("expected E531 for failed import source resolution, got: %s", diags.String())
	}
}

func TestImportSymbolHandlesValidationAndDependencyBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	r := &resolver{
		diags:     &diag.Diagnostics{},
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
	}
	source := &expandedModule{
		Ref: ModuleRef{ID: "m1", Label: "m1"},
		Exports: map[string]ModuleExport{
			"hidden": {
				Name:       "hidden",
				Kind:       ExportOther,
				Importable: false,
				Span:       span,
				ModuleID:   "m1",
				Stmt:       ast.AnalyseBlock{StepName: "hidden", Span: span},
			},
			"run": {
				Name:       "run",
				Kind:       ExportGlobal,
				Importable: true,
				Span:       span,
				ModuleID:   "m1",
				Stmt: ast.GlobalAssign{
					Name: "run",
					Expr: ast.QualifiedIdentExpr{Namespace: "ghost", Name: "dep", Span: span},
					Span: span,
				},
			},
		},
		Aliases: map[string]ModuleRef{
			"ghost": {ID: "ghost-module-id", Label: "ghost"},
		},
		Stmts: []ast.Stmt{},
	}
	target := &expandedModule{
		Ref:     ModuleRef{ID: "target", Label: "target"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}

	r.importSymbol(target, source, "missing", span, map[string]struct{}{}, map[string]struct{}{})
	if !hasDiagCode(r.diags, "E532") {
		t.Fatalf("expected E532 for missing symbol, got: %s", r.diags.String())
	}

	r.importSymbol(target, source, "hidden", span, map[string]struct{}{}, map[string]struct{}{})
	if !hasDiagCode(r.diags, "E533") {
		t.Fatalf("expected E533 for non-importable symbol, got: %s", r.diags.String())
	}

	inserted := map[string]struct{}{}
	r.importSymbol(target, source, "run", span, inserted, map[string]struct{}{})
	if _, ok := target.Exports["run"]; !ok {
		t.Fatalf("expected run to be imported even when a dependency module is unresolved")
	}
	if len(target.Stmts) != 1 {
		t.Fatalf("expected one imported statement, got %#v", target.Stmts)
	}

	r.importSymbol(target, source, "run", span, inserted, map[string]struct{}{})
	if len(target.Stmts) != 1 {
		t.Fatalf("expected inserted guard to skip duplicate import, got %#v", target.Stmts)
	}

	visitingTarget := &expandedModule{
		Ref:     ModuleRef{ID: "target2", Label: "target2"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}
	r.importSymbol(visitingTarget, source, "run", span, map[string]struct{}{}, map[string]struct{}{"m1::run": {}})
	if len(visitingTarget.Exports) != 0 || len(visitingTarget.Stmts) != 0 {
		t.Fatalf("expected visiting guard to skip import, got exports=%#v stmts=%#v", visitingTarget.Exports, visitingTarget.Stmts)
	}
}

func TestAddImportedSymbolAndStmtExportBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	source := &expandedModule{Ref: ModuleRef{ID: "m1", Label: "m1"}}
	r := &resolver{diags: &diag.Diagnostics{}}
	sym := ModuleExport{
		Name:       "x",
		Kind:       ExportGlobal,
		Importable: true,
		Span:       span,
		ModuleID:   "m1",
		Stmt:       ast.GlobalAssign{Name: "x", Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}, Span: span},
	}

	targetAliasCollision := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{"x": {ID: "m2", Label: "m2"}},
		Exports: map[string]ModuleExport{},
	}
	if ok := r.addImportedSymbol(targetAliasCollision, source, sym, span); ok {
		t.Fatalf("expected alias collision to reject imported symbol")
	}
	if !hasDiagCode(r.diags, "E534") {
		t.Fatalf("expected E534 for alias collision, got: %s", r.diags.String())
	}

	targetSame := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{"x": {Name: "x", ModuleID: "m1", Span: span}},
	}
	before := len(r.diags.Items)
	if ok := r.addImportedSymbol(targetSame, source, sym, span); ok {
		t.Fatalf("expected same-source duplicate to be ignored")
	}
	if len(r.diags.Items) != before {
		t.Fatalf("expected no new diagnostics for same-source duplicate, got: %s", r.diags.String())
	}

	targetConflict := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{"x": {Name: "x", ModuleID: "m2", Span: span}},
	}
	before = len(r.diags.Items)
	if ok := r.addImportedSymbol(targetConflict, source, sym, span); ok {
		t.Fatalf("expected cross-source conflict to fail")
	}
	if !hasDiagCode(r.diags, "E534") || len(r.diags.Items) == before {
		t.Fatalf("expected E534 for cross-source conflict, got: %s", r.diags.String())
	}

	targetFresh := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{},
	}
	if ok := r.addImportedSymbol(targetFresh, source, sym, span); !ok {
		t.Fatalf("expected fresh import to succeed")
	}
	if got := targetFresh.Exports["x"]; !got.Imported || got.ModuleID != "m1" {
		t.Fatalf("unexpected imported export metadata: %#v", got)
	}

	stmtCases := []struct {
		stmt           ast.Stmt
		wantName       string
		wantKind       ExportKind
		wantImportable bool
		wantOK         bool
	}{
		{stmt: ast.DoBlock{Name: "d"}, wantName: "d", wantKind: ExportDo, wantImportable: true, wantOK: true},
		{stmt: ast.SubmitBlock{Name: "s"}, wantName: "s", wantKind: ExportSubmit, wantImportable: true, wantOK: true},
		{stmt: ast.GlobalAssign{Name: "g"}, wantName: "g", wantKind: ExportGlobal, wantImportable: true, wantOK: true},
		{stmt: ast.AnalyseBlock{StepName: "a"}, wantName: "a", wantKind: ExportOther, wantImportable: true, wantOK: false},
		{stmt: ast.UseStmt{}, wantName: "", wantKind: ExportOther, wantImportable: false, wantOK: false},
	}
	for i, tc := range stmtCases {
		gotName, gotKind, gotImportable, gotOK := stmtExport(tc.stmt)
		if gotName != tc.wantName || gotKind != tc.wantKind || gotImportable != tc.wantImportable || gotOK != tc.wantOK {
			t.Fatalf("stmtExport case %d mismatch: got(name=%q kind=%q importable=%v ok=%v) want(name=%q kind=%q importable=%v ok=%v)",
				i, gotName, gotKind, gotImportable, gotOK, tc.wantName, tc.wantKind, tc.wantImportable, tc.wantOK)
		}
	}
}

func TestSymbolDependenciesAndModuleRefByIDBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ref0 := ModuleRef{ID: "m0", Label: "m0"}
	ref1 := ModuleRef{ID: "m1", Label: "m1"}
	ref2 := ModuleRef{ID: "m2", Label: "m2"}
	r := &resolver{
		raw: map[string]*rawModule{
			"m0": {Ref: ref0},
			"m1": {Ref: ref1},
		},
		expanded: map[string]*expandedModule{
			"m2": {Ref: ref2},
		},
		diags: &diag.Diagnostics{},
	}
	mod := &expandedModule{
		Ref: ref0,
		Exports: map[string]ModuleExport{
			"srcLocal":   {Name: "srcLocal", ModuleID: "m0"},
			"selLocal":   {Name: "selLocal", ModuleID: "m0"},
			"afterStep":  {Name: "afterStep", ModuleID: "m0"},
			"submitName": {Name: "submitName", ModuleID: "m2"},
			"broken":     {Name: "broken", ModuleID: "missing-module"},
		},
		Aliases: map[string]ModuleRef{
			"lib": ref1,
		},
	}

	doDeps := r.symbolDependencies(mod, ast.DoBlock{
		After: []string{"afterStep", "lib.dep"},
		WithItems: []ast.WithItem{
			{SourceExpr: "lib.src", SourceSlice: []string{"", "a"}, Span: span},
			{SourceExpr: "srcLocal", SourceSlice: []string{"ignored"}, Span: span},
			{SourceExpr: "missingExpr", SourceSlice: []string{"selLocal", "absent"}, Span: span},
			{SourceExpr: "broken", SourceSlice: []string{"none"}, Span: span},
			{Name: "", From: "lib", Span: span},
		},
		Span: span,
	})
	gotDo := make([]string, 0, len(doDeps))
	for _, dep := range doDeps {
		gotDo = append(gotDo, dep.Source.ID+":"+dep.Name)
	}
	wantDo := []string{"m0:afterStep", "m0:selLocal", "m0:srcLocal", "m1:dep", "m1:src"}
	if !reflect.DeepEqual(gotDo, wantDo) {
		t.Fatalf("unexpected do-block deps: got=%#v want=%#v", gotDo, wantDo)
	}

	globalDeps := r.symbolDependencies(mod, ast.GlobalAssign{
		Name: "g",
		Expr: ast.BinaryExpr{
			Left:  ast.IdentExpr{Name: "srcLocal", Span: span},
			Op:    "+",
			Right: ast.QualifiedIdentExpr{Namespace: "lib", Name: "dep", Span: span},
			Span:  span,
		},
		Span: span,
	})
	gotGlobal := make([]string, 0, len(globalDeps))
	for _, dep := range globalDeps {
		gotGlobal = append(gotGlobal, dep.Source.ID+":"+dep.Name)
	}
	wantGlobal := []string{"m0:srcLocal", "m1:dep"}
	if !reflect.DeepEqual(gotGlobal, wantGlobal) {
		t.Fatalf("unexpected global deps: got=%#v want=%#v", gotGlobal, wantGlobal)
	}

	submitDeps := r.symbolDependencies(mod, ast.SubmitBlock{
		After:    []string{"afterStep"},
		UseNames: []string{"lib.dep"},
		WithItems: []ast.WithItem{
			{Name: "submitName", Span: span},
			{Name: "renamed", From: "lib.dep", Span: span},
		},
		Span: span,
	})
	gotSubmit := make([]string, 0, len(submitDeps))
	for _, dep := range submitDeps {
		gotSubmit = append(gotSubmit, dep.Source.ID+":"+dep.Name)
	}
	wantSubmit := []string{"m0:afterStep", "m1:dep", "m2:submitName"}
	if !reflect.DeepEqual(gotSubmit, wantSubmit) {
		t.Fatalf("unexpected submit deps: got=%#v want=%#v", gotSubmit, wantSubmit)
	}

	analyseDeps := r.symbolDependencies(mod, ast.AnalyseBlock{
		WithItems: []ast.WithItem{
			{Name: "submitName", Span: span},
		},
		Span: span,
	})
	if len(analyseDeps) != 1 || analyseDeps[0].Source.ID != "m2" || analyseDeps[0].Name != "submitName" {
		t.Fatalf("unexpected analyse deps: %#v", analyseDeps)
	}

	if got, ok := r.moduleRefByID("m0"); !ok || got != ref0 {
		t.Fatalf("expected raw module ref for m0, got=%#v ok=%v", got, ok)
	}
	if got, ok := r.moduleRefByID("m2"); !ok || got != ref2 {
		t.Fatalf("expected expanded module ref for m2, got=%#v ok=%v", got, ok)
	}
	if got, ok := r.moduleRefByID("missing"); ok || got.ID != "" {
		t.Fatalf("expected missing module ref lookup to fail, got=%#v ok=%v", got, ok)
	}
}

func TestLoadResultExpandsAliasClosure(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "b.jbs", "b_value = 2\n")
	aPath := writeTestFile(t, dir, "a.jbs", "use b\n"+"a_value = 1\n")
	entry := writeTestFile(t, dir, "entry.jbs", "use a\n"+"root = 0\n")

	diags := &diag.Diagnostics{}
	res, err := LoadAndExpand(entry, dir, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}

	aRef, ok := res.Aliases["a"]
	if !ok {
		t.Fatalf("expected top-level alias for a in load result")
	}
	if _, ok := res.Modules[aRef.ID]; !ok {
		t.Fatalf("expected module info for alias a")
	}

	bPath := filepath.Join(dir, "b.jbs")
	foundA := false
	foundB := false
	for _, info := range res.Modules {
		if info == nil {
			continue
		}
		if info.Ref.Label == aPath {
			foundA = true
			if _, ok := info.Aliases["b"]; !ok {
				t.Fatalf("expected nested alias b inside module a")
			}
		}
		if info.Ref.Label == bPath {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("expected nested alias modules to be expanded, got modules=%#v", res.Modules)
	}
}

func TestAddLocalStmtBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	r := &resolver{diags: &diag.Diagnostics{}}

	mod := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{"run": {ID: "m1", Label: "m1"}},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}
	r.addLocalStmt(mod, ast.DoBlock{Name: "run", Span: span})
	if !hasDiagCode(r.diags, "E534") {
		t.Fatalf("expected E534 for local symbol colliding with alias, got: %s", r.diags.String())
	}

	mod = &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{"run": {Name: "run", Imported: true, Span: span}},
		Stmts:   []ast.Stmt{},
	}
	before := len(r.diags.Items)
	r.addLocalStmt(mod, ast.DoBlock{Name: "run", Span: span})
	if !hasDiagCode(r.diags, "E534") || len(r.diags.Items) == before {
		t.Fatalf("expected E534 for local symbol colliding with imported symbol, got: %s", r.diags.String())
	}

	mod = &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}
	r.addLocalStmt(mod, ast.GlobalAssign{Name: "value", Expr: ast.NumberExpr{Int: true, IntValue: 1, Raw: "1", Span: span}, Span: span})
	if got, ok := mod.Exports["value"]; !ok || got.Kind != ExportGlobal || got.Imported {
		t.Fatalf("expected local export to be recorded, got %#v", mod.Exports)
	}

	mod = &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]ModuleRef{},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}
	r.addLocalStmt(mod, ast.AnalyseBlock{StepName: "analysis", Span: span})
	if _, ok := mod.Exports["analysis"]; ok {
		t.Fatalf("analyse blocks should not be exported")
	}
	if len(mod.Stmts) != 1 {
		t.Fatalf("expected statement to be appended regardless of exportability")
	}
}
