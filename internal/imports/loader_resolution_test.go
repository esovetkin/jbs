package imports

import (
	"os"
	"path/filepath"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func TestLoadAndExpandHandlesNilDiagnosticsAndBrokenCwd(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", "do run {\n  echo ok\n}\n")

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

func TestLoadEmbeddedModuleCachesAndReportsMissingModules(t *testing.T) {
	r := &resolver{
		cwd:       t.TempDir(),
		diags:     &diag.Diagnostics{},
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
		sources:   map[string]string{},
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
	if _, ok := r.sources["shared/jsc.jbs"]; !ok {
		t.Fatalf("expected embedded source to be cached")
	}

	if _, err := r.loadEmbeddedModule("definitely_missing_embedded_module_for_loader_test"); err == nil {
		t.Fatalf("expected missing embedded module load to fail")
	}
}

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
		cwd:       t.TempDir(),
		diags:     &diag.Diagnostics{},
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
		sources:   map[string]string{},
	}

	if _, err := r.resolveBareModule("   "); err == nil {
		t.Fatalf("expected empty module name to fail")
	}

	embedded, err := r.resolveBareModule("jsc")
	if err != nil {
		t.Fatalf("resolveBareModule embedded module failed: %v", err)
	}
	if embedded.Label != "shared/jsc.jbs" {
		t.Fatalf("unexpected embedded module label: %q", embedded.Label)
	}

	writeTestFile(t, r.cwd, "localmod.jbs", "value = 1\n")
	local, err := r.resolveBareModule("localmod")
	if err != nil {
		t.Fatalf("resolveBareModule local module failed: %v", err)
	}
	if want := filepath.Join(r.cwd, "localmod.jbs"); local.Label != want {
		t.Fatalf("expected local module label %q, got %q", want, local.Label)
	}
}

func TestResolvePathModuleBranches(t *testing.T) {
	tmp := t.TempDir()
	diags := &diag.Diagnostics{}
	r := &resolver{
		cwd:       tmp,
		diags:     diags,
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
		sources:   map[string]string{},
	}

	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))
	if _, err := r.resolvePathModule("./bad.txt", "", span); err == nil {
		t.Fatalf("expected non-.jbs quoted path to fail")
	}
	if !hasDiagCode(diags, "E535") {
		t.Fatalf("expected E535 for invalid path suffix, got: %s", diags.String())
	}

	writeTestFile(t, tmp, "mod.jbs", "value = 1\n")
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
	writeTestFile(t, sub, "nested.jbs", "value = 2\n")
	ref1, err := r.resolvePathModule("./nested.jbs", sub, span)
	if err != nil {
		t.Fatalf("resolvePathModule with importer base failed: %v", err)
	}
	if want := filepath.Join(sub, "nested.jbs"); ref1.Label != want {
		t.Fatalf("expected resolved nested label %q, got %q", want, ref1.Label)
	}
}

func TestResolveUseSourceBranches(t *testing.T) {
	tmp := t.TempDir()
	diags := &diag.Diagnostics{}
	writeTestFile(t, tmp, "p.jbs", "value = 1\n")
	r := &resolver{
		cwd:       tmp,
		diags:     diags,
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
		sources:   map[string]string{},
	}
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))

	aliasedRef := ModuleRef{ID: "embed:jsc.jbs", Label: "shared/jsc.jbs"}
	current := &expandedModule{
		Ref:     ModuleRef{ID: "entry", Label: "entry"},
		BaseDir: filepath.Join(tmp, "imports"),
		Aliases: map[string]ModuleRef{"lib": aliasedRef},
		Exports: map[string]ModuleExport{},
		Stmts:   []ast.Stmt{},
	}
	gotAlias, err := r.resolveUseSource(current, ast.UseSource{Kind: ast.UseSourceBare, Value: "lib", Span: span})
	if err != nil {
		t.Fatalf("resolveUseSource alias lookup failed: %v", err)
	}
	if gotAlias != aliasedRef {
		t.Fatalf("expected aliased ref %#v, got %#v", aliasedRef, gotAlias)
	}

	if err := os.MkdirAll(current.BaseDir, 0o755); err != nil {
		t.Fatalf("mkdir importer base dir: %v", err)
	}
	writeTestFile(t, current.BaseDir, "nested.jbs", "value = 2\n")
	gotPath, err := r.resolveUseSource(current, ast.UseSource{Kind: ast.UseSourcePath, Value: "./nested.jbs", Span: span})
	if err != nil {
		t.Fatalf("resolveUseSource path failed: %v", err)
	}
	if want := filepath.Join(current.BaseDir, "nested.jbs"); gotPath.Label != want {
		t.Fatalf("expected non-empty path ref label %q, got %q", want, gotPath.Label)
	}

	if _, err := r.resolveUseSource(current, ast.UseSource{Kind: ast.UseSourceKind("unknown"), Value: "x", Span: span}); err == nil {
		t.Fatalf("expected unknown source kind to fail")
	}
}
