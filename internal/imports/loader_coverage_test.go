package imports

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestLoadAndExpandNilDiagnosticsAndCwdFailures(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
do run {
  echo ok
}
`)

	res, err := LoadAndExpand(entry, dir, nil)
	if err != nil {
		t.Fatalf("LoadAndExpand with nil diagnostics failed: %v", err)
	}
	if res == nil || len(res.Program.Stmts) != 1 {
		t.Fatalf("unexpected expanded program: %#v", res)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(origWD); chdirErr != nil {
			t.Fatalf("restore cwd: %v", chdirErr)
		}
	}()

	broken := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatalf("mkdir broken cwd: %v", err)
	}
	if err := os.Chdir(broken); err != nil {
		t.Fatalf("chdir broken cwd: %v", err)
	}
	if err := os.RemoveAll(broken); err != nil {
		t.Fatalf("remove broken cwd: %v", err)
	}

	if _, err := LoadAndExpand("entry.jbs", "", nil); err == nil {
		t.Fatalf("expected cwd resolution failure when cwd is empty and getwd fails")
	}
	if _, err := LoadAndExpand("entry.jbs", ".", &diag.Diagnostics{}); err == nil {
		t.Fatalf("expected absolute cwd normalization failure when getwd fails")
	}
}

func TestLoadEmbeddedModuleCacheHitAndReadError(t *testing.T) {
	r := &resolver{
		cwd:      t.TempDir(),
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}

	ref0, err := r.loadEmbeddedModule("jsc")
	if err != nil {
		t.Fatalf("first embedded load failed: %v", err)
	}
	ref1, err := r.loadEmbeddedModule("jsc")
	if err != nil {
		t.Fatalf("second embedded load failed: %v", err)
	}
	if ref0 != ref1 {
		t.Fatalf("expected cached embedded module ref match, got %#v vs %#v", ref0, ref1)
	}
	if len(r.raw) != 1 {
		t.Fatalf("expected one cached embedded module, got %d", len(r.raw))
	}

	if _, err := r.loadEmbeddedModule("definitely_missing_embedded_module_for_loader_test"); err == nil {
		t.Fatalf("expected missing embedded module load to fail")
	}
}

func TestProcessUseStmtDefaultAliasUsesSourceValue(t *testing.T) {
	r := &resolver{
		cwd:      t.TempDir(),
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}
	mod := &expandedModule{
		Ref:     moduleRef{ID: "entry", Label: "entry"},
		Symbols: map[string]symbolDecl{},
		Aliases: map[string]moduleRef{},
		Stmts:   []ast.Stmt{},
	}
	span := diag.NewSpan("entry.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 1))
	r.processUseStmt(mod, ast.UseStmt{
		Source: ast.UseSource{
			Kind:  ast.UseSourceBare,
			Value: "jsc",
			Span:  span,
		},
		Span: span,
	}, map[string]struct{}{})

	if len(r.diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", r.diags.String())
	}
	if _, ok := mod.Aliases["jsc"]; !ok {
		t.Fatalf("expected default alias 'jsc' to be created")
	}
}

func TestNormalizeWithRefsAnalyseBlockNormalizesSourceExpr(t *testing.T) {
	span := diag.NewSpan("entry.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	sourceRef := moduleRef{ID: "mod:src", Label: "src"}
	source := &expandedModule{
		Ref: sourceRef,
		Symbols: map[string]symbolDecl{
			"p": {
				Name:       "p",
				Kind:       symbolKindParam,
				Importable: true,
				Stmt:       ast.ParamBlock{Name: "p", Span: span},
				Span:       span,
				ModuleID:   sourceRef.ID,
			},
		},
		Aliases: map[string]moduleRef{},
		Stmts:   []ast.Stmt{},
	}
	r := &resolver{
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{sourceRef.ID: source},
	}
	target := &expandedModule{
		Ref:     moduleRef{ID: "entry", Label: "entry"},
		Symbols: map[string]symbolDecl{},
		Aliases: map[string]moduleRef{"lib": sourceRef},
		Stmts:   []ast.Stmt{},
	}
	stmt := ast.AnalyseBlock{
		StepName: "run",
		WithItems: []ast.WithItem{
			{
				SourceExpr: "lib.p",
				SourceSlice: []string{
					"x",
				},
				Span: span,
			},
		},
		Span: span,
	}

	normalized := r.normalizeWithRefs(target, stmt, map[string]struct{}{}).(ast.AnalyseBlock)
	if len(normalized.WithItems) != 1 {
		t.Fatalf("expected one with item, got %#v", normalized.WithItems)
	}
	if normalized.WithItems[0].SourceExpr != "p" {
		t.Fatalf("expected normalized source expr 'p', got %q", normalized.WithItems[0].SourceExpr)
	}
	if normalized.WithItems[0].Rejected {
		t.Fatalf("expected normalized analyse with-item to be accepted")
	}
	if _, ok := target.Symbols["p"]; !ok {
		t.Fatalf("expected normalize to import symbol p from alias module")
	}
}

func TestImportSymbolSkipsNilDependencySource(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	source := &expandedModule{
		Ref: moduleRef{ID: "m1", Label: "m1"},
		Symbols: map[string]symbolDecl{
			"run": {
				Name:       "run",
				Kind:       symbolKindParam,
				Importable: true,
				Span:       span,
				ModuleID:   "m1",
				Stmt: ast.ParamBlock{
					Name: "run",
					WithItems: []ast.WithItem{
						{Name: "dep", From: "ghost", Span: span},
					},
					Span: span,
				},
			},
		},
		Aliases: map[string]moduleRef{
			"ghost": {ID: "ghost-module-id", Label: "ghost"},
		},
		Stmts: []ast.Stmt{},
	}
	target := &expandedModule{
		Ref:     moduleRef{ID: "target", Label: "target"},
		Symbols: map[string]symbolDecl{},
		Aliases: map[string]moduleRef{},
		Stmts:   []ast.Stmt{},
	}
	r := &resolver{
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
	}

	r.importSymbol(target, source, "run", span, map[string]struct{}{}, map[string]struct{}{})
	if hasDiagCode(r.diags, "E532") || hasDiagCode(r.diags, "E533") || hasDiagCode(r.diags, "E534") {
		t.Fatalf("unexpected import diagnostics: %s", r.diags.String())
	}
	if _, ok := target.Symbols["run"]; !ok {
		t.Fatalf("expected target to include imported symbol run")
	}
}

func TestSymbolDependenciesSourceSliceBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ref0 := moduleRef{ID: "m0", Label: "m0"}
	ref1 := moduleRef{ID: "m1", Label: "m1"}
	r := &resolver{
		raw: map[string]*rawModule{
			"m0": {Ref: ref0},
			"m1": {Ref: ref1},
		},
		expanded: map[string]*expandedModule{},
		diags:    &diag.Diagnostics{},
	}
	mod := &expandedModule{
		Ref: ref0,
		Symbols: map[string]symbolDecl{
			"srcLocal": {Name: "srcLocal", ModuleID: "m0"},
			"selLocal": {Name: "selLocal", ModuleID: "m0"},
			"broken":   {Name: "broken", ModuleID: "missing-module"},
		},
		Aliases: map[string]moduleRef{
			"lib": ref1,
		},
	}

	stmt := ast.ParamBlock{
		Name: "p",
		WithItems: []ast.WithItem{
			// alias + source-slice branch with one empty selector (ignored) and one valid selector.
			{SourceExpr: "lib", SourceSlice: []string{"", "a"}, Span: span},
			// local source-expr branch.
			{SourceExpr: "srcLocal", SourceSlice: []string{"ignored"}, Span: span},
			// fallback to resolving selectors by local names.
			{SourceExpr: "missingExpr", SourceSlice: []string{"selLocal", "absent"}, Span: span},
			// resolveLocal(item.SourceExpr) where moduleRefByID fails.
			{SourceExpr: "broken", SourceSlice: []string{"none"}, Span: span},
			// add() guard for empty name in from-alias path.
			{Name: "", From: "lib", Span: span},
		},
		Span: span,
	}

	deps := r.symbolDependencies(mod, stmt)
	got := make([]string, 0, len(deps))
	for _, dep := range deps {
		got = append(got, dep.Source.ID+":"+dep.Name)
	}
	want := []string{"m0:selLocal", "m0:srcLocal", "m1:a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected source-slice deps: got=%#v want=%#v", got, want)
	}
}
