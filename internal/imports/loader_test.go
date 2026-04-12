package imports

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func load(t *testing.T, entry, cwd string) (*LoadResult, *diag.Diagnostics) {
	t.Helper()
	diags := &diag.Diagnostics{}
	res, err := LoadAndExpand(entry, cwd, diags)
	if err != nil {
		t.Fatalf("load and expand failed: %v", err)
	}
	return res, diags
}

func hasDiagCode(diags *diag.Diagnostics, code string) bool {
	if diags == nil {
		return false
	}
	for _, item := range diags.Items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func TestBareModuleResolvesEmbeddedBeforeLocal(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "jsc.jbs", `
let submit_defaults {
  queue = "local_queue"
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use submit_defaults from jsc
`)

	res, diags := load(t, entry, dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	found := false
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.LetBlock)
		if !ok || block.Name != "submit_defaults" {
			continue
		}
		if block.Span.File != "shared/jsc.jbs" {
			t.Fatalf("expected embedded submit_defaults from shared/jsc.jbs, got %s", block.Span.File)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected imported submit_defaults in expanded program")
	}
}

func TestQuotedPathUsesLocalFile(t *testing.T) {
	dir := t.TempDir()
	local := writeTestFile(t, dir, "jsc.jbs", `
let submit_defaults {
  queue = "local_queue"
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use submit_defaults from "./jsc.jbs"
`)

	res, diags := load(t, entry, dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	found := false
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.LetBlock)
		if !ok || block.Name != "submit_defaults" {
			continue
		}
		if block.Span.File != filepath.Clean(local) {
			t.Fatalf("expected local submit_defaults from %s, got %s", local, block.Span.File)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected imported submit_defaults in expanded program")
	}
}

func TestQuotedPathUsesEntryModuleDirectoryWhenCwdDiffers(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	foreignCwd := filepath.Join(root, "cwd")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.MkdirAll(foreignCwd, 0o755); err != nil {
		t.Fatalf("mkdir foreign cwd: %v", err)
	}

	lib := writeTestFile(t, projectDir, "lib/mod.jbs", `
let submit_defaults {
  queue = "batch"
}
`)
	entry := writeTestFile(t, projectDir, "main.jbs", `
use submit_defaults from "./lib/mod.jbs"
`)

	res, diags := load(t, entry, foreignCwd)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	found := false
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.LetBlock)
		if !ok || block.Name != "submit_defaults" {
			continue
		}
		if block.Span.File != filepath.Clean(lib) {
			t.Fatalf("expected submit_defaults from %s, got %s", lib, block.Span.File)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected imported submit_defaults in expanded program")
	}
}

func TestNestedQuotedPathUsesImporterModuleDirectory(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	foreignCwd := filepath.Join(root, "cwd")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.MkdirAll(foreignCwd, 0o755); err != nil {
		t.Fatalf("mkdir foreign cwd: %v", err)
	}

	aPath := writeTestFile(t, projectDir, "sub/a.jbs", `
use y from "./b.jbs"
let top {
  value = "ok"
}
`)
	writeTestFile(t, projectDir, "sub/b.jbs", `
let y {
  value = 1
}
`)
	entry := writeTestFile(t, projectDir, "main.jbs", `
use top from "./sub/a.jbs"
`)

	res, diags := load(t, entry, foreignCwd)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	found := false
	for _, stmt := range res.Program.Stmts {
		block, ok := stmt.(ast.LetBlock)
		if !ok || block.Name != "top" {
			continue
		}
		if block.Span.File != filepath.Clean(aPath) {
			t.Fatalf("expected top from %s, got %s", aPath, block.Span.File)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected imported top in expanded program")
	}
}

func TestUnknownModuleProducesError(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
use submit_defaults from missing_module
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E531") {
		t.Fatalf("expected E531, got: %s", diags.String())
	}
}

func TestMissingQuotedModuleProducesError(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
use x from "./missing.jbs"
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E531") {
		t.Fatalf("expected E531, got: %s", diags.String())
	}
}

func TestUnknownSymbolProducesError(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
use does_not_exist from jsc
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E532") {
		t.Fatalf("expected E532, got: %s", diags.String())
	}
}

func TestImportCycleProducesError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.jbs", `
use y from "./b.jbs"
let x {
  a = 1
}
`)
	entry := writeTestFile(t, dir, "b.jbs", `
use x from "./a.jbs"
let y {
  b = 2
}
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E530") {
		t.Fatalf("expected E530 for import cycle, got: %s", diags.String())
	}
}

func TestImportCollisionLocalVsImportedProducesError(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
let submit_defaults {
  queue = "local"
}
use submit_defaults from jsc
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E534") {
		t.Fatalf("expected E534, got: %s", diags.String())
	}
}

func TestImportCollisionImportedVsImportedProducesError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "m1.jbs", `
let x {
  a = 1
}
`)
	writeTestFile(t, dir, "m2.jbs", `
let x {
  b = 2
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use x from "./m1.jbs"
use x from "./m2.jbs"
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E534") {
		t.Fatalf("expected E534, got: %s", diags.String())
	}
}

func TestAliasCollisionProducesError(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
let cfg {
  a = 1
}
use "./m1.jbs" as cfg
`)
	writeTestFile(t, dir, "m1.jbs", `
let y {
  b = 2
}
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E534") {
		t.Fatalf("expected E534 for alias collision, got: %s", diags.String())
	}
}

func TestStepImportPullsDependencyClosure(t *testing.T) {
	dir := t.TempDir()
	lib := writeTestFile(t, dir, "lib.jbs", `
let submit_defaults {
  queue = "batch"
}
param p {
  a = (1,2)
  a
}
do prep with p {
  echo ${a}
}
submit run
  after prep
  use submit_defaults
  with p
{
  args_exec = "-lc hostname"
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use run from "./lib.jbs"
`)

	res, diags := load(t, entry, dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}
	names := make([]string, 0)
	for _, stmt := range res.Program.Stmts {
		switch n := stmt.(type) {
		case ast.LetBlock:
			names = append(names, "let:"+n.Name)
		case ast.ParamBlock:
			names = append(names, "param:"+n.Name)
		case ast.DoBlock:
			names = append(names, "do:"+n.Name)
		case ast.SubmitBlock:
			names = append(names, "submit:"+n.Name)
		}
	}
	got := strings.Join(names, ",")
	for _, want := range []string{"let:submit_defaults", "param:p", "do:prep", "submit:run"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected closure to include %s, got %s (module=%s)", want, got, lib)
		}
	}
}

func TestQuotedPathMustHaveJbsExtension(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
use "./bad.txt" as bad
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E535") {
		t.Fatalf("expected E535, got: %s", diags.String())
	}
}

func TestQualifiedWithNameIsNormalizedAndSymbolImported(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "lib.jbs", `
param p {
  x = 1
  x
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use lib
do s with lib.p {
  echo ${x}
}
`)

	res, diags := load(t, entry, dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	var foundParam bool
	var foundDo bool
	for _, stmt := range res.Program.Stmts {
		switch n := stmt.(type) {
		case ast.ParamBlock:
			if n.Name == "p" {
				foundParam = true
			}
		case ast.DoBlock:
			if n.Name == "s" {
				foundDo = true
				if len(n.WithItems) != 1 {
					t.Fatalf("expected one with item, got %#v", n.WithItems)
				}
				if n.WithItems[0].Name != "p" || n.WithItems[0].From != "" {
					t.Fatalf("expected normalized with item 'p', got %#v", n.WithItems[0])
				}
			}
		}
	}
	if !foundParam {
		t.Fatalf("expected param p imported from lib")
	}
	if !foundDo {
		t.Fatalf("expected do block s in expanded program")
	}
}

func TestQualifiedWithFromIsNormalizedAndSourceImported(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "lib.jbs", `
param p {
  x = (1,2)
  x
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use lib
do s with x from lib.p {
  echo ${x}
}
`)

	res, diags := load(t, entry, dir)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.String())
	}

	var foundParam bool
	for _, stmt := range res.Program.Stmts {
		switch n := stmt.(type) {
		case ast.ParamBlock:
			if n.Name == "p" {
				foundParam = true
			}
		case ast.DoBlock:
			if n.Name != "s" {
				continue
			}
			if len(n.WithItems) != 1 {
				t.Fatalf("expected one with item, got %#v", n.WithItems)
			}
			if n.WithItems[0].Name != "x" || n.WithItems[0].From != "p" {
				t.Fatalf("expected normalized with item 'x from p', got %#v", n.WithItems[0])
			}
		}
	}
	if !foundParam {
		t.Fatalf("expected param p imported from lib")
	}
}

func TestQualifiedWithUnknownAliasProducesError(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
do s with missing.p {
  echo ${x}
}
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E537") {
		t.Fatalf("expected E537 for unknown with alias, got: %s", diags.String())
	}
}

func TestLoadAndExpandEntryErrorsAndEmptyCwd(t *testing.T) {
	dir := t.TempDir()
	diags := &diag.Diagnostics{}
	if _, err := LoadAndExpand("missing.jbs", dir, diags); err == nil {
		t.Fatalf("expected missing entry path to return error")
	}

	entry := writeTestFile(t, dir, "entry.jbs", `
let l {
  x = 1
}
`)
	diags = &diag.Diagnostics{}
	res, err := LoadAndExpand(entry, "", diags)
	if err != nil {
		t.Fatalf("unexpected error when cwd is empty and entry is absolute: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags.String())
	}
	if res == nil || len(res.Program.Stmts) != 1 {
		t.Fatalf("expected one expanded statement, got %#v", res)
	}
}

func TestNormalizeEmbeddedName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "  ", want: ""},
		{in: "jsc", want: "jsc.jbs"},
		{in: " jsc.jbs ", want: "jsc.jbs"},
	}
	for _, tt := range tests {
		if got := normalizeEmbeddedName(tt.in); got != tt.want {
			t.Fatalf("normalizeEmbeddedName(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveUseSourceUnknownKind(t *testing.T) {
	r := &resolver{
		cwd:   t.TempDir(),
		diags: &diag.Diagnostics{},
		raw:   map[string]*rawModule{},
	}
	_, err := r.resolveUseSource(nil, ast.UseSource{
		Kind:  ast.UseSourceKind("unknown"),
		Value: "x",
		Span:  diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2)),
	})
	if err == nil {
		t.Fatalf("expected error for unknown use source kind")
	}
}

func TestModuleRefByID(t *testing.T) {
	r := &resolver{
		raw: map[string]*rawModule{
			"raw-id": {Ref: moduleRef{ID: "raw-id", Label: "raw"}},
		},
		expanded: map[string]*expandedModule{
			"exp-id": {Ref: moduleRef{ID: "exp-id", Label: "exp"}},
		},
	}
	if ref, ok := r.moduleRefByID("raw-id"); !ok || ref.Label != "raw" {
		t.Fatalf("expected raw module ref, got ref=%#v ok=%v", ref, ok)
	}
	if ref, ok := r.moduleRefByID("exp-id"); !ok || ref.Label != "exp" {
		t.Fatalf("expected expanded module ref, got ref=%#v ok=%v", ref, ok)
	}
	if _, ok := r.moduleRefByID("missing"); ok {
		t.Fatalf("expected missing id lookup to fail")
	}
}

func TestStmtSymbolKinds(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	tests := []struct {
		stmt       ast.Stmt
		wantName   string
		wantKind   symbolKind
		wantOK     bool
		importable bool
	}{
		{stmt: ast.LetBlock{Name: "l", Span: span}, wantName: "l", wantKind: symbolKindLet, wantOK: true, importable: true},
		{stmt: ast.ParamBlock{Name: "p", Span: span}, wantName: "p", wantKind: symbolKindParam, wantOK: true, importable: true},
		{stmt: ast.DoBlock{Name: "d", Span: span}, wantName: "d", wantKind: symbolKindDo, wantOK: true, importable: true},
		{stmt: ast.SubmitBlock{Name: "s", Span: span}, wantName: "s", wantKind: symbolKindSubmit, wantOK: true, importable: true},
		{stmt: ast.GlobalAssign{Name: "g", Span: span}, wantName: "g", wantKind: symbolKindGlobal, wantOK: true, importable: true},
		{stmt: ast.AnalyseBlock{StepName: "a", Span: span}, wantName: "a", wantKind: symbolKindOther, wantOK: true, importable: false},
		{stmt: ast.UseStmt{Span: span}, wantName: "", wantKind: symbolKindOther, wantOK: false, importable: false},
	}
	for _, tt := range tests {
		name, kind, ok, importable := stmtSymbol(tt.stmt)
		if name != tt.wantName || kind != tt.wantKind || ok != tt.wantOK || importable != tt.importable {
			t.Fatalf("stmtSymbol(%T)=(%q,%v,%v,%v), want (%q,%v,%v,%v)",
				tt.stmt, name, kind, ok, importable, tt.wantName, tt.wantKind, tt.wantOK, tt.importable)
		}
	}
}

func TestAddImportedSymbolBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	source := &expandedModule{Ref: moduleRef{ID: "m1", Label: "m1"}}
	sym := symbolDecl{Name: "x", Span: span, Importable: true}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		target := &expandedModule{
			Aliases: map[string]moduleRef{"x": {ID: "alias"}},
			Symbols: map[string]symbolDecl{},
		}
		if ok := r.addImportedSymbol(target, source, sym, span); ok {
			t.Fatalf("expected alias collision to fail")
		}
		if !hasDiagCode(diags, "E534") {
			t.Fatalf("expected E534 for alias collision, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		target := &expandedModule{
			Aliases: map[string]moduleRef{},
			Symbols: map[string]symbolDecl{
				"x": {Name: "x", ModuleID: "m1"},
			},
		}
		if ok := r.addImportedSymbol(target, source, sym, span); ok {
			t.Fatalf("expected duplicate imported symbol from same module to return false")
		}
		if len(diags.Items) != 0 {
			t.Fatalf("did not expect diagnostics for same-module duplicate, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		target := &expandedModule{
			Aliases: map[string]moduleRef{},
			Symbols: map[string]symbolDecl{
				"x": {Name: "x", ModuleID: "other", Span: span},
			},
		}
		if ok := r.addImportedSymbol(target, source, sym, span); ok {
			t.Fatalf("expected cross-module collision to fail")
		}
		if !hasDiagCode(diags, "E534") {
			t.Fatalf("expected E534 for cross-module collision, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		target := &expandedModule{
			Aliases: map[string]moduleRef{},
			Symbols: map[string]symbolDecl{},
		}
		if ok := r.addImportedSymbol(target, source, sym, span); !ok {
			t.Fatalf("expected successful symbol import")
		}
		added, exists := target.Symbols["x"]
		if !exists || !added.Imported || added.ModuleID != "m1" {
			t.Fatalf("unexpected added imported symbol: %#v", added)
		}
	}
}

func TestAddLocalStmtBranches(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	local := ast.LetBlock{Name: "x", Span: span}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		mod := &expandedModule{
			Aliases: map[string]moduleRef{"x": {ID: "m"}},
			Symbols: map[string]symbolDecl{},
			Stmts:   []ast.Stmt{},
		}
		r.addLocalStmt(mod, local)
		if !hasDiagCode(diags, "E534") {
			t.Fatalf("expected E534 for local/alias collision, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		mod := &expandedModule{
			Aliases: map[string]moduleRef{},
			Symbols: map[string]symbolDecl{"x": {Name: "x", Imported: true, Span: span}},
			Stmts:   []ast.Stmt{},
		}
		r.addLocalStmt(mod, local)
		if !hasDiagCode(diags, "E534") {
			t.Fatalf("expected E534 for local/imported collision, got: %s", diags.String())
		}
	}

	{
		diags := &diag.Diagnostics{}
		r := &resolver{diags: diags}
		prev := symbolDecl{Name: "x", Imported: false, ModuleID: "self", Span: span}
		mod := &expandedModule{
			Aliases: map[string]moduleRef{},
			Symbols: map[string]symbolDecl{"x": prev},
			Stmts:   []ast.Stmt{},
		}
		r.addLocalStmt(mod, local)
		if len(diags.Items) != 0 {
			t.Fatalf("did not expect diagnostics for duplicate local symbol, got: %s", diags.String())
		}
		if got := mod.Symbols["x"]; !reflect.DeepEqual(got, prev) {
			t.Fatalf("expected previous local symbol to be preserved, got %#v", got)
		}
	}
}

func TestExpandModuleMissingRawReturnsNil(t *testing.T) {
	r := &resolver{
		raw:       map[string]*rawModule{},
		expanded:  map[string]*expandedModule{},
		expanding: map[string]bool{},
		diags:     &diag.Diagnostics{},
	}
	if got := r.expandModule(moduleRef{ID: "missing", Label: "missing"}); got != nil {
		t.Fatalf("expected nil when raw module is missing, got %#v", got)
	}
}

func TestSymbolDependencies(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	ref0 := moduleRef{ID: "m0", Label: "m0"}
	ref1 := moduleRef{ID: "m1", Label: "m1"}
	ref2 := moduleRef{ID: "m2", Label: "m2"}
	r := &resolver{
		raw: map[string]*rawModule{
			"m0": {Ref: ref0},
			"m1": {Ref: ref1},
			"m2": {Ref: ref2},
		},
		expanded: map[string]*expandedModule{},
		diags:    &diag.Diagnostics{},
	}
	mod := &expandedModule{
		Ref: ref0,
		Symbols: map[string]symbolDecl{
			"p":    {Name: "p", ModuleID: "m0"},
			"prep": {Name: "prep", ModuleID: "m0"},
			"cfg":  {Name: "cfg", ModuleID: "m2"},
		},
		Aliases: map[string]moduleRef{
			"lib": ref1,
		},
	}

	doStmt := ast.DoBlock{
		Name:  "run",
		After: []string{"prep", "prep"},
		WithItems: []ast.WithItem{
			{Name: "p", Span: span},                    // from=="" resolveLocal(item.Name)
			{Name: "x", From: "lib", Span: span},       // alias branch
			{Name: "y", From: "p", Span: span},         // resolveLocal(item.From)
			{Name: "p", From: "missing", Span: span},   // fallback resolveLocal(item.Name)
			{Name: "absent", From: "none", Span: span}, // ignored
		},
		Span: span,
	}
	deps := r.symbolDependencies(mod, doStmt)
	got := make([]string, 0, len(deps))
	for _, dep := range deps {
		got = append(got, dep.Source.ID+":"+dep.Name)
	}
	want := []string{"m0:p", "m0:prep", "m1:x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected do dependencies: got=%#v want=%#v", got, want)
	}

	submitStmt := ast.SubmitBlock{
		Name:     "run_submit",
		After:    []string{"prep"},
		UseNames: []string{"cfg", "cfg"},
		WithItems: []ast.WithItem{
			{Name: "x", From: "lib", Span: span},
		},
		Span: span,
	}
	deps = r.symbolDependencies(mod, submitStmt)
	got = got[:0]
	for _, dep := range deps {
		got = append(got, dep.Source.ID+":"+dep.Name)
	}
	want = []string{"m0:prep", "m1:x", "m2:cfg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected submit dependencies: got=%#v want=%#v", got, want)
	}

	paramStmt := ast.ParamBlock{
		Name: "pp",
		WithItems: []ast.WithItem{
			{Name: "p", Span: span},
			{Name: "x", From: "lib", Span: span},
		},
		Span: span,
	}
	deps = r.symbolDependencies(mod, paramStmt)
	got = got[:0]
	for _, dep := range deps {
		got = append(got, dep.Source.ID+":"+dep.Name)
	}
	want = []string{"m0:p", "m1:x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected param dependencies: got=%#v want=%#v", got, want)
	}
}

func TestResolveBareModuleEmptyName(t *testing.T) {
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
}

func TestResolvePathModuleFallbackToResolverCwd(t *testing.T) {
	dir := t.TempDir()
	target := writeTestFile(t, dir, "lib.jbs", `
let x {
  a = 1
}
`)
	r := &resolver{
		cwd:      dir,
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}
	ref, err := r.resolvePathModule("./lib.jbs", "", diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2)))
	if err != nil {
		t.Fatalf("unexpected resolvePathModule error: %v", err)
	}
	if ref.Label != filepath.Clean(target) {
		t.Fatalf("expected resolved label %s, got %s", filepath.Clean(target), ref.Label)
	}
}

func TestResolveUseSourcePrefersAlias(t *testing.T) {
	ref := moduleRef{ID: "file:/tmp/m.jbs", Label: "/tmp/m.jbs"}
	r := &resolver{
		cwd:      t.TempDir(),
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
		sources:  map[string]string{},
	}
	current := &expandedModule{Aliases: map[string]moduleRef{"lib": ref}}
	got, err := r.resolveUseSource(current, ast.UseSource{
		Kind:  ast.UseSourceBare,
		Value: "lib",
		Span:  diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2)),
	})
	if err != nil {
		t.Fatalf("unexpected resolveUseSource error: %v", err)
	}
	if got != ref {
		t.Fatalf("expected alias module ref %#v, got %#v", ref, got)
	}
}

func TestImportAliasCollisionDifferentModulesProducesE536(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "m1.jbs", `
let a {
  x = 1
}
`)
	writeTestFile(t, dir, "m2.jbs", `
let b {
  x = 2
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use "./m1.jbs" as lib
use "./m2.jbs" as lib
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E536") {
		t.Fatalf("expected E536, got: %s", diags.String())
	}
}

func TestImportAliasSameModuleTwiceNoError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "m1.jbs", `
let a {
  x = 1
}
`)
	entry := writeTestFile(t, dir, "entry.jbs", `
use "./m1.jbs" as lib
use "./m1.jbs" as lib
do s {
  echo ok
}
`)
	res, diags := load(t, entry, dir)
	if diags.HasErrors() {
		t.Fatalf("did not expect errors, got: %s", diags.String())
	}
	found := false
	for _, stmt := range res.Program.Stmts {
		if block, ok := stmt.(ast.DoBlock); ok && block.Name == "s" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected do block s in expanded program")
	}
}

func TestQualifiedWithInvalidSyntaxProducesE537(t *testing.T) {
	dir := t.TempDir()
	entry := writeTestFile(t, dir, "entry.jbs", `
use jsc
do s with jsc.submit_defaults.extra {
  echo ok
}
`)
	_, diags := load(t, entry, dir)
	if !hasDiagCode(diags, "E537") {
		t.Fatalf("expected E537 for invalid qualified with reference, got: %s", diags.String())
	}
}

func TestNormalizeWithRefMissingExpandedSourceReturnsSymbolName(t *testing.T) {
	r := &resolver{
		diags:    &diag.Diagnostics{},
		raw:      map[string]*rawModule{},
		expanded: map[string]*expandedModule{},
	}
	mod := &expandedModule{
		Aliases: map[string]moduleRef{
			"lib": {ID: "missing-module", Label: "missing-module"},
		},
		Symbols: map[string]symbolDecl{},
	}
	got := r.normalizeWithRef(mod, "lib.value", diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2)), map[string]struct{}{})
	if got != "value" {
		t.Fatalf("expected normalized name 'value', got %q", got)
	}
	if len(r.diags.Items) != 0 {
		t.Fatalf("did not expect diagnostics when source expansion is unavailable, got: %s", r.diags.String())
	}
}

func TestImportSymbolNonImportableAndGuards(t *testing.T) {
	span := diag.NewSpan("in.jbs", diag.NewPos(0, 1, 1), diag.NewPos(1, 1, 2))
	source := &expandedModule{
		Ref: moduleRef{ID: "m1", Label: "m1"},
		Symbols: map[string]symbolDecl{
			"x": {
				Name:       "x",
				Kind:       symbolKindOther,
				Importable: false,
				Span:       span,
			},
		},
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
	r.importSymbol(target, source, "x", span, map[string]struct{}{}, map[string]struct{}{})
	if !hasDiagCode(r.diags, "E533") {
		t.Fatalf("expected E533 for non-importable symbol, got: %s", r.diags.String())
	}

	r.diags = &diag.Diagnostics{}
	r.importSymbol(target, source, "missing", span, map[string]struct{}{}, map[string]struct{}{})
	if !hasDiagCode(r.diags, "E532") {
		t.Fatalf("expected E532 for unknown symbol, got: %s", r.diags.String())
	}

	r.diags = &diag.Diagnostics{}
	source.Symbols["ok"] = symbolDecl{Name: "ok", Importable: true, Span: span}
	inserted := map[string]struct{}{"m1::ok": {}}
	r.importSymbol(target, source, "ok", span, inserted, map[string]struct{}{})
	if _, exists := target.Symbols["ok"]; exists {
		t.Fatalf("did not expect symbol insertion when key is already marked inserted")
	}

	r.diags = &diag.Diagnostics{}
	visiting := map[string]struct{}{"m1::ok": {}}
	r.importSymbol(target, source, "ok", span, map[string]struct{}{}, visiting)
	if _, exists := target.Symbols["ok"]; exists {
		t.Fatalf("did not expect symbol insertion when key is already being visited")
	}
}
