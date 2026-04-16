package imports

import (
	"os"
	"path/filepath"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestNormalizeEmbeddedName(t *testing.T) {
	if got := normalizeEmbeddedName("  "); got != "" {
		t.Fatalf("expected empty normalized embedded name, got %q", got)
	}
	if got := normalizeEmbeddedName("jsc"); got != "jsc.jbs" {
		t.Fatalf("expected jsc.jbs, got %q", got)
	}
	if got := normalizeEmbeddedName("foo.jbs"); got != "foo.jbs" {
		t.Fatalf("expected foo.jbs unchanged, got %q", got)
	}
}

func TestResolveBareModuleBranches(t *testing.T) {
	r := &resolver{
		cwd:      t.TempDir(),
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}

	if _, err := r.resolveBareModule("   "); err == nil {
		t.Fatalf("expected empty module name to fail")
	}

	writeTestFile(t, r.cwd, "localmod.jbs", "do run { echo ok }")
	ref, err := r.resolveBareModule("localmod")
	if err != nil {
		t.Fatalf("resolveBareModule local module failed: %v", err)
	}
	if ref.ID == "" || ref.Label == "" {
		t.Fatalf("expected non-empty module ref, got %#v", ref)
	}
}

func TestResolvePathModuleBranches(t *testing.T) {
	tmp := t.TempDir()
	diags := &diag.Diagnostics{}
	r := &resolver{
		cwd:      tmp,
		diags:    diags,
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}

	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	if _, err := r.resolvePathModule("./bad.txt", "", span); err == nil {
		t.Fatalf("expected non-.jbs quoted path to fail")
	}
	if !hasDiagCode(diags, "E535") {
		t.Fatalf("expected E535 for invalid path suffix, got: %s", diags.String())
	}

	writeTestFile(t, tmp, "mod.jbs", "do run { echo ok }")
	ref0, err := r.resolvePathModule("./mod.jbs", "", span)
	if err != nil {
		t.Fatalf("resolvePathModule with fallback cwd failed: %v", err)
	}
	if want := filepath.Join(tmp, "mod.jbs"); ref0.Label != want {
		t.Fatalf("expected resolved label %q, got %q", want, ref0.Label)
	}

	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	writeTestFile(t, sub, "nested.jbs", "do run { echo ok }")
	ref1, err := r.resolvePathModule("./nested.jbs", sub, span)
	if err != nil {
		t.Fatalf("resolvePathModule with importer base failed: %v", err)
	}
	if want := filepath.Join(sub, "nested.jbs"); ref1.Label != want {
		t.Fatalf("expected resolved nested label %q, got %q", want, ref1.Label)
	}
}

func TestResolveUseSourceBranches(t *testing.T) {
	diags := &diag.Diagnostics{}
	tmp := t.TempDir()
	writeTestFile(t, tmp, "p.jbs", "do run { echo ok }")
	r := &resolver{
		cwd:      tmp,
		diags:    diags,
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))

	aliasedRef := moduleRef{ID: "embed:jsc.jbs", Label: "shared/jsc.jbs"}
	current := &expandedModule{
		Ref:     moduleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]moduleRef{"lib": aliasedRef},
		Symbols: map[string]symbolDecl{},
		Stmts:   []ast.Stmt{},
	}
	gotAlias, err := r.resolveUseSource(current, ast.UseSource{Kind: ast.UseSourceBare, Value: "lib", Span: span})
	if err != nil {
		t.Fatalf("resolveUseSource alias lookup failed: %v", err)
	}
	if gotAlias != aliasedRef {
		t.Fatalf("expected aliased ref %#v, got %#v", aliasedRef, gotAlias)
	}

	gotPath, err := r.resolveUseSource(current, ast.UseSource{Kind: ast.UseSourcePath, Value: "./p.jbs", Span: span})
	if err != nil {
		t.Fatalf("resolveUseSource path failed: %v", err)
	}
	if gotPath.Label == "" {
		t.Fatalf("expected non-empty path ref label")
	}

	if _, err := r.resolveUseSource(current, ast.UseSource{Kind: ast.UseSourceKind("unknown"), Value: "x", Span: span}); err == nil {
		t.Fatalf("expected unknown source kind to fail")
	}
}

func TestProcessUseStmtAliasAndErrorBranches(t *testing.T) {
	diags := &diag.Diagnostics{}
	r := &resolver{
		cwd:      t.TempDir(),
		diags:    diags,
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	mod := &expandedModule{
		Ref: moduleRef{ID: "entry", Label: "entry"},
		Symbols: map[string]symbolDecl{
			"jsc": {Name: "jsc", Span: span, ModuleID: "entry"},
		},
		Aliases: map[string]moduleRef{
			"taken": {ID: "mod:a", Label: "a"},
		},
		Stmts: []ast.Stmt{},
	}

	r.processUseStmt(mod, ast.UseStmt{
		Alias: "taken",
		Source: ast.UseSource{
			Kind:  ast.UseSourceBare,
			Value: "jsc",
			Span:  span,
		},
		Span: span,
	}, map[string]struct{}{})
	if !hasDiagCode(diags, "E536") {
		t.Fatalf("expected E536 for alias collision, got: %s", diags.String())
	}

	before := len(diags.Items)
	r.processUseStmt(mod, ast.UseStmt{
		Alias: "jsc",
		Source: ast.UseSource{
			Kind:  ast.UseSourceBare,
			Value: "jsc",
			Span:  span,
		},
		Span: span,
	}, map[string]struct{}{})
	if !hasDiagCode(diags, "E534") || len(diags.Items) == before {
		t.Fatalf("expected E534 for alias-symbol collision, got: %s", diags.String())
	}

	before = len(diags.Items)
	r.processUseStmt(mod, ast.UseStmt{
		Names: []string{"x"},
		Source: ast.UseSource{
			Kind:  ast.UseSourceBare,
			Value: "missing_module_for_use_stmt_test",
			Span:  span,
		},
		Span: span,
	}, map[string]struct{}{})
	if !hasDiagCode(diags, "E531") || len(diags.Items) == before {
		t.Fatalf("expected E531 for failed import source resolution, got: %s", diags.String())
	}
}

func TestAddImportedSymbolBranchesAndStmtSymbol(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	source := &expandedModule{Ref: moduleRef{ID: "m1", Label: "m1"}}
	diags := &diag.Diagnostics{}
	r := &resolver{diags: diags}

	sym := symbolDecl{
		Name:       "x",
		Kind:       symbolKindParam,
		Importable: true,
		Span:       span,
		ModuleID:   "m1",
		Stmt:       ast.ParamBlock{Name: "x", Span: span},
	}

	targetAliasCollision := &expandedModule{
		Ref:     moduleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]moduleRef{"x": {ID: "m2", Label: "m2"}},
		Symbols: map[string]symbolDecl{},
	}
	if ok := r.addImportedSymbol(targetAliasCollision, source, sym, span); ok {
		t.Fatalf("expected alias collision to reject imported symbol")
	}
	if !hasDiagCode(diags, "E534") {
		t.Fatalf("expected E534 for alias collision, got: %s", diags.String())
	}

	targetSame := &expandedModule{
		Ref:     moduleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]moduleRef{},
		Symbols: map[string]symbolDecl{"x": {Name: "x", ModuleID: "m1", Span: span}},
	}
	before := len(diags.Items)
	if ok := r.addImportedSymbol(targetSame, source, sym, span); ok {
		t.Fatalf("expected same-source duplicate to be ignored")
	}
	if len(diags.Items) != before {
		t.Fatalf("expected no new diagnostics for same-source duplicate, got: %s", diags.String())
	}

	targetConflict := &expandedModule{
		Ref:     moduleRef{ID: "entry", Label: "entry"},
		Aliases: map[string]moduleRef{},
		Symbols: map[string]symbolDecl{"x": {Name: "x", ModuleID: "m2", Span: span}},
	}
	before = len(diags.Items)
	if ok := r.addImportedSymbol(targetConflict, source, sym, span); ok {
		t.Fatalf("expected cross-source conflict to fail")
	}
	if !hasDiagCode(diags, "E534") || len(diags.Items) == before {
		t.Fatalf("expected E534 for cross-source conflict, got: %s", diags.String())
	}

	stmtCases := []struct {
		stmt           ast.Stmt
		wantKind       symbolKind
		wantImportable bool
		wantOK         bool
	}{
		{stmt: ast.LetBlock{Name: "l"}, wantKind: symbolKindLet, wantImportable: true, wantOK: true},
		{stmt: ast.ParamBlock{Name: "p"}, wantKind: symbolKindParam, wantImportable: true, wantOK: true},
		{stmt: ast.DoBlock{Name: "d"}, wantKind: symbolKindDo, wantImportable: true, wantOK: true},
		{stmt: ast.SubmitBlock{Name: "s"}, wantKind: symbolKindSubmit, wantImportable: true, wantOK: true},
		{stmt: ast.GlobalAssign{Name: "g"}, wantKind: symbolKindGlobal, wantImportable: true, wantOK: true},
		{stmt: ast.AnalyseBlock{StepName: "a"}, wantKind: symbolKindOther, wantImportable: true, wantOK: false},
		{stmt: ast.UseStmt{}, wantKind: symbolKindOther, wantImportable: false, wantOK: false},
	}
	for i, tc := range stmtCases {
		_, gotKind, gotImportable, gotOK := stmtSymbol(tc.stmt)
		if gotKind != tc.wantKind || gotImportable != tc.wantImportable || gotOK != tc.wantOK {
			t.Fatalf("stmtSymbol case %d mismatch: got(kind=%q importable=%v ok=%v) want(kind=%q importable=%v ok=%v)",
				i, gotKind, gotImportable, gotOK, tc.wantKind, tc.wantImportable, tc.wantOK)
		}
	}
}
