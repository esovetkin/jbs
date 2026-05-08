package imports

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/ast"
	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

func TestLoadAndExpandHandlesNilDiagnosticsAndBrokenCwd(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", "do run {\n  echo ok\n}\n")

	res, err := LoadAndExpand(entry, dir, nil)
	if err != nil {
		t.Fatalf("LoadAndExpand with nil diagnostics failed: %v", err)
	}
	if res == nil || res.Modules[res.Entry.ID] == nil {
		t.Fatalf("unexpected load result: %#v", res)
	}
	if got := len(res.Modules[res.Entry.ID].Program.Stmts); got != 1 {
		t.Fatalf("unexpected entry program stmt count: %d", got)
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

func TestResolveBareModuleBranches(t *testing.T) {
	r := &resolver{
		cwd:     t.TempDir(),
		diags:   &diag.Diagnostics{},
		raw:     map[string]*rawModule{},
		modules: map[string]*ModuleInfo{},
		loading: map[string]bool{},
		sources: map[string]string{},
	}

	if _, err := r.resolveBareModule("   ", r.cwd); err == nil {
		t.Fatalf("expected empty module name to fail")
	}

	writeTestFile(t, r.cwd, "localmod.jbs", "value = 1\n")
	_, err := r.resolveBareModule("localmod", r.cwd)
	if err == nil {
		t.Fatalf("expected bare local module import to fail")
	}
	var bareErr *bareModuleResolutionError
	if !errors.As(err, &bareErr) {
		t.Fatalf("expected bareModuleResolutionError, got %T (%v)", err, err)
	}
	if want := filepath.Join(r.cwd, "localmod.jbs"); bareErr.LocalPath != want {
		t.Fatalf("expected bare local module candidate %q, got %#v", want, bareErr)
	}
}

func TestLoadAndExpandNestedQuotedImportsIgnoreProcessCwd(t *testing.T) {
	projectDir := t.TempDir()
	unrelatedCwd := t.TempDir()
	libDir := filepath.Join(projectDir, "lib", "nested")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatalf("mkdir nested lib: %v", err)
	}
	writeTestFile(t, libDir, "b.jbs", "base = 41\n")
	writeTestFile(t, filepath.Join(projectDir, "lib"), "a.jbs", "use base from \"./nested/b.jbs\"\nvalue = base + 1\n")
	entry := writeTestFile(t, projectDir, "main.jbs", "use value from \"./lib/a.jbs\"\nresult = value\n")

	diags := &diag.Diagnostics{}
	res, err := LoadAndExpand(entry, unrelatedCwd, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil load result")
	}
	if len(diags.Items) != 0 {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	for _, want := range []string{
		entry,
		filepath.Join(projectDir, "lib", "a.jbs"),
		filepath.Join(projectDir, "lib", "nested", "b.jbs"),
	} {
		if _, ok := res.Sources[want]; !ok {
			t.Fatalf("expected source %q in load result, got %#v", want, res.Sources)
		}
	}
}

func TestLoadAndExpandUsesProvidedSymlinkPathAsSourceLabel(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real")
	linkDir := filepath.Join(tmp, "link")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real dir: %v", err)
	}
	writeTestFile(t, realDir, "input.jbs", "x = 1\n")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	input := filepath.Join(linkDir, "input.jbs")
	diags := &diag.Diagnostics{}
	res, err := LoadAndExpand(input, tmp, diags)
	if err != nil {
		t.Fatalf("LoadAndExpand failed: %v", err)
	}
	if _, ok := res.Sources[filepath.Clean(input)]; !ok {
		t.Fatalf("expected symlink path label %q, got %#v", filepath.Clean(input), res.Sources)
	}
}

func TestResolvePathModuleBranches(t *testing.T) {
	tmp := t.TempDir()
	diags := &diag.Diagnostics{}
	r := &resolver{
		cwd:     tmp,
		diags:   diags,
		raw:     map[string]*rawModule{},
		modules: map[string]*ModuleInfo{},
		loading: map[string]bool{},
		sources: map[string]string{},
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
		cwd:     tmp,
		diags:   diags,
		raw:     map[string]*rawModule{},
		modules: map[string]*ModuleInfo{},
		loading: map[string]bool{},
		sources: map[string]string{},
	}
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(0, 1, 2))

	aliasedRef := ModuleRef{ID: "file:/tmp/lib.jbs", Label: "/tmp/lib.jbs"}
	aliases := map[string]ModuleRef{"lib": aliasedRef}
	gotAlias, err := r.resolveUseSource(aliases, filepath.Join(tmp, "imports"), ast.UseSource{Kind: ast.UseSourceBare, Value: "lib", Span: span})
	if err != nil {
		t.Fatalf("resolveUseSource alias lookup failed: %v", err)
	}
	if gotAlias != aliasedRef {
		t.Fatalf("expected aliased ref %#v, got %#v", aliasedRef, gotAlias)
	}

	baseDir := filepath.Join(tmp, "imports")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir importer base dir: %v", err)
	}
	writeTestFile(t, baseDir, "nested.jbs", "value = 2\n")
	gotPath, err := r.resolveUseSource(aliases, baseDir, ast.UseSource{Kind: ast.UseSourcePath, Value: "./nested.jbs", Span: span})
	if err != nil {
		t.Fatalf("resolveUseSource path failed: %v", err)
	}
	if want := filepath.Join(baseDir, "nested.jbs"); gotPath.Label != want {
		t.Fatalf("expected non-empty path ref label %q, got %q", want, gotPath.Label)
	}

	if _, err := r.resolveUseSource(aliases, baseDir, ast.UseSource{Kind: ast.UseSourceKind("unknown"), Value: "x", Span: span}); err == nil {
		t.Fatalf("expected unknown source kind to fail")
	}
}
