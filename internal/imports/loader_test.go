package imports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
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
